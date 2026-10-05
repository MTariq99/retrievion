package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/models"
	"golang.org/x/crypto/bcrypt"
)

const (
	salt = 10
)

func HashPassword(password string) string {
	hashedPass, err := bcrypt.GenerateFromPassword([]byte(password), salt)
	if err != nil {
		panic(err)
	}
	return string(hashedPass)
}

func ComparePassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

func GenerateRandomState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func GenerateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func GetUserClaims(c *gin.Context) (*models.UserClaims, bool) {
	userIdVal, exists := c.Get("user_id")
	if !exists {
		return nil, false
	}
	userId, ok := userIdVal.(uuid.UUID)
	if !ok {
		return nil, false
	}

	userEmailVal, exists := c.Get("user_email")
	if !exists {
		return nil, false
	}
	userEmail, ok := userEmailVal.(string)
	if !ok {
		return nil, false
	}

	opaqueTokenIdVal, exists := c.Get("opaque_token_id")
	if !exists {
		return nil, false
	}
	opaqueTokenId, ok := opaqueTokenIdVal.(string)
	if !ok {
		return nil, false
	}

	tenantIdVal, exists := c.Get("tenant_id")
	if !exists {
		return nil, false
	}
	tenantId, ok := tenantIdVal.(uuid.UUID)
	if !ok {
		return nil, false
	}

	claims := &models.UserClaims{
		UserId:            userId,
		UserEmail:         userEmail,
		UserOpaqueTokenId: opaqueTokenId,
		TenantID:          tenantId,
	}

	return claims, true
}

func ParseStrToInt64(s string) (int64, error) {
	Id64, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid ID format: %w", err)
	}
	return Id64, nil
}

func ExtractRefreshToken(c *gin.Context) (*string, error) {
	cookie, err := c.Request.Cookie("refresh-token")
	if err != nil {
		return nil, fmt.Errorf("refresh token not found")
	}

	token := cookie.Value
	if token == "" {
		return nil, fmt.Errorf("refresh token is empty")
	}

	return &token, nil
}

func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func GenerateOTP() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(900000))
	return fmt.Sprintf("%06d", n.Int64()+100000) // always 6 digits: 100000–999999
}

func ParseInt64(strVal, label string) (int64, error) {
	if strVal == "" {
		return 0, fmt.Errorf("%s is required", label)
	}
	parsed, err := strconv.ParseInt(strVal, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: must be a number", label)
	}
	return parsed, nil
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

var ErrQuotaExceeded = errors.New("free tier limit reached")

func GetContextUserID(c *gin.Context) (int64, bool) {
	v, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	id, ok := v.(int64)
	return id, ok
}

func ParseIDParam(c *gin.Context) (int64, error) {
	raw := c.Param("id")
	if raw == "" {
		return 0, fmt.Errorf("user ID is required")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid user ID")
	}
	return id, nil
}

func BuildOffsetPagination(page int, limit int, totalItems int64, currentCount int) models.PaginationResponse {
	totalPages := int(
		math.Ceil(
			float64(totalItems) / float64(limit),
		),
	)

	hasNext := page < totalPages
	hasPrevious := page > 1

	var nextPage *int
	var previousPage *int

	if hasNext {
		n := page + 1
		nextPage = &n
	}

	if hasPrevious {
		p := page - 1
		previousPage = &p
	}

	from := ((page - 1) * limit) + 1
	to := from + currentCount - 1

	remaining := totalItems - int64(to)

	if remaining < 0 {
		remaining = 0
	}

	return models.PaginationResponse{
		Page:         page,
		Limit:        limit,
		TotalItems:   totalItems,
		TotalPages:   totalPages,
		CurrentCount: currentCount,

		HasNext:     hasNext,
		HasPrevious: hasPrevious,

		NextPage:     nextPage,
		PreviousPage: previousPage,

		RemainingItems: remaining,

		From: from,
		To:   to,
	}
}

func BuildCursorPagination(limit int, currentCount int, nextCursor string) models.PaginationResponse {
	hasNext := false

	if currentCount == limit {
		hasNext = true
	}

	return models.PaginationResponse{
		Limit:        limit,
		CurrentCount: currentCount,
		HasNext:      hasNext,
		NextCursor:   nextCursor,
	}
}

func GetPagination(c *gin.Context) (*models.PaginationQuery, error) {
	paginationData, exists := c.Get("pagination")
	if !exists {
		return nil, fmt.Errorf("pagination does not exists")
	}
	pagination := paginationData.(models.PaginationQuery)
	return &pagination, nil
}

func UnixToTime(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}

func B2ObjectKey(tenantID, documentID, versionID uuid.UUID, filename string) string {
	return fmt.Sprintf("tenants/%s/documents/%s/versions/%s/%s", tenantID, documentID, versionID, filename)
}
