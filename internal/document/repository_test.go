package document

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/errs"
)

func newMockRepository(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create SQL mock: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	return &Repository{db: db, q: db}, mock
}

func TestFindDocumentScopesQueryToTenant(t *testing.T) {
	repo, mock := newMockRepository(t)

	tenantID := uuid.New()
	documentID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta("WHERE d.id = $1")).
		WithArgs(documentID, tenantID).
		WillReturnError(sql.ErrNoRows)

	document, err := repo.FindDocument(context.Background(), tenantID, documentID)
	if err != nil {
		t.Fatalf("FindDocument() error = %v", err)
	}
	if document != nil {
		t.Fatalf("FindDocument() = %#v, want nil for document outside requested tenant", document)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestFindDocumentQueryIncludesTenantAndSoftDeleteFilters(t *testing.T) {
	repo, mock := newMockRepository(t)

	tenantID := uuid.New()
	documentID := uuid.New()

	mock.ExpectQuery(`(?s)WHERE d\.id = \$1\s+AND d\.tenant_id = \$2\s+AND d\.deleted_at IS NULL`).
		WithArgs(documentID, tenantID).
		WillReturnError(sql.ErrNoRows)

	_, err := repo.FindDocument(context.Background(), tenantID, documentID)
	if err != nil {
		t.Fatalf("FindDocument() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestDeleteDocumentScopesUpdateToTenant(t *testing.T) {
	repo, mock := newMockRepository(t)

	tenantID := uuid.New()
	documentID := uuid.New()

	mock.ExpectExec(`(?s)UPDATE documents\s+SET deleted_at = NOW\(\), updated_at = NOW\(\)\s+WHERE id = \$1\s+AND tenant_id = \$2\s+AND deleted_at IS NULL`).
		WithArgs(documentID, tenantID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.DeleteDocument(context.Background(), tenantID, documentID)
	if !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("DeleteDocument() error = %v, want %v", err, errs.ErrNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestDeleteDocumentRejectsMissingIDsWithoutSQL(t *testing.T) {
	tests := []struct {
		name       string
		tenantID   uuid.UUID
		documentID uuid.UUID
	}{
		{name: "missing tenant", tenantID: uuid.Nil, documentID: uuid.New()},
		{name: "missing document", tenantID: uuid.New(), documentID: uuid.Nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepository(t)

			err := repo.DeleteDocument(context.Background(), tt.tenantID, tt.documentID)
			if !errors.Is(err, errs.ErrMissingRequiredFields) {
				t.Fatalf("DeleteDocument() error = %v, want %v", err, errs.ErrMissingRequiredFields)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unexpected SQL operation: %v", err)
			}
		})
	}
}

func TestFindDocumentQueryIsTenantScoped(t *testing.T) {
	repo, mock := newMockRepository(t)

	tenantID := uuid.New()
	documentID := uuid.New()

	mock.ExpectQuery(`(?s)FROM documents d.*WHERE d\.id = \$1.*AND d\.tenant_id = \$2.*AND d\.deleted_at IS NULL`).
		WithArgs(documentID, tenantID).
		WillReturnError(sql.ErrNoRows)

	_, err := repo.FindDocument(context.Background(), tenantID, documentID)
	if err != nil {
		t.Fatalf("FindDocument() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}

	_ = strings.TrimSpace("") // remove this line; it is not needed
}
