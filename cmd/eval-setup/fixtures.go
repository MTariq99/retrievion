package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/embedding"
	"github.com/mtariq99/retrievion/models"
	"go.uber.org/zap"
)

const (
	fixtureDataset      = "resume-smoke-fixture-v1"
	fixtureDocumentName = "__retrievion_eval_fixture_v1__"
)

type evalFixture struct {
	question string
	answer   string
	content  string
}

var evalFixtures = []evalFixture{
	{
		question: "Which database does Retrievion use for vector similarity search?",
		answer:   "Retrievion uses PostgreSQL with pgvector for vector similarity search.",
		content:  "Retrievion uses PostgreSQL with the pgvector extension for vector similarity search. Document chunks store 1536-dimensional embeddings, and vector retrieval filters results by tenant and active document version.",
	},
	{
		question: "How does Retrievion combine vector and keyword search results?",
		answer:   "Retrievion combines vector and keyword results using reciprocal rank fusion with K equal to 60.",
		content:  "Retrievion combines semantic vector search and PostgreSQL full-text keyword search using reciprocal rank fusion. The reciprocal rank fusion constant is K=60. Both retrieval paths enforce tenant isolation and filter out deleted documents and inactive document versions.",
	},
	{
		question: "How are evaluation cases isolated between tenants?",
		answer:   "Evaluation cases are selected using both the dataset name and the evaluation tenant ID. Global cases are excluded.",
		content:  "Retrievion evaluation selects cases using both dataset_name and tenant_id. Evaluation cases whose tenant_id is NULL are global cases and are excluded from tenant-specific evaluation runs. Expected chunk references must belong to the same tenant and active document version.",
	},
}

