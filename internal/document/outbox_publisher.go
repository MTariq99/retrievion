package document

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/storage"
)

const (
	outboxBatchSize   = 25
	outboxMaxAttempts = 10
)

type OutboxPublisher struct {
	repo  *Repository
	queue storage.Queue
	log   *zap.Logger
}

func NewOutboxPublisher(repo *Repository, queue storage.Queue, logger *zap.Logger) *OutboxPublisher {
	return &OutboxPublisher{
		repo:  repo,
		queue: queue,
		log:   logger,
	}
}

func (p *OutboxPublisher) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		if err := p.publishBatch(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			p.log.Error("outbox publisher batch failed", zap.Error(err))
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (p *OutboxPublisher) publishBatch(ctx context.Context) error {
	events, err := p.repo.ClaimOutboxEvents(ctx, outboxBatchSize, 30*time.Second)
	if err != nil {
		return err
	}

	for _, event := range events {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		rk, err := routingKeyFor(event.EventType)
		if err == nil {
			err = p.queue.Publish(ctx, rk, event.Payload)
		}
		if err == nil {
			err = p.repo.MarkOutboxEventPublished(ctx, event)
			if err == nil {
				continue
			}

			p.log.Error("published event but could not mark outbox row", zap.String("event_id", event.ID.String()), zap.Error(err))
			continue
		}

		delay := retryDelay(event.Attempts)
		if recordErr := p.repo.MarkOutboxEventFailed(ctx, event, err, delay, outboxMaxAttempts); recordErr != nil {
			p.log.Error("could not record outbox publish failure", zap.String("event_id", event.ID.String()), zap.Error(recordErr))
			continue
		}

		if event.Attempts >= outboxMaxAttempts {
			p.log.Error("outbox event dead-lettered after retries", zap.String("event_id", event.ID.String()), zap.Int("attempts", event.Attempts), zap.Error(err))
		} else {
			p.log.Warn("outbox publish failed; retry scheduled", zap.String("event_id", event.ID.String()), zap.Int("attempts", event.Attempts), zap.Error(err))
		}
	}

	return nil
}

func retryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 8 {
		attempts = 8
	}
	return time.Duration(1<<attempts) * time.Second
}

func routingKeyFor(eventType string) (string, error) {
	switch eventType {
	case consts.EventTypeIngestionJobCreated:
		return consts.IngestionRoutingKey, nil
	default:
		return "", fmt.Errorf("no routing key for outbox event type %q", eventType)
	}
}
