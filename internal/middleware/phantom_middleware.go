package middleware

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/mtariq99/retrievion/internal/auth"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/enums"
	"github.com/mtariq99/retrievion/internal/jwtclaims"
	"github.com/mtariq99/retrievion/models"
	"go.uber.org/zap"
)

type PhantomTokenMiddleware struct {
	opaqueService *auth.OpaqueTokenService
	jwtPrivateKey ed25519.PrivateKey
	issuer        string
	logger        *zap.Logger
}

func NewPhantomTokenMiddleware(opaqueService *auth.OpaqueTokenService, cfg *models.Config, logger *zap.Logger) (*PhantomTokenMiddleware, error) {
	privateKeyBytes, err := base64.StdEncoding.DecodeString(cfg.Auth.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key: %w", err)
	}

	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key size")
	}

	return &PhantomTokenMiddleware{
		opaqueService: opaqueService,
		jwtPrivateKey: ed25519.PrivateKey(privateKeyBytes),
		issuer:        cfg.Auth.Issuer,
		logger:        logger,
	}, nil
}

func (m *PhantomTokenMiddleware) ExchangeOpaqueForJWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.Request.Header.Get("Authorization")
		if len(authHeader) == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				models.ErrorResponse{Error: "missing authorization token"})
			return
		}

		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || strings.ToUpper(tokenParts[0]) != "BEARER" {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				models.ErrorResponse{Error: "invalid authorization token format"})
			return
		}

		opaqueToken := tokenParts[1]

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		introspection, err := m.opaqueService.IntrospectToken(ctx, opaqueToken)
		if err != nil || !introspection.Active {
			m.logger.Warn("token introspection failed", zap.Error(err), zap.String("path", c.Request.URL.Path))
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				models.ErrorResponse{Error: "invalid or expired token"})
			return
		}

		if introspection.IsService {
			if introspection.TokenType != "service" || introspection.ClientID == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, models.ErrorResponse{Error: "invalid service token"})
				return
			}
		} else {
			if introspection.TokenType != "access" || introspection.UserID == uuid.Nil || introspection.TenantID == uuid.Nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized,
					models.ErrorResponse{Error: "invalid access token"})
				return
			}
			if err := m.opaqueService.ValidateUserTenantMembership(ctx, introspection.TenantID, introspection.UserID); err != nil {
				m.logger.Warn("tenant membership validation failed", zap.Error(err), zap.String("path", c.Request.URL.Path))
				c.AbortWithStatusJSON(http.StatusUnauthorized,
					models.ErrorResponse{Error: "invalid or unauthorized token"})
				return
			}
		}

		jwtToken, err := m.generateInternalJWT(introspection)
		if err != nil {
			m.logger.Warn(
				"token introspection failed",
				zap.Error(err),
				zap.String("path", c.Request.URL.Path),
			)
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				models.ErrorResponse{Error: "failed to generate internal token"})
			return
		}

		c.Request.Header.Set(consts.Authorization, "Bearer "+jwtToken)
		c.Set(consts.OpaqueTokenID, opaqueToken)
		c.Set(consts.UserID, introspection.UserID)
		c.Set(consts.UserEmail, introspection.UserEmail)
		c.Set(consts.TenantID, introspection.TenantID)
		c.Set(consts.IsService, introspection.IsService)
		c.Set(consts.ClientID, introspection.ClientID)
		c.Set(consts.UserRole, enums.UserRole(introspection.UserRole))
		c.Next()
	}
}

func (m *PhantomTokenMiddleware) generateInternalJWT(introspection *models.IntrospectionResponse) (string, error) {
	claims := jwtclaims.UserClaims{
		UserId:    introspection.UserID,
		UserEmail: introspection.UserEmail,
		UserName:  introspection.UserName,
		Scopes:    introspection.Scopes,
		TenantID:  introspection.TenantID,
		IsService: introspection.IsService,
		ClientID:  introspection.ClientID,
		UserRole:  enums.UserRole(introspection.UserRole),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    m.issuer,
			Subject:   fmt.Sprintf("%d", introspection.UserID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(m.jwtPrivateKey)
}
