package evaluation

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mtariq99/retrievion/internal/embedding"
	"github.com/mtariq99/retrievion/internal/retrieval"
	"github.com/mtariq99/retrievion/models"
)

type RetrievalMetrics struct {
	PrecisionAtK float64
	RecallAtK    float64
	MRR          float64
	HitRateAtK   float64
	NDCGAtK      float64
}

func EvaluateRetrieval(ctx context.Context, retrievalSvc *retrieval.Service, embedder *embedding.Service, tenantID uuid.UUID, evalUserID uuid.UUID, question string, expectedChunkIDs []uuid.UUID, k int) (RetrievalMetrics, []uuid.UUID, error) {
	if k <= 0 {
		return RetrievalMetrics{}, nil, fmt.Errorf("k must be greater than zero")
	}
	embeddings, err := embedder.Embed(ctx, []string{question})
	if err != nil {
		return RetrievalMetrics{}, nil, fmt.Errorf("embed eval question: %w", err)
	}
	if len(embeddings) == 0 {
		return RetrievalMetrics{}, nil, fmt.Errorf("embedding provider returned no embedding")
	}

	results, err := retrievalSvc.Retrieve(ctx, models.RetrievalQuery{
		TenantID:       tenantID,
		UserID:         evalUserID,
		Query:          question,
		QueryEmbedding: embeddings[0],
		Limit:          k,
	})
	if err != nil {
		return RetrievalMetrics{}, nil, err
	}

	retrievedIDs := make([]uuid.UUID, 0, len(results))
	for _, r := range results {
		retrievedIDs = append(retrievedIDs, r.ChunkID)
	}

	expectedSet := make(map[uuid.UUID]struct{}, len(expectedChunkIDs))
	for _, id := range expectedChunkIDs {
		expectedSet[id] = struct{}{}
	}

	var relevantRetrieved int
	firstHitRank := -1
	seenRetrieved := make(map[uuid.UUID]struct{}, len(retrievedIDs))

	for i, id := range retrievedIDs {
		if _, duplicate := seenRetrieved[id]; duplicate {
			continue
		}
		seenRetrieved[id] = struct{}{}

		if _, relevant := expectedSet[id]; relevant {
			relevantRetrieved++
			if firstHitRank == -1 {
				firstHitRank = i + 1
			}
		}
	}

	precision := float64(relevantRetrieved) / float64(k)

	recall := 0.0
	if len(expectedSet) > 0 {
		recall = float64(relevantRetrieved) / float64(len(expectedSet))
	}

	mrr := 0.0
	if firstHitRank > 0 {
		mrr = 1.0 / float64(firstHitRank)
	}
	hitRateAtK, ndcgAtK := calculateRankingMetrics(expectedChunkIDs, retrievedIDs, k)

	return RetrievalMetrics{
		PrecisionAtK: precision,
		RecallAtK:    recall,
		MRR:          mrr,
		HitRateAtK:   hitRateAtK,
		NDCGAtK:      ndcgAtK,
	}, retrievedIDs, nil
}

func RunRetrievalEvalDataset(ctx context.Context, pool *pgxpool.Pool, retrievalSvc *retrieval.Service, embedder *embedding.Service, tenantID uuid.UUID, evalUserID uuid.UUID, datasetName string, k int) (avgPrecision, avgRecall, avgMRR, avgHitRate, avgNDCG float64, count int, err error) {
	if tenantID == uuid.Nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("evaluation tenant ID must not be empty")
	}
	if evalUserID == uuid.Nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("evaluation user ID must not be empty")
	}

	rows, err := pool.Query(ctx, `
		SELECT question, expected_chunk_ids
		FROM eval_cases
		WHERE dataset_name = $1 AND tenant_id = $2
	`, datasetName, tenantID)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	defer rows.Close()

	var totalP, totalR, totalMRR, totalHitRate, totalNDCG float64
	for rows.Next() {
		var question string
		var expected []uuid.UUID
		if err := rows.Scan(&question, &expected); err != nil {
			return 0, 0, 0, 0, 0, 0, err
		}

		m, _, err := EvaluateRetrieval(ctx, retrievalSvc, embedder, tenantID, evalUserID, question, expected, k)
		if err != nil {
			continue
		}

		totalP += m.PrecisionAtK
		totalR += m.RecallAtK
		totalMRR += m.MRR
		count++
		totalHitRate += m.HitRateAtK
		totalNDCG += m.NDCGAtK
	}

	if count == 0 {
		return 0, 0, 0, 0, 0, 0, nil
	}
	return totalP / float64(count),
		totalR / float64(count),
		totalMRR / float64(count),
		totalHitRate / float64(count),
		totalNDCG / float64(count),
		count,
		nil
}

func calculateRankingMetrics(expectedChunkIDs, retrievedIDs []uuid.UUID, k int) (hitRateAtK, ndcgAtK float64) {
	if k <= 0 {
		return 0, 0
	}

	relevant := make(map[uuid.UUID]struct{}, len(expectedChunkIDs))
	for _, id := range expectedChunkIDs {
		relevant[id] = struct{}{}
	}

	limit := min(len(retrievedIDs), k)

	seen := make(map[uuid.UUID]struct{}, limit)
	var dcg float64

	for i := range limit {
		id := retrievedIDs[i]
		// a chunk can contribute at most once
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := relevant[id]; ok {
			if hitRateAtK == 0 {
				hitRateAtK = 1
			}
			rank := i + 1
			dcg += 1 / math.Log2(float64(rank+1))
		}
	}
	idealRelevant := min(len(relevant), k)
	var idcg float64
	for i := range idealRelevant {
		idcg += 1 / math.Log2(float64(i+2))
	}

	if idcg > 0 {
		ndcgAtK = dcg / idcg
	}

	return hitRateAtK, ndcgAtK
}
