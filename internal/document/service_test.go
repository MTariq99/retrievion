package document

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
)

type membershipFake struct {
	membership *models.TenantMember
	err        error
	calls      int
}

func (f *membershipFake) FindUserByID(context.Context, uuid.UUID) (*models.User, error) {
	return nil, nil
}

func (f *membershipFake) FindUserByEmail(context.Context, string) (*models.User, error) {
	return nil, nil
}

func (f *membershipFake) GetMembership(context.Context, uuid.UUID, uuid.UUID) (*models.TenantMember, error) {
	f.calls++
	return f.membership, f.err
}

type documentReaderFake struct {
	findBySHA256Calls int
	findByUserIDCalls int
	findDocumentCalls int
	document          *models.Document
	err               error
}

func (f *documentReaderFake) FindBySHA256(context.Context, uuid.UUID, string) (*models.Document, error) {
	f.findBySHA256Calls++
	return f.document, f.err
}

func (f *documentReaderFake) FindByUserID(context.Context, uuid.UUID, uuid.UUID) ([]*models.Document, error) {
	f.findByUserIDCalls++
	return nil, f.err
}

func (f *documentReaderFake) FindDocument(context.Context, uuid.UUID, uuid.UUID) (*models.Document, error) {
	f.findDocumentCalls++
	return f.document, f.err
}

type documentMutatorFake struct {
	createDocumentCalls     int
	createVersionCalls      int
	createIngestionJobCalls int
	deleteDocumentCalls     int
	deleteTenantID          uuid.UUID
	deleteDocumentID        uuid.UUID
	err                     error
}

func (f *documentMutatorFake) CreateDocument(context.Context, *models.Document) error {
	f.createDocumentCalls++
	return f.err
}

func (f *documentMutatorFake) CreateDocumentVersion(context.Context, *models.DocumentVersion) error {
	f.createVersionCalls++
	return f.err
}

func (f *documentMutatorFake) CreateIngestionJob(context.Context, *models.IngestionJob) error {
	f.createIngestionJobCalls++
	return f.err
}

func (f *documentMutatorFake) DeleteDocument(_ context.Context, tenantID, documentID uuid.UUID) error {
	f.deleteDocumentCalls++
	f.deleteTenantID = tenantID
	f.deleteDocumentID = documentID
	return f.err
}

type txRunnerFake struct {
	calls int
	err   error
}

func (f *txRunnerFake) WithTx(_ context.Context, fn func(tx documentMutator) error) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	return fn(&documentMutatorFake{})
}

type storageFake struct {
	uploadCalls        int
	deleteCalls        int
	getURLCalls        int
	presignGetURLCalls int
}

func (f *storageFake) Upload(context.Context, string, io.Reader, int64, string) (string, error) {
	f.uploadCalls++
	return "", nil
}

func (f *storageFake) Delete(context.Context, string) error {
	f.deleteCalls++
	return nil
}

func (f *storageFake) Exists(context.Context, string) (bool, error) {
	return false, nil
}

func (f *storageFake) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}

func (f *storageFake) GetURL(string) string {
	f.getURLCalls++
	return "https://example.invalid/document"
}

func (f *storageFake) PresignGetURL(context.Context, string, time.Duration) (string, error) {
	f.presignGetURLCalls++
	return "", nil
}

type queueFake struct {
	publishCalls int
}

func (f *queueFake) Publish(context.Context, string, []byte) error {
	f.publishCalls++
	return nil
}

func (f *queueFake) Consume(context.Context, string, func(models.QueueMessage) error) error {
	return nil
}

func (f *queueFake) Ack(context.Context, models.QueueMessage) error {
	return nil
}

func (f *queueFake) Nack(context.Context, models.QueueMessage) error {
	return nil
}

type testDocumentService struct {
	service *Service
	members *membershipFake
	reader  *documentReaderFake
	mutator *documentMutatorFake
	tx      *txRunnerFake
	storage *storageFake
	queue   *queueFake
}

func newTestDocumentService(membership *models.TenantMember, membershipErr error) *testDocumentService {
	members := &membershipFake{
		membership: membership,
		err:        membershipErr,
	}
	reader := &documentReaderFake{}
	mutator := &documentMutatorFake{}
	tx := &txRunnerFake{}
	objectStore := &storageFake{}
	queue := &queueFake{}

	return &testDocumentService{
		service: &Service{
			users:           members,
			documentReader:  reader,
			documentMutator: mutator,
			tx:              tx,
			B2:              objectStore,
			queue:           queue,
		},
		members: members,
		reader:  reader,
		mutator: mutator,
		tx:      tx,
		storage: objectStore,
		queue:   queue,
	}
}

