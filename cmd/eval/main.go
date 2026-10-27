package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/google/uuid"
	contextlogger "github.com/mtariq99/retrievion/pkg/logger"
	"go.uber.org/zap"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mtariq99/retrievion/internal/config"
	"github.com/mtariq99/retrievion/internal/embedding"
	"github.com/mtariq99/retrievion/internal/evaluation"
	"github.com/mtariq99/retrievion/internal/llm"
	"github.com/mtariq99/retrievion/internal/prompt"
	"github.com/mtariq99/retrievion/internal/retrieval"
	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/internal/tokenizer"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "evaluation failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	dataset := flag.String("dataset", "", "evaluation dataset name (required)")
	tenantIDValue := flag.String("tenant-id", "", "Tenant UUID (required)")
	evalUserIDValue := flag.String("eval-user-id", "", "Evaluation user UUID (required)")
	k := flag.Int("k", 10, "Number of retrieved documents to evaluate")
	promptVersion := flag.String("prompt-version", "", "Prompt version label")
	model := flag.String("model", "", "LLM model label; defaults to configured model")
	baseline := flag.Bool("baseline", false, "Mark this evaluation run as the baseline")

	flag.Parse()
	if strings.TrimSpace(*dataset) == "" {
		return fmt.Errorf("--dataset is required")
	}
	if strings.TrimSpace(*tenantIDValue) == "" {
		return fmt.Errorf("--tenant-id is required")
	}
	if strings.TrimSpace(*evalUserIDValue) == "" {
		return fmt.Errorf("--eval-user-id is required")
	}
	if *k <= 0 {
		return fmt.Errorf("--k must be greater than zero")
	}

	tenantID, err := uuid.Parse(*tenantIDValue)
	if err != nil {
		return fmt.Errorf("invalid --tenant-id: %w", err)
	}

	evalUserID, err := uuid.Parse(*evalUserIDValue)
	if err != nil {
		return fmt.Errorf("invalid --eval-user-id: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger, err := storage.BuildLogger(cfg.App.Env)
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	contextlogger.Init()
	// Retrieval repositories use database/sql.
	db, err := storage.InitDB(cfg, logger)
	if err != nil {
		return fmt.Errorf("initialize retrieval database: %w", err)
	}
	defer db.Close()

	// Evaluation persistence uses pgxpool. Use the same configured DSN.
	dsn := cfg.Database.DSN
	if dsn == "" {
		dsn = cfg.Database.URL
	}
	if dsn == "" {
		return fmt.Errorf("DATABASE_DSN or DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("initialize evaluation database pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping evaluation database: %w", err)
	}

	redisClient, err := storage.NewRedis(cfg)
	if err != nil {
		return fmt.Errorf("initialize Redis: %w", err)
	}
	defer redisClient.Close()

	counter, err := tokenizer.NewCounter(cfg)
	if err != nil {
		return fmt.Errorf("initialize tokenizer: %w", err)
	}

	embeddingProvider, err := embedding.NewGeminiProvider(cfg)
	if err != nil {
		return fmt.Errorf("initialize embedding provider: %w", err)
	}
	embeddingService := embedding.NewService(embeddingProvider, logger)

	llmClient, err := llm.NewClient(cfg, redisClient)
	if err != nil {
		return fmt.Errorf("initialize LLM client: %w", err)
	}

	llmService := llm.NewService(llmClient, cfg, llm.NewPricing(cfg), logger)

	retrievalService := retrieval.NewService(retrieval.NewVectorRepository(db), retrieval.NewKeywordRepository(db), retrieval.NewFusion(), retrieval.NewNoopReranker())

	promptBuilder := prompt.NewBuilder(counter)

	modelName := strings.TrimSpace(*model)
	if modelName == "" {
		modelName = cfg.LLM.Model
	}
	if modelName == "" {
		return fmt.Errorf("--model is required when LLM_MODEL is not configured")
	}

	opts := evaluation.RunOptions{
		DatasetName:    strings.TrimSpace(*dataset),
		PromptVersion:  strings.TrimSpace(*promptVersion),
		Model:          modelName,
		K:              *k,
		MarkAsBaseline: *baseline,
		TenantID:       tenantID,
		EvalUserID:     evalUserID,
	}

	logger.Info(
		"starting retrieval evaluation",
		zap.String("dataset", opts.DatasetName),
		zap.String("tenant_id", tenantID.String()),
		zap.String("eval_user_id", evalUserID.String()),
		zap.Int("k", opts.K),
		zap.String("model", opts.Model),
		zap.Bool("baseline", opts.MarkAsBaseline),
	)
	runID, passRate, err := evaluation.RunFullEvaluation(ctx, pool, retrievalService, embeddingService, llmService, llmClient, promptBuilder, counter, cfg, opts)

	if runID != uuid.Nil {
		fmt.Printf("Evaluation run ID: %s\n", runID)
		fmt.Printf("Pass rate: %.2f%%\n", passRate*100)
	}

	if err != nil {
		return fmt.Errorf("run full evaluation: %w", err)
	}

	logger.Info(
		"evaluation completed",
		zap.String("run_id", runID.String()),
		zap.Float64("pass_rate", passRate),
	)
	return nil
}
