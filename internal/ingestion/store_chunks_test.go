package ingestion

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/models"
)

func newStoreChunksMock(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock database: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return NewRepository(db), mock
}

func expectChunkInsert(mock sqlmock.Sqlmock, args ...driver.Value) {
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO document_chunks")).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func expectChunkLookup(
	mock sqlmock.Sqlmock,
	versionID uuid.UUID,
	parentID any,
	contentHash string,
	id, tenantID, documentID uuid.UUID,
	content string,
	embeddingIsNull bool,
) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, tenant_id, document_id, content,")).
		WithArgs(versionID, parentID, contentHash).
		WillReturnRows(
			sqlmock.NewRows([]string{
				"id", "tenant_id", "document_id", "content", "embedding_is_null",
			}).AddRow(id, tenantID, documentID, content, embeddingIsNull),
		)
}

func TestStoreChunksRetryReusesPersistedIDsAndEmbedding(t *testing.T) {
	repo, mock := newStoreChunksMock(t)

	tenantID := uuid.New()
	documentID := uuid.New()
	versionID := uuid.New()

	// IDs generated on the original ingestion attempt.
	persistedParentID := uuid.New()
	persistedChildID := uuid.New()

	// The retry generates different IDs for the same content.
	retryParentID := uuid.New()
	retryChildID := uuid.New()

	retryParentRef := retryParentID
	chunks := []models.Chunk{
		{
			ID:                retryParentID,
			DocumentVersionID: versionID,
			Content:           "parent content",
			ContentHash:       "parent-hash",
			ChunkIndex:        0,
			TokenCount:        10,
			IsParent:          true,
		},
		{
			ID:                retryChildID,
			ParentChunkID:     &retryParentRef,
			DocumentVersionID: versionID,
			Content:           "child content",
			ContentHash:       "child-hash",
			ChunkIndex:        1,
			TokenCount:        5,
		},
	}

	embeddings := map[uuid.UUID][]float32{
		retryChildID: {0.1, 0.2},
	}

	mock.ExpectBegin()

	// Parent insertion conflicts with the existing natural key.
	expectChunkInsert(
		mock,
		retryParentID, tenantID, documentID, versionID,
		nil, 0, "parent content", "parent-hash",
		nil, nil, nil, 10,
	)
	expectChunkLookup(
		mock, versionID, nil, "parent-hash",
		persistedParentID, tenantID, documentID,
		"parent content", true,
	)

	// The retry's child must reference the persisted parent ID, not the
	// newly generated retryParentID.
	expectChunkInsert(
		mock,
		retryChildID, tenantID, documentID, versionID,
		persistedParentID, 1, "child content", "child-hash",
		"[0.1,0.2]", nil, nil, 5,
	)
	expectChunkLookup(
		mock, versionID, persistedParentID, "child-hash",
		persistedChildID, tenantID, documentID,
		"child content", false,
	)

	// Existing non-NULL embedding: no repair UPDATE should occur.
	mock.ExpectCommit()

	if err := repo.StoreChunks(
		context.Background(), tenantID, documentID, chunks, embeddings,
	); err != nil {
		t.Fatalf("StoreChunks returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestStoreChunksRepairsMissingChildEmbedding(t *testing.T) {
	repo, mock := newStoreChunksMock(t)

	tenantID := uuid.New()
	documentID := uuid.New()
	versionID := uuid.New()

	persistedParentID := uuid.New()
	persistedChildID := uuid.New()
	retryParentID := uuid.New()
	retryChildID := uuid.New()
	retryParentRef := retryParentID

	chunks := []models.Chunk{
		{
			ID:                retryParentID,
			DocumentVersionID: versionID,
			Content:           "parent content",
			ContentHash:       "parent-hash",
			TokenCount:        10,
			IsParent:          true,
		},
		{
			ID:                retryChildID,
			ParentChunkID:     &retryParentRef,
			DocumentVersionID: versionID,
			Content:           "child content",
			ContentHash:       "child-hash",
			TokenCount:        5,
		},
	}

	embeddings := map[uuid.UUID][]float32{
		retryChildID: {0.1, 0.2},
	}

	mock.ExpectBegin()

	expectChunkInsert(
		mock,
		retryParentID, tenantID, documentID, versionID,
		nil, 0, "parent content", "parent-hash",
		nil, nil, nil, 10,
	)
	expectChunkLookup(
		mock, versionID, nil, "parent-hash",
		persistedParentID, tenantID, documentID,
		"parent content", true,
	)

	expectChunkInsert(
		mock,
		retryChildID, tenantID, documentID, versionID,
		persistedParentID, 0, "child content", "child-hash",
		"[0.1,0.2]", nil, nil, 5,
	)
	expectChunkLookup(
		mock, versionID, persistedParentID, "child-hash",
		persistedChildID, tenantID, documentID,
		"child content", true,
	)

	mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE document_chunks SET embedding = $2 WHERE id = $1 AND embedding IS NULL",
	)).
		WithArgs(persistedChildID, "[0.1,0.2]").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT embedding IS NULL FROM document_chunks WHERE id = $1",
	)).
		WithArgs(persistedChildID).
		WillReturnRows(
			sqlmock.NewRows([]string{"embedding_is_null"}).AddRow(false),
		)

	mock.ExpectCommit()

	if err := repo.StoreChunks(
		context.Background(), tenantID, documentID, chunks, embeddings,
	); err != nil {
		t.Fatalf("StoreChunks returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestStoreChunksMissingChildEmbeddingRollsBack(t *testing.T) {
	repo, mock := newStoreChunksMock(t)

	tenantID := uuid.New()
	documentID := uuid.New()
	versionID := uuid.New()

	parentID := uuid.New()
	childID := uuid.New()
	parentRef := parentID

	chunks := []models.Chunk{
		{
			ID:                parentID,
			DocumentVersionID: versionID,
			Content:           "parent content",
			ContentHash:       "parent-hash",
			TokenCount:        10,
			IsParent:          true,
		},
		{
			ID:                childID,
			ParentChunkID:     &parentRef,
			DocumentVersionID: versionID,
			Content:           "child content",
			ContentHash:       "child-hash",
			TokenCount:        5,
		},
	}

	mock.ExpectBegin()

	expectChunkInsert(
		mock,
		parentID, tenantID, documentID, versionID,
		nil, 0, "parent content", "parent-hash",
		nil, nil, nil, 10,
	)
	expectChunkLookup(
		mock, versionID, nil, "parent-hash",
		parentID, tenantID, documentID,
		"parent content", true,
	)

	// The child has no embedding. StoreChunks must fail before attempting
	// its INSERT and must roll back the transaction.
	mock.ExpectRollback()

	err := repo.StoreChunks(
		context.Background(), tenantID, documentID, chunks,
		map[uuid.UUID][]float32{},
	)
	if err == nil {
		t.Fatal("expected an error when a child embedding is missing")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
