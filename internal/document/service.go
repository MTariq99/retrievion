package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/auth"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/internal/filetype"
	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/models"
	"github.com/mtariq99/retrievion/pkg/objectstorage"
)

type userReader interface {
	FindUserByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	FindUserByEmail(ctx context.Context, email string) (*models.User, error)
}
type documentMutator interface {
	CreateDocument(ctx context.Context, document *models.Document) error
	CreateDocumentVersion(ctx context.Context, version *models.DocumentVersion) error
	CreateIngestionJob(ctx context.Context, job *models.IngestionJob) error
	DeleteDocument(ctx context.Context, userID, tenantID, documentID uuid.UUID) error
}

type documentReader interface {
	FindBySHA256(ctx context.Context, tenantID uuid.UUID, sha256 string) (*models.Document, error)
	FindByUserID(ctx context.Context, userID, tenantID uuid.UUID) ([]*models.Document, error)
	FindDocument(ctx context.Context, documentID uuid.UUID) (*models.Document, error)
}

type txRunner interface {
	WithTx(ctx context.Context, fn func(tx documentMutator) error) error
}

type Service struct {
	documentMutator documentMutator
	documentReader  documentReader
	users           userReader
	tx              txRunner
	B2              objectstorage.Storage
	queue           storage.Queue
}

func NewService(b2 objectstorage.Storage, repo *Repository, cfg *models.Config, usersRepo *auth.Repository, q storage.Queue) *Service {
	return &Service{
		documentMutator: repo,
		documentReader:  repo,
		users:           usersRepo,
		B2:              b2,
		queue:           q,
		tx:              repo,
	}
}

func (s *Service) UploadDocument(ctx context.Context, userID, tenantID uuid.UUID, file *multipart.FileHeader) (*models.UploadResult, error) {
	if file == nil {
		return nil, errs.ErrMissingDocument
	}

	if file.Size <= 0 {
		return nil, errs.ErrEmptyFile
	}

	if file.Size > consts.MaxDocumentSize {
		return nil, errs.ErrFileTooLarge
	}

	filename := filepath.Base(file.Filename)
	if filename == "" || filename == "." {
		return nil, errs.ErrInvalidFilename
	}

	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer src.Close()
	mimeType, err := filetype.Detect(src, file.Size)
	if err != nil {
		return nil, fmt.Errorf("detect file type: %w", err)
	}
	if mimeType == "" {
		return nil, errs.ErrUnsupportedFileType
	}

	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("reset file: %w", err)
	}

	hash := sha256.New()

	if _, err := io.Copy(hash, src); err != nil {
		return nil, fmt.Errorf("calculate sha256: %w", err)
	}

	sha256Hash := hex.EncodeToString(hash.Sum(nil))

	existing, err := s.documentReader.FindBySHA256(ctx, tenantID, sha256Hash)
	if err != nil && !errors.Is(err, errs.ErrNotFound) {
		return nil, fmt.Errorf("check duplicate: %w", err)
	}

	if existing != nil {
		return nil, errs.ErrDuplicateDocument
	}

	documentID := uuid.New()
	versionID := uuid.New()
	jobID := uuid.New()

	objectKey := fmt.Sprintf("tenants/%s/documents/%s/versions/%s/%s", tenantID, documentID, versionID, filename)

	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("reset file before upload: %w", err)
	}

	if _, err := s.B2.Upload(ctx, objectKey, src, file.Size, mimeType); err != nil {
		return nil, fmt.Errorf("upload object: %w", err)
	}

	document := &models.Document{
		ID:       documentID,
		TenantID: tenantID,
		Name:     filename,
		MimeType: mimeType,
		Status:   consts.DocumentStatusPending,
	}

	version := &models.DocumentVersion{
		ID:          versionID,
		DocumentID:  documentID,
		Version:     1,
		ContentHash: sha256Hash,
		StorageKey:  objectKey,
		Status:      consts.DocumentStatusPending,
	}

	job := &models.IngestionJob{
		ID:                jobID,
		TenantID:          tenantID,
		DocumentID:        documentID,
		Status:            "pending",
		DocumentVersionID: versionID,
	}

	err = s.tx.WithTx(ctx, func(tx documentMutator) error {
		if err := tx.CreateDocument(ctx, document); err != nil {
			return err
		}
		if err := tx.CreateDocumentVersion(ctx, version); err != nil {
			return err
		}
		if err := tx.CreateIngestionJob(ctx, job); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		_ = s.B2.Delete(ctx, objectKey)
		return nil, fmt.Errorf("create document records: %w", err)
	}

	body, err := storage.MarshalIngestionJob(jobID.String(), documentID.String(), versionID.String())
	if err != nil {
		return nil, fmt.Errorf("marshal ingestion event: %w", err)
	}

	if err := s.queue.Publish(ctx, consts.IngestionRoutingKey, body); err != nil {
		return nil, fmt.Errorf("publish ingestion event: %w", err)
	}

	return &models.UploadResult{
		DocumentID: documentID,
		VersionID:  versionID,
		JobID:      jobID,
		Status:     consts.DocumentStatusProcessing,
	}, nil
}

func (s *Service) ListDocuments(ctx context.Context, userID, tenantID uuid.UUID) ([]*models.Document, error) {
	if userID == (uuid.UUID{}) {
		return nil, fmt.Errorf("user ID is required")
	}

	if tenantID == (uuid.UUID{}) {
		return nil, fmt.Errorf("tenant ID is required")
	}

	documents, err := s.documentReader.FindByUserID(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}

	for _, document := range documents {
		if document == nil {
			continue
		}

		for _, version := range document.Versions {
			if version == nil || version.StorageKey == "" {
				continue
			}

			version.StorageURL = s.B2.GetURL(version.StorageKey)
		}
	}

	return documents, nil
}

func (s *Service) GetDocument(ctx context.Context, documentID uuid.UUID) (*models.Document, error) {
	if documentID == (uuid.UUID{}) {
		return nil, fmt.Errorf("user ID is required")
	}

	document, err := s.documentReader.FindDocument(ctx, documentID)
	if err != nil {
		return nil, err
	}

	for _, version := range document.Versions {
		if version == nil || version.StorageKey == "" {
			continue
		}

		version.StorageURL = s.B2.GetURL(version.StorageKey)
	}

	return document, nil
}

func (s *Service) DeleteDocument(ctx context.Context, userID, tetantID, documentID uuid.UUID) error {
	if userID == (uuid.UUID{}) || documentID == (uuid.UUID{}) {
		return errs.ErrMissingRequiredFields
	}
	if err := s.documentMutator.DeleteDocument(ctx, userID, tetantID, documentID); err != nil {
		return err
	}
	return nil
}
