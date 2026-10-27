package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/enums"
	"github.com/mtariq99/retrievion/models"
)

func GenerateToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func GetUserClaims(c *gin.Context) (*models.UserClaims, bool) {
	userIdVal, exists := c.Get(consts.UserID)
	if !exists {
		return nil, false
	}
	userId, ok := userIdVal.(uuid.UUID)
	if !ok {
		return nil, false
	}

	userEmailVal, exists := c.Get(consts.UserEmail)
	if !exists {
		return nil, false
	}
	userEmail, ok := userEmailVal.(string)
	if !ok {
		return nil, false
	}

	opaqueTokenIdVal, exists := c.Get(consts.OpaqueTokenID)
	if !exists {
		return nil, false
	}
	opaqueTokenId, ok := opaqueTokenIdVal.(string)
	if !ok {
		return nil, false
	}

	tenantIdVal, exists := c.Get(consts.TenantID)
	if !exists {
		return nil, false
	}
	tenantId, ok := tenantIdVal.(uuid.UUID)
	if !ok {
		return nil, false
	}
	IsServiceBool, exists := c.Get(consts.IsService)
	if !exists {
		return nil, false
	}
	IsService, ok := IsServiceBool.(bool)
	if !ok {
		return nil, false
	}
	clientIDVal, exists := c.Get(consts.ClientID)
	if !exists {
		return nil, false
	}
	clientID, ok := clientIDVal.(string)
	if !ok {
		return nil, false
	}

	userRoleVal, exists := c.Get(consts.UserRole)
	if !exists {
		return nil, false
	}
	userRole, ok := userRoleVal.(enums.UserRole)
	if !ok {
		return nil, false
	}

	claims := &models.UserClaims{
		UserId:            userId,
		UserEmail:         userEmail,
		UserOpaqueTokenId: opaqueTokenId,
		TenantID:          tenantId,
		IsService:         IsService,
		ClientID:          clientID,
		UserRole:          userRole,
	}

	return claims, true
}

func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func HashTokenBytes(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func ParseUUID(strVal, label string) (uuid.UUID, error) {
	if strVal == "" {
		return uuid.Nil, fmt.Errorf("%s is required", label)
	}
	id, err := uuid.Parse(strVal)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid %s: must be a valid UUID", label)
	}
	return id, nil
}

func UnixToTime(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}

func B2ObjectKey(tenantID, documentID, versionID uuid.UUID, filename string) string {
	return fmt.Sprintf("tenants/%s/documents/%s/versions/%s/%s", tenantID, documentID, versionID, filename)
}

// Only the SHA-256 of the token is stored, so a Redis leak can't be replayed.
func ChallengeKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return consts.LoginChallengePrefix + hex.EncodeToString(sum[:])
}
