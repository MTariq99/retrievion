package middleware

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/mtariq99/retrievion/internal/auth"
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

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		introspection, err := m.opaqueService.IntrospectToken(ctx, opaqueToken)
		if err != nil || !introspection.Active {
			m.logger.Warn("token introspection failed", zap.Error(err), zap.String("path", c.Request.URL.Path))
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				models.ErrorResponse{Error: "invalid or expired token"})
			return
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

		c.Request.Header.Set("Authorization", "Bearer "+jwtToken)
		c.Set("opaque_token_id", opaqueToken)
		c.Set("user_id", introspection.UserID)
		c.Set("user_email", introspection.UserEmail)
		c.Set("tenant_id", introspection.TenantID)
		c.Set("is_service", introspection.IsService)
		c.Set("client_id", introspection.ClientID)

		c.Next()
	}
}

func (m *PhantomTokenMiddleware) generateInternalJWT(introspection *models.IntrospectionResponse) (string, error) {
	claims := models.UserClaims{
		UserId:    introspection.UserID,
		UserEmail: introspection.UserEmail,
		UserName:  introspection.UserName,
		Scopes:    introspection.Scopes,
		TenantID:  introspection.TenantID,
		IsService: introspection.IsService, // new
		ClientID:  introspection.ClientID,  // new
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
