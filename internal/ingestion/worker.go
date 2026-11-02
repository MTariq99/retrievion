package ingestion

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/models"
)

type Worker struct {
	queue    storage.Queue
	pipeline *Pipeline
	logger   *zap.Logger
}

func NewWorker(queue storage.Queue, pipeline *Pipeline, logger *zap.Logger) *Worker {
	return &Worker{queue: queue, pipeline: pipeline, logger: logger}
}

func (w *Worker) Run(ctx context.Context) error {
	return w.queue.Consume(ctx, consts.IngestionQueueName, func(msg []byte) error {
		return w.handle(ctx, msg)
	})
}

func (w *Worker) handle(ctx context.Context, msg []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("ingestion worker panic", zap.Any("recover", r))
			err = fmt.Errorf("ingestion worker panic: %v", r)
		}
	}()

	job, parseErr := ParseJob(msg)
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

func ParseJob(body []byte) (models.Job, error) {
	var p models.JobPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return models.Job{}, fmt.Errorf("unmarshal ingestion job: %w", err)
	}

	jobID, err := uuid.Parse(p.JobID)
	if err != nil {
		return models.Job{}, fmt.Errorf("parse job_id: %w", err)
	}
	documentID, err := uuid.Parse(p.DocumentID)
	if err != nil {
		return models.Job{}, fmt.Errorf("parse document_id: %w", err)
	}
	versionID, err := uuid.Parse(p.DocumentVersionID)
	if err != nil {
		return models.Job{}, fmt.Errorf("parse version_id: %w", err)
	}

	return models.Job{JobID: jobID, DocumentID: documentID, DocumentVersionID: versionID}, nil
}
