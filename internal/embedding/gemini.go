package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
)

type GeminiProvider struct {
	apiKey     string
	model      string
	dimension  int
	httpClient *http.Client
}

func NewGeminiProvider(cfg *models.Config) (*GeminiProvider, error) {
	apiKey := cfg.Embedding.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("embedding: API key is required (config Embedding.APIKey)")
	}

	model := cfg.Embedding.Model
	if model == "" {
		model = consts.DefaultGeminiEmbeddingModel
	}

	return &GeminiProvider{
		apiKey:    apiKey,
		model:     model,
		dimension: consts.GeminiDimension,
		httpClient: &http.Client{
			Timeout: consts.GeminiTimeout,
		},
	}, nil
}

func (g *GeminiProvider) Metadata() models.EmbeddingModelMetadata {
	return models.EmbeddingModelMetadata{
		Provider:  consts.GeminiProvider,
		Model:     g.model,
		Dimension: g.dimension,
	}
}

func (g *GeminiProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, errs.ErrEmptyInput
	}
	if len(texts) > consts.GeminiMaxBatchSize {
		return nil, fmt.Errorf("embedding: batch of %d exceeds gemini limit of %d, use Service instead", len(texts), consts.GeminiMaxBatchSize)
	}

	reqBody := geminiBatchRequest{Requests: make([]geminiEmbedRequest, len(texts))}
	for i, t := range texts {
		reqBody.Requests[i] = geminiEmbedRequest{
			Model:                "models/" + g.model,
			Content:              geminiContent{Parts: []geminiPart{{Text: t}}},
			OutputDimensionality: g.dimension,
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("embedding: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:batchEmbedContents?key=%s", consts.GeminiBaseURL, g.model, g.apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("embedding: http request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embedding: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &ProviderError{StatusCode: resp.StatusCode, Body: string(respBytes)}
	}

	var parsed geminiBatchResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("embedding: unmarshal response: %w", err)
	}

	if len(parsed.Embeddings) != len(texts) {
		return nil, errs.ErrDimensionMismatch
	}

	vectors := make([][]float32, len(parsed.Embeddings))
	for i, e := range parsed.Embeddings {
		if len(e.Values) != g.dimension {
			return nil, fmt.Errorf("embedding: dimension mismatch for item %d: got %d, want %d", i, len(e.Values), g.dimension)
		}
		vectors[i] = e.Values
	}

	return vectors, nil
}

type geminiBatchRequest struct {
	Requests []geminiEmbedRequest `json:"requests"`
}

type geminiEmbedRequest struct {
	Model                string        `json:"model"`
	Content              geminiContent `json:"content"`
	OutputDimensionality int           `json:"outputDimensionality"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiBatchResponse struct {
	Embeddings []geminiEmbedding `json:"embeddings"`
}

type geminiEmbedding struct {
	Values []float32 `json:"values"`
}
