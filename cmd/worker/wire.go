//go:build wireinject
// +build wireinject

package main

import (
	"database/sql"

	"github.com/google/wire"
	"go.uber.org/zap"

	"github.com/mtariq99/retrievion/internal/config"
	"github.com/mtariq99/retrievion/internal/embedding"
	"github.com/mtariq99/retrievion/internal/ingestion"
	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/internal/tokenizer"
	"github.com/mtariq99/retrievion/pkg/objectstorage"
)

func InitializeWorker(
	db *sql.DB,
	logger *zap.Logger,
) (*ingestion.Worker, func(), error) {
	wire.Build(
		config.Load,

		objectstorage.NewB2Storage,
		storage.NewRabbitMQQueue,
		storage.ProvideQueue,
		tokenizer.NewCounter,

		// embedding
		embedding.NewGeminiProvider,
		wire.Bind(new(embedding.Provider), new(*embedding.GeminiProvider)),
		embedding.NewService,
		wire.Bind(new(ingestion.EmbeddingProvider), new(*embedding.Service)),

		// ingestion
		ingestion.NewRepository,
		ingestion.NewPDFParser,
		wire.Bind(new(ingestion.Parser), new(*ingestion.PDFParser)),
		ingestion.NewParentChildChunker,
		wire.Bind(new(ingestion.Chunker), new(*ingestion.ParentChildChunker)),
		ingestion.NewPipeline,
		ingestion.NewWorker,
	)

	return nil, nil, nil
}
