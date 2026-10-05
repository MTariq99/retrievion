//go:build wireinject
// +build wireinject

package main

import (
	"database/sql"

	"github.com/google/wire"
	"go.uber.org/zap"

	"github.com/mtariq99/retrievion/internal/auth"
	"github.com/mtariq99/retrievion/internal/chat"
	"github.com/mtariq99/retrievion/internal/citation"
	"github.com/mtariq99/retrievion/internal/conversation"
	"github.com/mtariq99/retrievion/internal/embedding"
	"github.com/mtariq99/retrievion/internal/llm"
	"github.com/mtariq99/retrievion/internal/prompt"
	"github.com/mtariq99/retrievion/internal/retrieval"

	"github.com/mtariq99/retrievion/internal/document"

	"github.com/mtariq99/retrievion/internal/config"
	"github.com/mtariq99/retrievion/internal/tokenizer"

	httpHandler "github.com/mtariq99/retrievion/internal/http"
	"github.com/mtariq99/retrievion/internal/middleware"
	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/pkg/objectstorage"
)

func InitializeAPI(
	db *sql.DB,
	logger *zap.Logger,
) (*httpHandler.Handler, func(), error) {
	wire.Build(
		config.Load,

		storage.NewRedis,
		objectstorage.NewB2Storage,
		storage.NewRabbitMQQueue,
		storage.ProvideQueue,
		tokenizer.NewCounter,
		auth.NewTokenConfig,
		auth.NewOpaqueTokenService,
		auth.NewRepository,
		auth.NewClientServiceRepository,
		auth.NewClientService,
		auth.NewService,
		auth.NewHandler,

		// documents
		document.NewRepository,
		document.NewService,
		document.NewHandler,

		// conversation
		conversation.NewRepository,
		conversation.NewService,

		// retrieval
		retrieval.NewVectorRepository,
		wire.Bind(new(retrieval.VectorSearcher), new(*retrieval.VectorRepository)),
		retrieval.NewKeywordRepository,
		wire.Bind(new(retrieval.KeywordSearcher), new(*retrieval.KeywordRepository)),
		retrieval.NewFusion,
		wire.Bind(new(retrieval.ResultFusion), new(*retrieval.Fusion)),
		retrieval.NewNoopReranker,
		wire.Bind(new(retrieval.Reranker), new(*retrieval.NoopReranker)),
		retrieval.NewService,

		// prompt
		prompt.NewBuilder,

		// citation
		citation.NewService,

		// embedding
		embedding.NewGeminiProvider,
		wire.Bind(new(embedding.Provider), new(*embedding.GeminiProvider)),
		embedding.NewService,

		// llm
		llm.NewClient,
		wire.Bind(new(llm.Provider), new(*llm.Client)),
		llm.NewPricing,
		llm.NewService,

		// chat
		chat.NewRepository,
		chat.NewService,
		chat.NewHandler,

		middleware.NewPhantomTokenMiddleware,

		httpHandler.NewHandler,
	)

	return nil, nil, nil
}
