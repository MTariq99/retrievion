package embedding

import (
	"context"
	"errors"

	"github.com/mtariq99/retrievion/models"
)

var (
	ErrEmptyInput        = errors.New("embedding: no texts provided")
	ErrDimensionMismatch = errors.New("embedding: returned vector count does not match input count")
)

// Provider abstracts a single embedding model call. Implementations should
// NOT batch, retry, or rate-limit internally — that's Service's job. A
// Provider should do exactly one thing: turn texts into vectors, or fail.
type Provider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Metadata() models.EmbeddingModelMetadata
}

// ProviderError carries the HTTP status so Service can decide retryable
// (429/5xx) vs. not (400/401/403), consistent with README §50's policy.
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