func TestAuthorizeTenant(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	dbErr := errors.New("database unavailable")

	tests := []struct {
		name       string
		userID     uuid.UUID
		tenantID   uuid.UUID
		membership *models.TenantMember
		lookupErr  error
		wantErr    error
		wantLookup int
	}{
		{
			name:       "valid membership",
			userID:     userID,
			tenantID:   tenantID,
			membership: &models.TenantMember{UserID: userID, TenantID: tenantID},
			wantLookup: 1,
		},
		{
			name:       "membership missing",
			userID:     userID,
			tenantID:   tenantID,
			lookupErr:  errs.ErrNotFound,
			wantErr:    errs.ErrUnauthorized,
			wantLookup: 1,
		},
		{
			name:       "membership lookup fails",
			userID:     userID,
			tenantID:   tenantID,
			lookupErr:  dbErr,
			wantErr:    dbErr,
			wantLookup: 1,
		},
		{
			name:       "nil membership",
			userID:     userID,
			tenantID:   tenantID,
			wantErr:    errs.ErrUnauthorized,
			wantLookup: 1,
		},
		{
			name:       "membership belongs to another tenant",
			userID:     userID,
			tenantID:   tenantID,
			membership: &models.TenantMember{UserID: userID, TenantID: uuid.New()},
			wantErr:    errs.ErrUnauthorized,
			wantLookup: 1,
		},
		{
			name:       "membership belongs to another user",
			userID:     userID,
			tenantID:   tenantID,
			membership: &models.TenantMember{UserID: uuid.New(), TenantID: tenantID},
			wantErr:    errs.ErrUnauthorized,
			wantLookup: 1,
		},
		{
			name:       "nil user ID",
			userID:     uuid.Nil,
			tenantID:   tenantID,
			wantErr:    errs.ErrUnauthorized,
			wantLookup: 0,
		},
		{
			name:       "nil tenant ID",
			userID:     userID,
			tenantID:   uuid.Nil,
			wantErr:    errs.ErrUnauthorized,
			wantLookup: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDocumentService(tt.membership, tt.lookupErr)

			err := deps.service.authorizeTenant(context.Background(), tt.userID, tt.tenantID)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("authorizeTenant() error = %v, want error matching %v", err, tt.wantErr)
			}
			if deps.members.calls != tt.wantLookup {
				t.Fatalf("membership lookup calls = %d, want %d", deps.members.calls, tt.wantLookup)
			}
		})
	}
}

