// internal/evaluation/runner.go
package evaluation

import (
	"context"

	"github.com/google/uuid"
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

func RunFullEvaluation(
	ctx context.Context,
	pool *pgxpool.Pool,
	retrievalSvc *retrieval.Service,
	embedder *embedding.Service,
	llmSvc *llm.Service,
	llmClient *llm.Client,
	promptBuilder *prompt.Builder,
	counter tokenizer.Counter, // NEW — injected once at startup, shared with chat.Service
	cfg *models.Config,
	opts RunOptions,
) (evalRunID uuid.UUID, passRate float64, err error) {
	log := logger.FromContext(ctx)

	evalRunID = uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO eval_runs (id, dataset_name, prompt_version, model, is_baseline, started_at)
		VALUES ($1, $2, $3, $4, $5, now())
	`, evalRunID, opts.DatasetName, opts.PromptVersion, opts.Model, opts.MarkAsBaseline)
	if err != nil {
		return evalRunID, 0, err
	}

	rows, err := pool.Query(ctx, `
		SELECT id, question, expected_answer, expected_chunk_ids
		FROM eval_cases WHERE dataset_name = $1
	`, opts.DatasetName)
	if err != nil {
		return evalRunID, 0, err
	}
	defer rows.Close()

	var totalPrecision, totalRecall, totalFaithfulness float64
	var passed, total int

	for rows.Next() {
		var caseID uuid.UUID
		var question, expectedAnswer string
		var expectedChunkIDs []uuid.UUID
		if err := rows.Scan(&caseID, &question, &expectedAnswer, &expectedChunkIDs); err != nil {
			continue
		}
		total++

		retMetrics, retrievedIDs, err := EvaluateRetrieval(
			ctx, retrievalSvc, embedder, opts.TenantID, opts.EvalUserID,
			question, expectedChunkIDs, opts.K,
		)
		if err != nil {
			log.Error("retrieval eval failed for case", "case_id", caseID, "error", err)
			continue
		}

		contextTexts, answer, err := generateAnswerForEval(
			ctx, retrievalSvc, embedder, llmSvc, promptBuilder, counter, cfg,
			opts.TenantID, opts.EvalUserID, question, opts.K,
		)
		if err != nil {
			log.Error("generation failed for eval case", "case_id", caseID, "error", err)
			continue
		}

		faith, err := EvaluateFaithfulness(ctx, llmClient, question, answer, contextTexts)
		if err != nil {
			log.Error("faithfulness eval failed for case", "case_id", caseID, "error", err)
			continue
		}

		casePassed := retMetrics.RecallAtK > 0 && faith.Supported

		_, err = pool.Exec(ctx, `
			INSERT INTO eval_results (
				eval_run_id, eval_case_id, retrieved_chunk_ids, generated_answer,
				precision_at_k, recall_at_k, faithfulness_score, passed, judge_reasoning
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, evalRunID, caseID, retrievedIDs, answer,
			retMetrics.PrecisionAtK, retMetrics.RecallAtK, faith.Score, casePassed, faith.Reasoning)
		if err != nil {
			log.Error("failed to persist eval result", "error", err)
		}

		totalPrecision += retMetrics.PrecisionAtK
		totalRecall += retMetrics.RecallAtK
		totalFaithfulness += faith.Score
		if casePassed {
			passed++
		}
	}

	if total == 0 {
		return evalRunID, 0, nil
	}

	avgPrecision := totalPrecision / float64(total)
	avgRecall := totalRecall / float64(total)
	avgFaithfulness := totalFaithfulness / float64(total)
	passRate = float64(passed) / float64(total)

	_, err = pool.Exec(ctx, `
		UPDATE eval_runs
		SET completed_at = now(), avg_precision_at_k = $1, avg_recall_at_k = $2,
		    avg_faithfulness = $3, pass_rate = $4
		WHERE id = $5
	`, avgPrecision, avgRecall, avgFaithfulness, passRate, evalRunID)

	metrics.EvaluationPassRate.WithLabelValues(opts.DatasetName).Set(passRate)
	metrics.GroundednessScore.WithLabelValues(opts.DatasetName).Set(avgFaithfulness)

	log.Info(
		"evaluation run completed",
		"eval_run_id", evalRunID,
		"dataset", opts.DatasetName,
		"pass_rate", passRate,
		"avg_faithfulness", avgFaithfulness,
		"avg_recall", avgRecall,
	)

	return evalRunID, passRate, err
}

func generateAnswerForEval(
	ctx context.Context,
	retrievalSvc *retrieval.Service,
	embedder *embedding.Service,
	llmSvc *llm.Service,
	promptBuilder *prompt.Builder,
	counter tokenizer.Counter,
	cfg *models.Config,
	tenantID, evalUserID uuid.UUID,
	question string,
	k int,
) (contextTexts []string, answer string, err error) {
	embeddings, err := embedder.Embed(ctx, []string{question})
	if err != nil {
		return nil, "", err
	}
	if len(embeddings) == 0 {
		return nil, "", err
	}

	results, err := retrievalSvc.Retrieve(ctx, models.RetrievalQuery{
		TenantID:       tenantID,
		UserID:         evalUserID,
		Query:          question,
		QueryEmbedding: embeddings[0],
		Limit:          k,
	})
	if err != nil {
		return nil, "", err
	}

	for _, r := range results {
		contextTexts = append(contextTexts, r.Content)
	}

	overhead := promptBuilder.SystemPromptTokens() + counter.Count(question)
	maxContextTokens := cfg.LLM.MaxContextTokens - overhead
	if maxContextTokens < 0 {
		maxContextTokens = 0
	}

	contextText, _ := chat.BuildContext(results, maxContextTokens, counter)

	genReq, err := promptBuilder.Build(&models.Conversation{}, question, contextText)
	if err != nil {
		return nil, "", err
	}

	result, err := llmSvc.Generate(ctx, genReq.Prompt)
	if err != nil {
		return nil, "", err
	}

	return contextTexts, result.Text, nil
}

func GetBaselineScore(ctx context.Context, pool *pgxpool.Pool, datasetName string) (avgFaithfulness float64, found bool, err error) {
	row := pool.QueryRow(ctx, `
		SELECT avg_faithfulness FROM eval_runs
		WHERE dataset_name = $1 AND is_baseline = true
		ORDER BY started_at DESC LIMIT 1
	`, datasetName)
	err = row.Scan(&avgFaithfulness)
	if err != nil {
		return 0, false, nil
	}
	return avgFaithfulness, true, nil
}