func ensureEvalFixtures(
	ctx context.Context,
	db *sql.DB,
	cfg *models.Config,
	logger *zap.Logger,
	tenantID string,
) error {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return fmt.Errorf("parse fixture tenant ID: %w", err)
	}

	provider, err := embedding.NewGeminiProvider(cfg)
	if err != nil {
		return fmt.Errorf("initialize fixture embedding provider: %w", err)
	}
	embedder := embedding.NewService(provider, logger)

	// Create or reuse the fixture document and active version.
	var documentID string
	err = db.QueryRowContext(ctx, `
		SELECT id::text
		FROM documents
		WHERE tenant_id = $1
		  AND name = $2
		  AND deleted_at IS NULL
		ORDER BY created_at
		LIMIT 1
	`, tenantUUID, fixtureDocumentName).Scan(&documentID)

	if err == sql.ErrNoRows {
		err = db.QueryRowContext(ctx, `
			INSERT INTO documents (tenant_id, name, mime_type, status)
			VALUES ($1, $2, 'text/plain', 'active')
			RETURNING id::text
		`, tenantUUID, fixtureDocumentName).Scan(&documentID)
	}
	if err != nil {
		return fmt.Errorf("find or create fixture document: %w", err)
	}

	var versionID string
	err = db.QueryRowContext(ctx, `
		SELECT id::text
		FROM document_versions
		WHERE document_id = $1
		  AND version = 1
	`, documentID).Scan(&versionID)

	if err == sql.ErrNoRows {
		contentHash := sha256.Sum256([]byte(strings.Join(fixtureContents(), "\n")))
		err = db.QueryRowContext(ctx, `
			INSERT INTO document_versions
				(document_id, version, content_hash, storage_key, status)
			VALUES ($1, 1, $2, $3, 'active')
			RETURNING id::text
		`, documentID, hex.EncodeToString(contentHash[:]),
			"eval-fixture://retrievion/resume-smoke-fixture-v1").Scan(&versionID)
	}
	if err != nil {
		return fmt.Errorf("find or create fixture document version: %w", err)
	}

	// Do not call Gemini for chunks that already have embeddings.
	for i, fixture := range evalFixtures {
		contentHash := sha256.Sum256([]byte(fixture.content))
		hash := hex.EncodeToString(contentHash[:])

		var chunkID string
		var existingEmbedding sql.NullString
		err := db.QueryRowContext(ctx, `
			SELECT id::text, embedding::text
			FROM document_chunks
			WHERE document_version_id = $1
			  AND content_hash = $2
			  AND parent_chunk_id IS NULL
			LIMIT 1
		`, versionID, hash).Scan(&chunkID, &existingEmbedding)

		chunkMissing := errors.Is(err, sql.ErrNoRows)

		if err == nil && existingEmbedding.Valid && strings.TrimSpace(existingEmbedding.String) != "" {
			if err := ensureFixtureCase(ctx, db, tenantUUID, fixture, chunkID); err != nil {
				return err
			}
			continue
		}
		if err != nil && !chunkMissing {
			return fmt.Errorf("look up fixture chunk %d: %w", i, err)
		}

		vectors, err := embedder.Embed(ctx, []string{fixture.content})
		if err != nil {
			return fmt.Errorf("embed fixture chunk %d: %w", i, err)
		}
		if len(vectors) != 1 || len(vectors[0]) != 1536 {
			return fmt.Errorf("fixture chunk %d: expected one 1536-dimensional embedding", i)
		}
		vector := formatFixtureVector(vectors[0])

		if chunkMissing {
			err = db.QueryRowContext(ctx, `
				INSERT INTO document_chunks (
					tenant_id, document_id, document_version_id,
					chunk_index, content, content_hash, embedding,
					section, metadata, token_count
				)
				VALUES (
					$1, $2, $3, $4, $5, $6, $7::vector,
					'evaluation-fixture', '{"fixture":true}'::jsonb, $8
				)
				RETURNING id::text
			`, tenantUUID, documentID, versionID, i, fixture.content,
				hash, vector, len(strings.Fields(fixture.content))).Scan(&chunkID)
		} else {
			_, err = db.ExecContext(ctx, `
				UPDATE document_chunks
				SET embedding = $1::vector
				WHERE id = $2
			`, vector, chunkID)
		}
		if err != nil {
			return fmt.Errorf("persist fixture chunk %d: %w", i, err)
		}

		if err := ensureFixtureCase(ctx, db, tenantUUID, fixture, chunkID); err != nil {
			return err
		}
	}

	// Verify that every fixture chunk has a real stored vector before reporting success.
	var missing int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM document_chunks
		WHERE document_version_id = $1
		  AND metadata->>'fixture' = 'true'
		  AND embedding IS NULL
	`, versionID).Scan(&missing); err != nil {
		return fmt.Errorf("validate fixture embeddings: %w", err)
	}
	if missing != 0 {
		return fmt.Errorf("%d fixture chunks are missing embeddings", missing)
	}

	logger.Info(
		"evaluation fixtures are ready",
		zap.String("dataset", fixtureDataset),
		zap.String("tenant_id", tenantID),
		zap.Int("fixture_count", len(evalFixtures)),
	)
	return nil
}

func ensureFixtureCase(
	ctx context.Context,
	db *sql.DB,
	tenantID uuid.UUID,
	fixture evalFixture,
	chunkID string,
) error {
	// Update an existing fixture case in place so reruns preserve eval history.
	result, err := db.ExecContext(ctx, `
		UPDATE eval_cases
		SET expected_answer = $1,
		    expected_chunk_ids = ARRAY[$2::uuid],
		    metadata = '{"fixture":"retrievion-eval-v1"}'::jsonb
		WHERE dataset_name = $3
		  AND tenant_id = $4
		  AND question = $5
		  AND metadata->>'fixture' = 'retrievion-eval-v1'
	`, fixture.answer, chunkID, fixtureDataset, tenantID, fixture.question)
	if err != nil {
		return fmt.Errorf("update fixture evaluation case: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check fixture evaluation case update: %w", err)
	}
	if rows > 0 {
		return nil
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO eval_cases (
			dataset_name, question, expected_answer,
			expected_chunk_ids, tenant_id, metadata
		)
		VALUES (
			$1, $2, $3, ARRAY[$4::uuid], $5,
			'{"fixture":"retrievion-eval-v1"}'::jsonb
		)
	`, fixtureDataset, fixture.question, fixture.answer, chunkID, tenantID)
	if err != nil {
		return fmt.Errorf("insert fixture evaluation case: %w", err)
	}
	return nil
}

func fixtureContents() []string {
	contents := make([]string, len(evalFixtures))
	for i, fixture := range evalFixtures {
		contents[i] = fixture.content
	}
	return contents
}

func formatFixtureVector(vector []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, value := range vector {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
