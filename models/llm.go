package models

import (
	"time"

	"github.com/mtariq99/retrievion/internal/consts"
)

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

type StreamEvent struct {
	Type      consts.StreamEventType
	Content   string
	Usage     *TokenUsage
	Err       error
	Timestamp time.Time
}
