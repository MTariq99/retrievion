package tokenizer

import (
	"fmt"
	"strings"

	"github.com/mtariq99/retrievion/models"
	genai "google.golang.org/genai"
	geminitokenizer "google.golang.org/genai/tokenizer"
)

type Counter interface {
	Count(text string) int
}

type Tokenizer struct {
	local *geminitokenizer.LocalTokenizer
}

func New(cfg *models.Config) (*Tokenizer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("tokenizer: config is required")
	}

	model := strings.TrimSpace(cfg.LLM.Model)
	if model == "" {
		return nil, fmt.Errorf("tokenizer: LLM_MODEL is required")
	}

	local, err := geminitokenizer.NewLocalTokenizer(model)
	if err != nil {
		return nil, fmt.Errorf(
			"tokenizer: initialize Gemini tokenizer for model %q: %w",
			model,
			err,
		)
	}

	return &Tokenizer{
		local: local,
	}, nil
}

func NewCounter(cfg *models.Config) (Counter, error) {
	t, err := New(cfg)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (t *Tokenizer) Count(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}

	contents := []*genai.Content{
		genai.NewContentFromText(text, "user"),
	}

	result, err := t.local.CountTokens(contents, nil)
	if err != nil {
		// This should be extremely rare because the tokenizer is
		// initialized successfully during application startup.
		//
		// Keep the application alive rather than returning zero,
		// which would be much more dangerous for context budgeting.
		return fallbackEstimate(text)
	}

	if result == nil || result.TotalTokens < 1 {
		return 1
	}

	return int(result.TotalTokens)
}

func fallbackEstimate(text string) int {
	tokens := len(text) / 4

	if tokens < 1 && strings.TrimSpace(text) != "" {
		return 1
	}

	return tokens
}
