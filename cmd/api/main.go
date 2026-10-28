package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/mtariq99/retrievion/internal/config"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/middleware"
	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/models"
	"go.uber.org/zap"
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
	migrationDSN := cfg.Database.DSN
	if migrationDSN == "" {
		migrationDSN = cfg.Database.URL
	}

	if err = storage.RunMigrations(migrationDSN, cfg.Database.MigrationPath, logger); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}

	handler, cleanup, err := InitializeAPI(db, logger)
	if err != nil {
		logger.Fatal("wire init failed", zap.Error(err))
	}
	defer cleanup()

	r := gin.New()
	r.Use(middleware.SentryMiddleware(false))
	r.Use(middleware.Logging(logger))
	r.Use(gin.Recovery())
	r.Use(buildCORS(cfg))

	handler.RegisterRoutes(r)
	addr := fmt.Sprintf(":%s", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("API server starting", zap.String("addr", addr), zap.String("env", cfg.App.Env))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("server listen error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutdown signal received, draining connections...")

	ctx, cancel := context.WithTimeout(context.Background(), consts.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("forced shutdown after timeout", zap.Error(err))
	} else {
		logger.Info("server exited cleanly")
	}
}

func buildCORS(cfg *models.Config) gin.HandlerFunc {
	origins := strings.Split(cfg.Server.AllowedOrigins, ",")
	for i, o := range origins {
		origins[i] = strings.TrimSpace(o)
	}

	return cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}
