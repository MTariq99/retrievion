package models

import (
	"time"

	"github.com/mtariq99/retrievion/internal/consts"
)

// GenerateRequest is provider-agnostic input to a single LLM call. The
// prompt itself is fully assembled upstream by prompt/builder.go — this
// package doesn't know about system instructions, retrieved context, or
// conversation history, only the final string and generation params.
type GenerateRequest struct {
	Prompt      string
	MaxTokens   int
	Temperature float64
}

type GenerateResponse struct {
	Text  string
	Usage TokenUsage
}

type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type ModelConfig struct {
	Name            string
	MaxOutputTokens int
	Temperature     float64
}

// StreamEvent is emitted per chunk while streaming (README §31, §66).
// chat/service.go forwards these to the SSE encoder essentially unchanged.
type StreamEvent struct {
	Type      consts.StreamEventType
	Content   string      // set on Token events
	Usage     *TokenUsage // set on the Done event
	Err       error       // set on the Error event
	Timestamp time.Time
}

type GeminiRequest struct {
	Contents         []Content        `json:"contents"`
	GenerationConfig GenerationConfig `json:"generationConfig"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type GenerationConfig struct {
	MaxOutputTokens  int             `json:"maxOutputTokens,omitempty"`
	Temperature      float64         `json:"temperature,omitempty"`
	ThinkingConfig   *ThinkingConfig `json:"thinkingConfig,omitempty"`
	ResponseMimeType string          `json:"responseMimeType,omitempty"`
	ResponseSchema   interface{}     `json:"responseSchema,omitempty"`
}

type ThinkingConfig struct {
	ThinkingBudget int `json:"thinkingBudget"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

type GeminiResult struct {
	Text             string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}
