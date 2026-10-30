//go:build wireinject
// +build wireinject

package main

import (
	"database/sql"

	"github.com/google/wire"
	"go.uber.org/zap"

	"github.com/mtariq99/retrievion/internal/auth"
	"github.com/mtariq99/retrievion/internal/chat"
	"github.com/mtariq99/retrievion/internal/tenants"

	"github.com/mtariq99/retrievion/internal/citation"
	"github.com/mtariq99/retrievion/internal/conversation"
	"github.com/mtariq99/retrievion/internal/document"
	"github.com/mtariq99/retrievion/internal/embedding"
	"github.com/mtariq99/retrievion/internal/llm"
	"github.com/mtariq99/retrievion/internal/prompt"
	"github.com/mtariq99/retrievion/internal/retrieval"

	"github.com/mtariq99/retrievion/internal/config"
	httpHandler "github.com/mtariq99/retrievion/internal/http"
	"github.com/mtariq99/retrievion/internal/middleware"
	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/internal/tokenizer"
	"github.com/mtariq99/retrievion/pkg/objectstorage"
)

func InitializeAPI(
	db *sql.DB,
	logger *zap.Logger,
) (*httpHandler.Handler, func(), error) {
	wire.Build(
		config.Load,
		// Infrastructure
		storage.NewRedis,
		objectstorage.NewB2Storage,
		storage.NewRabbitMQQueue,
		storage.ProvideQueue,
		wire.Bind(
			new(tenants.InvitationPublisher),
			new(*storage.RabbitMQQueue),
		),
		wire.Bind(
			new(auth.OpaqueTokenStore),
			new(*storage.RedisClient),
		),
		wire.Bind(
			new(auth.ChallengeStore),
			new(*storage.RedisClient),
		),
		wire.Bind(
			new(auth.TenantMembershipReader),
			new(*auth.Repository),
		),
		tokenizer.NewCounter,

		// Authentication
		auth.NewTokenConfig,
		auth.NewOpaqueTokenService,
		auth.NewRepository,
		wire.Bind(
			new(document.MembershipReader),
			new(*auth.Repository),
		),
		auth.NewClientServiceRepository,
		auth.NewClientService,
		auth.NewService,
		auth.NewHandler,

		// Documents
		document.NewRepository,
		document.NewService,
		document.NewHandler,

		// Conversations
		conversation.NewRepository,
		conversation.NewService,

		// Retrieval
		retrieval.NewVectorRepository,
		wire.Bind(
			new(retrieval.VectorSearcher),
			new(*retrieval.VectorRepository),
		),
		retrieval.NewKeywordRepository,
		wire.Bind(
			new(retrieval.KeywordSearcher),
			new(*retrieval.KeywordRepository),
		),
		retrieval.NewFusion,
		wire.Bind(
			new(retrieval.ResultFusion),
			new(*retrieval.Fusion),
		),
		retrieval.ProvideReranker,
		retrieval.NewService,

		// Prompt
		prompt.NewBuilder,

		// Citation
		citation.NewService,

		// Embedding
		embedding.NewGeminiProvider,
		wire.Bind(
			new(embedding.Provider),
			new(*embedding.GeminiProvider),
		),
		embedding.NewService,

		// LLM
		llm.NewClient,
		wire.Bind(
			new(llm.Provider),
			new(*llm.Client),
		),
		llm.NewPricing,
		llm.NewService,

		// Chat
		chat.NewRepository,
		chat.NewService,
		chat.NewHandler,

		// Tenant invitations
		tenants.NewTanantRepository,
		tenants.NewTenantService,
		tenants.NewTenantHandler,
		wire.Bind(
			new(tenants.UserReader),
			new(*auth.Repository),
		),

		// Middleware and HTTP handler
		middleware.NewPhantomTokenMiddleware,
		httpHandler.NewHandler,
	)

	return nil, nil, nil
}
