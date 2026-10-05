package conversation

import (
	"context"

	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
)

type conversationMutator interface {
	CreateConversation(ctx context.Context, conversation *models.Conversation) error
	DeleteConversation(ctx context.Context, userID, tenantID, conversationID uuid.UUID) error
}

type conversationReader interface {
	GetConversation(ctx context.Context, userID, tenantID, conversationID uuid.UUID) (*models.Conversation, error)
	ListConversations(ctx context.Context, userID, tenantID uuid.UUID) ([]*models.Conversation, error)
}

type Service struct {
	conversationMutator conversationMutator
	conversationReader  conversationReader
	config              *models.Config
}

func NewService(repo *Repository, cfg *models.Config) *Service {
	return &Service{
		conversationMutator: repo,
		conversationReader:  repo,
		config:              cfg,
	}
}

func (s *Service) CreateConversation(ctx context.Context, userID, tenantID uuid.UUID, title, summary *string) (*models.Conversation, error) {
	if userID == (uuid.UUID{}) || tenantID == (uuid.UUID{}) || title == nil {
		return nil, errs.ErrMissingRequiredFields
	}

	if *title == "" {
		return nil, errs.ErrMissingRequiredFields
	}

	conversation := &models.Conversation{
		ID:       uuid.New(),
		TenantID: tenantID,
		UserID:   userID,
		Title:    title,
		Summary:  summary,
	}

	if err := s.conversationMutator.CreateConversation(ctx, conversation); err != nil {
		return nil, err
	}

	return conversation, nil
}

func (s *Service) GetConversation(ctx context.Context, userID, tenantID, conversationID uuid.UUID) (*models.Conversation, error) {
	if userID == (uuid.UUID{}) || tenantID == (uuid.UUID{}) || conversationID == (uuid.UUID{}) {
		return nil, errs.ErrMissingRequiredFields
	}

	return s.conversationReader.GetConversation(ctx, userID, tenantID, conversationID)
}

func (s *Service) ListConversations(ctx context.Context, userID, tenantID uuid.UUID) ([]*models.Conversation, error) {
	if userID == (uuid.UUID{}) || tenantID == (uuid.UUID{}) {
		return nil, errs.ErrMissingRequiredFields
	}

	return s.conversationReader.ListConversations(ctx, userID, tenantID)
}

func (s *Service) DeleteConversation(ctx context.Context, userID, tenantID, conversationID uuid.UUID) error {
	if userID == (uuid.UUID{}) || tenantID == (uuid.UUID{}) || conversationID == (uuid.UUID{}) {
		return errs.ErrMissingRequiredFields
	}

	return s.conversationMutator.DeleteConversation(ctx, userID, tenantID, conversationID)
}
