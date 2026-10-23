package embedding

import (
	"context"

	"github.com/mtariq99/retrievion/models"
)

type Provider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Metadata() models.EmbeddingModelMetadata
}

type ProviderError struct {
	StatusCode int
	Body       string
}

func (e *ProviderError) Error() string {
	if e.Body != "" {
		return errFmt(e.StatusCode, e.Body)
	}
	return errFmtNoBody(e.StatusCode)
}

func (e *ProviderError) Retryable() bool {
	return e.StatusCode == 429 || e.StatusCode >= 500
}

func errFmt(status int, body string) string {
	return "embedding: provider returned status " + itoa(status) + ": " + body
}

func errFmtNoBody(status int) string {
	return "embedding: provider returned status " + itoa(status)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
