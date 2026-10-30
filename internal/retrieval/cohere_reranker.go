package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/models"
)

type CohereReranker struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

var _ Reranker = (*CohereReranker)(nil)

func NewCohereReranker(cfg models.RerankerConfig) (*CohereReranker, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("cohere: API key is required")
	}

	model := strings.TrimSpace(cfg.RerankerModel)
	if model == "" {
		model = consts.DefaultCohereModel
	}

	timeout := max(time.Duration(cfg.RerankerTimeout)*time.Second, consts.MinTimeout)

	return &CohereReranker{
		apiKey:   apiKey,
		model:    model,
		endpoint: consts.DefaultCohereEndpoint,
		client:   &http.Client{Timeout: timeout},
	}, nil
}

type cohereRerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

type cohereResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

type cohereRerankResponse struct {
	Results []cohereResult `json:"results"`
}

func (r *CohereReranker) Rerank(ctx context.Context, query string, candidates []models.RetrievedChunk) ([]models.RetrievedChunk, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("cohere: rerank query is empty")
	}
	if len(candidates) > consts.MaxDocuments {
		return nil, fmt.Errorf("cohere: %d candidates exceeds limit of %d", len(candidates), consts.MaxDocuments)
	}

	documents := make([]string, len(candidates))
	for i, c := range candidates {
		if strings.TrimSpace(c.Content) == "" {
			return nil, fmt.Errorf("cohere: candidate %d has empty content", i)
		}
		documents[i] = c.Content
	}

	body, err := json.Marshal(cohereRerankRequest{
		Model:     r.model,
		Query:     query,
		Documents: documents,
		TopN:      len(documents),
	})
	if err != nil {
		return nil, fmt.Errorf("cohere: marshal request: %w", err)
	}

	result, err := r.doWithRetry(ctx, body)
	if err != nil {
		return nil, err
	}
	return reorder(candidates, result.Results)
}

func (r *CohereReranker) doWithRetry(ctx context.Context, body []byte) (*cohereRerankResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= consts.MaxRerankRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		res, retryable, err := r.do(ctx, body)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if !retryable {
			break
		}
	}
	return nil, lastErr
}

// do performs a single attempt. The bool reports whether the error is retryable.
func (r *CohereReranker) do(ctx context.Context, body []byte) (*cohereRerankResponse, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("cohere: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, ctx.Err() == nil, fmt.Errorf("cohere: request failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, consts.MaxErrorBody))
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, consts.MaxErrorBody))
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retryable, fmt.Errorf("cohere: status %d: %s", resp.StatusCode, bytes.TrimSpace(msg))
	}

	var out cohereRerankResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, consts.MaxResponseBody)).Decode(&out); err != nil {
		return nil, false, fmt.Errorf("cohere: decode response: %w", err)
	}
	return &out, false, nil
}

func reorder(candidates []models.RetrievedChunk, results []cohereResult) ([]models.RetrievedChunk, error) {
	if len(results) == 0 {
		return nil, errors.New("cohere: empty results for non-empty candidates")
	}

	sorted := make([]cohereResult, len(results))
	copy(sorted, results)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].RelevanceScore > sorted[j].RelevanceScore
	})

	ranked := make([]models.RetrievedChunk, 0, len(sorted))
	seen := make(map[int]struct{}, len(sorted))
	for _, item := range sorted {
		if item.Index < 0 || item.Index >= len(candidates) {
			return nil, fmt.Errorf("cohere: invalid candidate index %d", item.Index)
		}
		if _, dup := seen[item.Index]; dup {
			return nil, fmt.Errorf("cohere: duplicate candidate index %d", item.Index)
		}
		seen[item.Index] = struct{}{}
		ranked = append(ranked, candidates[item.Index])
	}
	return ranked, nil
}
