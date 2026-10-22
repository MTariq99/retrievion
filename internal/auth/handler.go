package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/internal/response"
	"github.com/mtariq99/retrievion/models"
	"github.com/mtariq99/retrievion/utils"
)

type Handler struct {
	tokenService  *OpaqueTokenService
	AuthService   *Service
	Config        *models.Config
	clientService *ClientService
}

func NewHandler(tokenService *OpaqueTokenService, service *Service, cfg *models.Config, clientSerive *ClientService) *Handler {
	return &Handler{
		tokenService:  tokenService,
		AuthService:   service,
		Config:        cfg,
		clientService: clientSerive,
	}
}

func (h *Handler) CreateUser(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(c, err)
		return
	}

	user, tenant, err := h.AuthService.Register(ctx, req)
	if err != nil {
		errs.HandlerError(c, err)
		return
	}

	payload := gin.H{"user": user}
	if tenant != nil {
		payload["tenant"] = tenant
	}

	response.WriteSuccessResponse(c, "user created successfully", payload)
}

func (h *Handler) Login(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(c, err)
		return
	}

	clientIP := c.ClientIP()
	userAgent := c.Request.UserAgent()

	loginResp, err := h.AuthService.Login(ctx, req, clientIP, userAgent)
	if err != nil {
		errs.HandlerError(c, err)
		return
	}

	response.WriteSuccessResponse(c, "login successful", loginResp)
}

func (h *Handler) Me(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	userIDVal, exists := c.Get("user_id")
	if !exists {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}

	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}

	user, err := h.AuthService.GetByID(ctx, userID)
	if err != nil {
		errs.HandlerError(c, err)
		return
	}

	tenants, err := h.AuthService.GetUserTenants(ctx, userID)
	if err != nil {
		errs.HandlerError(c, err)
		return
	}

	response.WriteSuccessResponse(c, "ok", gin.H{
		"user":    user,
		"tenants": tenants,
	})
}

func (h *Handler) Refresh(c *gin.Context) {
	var req models.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	pair, err := h.tokenService.RefreshAccessToken(c.Request.Context(), req.RefreshToken, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "invalid or expired refresh token"})
		return
	}

	c.JSON(http.StatusOK, pair)
}

func (h *Handler) Logout(c *gin.Context) {
	tokenID, exists := c.Get("opaque_token_id")
	if !exists {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "no active session"})
		return
	}

	if err := h.tokenService.RevokeToken(c.Request.Context(), tokenID.(string)); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to revoke token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (h *Handler) IssueServiceToken(c *gin.Context) {
	ctx := c.Request.Context()

	var req models.ServiceTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(c, errs.ErrInvalidRequest)
		return
	}
	parsedClientID, err := utils.ParseUUID(req.ClientID, "client ID")
	if err != nil {
		errs.HandlerError(c, errs.ErrMissingID)
		return
	}

	token, expiresAt, err := h.clientService.IssueToken(ctx, parsedClientID, req.ClientSecret)
	if err != nil {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}

	response.WriteSuccessResponse(
		c,
		"service token issued",
		models.ServiceTokenResponse{
			AccessToken: token,
			ExpiresAt:   expiresAt,
		},
	)
}