func TestDocumentOperationsDenyRequestsWithoutMembership(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	documentID := uuid.New()

	tests := []struct {
		name string
		run  func(*Service) error
	}{
		{
			name: "upload",
			run: func(s *Service) error {
				_, err := s.UploadDocument(context.Background(), userID, tenantID, nil)
				return err
			},
		},
		{
			name: "list",
			run: func(s *Service) error {
				_, err := s.ListDocuments(context.Background(), userID, tenantID)
				return err
			},
		},
		{
			name: "get",
			run: func(s *Service) error {
				_, err := s.GetDocument(context.Background(), userID, tenantID, documentID)
				return err
			},
		},
		{
			name: "delete",
			run: func(s *Service) error {
				return s.DeleteDocument(context.Background(), userID, tenantID, documentID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDocumentService(nil, errs.ErrNotFound)

			err := tt.run(deps.service)
			if !errors.Is(err, errs.ErrUnauthorized) {
				t.Fatalf("operation error = %v, want %v", err, errs.ErrUnauthorized)
			}

			if deps.members.calls != 1 {
				t.Fatalf("membership lookup calls = %d, want 1", deps.members.calls)
			}
			if deps.reader.findBySHA256Calls != 0 ||
				deps.reader.findByUserIDCalls != 0 ||
				deps.reader.findDocumentCalls != 0 {
				t.Fatal("document reader was called despite denied membership")
			}
			if deps.storage.uploadCalls != 0 ||
				deps.storage.deleteCalls != 0 ||
				deps.storage.getURLCalls != 0 ||
				deps.storage.presignGetURLCalls != 0 {
				t.Fatal("object storage was called despite denied membership")
			}
			if deps.mutator.createDocumentCalls != 0 ||
				deps.mutator.createVersionCalls != 0 ||
				deps.mutator.createIngestionJobCalls != 0 ||
				deps.mutator.deleteDocumentCalls != 0 {
				t.Fatal("document mutator was called despite denied membership")
			}
			if deps.tx.calls != 0 {
				t.Fatal("transaction was started despite denied membership")
			}
			if deps.queue.publishCalls != 0 {
				t.Fatal("ingestion job was published despite denied membership")
			}
		})
	}
}

func TestDocumentOperationsFailClosedWhenMembershipLookupFails(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	documentID := uuid.New()
	dbErr := errors.New("membership database unavailable")

	operations := []struct {
		name string
		run  func(*Service) error
	}{
		{
			name: "upload",
			run: func(s *Service) error {
				_, err := s.UploadDocument(context.Background(), userID, tenantID, nil)
				return err
			},
		},
		{
			name: "list",
			run: func(s *Service) error {
				_, err := s.ListDocuments(context.Background(), userID, tenantID)
				return err
			},
		},
		{
			name: "get",
			run: func(s *Service) error {
				_, err := s.GetDocument(context.Background(), userID, tenantID, documentID)
				return err
			},
		},
		{
			name: "delete",
			run: func(s *Service) error {
				return s.DeleteDocument(context.Background(), userID, tenantID, documentID)
			},
		},
	}

	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			deps := newTestDocumentService(nil, dbErr)

			err := op.run(deps.service)
			if !errors.Is(err, dbErr) {
				t.Fatalf("operation error = %v, want wrapped database error %v", err, dbErr)
			}
			if deps.reader.findBySHA256Calls != 0 ||
				deps.reader.findByUserIDCalls != 0 ||
				deps.reader.findDocumentCalls != 0 ||
				deps.mutator.deleteDocumentCalls != 0 ||
				deps.storage.uploadCalls != 0 ||
				deps.storage.getURLCalls != 0 ||
				deps.tx.calls != 0 ||
				deps.queue.publishCalls != 0 {
				t.Fatal("protected dependency was called after membership lookup failed")
			}
		})
	}
}

func TestDeleteDocumentPassesAuthorizedTenantAndDocumentIDs(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	documentID := uuid.New()

	deps := newTestDocumentService(
		&models.TenantMember{UserID: userID, TenantID: tenantID},
		nil,
	)

	if err := deps.service.DeleteDocument(context.Background(), userID, tenantID, documentID); err != nil {
		t.Fatalf("DeleteDocument() error = %v", err)
	}

	if deps.mutator.deleteDocumentCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", deps.mutator.deleteDocumentCalls)
	}
	if deps.mutator.deleteTenantID != tenantID {
		t.Fatalf("delete tenant ID = %s, want %s", deps.mutator.deleteTenantID, tenantID)
	}
	if deps.mutator.deleteDocumentID != documentID {
		t.Fatalf("delete document ID = %s, want %s", deps.mutator.deleteDocumentID, documentID)
	}
}

func TestListDocumentsAllowsAuthorizedTenantMember(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()

	deps := newTestDocumentService(
		&models.TenantMember{UserID: userID, TenantID: tenantID},
		nil,
	)

	if _, err := deps.service.ListDocuments(context.Background(), userID, tenantID); err != nil {
		t.Fatalf("ListDocuments() error = %v", err)
	}

	if deps.members.calls != 1 {
		t.Fatalf("membership calls = %d, want 1", deps.members.calls)
	}
	if deps.reader.findByUserIDCalls != 1 {
		t.Fatalf("list query calls = %d, want 1", deps.reader.findByUserIDCalls)
	}
}

func TestGetDocumentAllowsAuthorizedTenantMember(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	documentID := uuid.New()

	expected := &models.Document{
		ID:       documentID,
		TenantID: tenantID,
	}

	deps := newTestDocumentService(
		&models.TenantMember{UserID: userID, TenantID: tenantID},
		nil,
	)
	deps.reader.document = expected

	got, err := deps.service.GetDocument(
		context.Background(),
		userID,
		tenantID,
		documentID,
	)
	if err != nil {
		t.Fatalf("GetDocument() error = %v", err)
	}
	if got != expected {
		t.Fatal("GetDocument() returned an unexpected document")
	}
	if deps.members.calls != 1 {
		t.Fatalf("membership calls = %d, want 1", deps.members.calls)
	}
	if deps.reader.findDocumentCalls != 1 {
		t.Fatalf("document query calls = %d, want 1", deps.reader.findDocumentCalls)
	}
}
