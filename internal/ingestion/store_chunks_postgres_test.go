package ingestion

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/mtariq99/retrievion/models"
)

func TestStoreChunksPostgresRetry(t *testing.T) {
	dsn := os.Getenv("RETRIEVION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set RETRIEVION_TEST_DATABASE_URL to run PostgreSQL integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL connection: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close integration-test database: %v", err)
		}
	})
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	repo := NewRepository(db)

	tenantID := uuid.New()
	documentID := uuid.New()
	versionID := uuid.New()

	// Fixture cleanup cascades through documents and document versions.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 10*time.Second,
		)
		defer cleanupCancel()

		_, cleanupErr := db.ExecContext(
			cleanupCtx,
			`DELETE FROM tenants WHERE id = $1`,
			tenantID,
		)
		if cleanupErr != nil {
			t.Errorf("clean up integration-test tenant: %v", cleanupErr)
		}
	})

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO tenants (id, name) VALUES ($1, $2)`,
		tenantID, "store-chunks-integration-"+tenantID.String(),
	); err != nil {
		t.Fatalf("create test tenant: %v", err)
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO documents (id, tenant_id, name, mime_type, status)
		 VALUES ($1, $2, $3, $4, 'pending')`,
		documentID, tenantID, "integration-test.txt", "text/plain",
	); err != nil {
		t.Fatalf("create test document: %v", err)
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO document_versions
		 (id, document_id, version, content_hash, storage_key, status)
		 VALUES ($1, $2, 1, $3, $4, 'pending')`,
		versionID, documentID, "fixture-"+versionID.String(),
		"integration-tests/"+versionID.String(),
	); err != nil {
		t.Fatalf("create test version: %v", err)
	}

	// Attempt one: persist a parent and an embedded child.
	parentID := uuid.New()
	childID := uuid.New()
	parentRef := parentID

	firstChunks := []models.Chunk{
		{
			ID:                parentID,
			DocumentVersionID: versionID,
			ChunkIndex:        0,
			Content:           "integration parent content",
			ContentHash:       "parent-" + versionID.String(),
			TokenCount:        4,
			IsParent:          true,
		},
		{
			ID:                childID,
			ParentChunkID:     &parentRef,
			DocumentVersionID: versionID,
			ChunkIndex:        0,
			Content:           "integration child content",
			ContentHash:       "child-" + versionID.String(),
			TokenCount:        3,
		},
	}

	originalVector := integrationTestEmbedding(0.1)
	if err := repo.StoreChunks(
		ctx, tenantID, documentID, firstChunks,
		map[uuid.UUID][]float32{childID: originalVector},
	); err != nil {
		t.Fatalf("first StoreChunks call: %v", err)
	}

	// Capture the persisted IDs and vector before retrying.
	var persistedParentID, persistedChildID uuid.UUID
	var persistedParentRef uuid.UUID
	var storedVector string

	if err := db.QueryRowContext(
		ctx,
		`SELECT id FROM document_chunks
		 WHERE document_version_id = $1 AND content_hash = $2`,
		versionID, firstChunks[0].ContentHash,
	).Scan(&persistedParentID); err != nil {
		t.Fatalf("read persisted parent: %v", err)
	}

	if err := db.QueryRowContext(
		ctx,
		`SELECT id, parent_chunk_id, embedding::text
		 FROM document_chunks
		 WHERE document_version_id = $1 AND content_hash = $2`,
		versionID, firstChunks[1].ContentHash,
	).Scan(&persistedChildID, &persistedParentRef, &storedVector); err != nil {
		t.Fatalf("read persisted child: %v", err)
	}

	if persistedParentRef != persistedParentID {
		t.Fatalf("initial child parent ID = %s; want %s",
			persistedParentRef, persistedParentID)
	}

	// Attempt two: simulate a retry with freshly generated chunk IDs.
	retryParentID := uuid.New()
	retryChildID := uuid.New()
	retryParentRef := retryParentID

	retryChunks := []models.Chunk{
		{
			ID:                retryParentID,
			DocumentVersionID: versionID,
			ChunkIndex:        0,
			Content:           firstChunks[0].Content,
			ContentHash:       firstChunks[0].ContentHash,
			TokenCount:        firstChunks[0].TokenCount,
			IsParent:          true,
		},
		{
			ID:                retryChildID,
			ParentChunkID:     &retryParentRef,
			DocumentVersionID: versionID,
			ChunkIndex:        0,
			Content:           firstChunks[1].Content,
			ContentHash:       firstChunks[1].ContentHash,
			TokenCount:        firstChunks[1].TokenCount,
		},
	}

	retryVector := integrationTestEmbedding(0.9)
	if err := repo.StoreChunks(
		ctx, tenantID, documentID, retryChunks,
		map[uuid.UUID][]float32{retryChildID: retryVector},
	); err != nil {
		t.Fatalf("retry StoreChunks call: %v", err)
	}

	var chunkCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT count(*) FROM document_chunks WHERE document_version_id = $1`,
		versionID,
	).Scan(&chunkCount); err != nil {
		t.Fatalf("count persisted chunks: %v", err)
	}
	if chunkCount != 2 {
		t.Fatalf("persisted chunk count = %d; want 2", chunkCount)
	}

	var retryPersistedParentID, retryPersistedChildID uuid.UUID
	var retryPersistedParentRef uuid.UUID
	var retryStoredVector string

	if err := db.QueryRowContext(
		ctx,
		`SELECT id FROM document_chunks
		 WHERE document_version_id = $1 AND content_hash = $2`,
		versionID, retryChunks[0].ContentHash,
	).Scan(&retryPersistedParentID); err != nil {
		t.Fatalf("read parent after retry: %v", err)
	}

	if err := db.QueryRowContext(
		ctx,
		`SELECT id, parent_chunk_id, embedding::text
		 FROM document_chunks
		 WHERE document_version_id = $1 AND content_hash = $2`,
		versionID, retryChunks[1].ContentHash,
	).Scan(
		&retryPersistedChildID, &retryPersistedParentRef, &retryStoredVector,
	); err != nil {
		t.Fatalf("read child after retry: %v", err)
	}

	if retryPersistedParentID != persistedParentID {
		t.Fatalf("retry changed parent ID: got %s; want %s",
			retryPersistedParentID, persistedParentID)
	}
	if retryPersistedChildID != persistedChildID {
		t.Fatalf("retry changed child ID: got %s; want %s",
			retryPersistedChildID, persistedChildID)
	}
	if retryPersistedParentRef != persistedParentID {
		t.Fatalf("retry child parent ID = %s; want canonical parent %s",
			retryPersistedParentRef, persistedParentID)
	}
	if retryStoredVector != storedVector {
		t.Fatalf("retry overwrote existing embedding: got %s; want %s",
			retryStoredVector, storedVector)
	}

	// Exercise repair of an existing child whose embedding is NULL.
	if _, err := db.ExecContext(
		ctx,
		`UPDATE document_chunks SET embedding = NULL WHERE id = $1`,
		persistedChildID,
	); err != nil {
		t.Fatalf("clear child embedding for repair test: %v", err)
	}

	repairParentID := uuid.New()
	repairChildID := uuid.New()
	repairParentRef := repairParentID
	repairChunks := []models.Chunk{
		{
			ID:                repairParentID,
			DocumentVersionID: versionID,
			Content:           firstChunks[0].Content,
			ContentHash:       firstChunks[0].ContentHash,
			TokenCount:        firstChunks[0].TokenCount,
			IsParent:          true,
		},
		{
			ID:                repairChildID,
			ParentChunkID:     &repairParentRef,
			DocumentVersionID: versionID,
			Content:           firstChunks[1].Content,
			ContentHash:       firstChunks[1].ContentHash,
			TokenCount:        firstChunks[1].TokenCount,
		},
	}

	if err := repo.StoreChunks(
		ctx, tenantID, documentID, repairChunks,
		map[uuid.UUID][]float32{repairChildID: originalVector},
	); err != nil {
		t.Fatalf("StoreChunks embedding repair: %v", err)
	}

	var embeddingIsNull bool
	if err := db.QueryRowContext(
		ctx,
		`SELECT embedding IS NULL FROM document_chunks WHERE id = $1`,
		persistedChildID,
	).Scan(&embeddingIsNull); err != nil {
		t.Fatalf("verify repaired embedding: %v", err)
	}
	if embeddingIsNull {
		t.Fatal("child embedding remained NULL after repair")
	}

	_ = fmt.Sprintf("%s", storedVector)
}

func integrationTestEmbedding(first float32) []float32 {
	embedding := make([]float32, 1536)
	embedding[0] = first
	return embedding
}
