package objectstorage

import (
	"context"
	"io"
	"time"
)

type Storage interface {
	Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) (string, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	// Download streams the object's contents. Caller must Close the reader.
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	// Returns object key or public URL depending on implementation.
	GetURL(key string) string
	// Generate temporary download URL.
	PresignGetURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}
