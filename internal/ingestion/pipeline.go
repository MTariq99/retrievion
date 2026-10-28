package ingestion

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mtariq99/retrievion/models"
	"github.com/mtariq99/retrievion/pkg/objectstorage"
)

type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Pipeline struct {
	repo     *Repository
	storage  objectstorage.Storage
	parser   Parser
	chunker  Chunker
	embedder EmbeddingProvider
	logger   *zap.Logger
}

func NewPipeline(repo *Repository, storage objectstorage.Storage, parser Parser, chunker Chunker, embedder EmbeddingProvider, logger *zap.Logger) *Pipeline {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Pipeline{repo: repo, storage: storage, parser: parser, chunker: chunker, embedder: embedder, logger: logger}
}

func (p *Pipeline) Process(ctx context.Context, job models.Job) error {
	attempt, err := p.repo.MarkJobProcessing(ctx, job.JobID)
	if err != nil {
		return err
	}
	job.Attempt = attempt

	version, err := p.repo.GetVersionForIngestion(ctx, job.DocumentVersionID)
	if err != nil {
		return p.fail(ctx, job.JobID, job.Attempt, fmt.Errorf("load version: %w", err))
	}
	tenantID := version.TenantID

	obj, err := p.storage.Download(ctx, version.StorageKey)
	if err != nil {
		return p.fail(ctx, job.JobID, job.Attempt, fmt.Errorf("download object: %w", err))
	}
	defer obj.Close()

	parsed, err := p.parser.Parse(ctx, obj, version.MimeType)
	if err != nil {
		return p.fail(ctx, job.JobID, job.Attempt, fmt.Errorf("parse document: %w", err))
	}
	for _, w := range parsed.Warnings {
		p.logger.Warn("document parsed with warnings", zap.String("job_id", job.JobID.String()), zap.String("warning", w))
	}

	chunks, err := p.chunker.Chunk(ctx, parsed, job.DocumentVersionID)
	if err != nil {
		return p.fail(ctx, job.JobID, job.Attempt, fmt.Errorf("chunk document: %w", err))
	}

	var texts []string
	var textChunkIDs []uuid.UUID
	for _, c := range chunks {
		if c.IsParent {
			continue // only children are embedded (README §19)
		}
		texts = append(texts, c.Content)
		textChunkIDs = append(textChunkIDs, c.ID)
	}

	embeddings := make(map[uuid.UUID][]float32, len(texts))
	if len(texts) > 0 {
		vectors, err := p.embedder.Embed(ctx, texts)
		if err != nil {
			return p.fail(ctx, job.JobID, job.Attempt, fmt.Errorf("generate embeddings: %w", err))
		}
		if len(vectors) != len(texts) {
			return p.fail(ctx, job.JobID, job.Attempt, fmt.Errorf("embedding count mismatch: got %d, want %d", len(vectors), len(texts)))
		}
		for i, id := range textChunkIDs {
			embeddings[id] = vectors[i]
		}
	}

	if err := p.repo.StoreChunks(ctx, tenantID, version.DocumentID, chunks, embeddings); err != nil {
		return p.fail(ctx, job.JobID, job.Attempt, fmt.Errorf("store chunks: %w", err))
	}

	if err := p.repo.ActivateVersion(ctx, job.JobID, job.DocumentID, job.DocumentVersionID, job.Attempt); err != nil {
		return p.fail(ctx, job.JobID, job.Attempt, fmt.Errorf("activate version: %w", err))
	}

	if err := p.repo.MarkJobCompleted(ctx, job.JobID, job.Attempt); err != nil {
		return fmt.Errorf("mark job completed: %w", err)
	}

	return nil
}

func (p *Pipeline) fail(ctx context.Context, jobID uuid.UUID, attempt int, cause error) error {
	if markErr := p.repo.MarkJobFailed(ctx, jobID, attempt, cause); markErr != nil {
		return fmt.Errorf("%w (also failed to record failure: %v)", cause, markErr)
	}
	return cause
}
