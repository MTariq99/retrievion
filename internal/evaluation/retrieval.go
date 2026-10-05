// internal/evaluation/retrieval.go
package evaluation

import (
	"context"
	"fmt"

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
}

func EvaluateRetrieval(ctx context.Context, retrievalSvc *retrieval.Service, embedder *embedding.Service, tenantID uuid.UUID, evalUserID uuid.UUID, question string, expectedChunkIDs []uuid.UUID, k int) (RetrievalMetrics, []uuid.UUID, error) {
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

	expectedSet := make(map[uuid.UUID]bool, len(expectedChunkIDs))
	for _, id := range expectedChunkIDs {
		expectedSet[id] = true
	}

	var relevantRetrieved int
	firstHitRank := -1
	for i, id := range retrievedIDs {
		if expectedSet[id] {
			relevantRetrieved++
			if firstHitRank == -1 {
				firstHitRank = i + 1
			}
		}
	}

	precision := 0.0
	if len(retrievedIDs) > 0 {
		precision = float64(relevantRetrieved) / float64(len(retrievedIDs))
	}

	recall := 0.0
	if len(expectedChunkIDs) > 0 {
		recall = float64(relevantRetrieved) / float64(len(expectedChunkIDs))
	}

	mrr := 0.0
	if firstHitRank > 0 {
		mrr = 1.0 / float64(firstHitRank)
	}

	return RetrievalMetrics{PrecisionAtK: precision, RecallAtK: recall, MRR: mrr}, retrievedIDs, nil
}

func RunRetrievalEvalDataset(ctx context.Context, pool *pgxpool.Pool, retrievalSvc *retrieval.Service, embedder *embedding.Service, tenantID uuid.UUID, evalUserID uuid.UUID, datasetName string, k int) (avgPrecision, avgRecall, avgMRR float64, count int, err error) {
	rows, err := pool.Query(ctx, `
		SELECT question, expected_chunk_ids
		FROM eval_cases
		WHERE dataset_name = $1
	`, datasetName)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	defer rows.Close()

	var totalP, totalR, totalMRR float64
	for rows.Next() {
		var question string
		var expected []uuid.UUID
		if err := rows.Scan(&question, &expected); err != nil {
			return 0, 0, 0, 0, err
		}

		m, _, err := EvaluateRetrieval(ctx, retrievalSvc, embedder, tenantID, evalUserID, question, expected, k)
		if err != nil {
			continue
		}

		totalP += m.PrecisionAtK
		totalR += m.RecallAtK
		totalMRR += m.MRR
		count++
	}

	if count == 0 {
		return 0, 0, 0, 0, nil
	}
	return totalP / float64(count), totalR / float64(count), totalMRR / float64(count), count, nil
}
