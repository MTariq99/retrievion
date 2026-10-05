package citation

import (
	"context"
	"regexp"
	"strconv"

	"github.com/mtariq99/retrievion/models"
)

type Resolver interface {
	Resolve(
		ctx context.Context,
		answer string,
		chunks []models.RetrievedChunk,
	) ([]models.Citation, []models.MessageSource, error)
}

type Service struct {
	// Add repository here later if DocumentName
	// needs to be fetched from PostgreSQL.
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Resolve(ctx context.Context, answer string, chunks []models.RetrievedChunk) ([]models.Citation, []models.MessageSource, error) {
	_ = ctx

	citations := make([]models.Citation, 0)
	messageSources := make([]models.MessageSource, 0)

	if len(chunks) == 0 {
		return citations, messageSources, nil
	}

	// Detect [1], [2], [3], etc.
	re := regexp.MustCompile(`\[(\d+)\]`)

	matches := re.FindAllStringSubmatch(answer, -1)

	seen := make(map[int]struct{})

	for _, match := range matches {
		if len(match) != 2 {
			continue
		}

		index, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}

		// Citation indexes are 1-based.
		chunkIndex := index - 1

		if chunkIndex < 0 || chunkIndex >= len(chunks) {
			continue
		}

		// Don't store the same citation twice.
		if _, exists := seen[index]; exists {
			continue
		}

		seen[index] = struct{}{}

		chunk := chunks[chunkIndex]

		citation := models.Citation{
			Index:      index,
			DocumentID: chunk.DocumentID,
			Page:       chunk.Page,
			Section:    chunk.Section,
		}

		citations = append(citations, citation)

		messageSource := models.MessageSource{
			ChunkID:       chunk.ChunkID,
			DocumentID:    chunk.DocumentID,
			CitationIndex: index,
		}

		messageSources = append(
			messageSources,
			messageSource,
		)
	}

	return citations, messageSources, nil
}
