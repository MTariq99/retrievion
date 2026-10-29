//go:build integration

package document

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/models"
)

func TestPublishedIngestionEventIsRoutable(t *testing.T) {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("RABBITMQ_URL not set")
	}

	cfg := &models.Config{}
	cfg.RabbitMQ.URL = url

	q, err := storage.NewRabbitMQQueue(cfg)
	require.NoError(t, err)
	defer q.Close()

	rk, err := routingKeyFor(consts.EventTypeIngestionJobCreated)
	require.NoError(t, err)

	require.NoError(t, q.Publish(context.Background(), rk, []byte(`{}`)))
}
