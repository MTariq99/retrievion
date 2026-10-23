package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/models"
)

type Client struct {
	apiKey      string
	httpClient  *http.Client
	GenerateURL string
	StreamURL   string
	redisClient *storage.RedisClient
}

func NewClient(cfg *models.Config, redisC *storage.RedisClient) (*Client, error) {
	key := cfg.LLM.GeminiAPIKey
	if key == "" {
		return nil, fmt.Errorf("llm: GEMINI_API_KEY is required")
	}
	gurl := cfg.LLM.GenerateURL
	if gurl == "" {
		return nil, fmt.Errorf("llm: GENERATEURL is required")
	}
	streamURL := cfg.LLM.StreamURL
	if streamURL == "" {
		return nil, fmt.Errorf("llm: STREAMURL is required")
	}

	return &Client{
		apiKey:      key,
		httpClient:  &http.Client{Timeout: 60 * time.Second},
		GenerateURL: gurl,
		StreamURL:   streamURL,
		redisClient: redisC,
	}, nil
}

func (c *Client) Generate(ctx context.Context, req models.GenerateRequest) (models.GenerateResponse, error) {
	result, err := c.doGenerate(ctx, req.Prompt, req.MaxTokens, req.Temperature)
	if err != nil {
		return models.GenerateResponse{}, err
	}
	return models.GenerateResponse{
		Text: result.Text,
		Usage: models.TokenUsage{
			PromptTokens:     result.PromptTokens,
			CompletionTokens: result.CompletionTokens,
			TotalTokens:      result.TotalTokens,
		},
	}, nil
}

func (c *Client) doGenerate(ctx context.Context, prompt string, maxTokens int, temperature float64) (*models.GeminiResult, error) {
	reqBody := models.GeminiRequest{
		Contents: []models.Content{
			{Parts: []models.Part{{Text: prompt}}},
		},
		GenerationConfig: models.GenerationConfig{
			MaxOutputTokens: maxTokens,
			Temperature:     temperature,
			ThinkingConfig:  &models.ThinkingConfig{ThinkingBudget: 0},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", c.GenerateURL, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("gemini: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &ProviderError{StatusCode: resp.StatusCode}
	}

	var gemResp models.GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gemResp); err != nil {
		return nil, fmt.Errorf("gemini: decode response: %w", err)
	}

	if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini: empty response")
	}

	return &models.GeminiResult{
		Text:             gemResp.Candidates[0].Content.Parts[0].Text,
		PromptTokens:     gemResp.UsageMetadata.PromptTokenCount,
		CompletionTokens: gemResp.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      gemResp.UsageMetadata.TotalTokenCount,
	}, nil
}

func (c *Client) Stream(ctx context.Context, req models.GenerateRequest, callback func(models.StreamEvent) error) error {
	reqBody := models.GeminiRequest{
		Contents: []models.Content{
			{Parts: []models.Part{{Text: req.Prompt}}},
		},
		GenerationConfig: models.GenerationConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
			ThinkingConfig:  &models.ThinkingConfig{ThinkingBudget: 0},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s?alt=sse&key=%s", c.StreamURL, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("gemini: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("gemini: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &ProviderError{StatusCode: resp.StatusCode}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var usage models.TokenUsage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "" {
			continue
		}

		var chunk models.GeminiResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			if cbErr := callback(models.StreamEvent{Type: consts.StreamEventError, Err: fmt.Errorf("decode chunk: %w", err), Timestamp: time.Now()}); cbErr != nil {
				return cbErr
			}
			continue
		}

		if chunk.UsageMetadata.TotalTokenCount > 0 {
			usage = models.TokenUsage{
				PromptTokens:     chunk.UsageMetadata.PromptTokenCount,
				CompletionTokens: chunk.UsageMetadata.CandidatesTokenCount,
				TotalTokens:      chunk.UsageMetadata.TotalTokenCount,
			}
		}

		if len(chunk.Candidates) == 0 || len(chunk.Candidates[0].Content.Parts) == 0 {
			continue
		}

		text := chunk.Candidates[0].Content.Parts[0].Text
		if text == "" {
			continue
		}

		if err := callback(models.StreamEvent{Type: consts.StreamEventToken, Content: text, Timestamp: time.Now()}); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("gemini: read stream: %w", err)
	}

	return callback(models.StreamEvent{Type: consts.StreamEventDone, Usage: &usage, Timestamp: time.Now()})
}

type ProviderError struct {
	StatusCode int
	Status     string
	Message    string
	Body       string
}

func (e *ProviderError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("gemini: status=%d message=%s", e.StatusCode, e.Message)
	}

	if e.Status != "" {
		return fmt.Sprintf("gemini: status=%d status=%s", e.StatusCode, e.Status)
	}

	return fmt.Sprintf("gemini: unexpected status %d", e.StatusCode)
}

func (e *ProviderError) ContextLengthExceeded() bool {
	message := strings.ToLower(
		e.Message + " " + e.Status + " " + e.Body,
	)

	return strings.Contains(message, "context length") ||
		strings.Contains(message, "context window") ||
		strings.Contains(message, "maximum context") ||
		strings.Contains(message, "too many tokens") ||
		strings.Contains(message, "input token") ||
		strings.Contains(message, "token limit")
}

func (e *ProviderError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests ||
		e.StatusCode >= 500
}

func (c *Client) GenerateStructured(ctx context.Context, prompt string, schema map[string]interface{}) (string, error) {
	reqBody := models.GeminiRequest{
		Contents: []models.Content{
			{Parts: []models.Part{{Text: prompt}}},
		},
		GenerationConfig: models.GenerationConfig{
			MaxOutputTokens:  2048,
			Temperature:      0.0, // judgment task — consistency over creativity
			ResponseMimeType: "application/json",
			ResponseSchema:   schema,
			ThinkingConfig:   &models.ThinkingConfig{ThinkingBudget: 0},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", c.GenerateURL, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("gemini: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("gemini: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", &ProviderError{StatusCode: resp.StatusCode}
	}

	var gemResp models.GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gemResp); err != nil {
		return "", fmt.Errorf("gemini: decode response: %w", err)
	}

	if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: empty response")
	}

	return gemResp.Candidates[0].Content.Parts[0].Text, nil
}
