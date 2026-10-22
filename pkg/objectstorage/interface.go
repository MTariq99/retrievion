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
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	GetURL(key string) string
	PresignGetURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}
