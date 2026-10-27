package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/mtariq99/retrievion/internal/auth"
	"github.com/mtariq99/retrievion/internal/chat"
	"github.com/mtariq99/retrievion/internal/document"
	"github.com/mtariq99/retrievion/internal/middleware"
	"github.com/mtariq99/retrievion/internal/tenants"
)

type Handler struct {
	logger        *zap.Logger
	phantom       *middleware.PhantomTokenMiddleware
	Auth          *auth.Handler
	document      *document.Handler
	chat          *chat.Handler
	TenantHandler *tenants.TenantHandler
}

func NewHandler(
	logger *zap.Logger,
	phantom *middleware.PhantomTokenMiddleware,
	AuthHandler *auth.Handler,
	document *document.Handler,
	chat *chat.Handler,
	th *tenants.TenantHandler,
) *Handler {
	return &Handler{
		logger:        logger,
		phantom:       phantom,
		Auth:          AuthHandler,
		document:      document,
		chat:          chat,
		TenantHandler: th,
	}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	// Health
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	authRoutes := r.Group("/auth")
	{
		authRoutes.POST("/register", h.Auth.CreateUser)
		authRoutes.POST("/login", h.Auth.Login)
		authRoutes.POST("/refresh", h.Auth.Refresh)
		authRoutes.POST("/logout", h.Auth.Logout)
		authRoutes.POST("/service/token", h.Auth.IssueServiceToken)
		authRoutes.POST("/select-tenant", h.Auth.SelectTenant)
	}
	api := r.Group("/api/v1")
	api.Use(h.phantom.ExchangeOpaqueForJWT())

	documents := api.Group("/documents")
	{
		documents.POST("", h.document.UploadDocument)
		documents.GET("", h.document.ListDocuments)
		documents.GET("/:id", h.document.GetDocument)
		documents.DELETE("/:id", h.document.DeleteDocument)
	}

	chats := api.Group("/chat")
	{
		chats.POST("/conversations", h.chat.CreateConversation)
		chats.GET("/conversations/:id", h.chat.GetConversation)
		chats.POST("/conversations/messages", h.chat.Chat)
		chats.POST("/conversations/:id/stream", h.chat.StreamChat)
	}

	tenantsInvitations := api.Group("/tenants/invitations")
	{
		tenantsInvitations.POST("", h.TenantHandler.InviteUsers)
	}
}
