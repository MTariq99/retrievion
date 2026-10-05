package retrieval

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/models"
)

type Retriever interface {
	Retrieve(
		ctx context.Context,
		query models.RetrievalQuery,
	) ([]models.RetrievedChunk, error)
}

type Service struct {
	vectorSearcher  VectorSearcher
	keywordSearcher KeywordSearcher
	fusion          ResultFusion
	reranker        Reranker
}

func NewService(
	vectorSearcher VectorSearcher,
	keywordSearcher KeywordSearcher,
	fusion ResultFusion,
	reranker Reranker,
) *Service {
	return &Service{
		vectorSearcher:  vectorSearcher,
		keywordSearcher: keywordSearcher,
		fusion:          fusion,
		reranker:        reranker,
	}
}

func (s *Service) Retrieve(
	ctx context.Context,
	query models.RetrievalQuery,
) ([]models.RetrievedChunk, error) {
	if query.TenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant ID is required")
	}

	if query.UserID == uuid.Nil {
		return nil, fmt.Errorf("user ID is required")
	}

	if query.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	if len(query.QueryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is required")
	}

	limit := query.Limit

	if limit <= 0 {
		limit = consts.DefaultRetrievalLimit
	}

	vectorResults, err := s.vectorSearcher.Search(
		ctx,
		query.TenantID,
		query.UserID,
		query.QueryEmbedding,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	keywordResults, err := s.keywordSearcher.Search(
		ctx,
		query.TenantID,
		query.UserID,
		query.Query,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}

	fusedResults := s.fusion.Fuse(
		vectorResults,
		keywordResults,
	)

	if len(fusedResults) == 0 {
		return []models.RetrievedChunk{}, nil
	}

	rerankedResults, err := s.reranker.Rerank(
		ctx,
		query.Query,
		fusedResults,
	)
	if err != nil {
		return nil, fmt.Errorf("rerank results: %w", err)
	}

	if len(rerankedResults) > consts.FinalRetrievalTopN {
		rerankedResults = rerankedResults[:consts.FinalRetrievalTopN]
	}

	return rerankedResults, nil
}
