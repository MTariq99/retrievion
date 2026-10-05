package document

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/internal/response"
	"github.com/mtariq99/retrievion/models"
	"github.com/mtariq99/retrievion/utils"
)

type Handler struct {
	service *Service
	Config  *models.Config
}

func NewHandler(config *models.Config, service *Service) *Handler {
	return &Handler{
		service: service,
		Config:  config,
	}
}

func (h *Handler) UploadDocument(c *gin.Context) {
	user, ok := utils.GetUserClaims(c)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}

	file, err := c.FormFile("document")
	if err != nil {
		errs.HandlerError(c, errs.ErrMissingDocument)
		return
	}

	res, err := h.service.UploadDocument(c.Request.Context(), user.UserId, user.TenantID, file)
	if err != nil {
		errs.HandlerError(c, err)
		return
	}

	response.WriteSuccessResponse(
		c,
		"document uploaded successfully",
		res,
	)
}

func (h *Handler) ListDocuments(c *gin.Context) {
	user, ok := utils.GetUserClaims(c)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	documents, err := h.service.ListDocuments(ctx, user.UserId, user.TenantID)
	if err != nil {
		errs.HandlerError(c, err)
		return
	}

	response.WriteSuccessResponse(c, "documents fetched successfully", documents)
}

func (h *Handler) GetDocument(c *gin.Context) {
	user, ok := utils.GetUserClaims(c)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	Id := c.Param("id")
	if Id == "" {
		errs.HandlerError(c, errs.ErrMissingDocumentID)
		return
	}

	parsedDocumentID, err := utils.ParseUUID(Id, "document id")
	if err != nil {
		errs.HandlerError(c, err)
		return
	}
	document, err := h.service.GetDocument(ctx, parsedDocumentID)
	if err != nil {
		errs.HandlerError(c, err)
		return
	}
	if document.TenantID != user.TenantID {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}

	response.WriteSuccessResponse(c, "document fetched successfully", document)
}

func (h *Handler) DeleteDocument(c *gin.Context) {
	user, ok := utils.GetUserClaims(c)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	id := c.Param("id")
	if id == "" {
		errs.HandlerError(c, errs.ErrMissingDocumentID)
		return
	}
	parsedDocumentID, err := utils.ParseUUID(id, "document id")
	if err != nil {
		errs.HandlerError(c, err)
		return
	}

	if err := h.service.DeleteDocument(ctx, user.UserId, user.TenantID, parsedDocumentID); err != nil {
		errs.HandlerError(c, err)
		return
	}

	response.WriteEmptySuccessResponse(c, "document deleted successfully")
}
