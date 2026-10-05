package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/conversation"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/internal/response"
	"github.com/mtariq99/retrievion/models"
	"github.com/mtariq99/retrievion/utils"
)

type Handler struct {
	chatService         *Service
	conversationService *conversation.Service
	Config              *models.Config
}

func NewHandler(config *models.Config, service *Service, conServ *conversation.Service) *Handler {
	return &Handler{
		chatService:         service,
		Config:              config,
		conversationService: conServ,
	}
}

func (h *Handler) CreateConversation(c *gin.Context) {
	user, ok := utils.GetUserClaims(c)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	req := &models.CreateConversationReq{}
	if err := c.ShouldBindJSON(req); err != nil {
		errs.HandlerError(c, errs.ErrInvalidRequest)
		return
	}
	if req.Title == nil {
		errs.HandlerError(c, errs.ErrInvalidTitle)
		return
	}
	converstion, err := h.conversationService.CreateConversation(ctx, user.UserId, user.TenantID, req.Title, req.Summary)
	if err != nil {
		errs.HandlerError(c, err)
		return
	}
	response.WriteSuccessResponse(c, "conversation created successfully", converstion)
}

func (h *Handler) GetConversation(c *gin.Context) {
	user, ok := utils.GetUserClaims(c)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	id := c.Param("id")
	if id == "" {
		errs.HandlerError(c, errs.ErrMissingID)
		return
	}
	parsedConvID, err := utils.ParseUUID(id, "conversation id")
	if err != nil {
		errs.HandlerError(c, errs.ErrInvalidID)
		return
	}
	conv, err := h.conversationService.GetConversation(ctx, user.UserId, user.TenantID, parsedConvID)
	if err != nil {
		errs.HandlerError(c, err)
		return
	}

	response.WriteSuccessResponse(c, "conversation fetched successfully", conv)
}

func (h *Handler) ListConversations(c *gin.Context) {
	user, ok := utils.GetUserClaims(c)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	conversations, err := h.conversationService.ListConversations(ctx, user.UserId, user.TenantID)
	if err != nil {
		errs.HandlerError(c, err)
		return
	}

	response.WriteSuccessResponse(c, "conversations fetched successfully", conversations)
}

func (h *Handler) DeleteConversation(c *gin.Context) {
	user, ok := utils.GetUserClaims(c)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(
		c.Request.Context(),
		5*time.Second,
	)
	defer cancel()

	id := c.Param("id")

	if id == "" {
		errs.HandlerError(c, errs.ErrMissingID)
		return
	}

	parsedConvID, err := utils.ParseUUID(id, "conversation id")
	if err != nil {
		errs.HandlerError(c, errs.ErrInvalidID)
		return
	}

	if err := h.conversationService.DeleteConversation(ctx, user.UserId, user.TenantID, parsedConvID); err != nil {
		errs.HandlerError(c, err)
		return
	}

	response.WriteSuccessResponse(c, "conversation deleted successfully", nil)
}

func (h *Handler) Chat(c *gin.Context) {
	user, ok := utils.GetUserClaims(c)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}

	var req models.ChatRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(c, errs.ErrInvalidRequest)
		return
	}

	if req.ConversationID == uuid.Nil {
		errs.HandlerError(c, errs.ErrInvalidID)
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		errs.HandlerError(c, errs.ErrInvalidRequest)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()

	result, err := h.chatService.Chat(ctx, user.UserId, user.TenantID, &req)
	if err != nil {
		errs.HandlerError(c, err)
		return
	}

	response.WriteSuccessResponse(c, "chat response generated successfully", result)
}

func (h *Handler) StreamChat(c *gin.Context) {
	user, ok := utils.GetUserClaims(c)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}

	var req models.ChatRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(c, errs.ErrInvalidRequest)
		return
	}

	if req.ConversationID == uuid.Nil {
		errs.HandlerError(c, errs.ErrInvalidID)
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		errs.HandlerError(c, errs.ErrInvalidRequest)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return
	}

	writeEvent := func(event string, data any) error {
		payload, err := json.Marshal(data)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(
			c.Writer,
			"event: %s\n",
			event,
		)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(
			c.Writer,
			"data: %s\n\n",
			payload,
		)
		if err != nil {
			return err
		}

		flusher.Flush()

		return nil
	}

	ctx := c.Request.Context()

	if err := writeEvent("message_start", gin.H{
		"conversation_id": req.ConversationID,
	}); err != nil {
		return
	}

	result, err := h.chatService.StreamChat(
		ctx,
		user.UserId,
		user.TenantID,
		&req,
		func(token string) error {
			return writeEvent("token", gin.H{
				"text": token,
			})
		},
	)
	if err != nil {
		_ = writeEvent("error", gin.H{
			"message": err.Error(),
		})
		return
	}

	for _, source := range result.Sources {
		if err := writeEvent("citation", source); err != nil {
			return
		}
	}

	_ = writeEvent("message_complete", result)
}
