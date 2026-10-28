package ingestion

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const (
	recoveryPollInterval = 30 * time.Second
	recoveryStaleAfter   = 5 * time.Minute
	recoveryBatchSize    = 25
)

type RecoveryWorker struct {
	repo   *Repository
	logger *zap.Logger
}

func NewRecoveryWorker(repo *Repository, logger *zap.Logger) *RecoveryWorker {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &RecoveryWorker{
		repo:   repo,
		logger: logger,
	}
}

func (w *RecoveryWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(recoveryPollInterval)
	defer ticker.Stop()

	// Recover immediately on startup, then poll periodically.
	if err := w.recover(ctx); err != nil && ctx.Err() == nil {
		w.logger.Error("stale ingestion job recovery failed", zap.Error(err))
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.recover(ctx); err != nil && ctx.Err() == nil {
				w.logger.Error("stale ingestion job recovery failed", zap.Error(err))
			}
		}
	}
}

func (w *RecoveryWorker) recover(ctx context.Context) error {
	recovered, err := w.repo.RecoverStaleJobs(
		ctx,
		recoveryStaleAfter,
		recoveryBatchSize,
	)
	if err != nil {
		return err
	}

	if recovered > 0 {
		w.logger.Warn(
			"recovered stale ingestion jobs",
			zap.Int("count", recovered),
		)
	}

	return nil
}
