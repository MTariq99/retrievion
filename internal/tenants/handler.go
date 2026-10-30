package tenants

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/internal/response"
	"github.com/mtariq99/retrievion/models"
	"github.com/mtariq99/retrievion/utils"
)

type TenantHandler struct {
	tenantService *TenantService
	cfg           *models.Config
}

func NewTenantHandler(ts *TenantService, cfg *models.Config) *TenantHandler {
	return &TenantHandler{
		tenantService: ts,
		cfg:           cfg,
	}
}

func (th *TenantHandler) InviteUsers(c *gin.Context) {
	claims, ok := utils.GetUserClaims(c)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	var req models.InviteUsers
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(c, errs.ErrInvalidRequest)
		return
	}
	if len(req.Users) < 1 {
		errs.HandlerError(c, errs.ErrInvalidInvitation)
		return
	}

	if err := th.tenantService.InviteUsers(ctx, claims.TenantID, claims.UserId, &req); err != nil {
		errs.HandlerError(c, err)
		return
	}

	response.WriteEmptySuccessResponse(c, "invitation sent successfully")
}

func (th *TenantHandler) AcceptInvitation(c *gin.Context) {
	claims, ok := utils.GetUserClaimsWithoutTenant(c)
	if !ok {
		errs.HandlerError(c, errs.ErrUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var req models.AcceptInvitationRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(c, errs.ErrInvalidRequest)
		return
	}

	if err := th.tenantService.AcceptInvitation(ctx, claims.UserId, &req); err != nil {
		errs.HandlerError(c, err)
		return
	}

	response.WriteEmptySuccessResponse(c, "invitation accepted successfully")
}
