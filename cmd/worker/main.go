package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"go.uber.org/zap"

	"github.com/mtariq99/retrievion/internal/config"
	"github.com/mtariq99/retrievion/internal/storage"
)

const shutdownTimeout = 30 * time.Second

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

	worker, cleanup, err := InitializeWorker(db, logger)
	if err != nil {
		logger.Fatal("wire init failed", zap.Error(err))
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(1)
	runErrCh := make(chan error, 1)

	go func() {
		defer wg.Done()
		logger.Info("ingestion worker starting", zap.String("env", cfg.App.Env))
		if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			runErrCh <- err
			return
		}
		runErrCh <- nil
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received, draining in-flight jobs...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if err := <-runErrCh; err != nil {
			logger.Error("worker exited with error", zap.Error(err))
			os.Exit(1)
		}
		logger.Info("worker exited cleanly")
	case <-shutdownCtx.Done():
		logger.Warn("forced shutdown after timeout; in-flight job may be requeued")
	}
}
