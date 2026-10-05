package ingestion

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/models"
)

const ingestionQueueName = "ingestion.jobs"

type Worker struct {
	queue    storage.Queue
	pipeline *Pipeline
	logger   *zap.Logger
}

func NewWorker(queue storage.Queue, pipeline *Pipeline, logger *zap.Logger) *Worker {
	return &Worker{queue: queue, pipeline: pipeline, logger: logger}
}

func (w *Worker) Run(ctx context.Context) error {
	return w.queue.Consume(ctx, ingestionQueueName, func(msg models.QueueMessage) error {
		return w.handle(ctx, msg)
	})
}

// handle returns an error to trigger Nack (→ dead-letter queue) or nil to
// trigger Ack — Consume in internal/storage/queue.go does both itself, so
// this must not call Ack/Nack directly.
func (w *Worker) handle(ctx context.Context, msg models.QueueMessage) (err error) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("ingestion worker panic", zap.Any("recover", r))
			err = fmt.Errorf("ingestion worker panic: %v", r)
		}
	}()

	job, parseErr := ParseJob(msg.Body)
	if parseErr != nil {
		w.logger.Error("invalid ingestion job payload", zap.Error(parseErr))
		return parseErr
	}

	if procErr := w.pipeline.Process(ctx, job); procErr != nil {
		w.logger.Error("ingestion job failed", zap.String("job_id", job.JobID.String()), zap.Error(procErr))
		return procErr
	}

	return nil
}
