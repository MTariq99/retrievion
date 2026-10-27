package evaluation

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mtariq99/retrievion/internal/chat"
	"github.com/mtariq99/retrievion/internal/embedding"
	"github.com/mtariq99/retrievion/internal/llm"
	"github.com/mtariq99/retrievion/internal/prompt"
	"github.com/mtariq99/retrievion/internal/retrieval"
	"github.com/mtariq99/retrievion/internal/tokenizer"
	"github.com/mtariq99/retrievion/models"
	"github.com/mtariq99/retrievion/pkg/logger"
	"github.com/mtariq99/retrievion/pkg/metrics"
)

type RunOptions struct {
	DatasetName    string
	PromptVersion  string
	Model          string
	K              int
	MarkAsBaseline bool
	TenantID       uuid.UUID
	EvalUserID     uuid.UUID
}

func RunFullEvaluation(ctx context.Context, pool *pgxpool.Pool, retrievalSvc *retrieval.Service, embedder *embedding.Service, llmSvc *llm.Service, llmClient *llm.Client, promptBuilder *prompt.Builder, counter tokenizer.Counter, cfg *models.Config, opts RunOptions) (evalRunID uuid.UUID, passRate float64, err error) {
	log := logger.FromContext(ctx)

	if opts.TenantID == uuid.Nil {
		return uuid.Nil, 0, fmt.Errorf("evaluation tenant ID must not be empty")
	}
	if opts.EvalUserID == uuid.Nil {
		return uuid.Nil, 0, fmt.Errorf("evaluation user ID must not be empty")
	}

	// The evaluation user must have an explicit membership in this tenant.
	var membershipRole string
	err = pool.QueryRow(
		ctx,
		`SELECT role
		 FROM tenant_members
		 WHERE tenant_id = $1 AND user_id = $2`,
		opts.TenantID,
		opts.EvalUserID,
	).Scan(&membershipRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, 0, fmt.Errorf(
			"evaluation user %s is not a member of tenant %s",
			opts.EvalUserID,
			opts.TenantID,
		)
	}
	if err != nil {
		return uuid.Nil, 0, fmt.Errorf(
			"validate evaluation user membership: %w",
			err,
		)
	}
	switch membershipRole {
	case "owner", "admin", "member", "viewer":
		// All valid membership roles may run this offline evaluation.
	default:
		return uuid.Nil, 0, fmt.Errorf(
			"evaluation user %s has invalid membership role %q in tenant %s",
			opts.EvalUserID,
			membershipRole,
			opts.TenantID,
		)
	}

	// Count only cases owned by this tenant. Global cases are excluded.
	var caseCount int
	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*)
		 FROM eval_cases
		 WHERE dataset_name = $1 AND tenant_id = $2`,
		opts.DatasetName,
		opts.TenantID,
	).Scan(&caseCount)
	if err != nil {
		return uuid.Nil, 0, fmt.Errorf(
			"count evaluation cases for dataset %q and tenant %s: %w",
			opts.DatasetName,
			opts.TenantID,
			err,
		)
	}
	if caseCount == 0 {
		return uuid.Nil, 0, fmt.Errorf(
			"evaluation dataset %q contains no cases for tenant %s (tenant-specific cases only; global cases are excluded)",
			opts.DatasetName,
			opts.TenantID,
		)
	}

	if err = validateExpectedChunkOwnership(ctx, pool, opts.DatasetName, opts.TenantID); err != nil {
		return uuid.Nil, 0, err
	}

	runEvalInsertQ := `
		INSERT INTO eval_runs (
			id, dataset_name, prompt_version, model, is_baseline, started_at
		)
		VALUES ($1, $2, $3, $4, $5, now())
	`

	evalRunID = uuid.New()

	_, err = pool.Exec(
		ctx,
		runEvalInsertQ,
		evalRunID,
		opts.DatasetName,
		opts.PromptVersion,
		opts.Model,
		opts.MarkAsBaseline,
	)
	if err != nil {
		return evalRunID, 0, fmt.Errorf("create evaluation run: %w", err)
	}

	evalCasesLookup := `
		SELECT id, question, expected_answer, expected_chunk_ids
		FROM eval_cases
		WHERE dataset_name = $1 AND tenant_id = $2
	`

	rows, err := pool.Query(
		ctx, evalCasesLookup, opts.DatasetName, opts.TenantID,
	)
	if err != nil {
		return evalRunID, 0, fmt.Errorf(
			"query evaluation cases for dataset %q: %w",
			opts.DatasetName,
			err,
		)
	}
	defer rows.Close()

	var totalPrecision, totalRecall, totalFaithfulness float64
	var totalHitRate, totalNDCG float64
	var passed, total, evaluated, failed int
	quotaExhausted := false

	for rows.Next() {
		var caseID uuid.UUID
		var question string
		var expectedAnswer *string
		var expectedChunkIDs []uuid.UUID

		if err := rows.Scan(
			&caseID,
			&question,
			&expectedAnswer,
			&expectedChunkIDs,
		); err != nil {
			failed++
			total++
			log.Error("failed to scan evaluation case", "error", err)
			continue
		}

		total++

		// Continue counting cases, but stop making requests after daily quota exhaustion.
		if quotaExhausted {
			continue
		}

		retMetrics, retrievedIDs, err := EvaluateRetrieval(
			ctx,
			retrievalSvc,
			embedder,
			opts.TenantID,
			opts.EvalUserID,
			question,
			expectedChunkIDs,
			opts.K,
		)
		if err != nil {
			failed++
			log.Error(
				"retrieval eval failed for case",
				"case_id", caseID,
				"error", err,
			)
			continue
		}

		contextText, answer, err := generateAnswerForEval(
			ctx,
			retrievalSvc,
			embedder,
			llmSvc,
			promptBuilder,
			counter,
			cfg,
			opts.TenantID,
			opts.EvalUserID,
			question,
			opts.K,
		)
		if err != nil {
			failed++
			log.Error(
				"generation failed for eval case",
				"case_id", caseID,
				"error", err,
			)
			if llm.IsDailyQuotaExceeded(err) {
				quotaExhausted = true
				log.Error("daily LLM quota exhausted; skipping remaining evaluation work")
			}
			continue
		}

		faith, err := EvaluateFaithfulness(
			ctx,
			llmClient,
			question,
			answer,
			contextText,
		)
		if err != nil {
			failed++
			log.Error(
				"faithfulness eval failed for case",
				"case_id", caseID,
				"error", err,
			)
			if llm.IsDailyQuotaExceeded(err) {
				quotaExhausted = true
				log.Error("daily LLM quota exhausted; skipping remaining evaluation work")
			}
			continue
		}

		casePassed := retMetrics.RecallAtK > 0 && faith.Supported

		resultEvalInsertQ := `
			INSERT INTO eval_results (
				eval_run_id,
				eval_case_id,
				retrieved_chunk_ids,
				generated_answer,
				precision_at_k,
				recall_at_k,
				hit_rate_at_k,
				ndcg_at_k,
				faithfulness_score,
				passed,
				judge_reasoning
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`

		_, err = pool.Exec(
			ctx,
			resultEvalInsertQ,
			evalRunID,
			caseID,
			retrievedIDs,
			answer,
			retMetrics.PrecisionAtK,
			retMetrics.RecallAtK,
			retMetrics.HitRateAtK,
			retMetrics.NDCGAtK,
			faith.Score,
			casePassed,
			faith.Reasoning,
		)
		if err != nil {
			failed++
			log.Error(
				"failed to persist eval result",
				"eval_run_id", evalRunID,
				"case_id", caseID,
				"error", err,
			)
			continue
		}

		evaluated++

		totalPrecision += retMetrics.PrecisionAtK
		totalRecall += retMetrics.RecallAtK
		totalHitRate += retMetrics.HitRateAtK
		totalNDCG += retMetrics.NDCGAtK
		totalFaithfulness += faith.Score

		if casePassed {
			passed++
		}
	}

	if err := rows.Err(); err != nil {
		return evalRunID, 0, fmt.Errorf(
			"iterate evaluation cases for dataset %q: %w",
			opts.DatasetName,
			err,
		)
	}

	if evaluated == 0 {
		return evalRunID, 0, fmt.Errorf(
			"evaluation run %s produced no persisted results for dataset %q (total cases: %d, failed cases: %d)",
			evalRunID,
			opts.DatasetName,
			total,
			failed,
		)
	}

	denominator := float64(evaluated)

	avgPrecision := totalPrecision / denominator
	avgRecall := totalRecall / denominator
	avgHitRate := totalHitRate / denominator
	avgNDCG := totalNDCG / denominator
	avgFaithfulness := totalFaithfulness / denominator
	passRate = float64(passed) / denominator
	complete := failed == 0 && evaluated == total
	var storedPassRate any = passRate
	if !complete {
		// NULL represents unknown for an incomplete run, not a misleading percentage.
		storedPassRate = nil
	}

	updateRunEvalQ := `
		UPDATE eval_runs
		SET completed_at = now(),
			avg_precision_at_k = $1,
			avg_recall_at_k = $2,
			avg_hit_rate_at_k = $3,
			avg_ndcg_at_k = $4,
			avg_faithfulness = $5,
			pass_rate = $6
		WHERE id = $7
	`

	_, err = pool.Exec(
		ctx,
		updateRunEvalQ,
		avgPrecision,
		avgRecall,
		avgHitRate,
		avgNDCG,
		avgFaithfulness,
		storedPassRate,
		evalRunID,
	)
	if err != nil {
		return evalRunID, passRate, fmt.Errorf(
			"update evaluation run metrics: %w",
			err,
		)
	}

	if complete {
		metrics.EvaluationPassRate.
			WithLabelValues(opts.DatasetName).
			Set(passRate)
	}
	metrics.GroundednessScore.
		WithLabelValues(opts.DatasetName).
		Set(avgFaithfulness)

	log.Info(
		"evaluation run finished",
		"eval_run_id", evalRunID,
		"dataset", opts.DatasetName,
		"complete", complete,
		"pass_rate", storedPassRate,
		"avg_precision", avgPrecision,
		"avg_recall", avgRecall,
		"avg_hit_rate_at_k", avgHitRate,
		"avg_ndcg_at_k", avgNDCG,
		"avg_faithfulness", avgFaithfulness,
		"total_cases", total,
		"evaluated_cases", evaluated,
		"failed_cases", failed,
	)

	if !complete {
		reason := ""
		if quotaExhausted {
			reason = "; stopped because the LLM daily quota was exhausted"
		}
		return evalRunID, passRate, fmt.Errorf(
			"evaluation run %s incomplete for dataset %q: total cases=%d, evaluated cases=%d, failed cases=%d%s",
			evalRunID,
			opts.DatasetName,
			total,
			evaluated,
			failed,
			reason,
		)
	}

	return evalRunID, passRate, nil
}

func generateAnswerForEval(ctx context.Context, retrievalSvc *retrieval.Service, embedder *embedding.Service, llmSvc *llm.Service, promptBuilder *prompt.Builder, counter tokenizer.Counter, cfg *models.Config, tenantID, evalUserID uuid.UUID, question string, k int) (contextText string, answer string, err error) {
	embeddings, err := embedder.Embed(ctx, []string{question})
	if err != nil {
		return "", "", err
	}
	if len(embeddings) == 0 {
		return "", "", fmt.Errorf("embedding provider returned no embedding")
	}
	retrieveQuery := models.RetrievalQuery{
		TenantID:       tenantID,
		UserID:         evalUserID,
		Query:          question,
		QueryEmbedding: embeddings[0],
		Limit:          k,
	}
	results, err := retrievalSvc.Retrieve(ctx, retrieveQuery)
	if err != nil {
		return "", "", err
	}

	overhead := promptBuilder.SystemPromptTokens() + counter.Count(question)
	maxContextTokens := cfg.LLM.MaxContextTokens - overhead
	if maxContextTokens < 0 {
		maxContextTokens = 0
	}

	contextText, _ = chat.BuildContext(results, maxContextTokens, counter)

	genReq, err := promptBuilder.Build(&models.Conversation{}, question, contextText)
	if err != nil {
		return "", "", err
	}

	result, err := llmSvc.Generate(ctx, genReq.Prompt)
	if err != nil {
		return "", "", err
	}

	return contextText, result.Text, nil
}

func GetBaselineScore(ctx context.Context, pool *pgxpool.Pool, datasetName string) (avgFaithfulness float64, found bool, err error) {
	row := pool.QueryRow(ctx, `
		SELECT avg_faithfulness FROM eval_runs
		WHERE dataset_name = $1 AND is_baseline = true
		ORDER BY started_at DESC LIMIT 1
	`, datasetName)
	err = row.Scan(&avgFaithfulness)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}

		return 0, false, fmt.Errorf("get baseline score: %w", err)
	}

	return avgFaithfulness, true, nil
}

// validateExpectedChunkOwnership rejects ground-truth references that cannot
// be resolved to chunks belonging to this tenant's active documents/versions.
// Global evaluation cases are intentionally excluded.
func validateExpectedChunkOwnership(ctx context.Context, pool *pgxpool.Pool, datasetName string, tenantID uuid.UUID) error {
	const query = `
		SELECT ec.id, expected.chunk_id
		FROM eval_cases ec
		CROSS JOIN LATERAL unnest(
			COALESCE(ec.expected_chunk_ids, ARRAY[]::uuid[])
		) AS expected(chunk_id)
		LEFT JOIN document_chunks dc
			ON dc.id = expected.chunk_id
			AND dc.tenant_id = ec.tenant_id
		LEFT JOIN documents d
			ON d.id = dc.document_id
			AND d.tenant_id = ec.tenant_id
			AND d.deleted_at IS NULL
			AND d.status = 'active'
		LEFT JOIN document_versions dv
			ON dv.id = dc.document_version_id
			AND dv.document_id = dc.document_id
			AND dv.status = 'active'
		WHERE ec.dataset_name = $1
			AND ec.tenant_id = $2
			AND (
				dc.id IS NULL
				OR d.id IS NULL
				OR dv.id IS NULL
			)
		LIMIT 1
	`

	var caseID, chunkID uuid.UUID
	err := pool.QueryRow(ctx, query, datasetName, tenantID).
		Scan(&caseID, &chunkID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf(
			"validate expected chunk ownership for dataset %q: %w",
			datasetName, err,
		)
	}

	return fmt.Errorf("evaluation case %s references chunk %s that does not belong to tenant %s or is not in an active document version", caseID, chunkID, tenantID)
}
