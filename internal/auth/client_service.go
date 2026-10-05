package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/models"
	"golang.org/x/crypto/bcrypt"
)

type ClientService struct {
	repo       *ServiceClientRepository
	tokenStore *OpaqueTokenService
}

func NewClientService(repo *ServiceClientRepository, tokenStore *OpaqueTokenService) *ClientService {
	return &ClientService{
		repo:       repo,
		tokenStore: tokenStore,
	}
}

func (s *ClientService) IssueToken(ctx context.Context, clientID uuid.UUID, clientSecret string) (string, time.Time, error) {
	sc, err := s.repo.GetByClientID(ctx, clientID)
	if err != nil {
		return "", time.Time{}, errors.New("invalid client credentials")
	}

	if sc.RevokedAt != nil {
		return "", time.Time{}, errors.New("client credential revoked")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(sc.SecretHash), []byte(clientSecret)); err != nil {
		return "", time.Time{}, errors.New("invalid client credentials")
	}

	token, expiresAt, err := s.tokenStore.GenerateServiceToken(ctx, sc.ClientID, sc.TenantID)
	if err != nil {
		return "", time.Time{}, err
	}

	return token, expiresAt, nil
}

func (s *ClientService) GetClientByID(ctx context.Context, clientID uuid.UUID) (*models.ServiceClient, error) {
	if clientID == uuid.Nil {
		return nil, fmt.Errorf("missing client ID")
	}

	client, err := s.repo.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, err
	}

	return client, nil
}
