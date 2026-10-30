package retrieval

import (
	"fmt"
	"strings"

	"github.com/mtariq99/retrievion/models"
)

func ProvideReranker(cfg *models.Config) (Reranker, error) {
	if cfg == nil {
		return nil, fmt.Errorf("reranker config is nil")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Reranker.Provider)) {
	case "", "noop", "none":
		return NewNoopReranker(), nil
	case "cohere":
		return NewCohereReranker(cfg.Reranker)
	default:
		return nil, fmt.Errorf("unsupported reranker provider: %q", cfg.Reranker.Provider)

	}
}
