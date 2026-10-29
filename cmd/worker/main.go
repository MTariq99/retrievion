package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/mtariq99/retrievion/internal/config"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	logger, err := storage.BuildLogger(cfg.App.Env)
	if err != nil {
		log.Fatalf("logger: %v", err)
	}
	defer logger.Sync()

	db, err := storage.InitDB(cfg, logger)
	if err != nil {
		logger.Fatal("db init failed", zap.Error(err))
	}
	defer db.Close()

	workers, cleanup, err := InitializeWorker(db, logger)
	if err != nil {
		logger.Fatal("wire init failed", zap.Error(err))
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		logger.Info("ingestion worker starting", zap.String("env", cfg.App.Env))
		return workers.Ingestion.Run(gctx)
	})
	g.Go(func() error {
		return workers.Invitations.Run(gctx)
	})
	g.Go(func() error {
		logger.Info("outbox publisher starting")
		return workers.OutboxPublisher.Run(gctx)
	})
	g.Go(func() error {
		logger.Info("ingestion recovery worker starting")
		return workers.Recovery.Run(gctx)
	})
	g.Go(func() error {
		logger.Info("invitation outbox relay starting")
		workers.OutboxRelay.Run(gctx)
		return nil
	})
	errCh := make(chan error, 1)
	go func() { errCh <- g.Wait() }()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("worker exited with error", zap.Error(err))
			os.Exit(1)
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining in-flight jobs...")
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("worker exited with error", zap.Error(err))
				os.Exit(1)
			}
			logger.Info("worker exited cleanly")
		case <-time.After(consts.ShutdownTimeout):
			logger.Warn("forced shutdown after timeout; in-flight jobs may be redelivered")
		}
	}
}
