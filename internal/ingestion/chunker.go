package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/tokenizer"
	"github.com/mtariq99/retrievion/models"
)

const (
	maxChildTokens  = 700
	minChildTokens  = 50
	overlapFraction = 0.15
	maxParentTokens = 2000
)

type Chunker interface {
	Chunk(ctx context.Context, doc *models.ParsedDocument, versionID uuid.UUID) ([]models.Chunk, error)
}

type ParentChildChunker struct {
	tokenizer tokenizer.Counter
}

func NewParentChildChunker(t tokenizer.Counter) *ParentChildChunker {
	return &ParentChildChunker{
		tokenizer: t,
	}
}

func (c *ParentChildChunker) Chunk(ctx context.Context, doc *models.ParsedDocument, versionID uuid.UUID) ([]models.Chunk, error) {
	var chunks []models.Chunk
	index := 0
	seenParents := make(map[string]bool)

	for _, section := range doc.Sections {
		page := 0
		if section.Page != nil {
			page = *section.Page
		}

		for _, parentText := range c.splitParents(section.Content) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			if strings.TrimSpace(parentText) == "" {
				continue
			}

			parentHash := hashContent(parentText)
			if seenParents[parentHash] {
				continue
			}
			seenParents[parentHash] = true

			parentID := uuid.New()

			chunks = append(chunks, models.Chunk{
				ID:                parentID,
				DocumentVersionID: versionID,
				Content:           parentText,
				ContentHash:       parentHash,
				Page:              page,
				Section:           section.Title,
				ChunkIndex:        index,
				TokenCount:        c.tokenizer.Count(parentText),
				IsParent:          true,
			})

			index++

			paragraphs := splitParagraphs(parentText)

			children := splitIntoTokenBounds(paragraphs, maxChildTokens, minChildTokens, overlapFraction, c.tokenizer)

			seenChildren := make(map[string]bool)

			for _, child := range children {
				child = strings.TrimSpace(child)

				if child == "" {
					continue
				}

				childHash := hashContent(child)
				if seenChildren[childHash] {
					continue
				}
				seenChildren[childHash] = true

				pid := parentID

				chunks = append(chunks, models.Chunk{
					ID:                uuid.New(),
					ParentChunkID:     &pid,
					DocumentVersionID: versionID,
					Content:           child,
					ContentHash:       childHash,
					Page:              page,
					Section:           section.Title,
					ChunkIndex:        index,
					TokenCount:        c.tokenizer.Count(child),
					IsParent:          false,
				})

				index++
			}
		}
	}

	return chunks, nil
}

func (c *ParentChildChunker) splitParents(content string) []string {
	if c.tokenizer.Count(content) <= maxParentTokens {
		return []string{content}
	}

	var (
		parents []string
		current []string
		tokens  int
	)
	for _, p := range splitParagraphs(content) {
		t := c.tokenizer.Count(p)
		if tokens+t > maxParentTokens && len(current) > 0 {
			parents = append(parents, strings.Join(current, "\n"))
			current, tokens = nil, 0
		}
		current = append(current, p)
		tokens += t
	}
	if len(current) > 0 {
		parents = append(parents, strings.Join(current, "\n"))
	}
	return parents
}

func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n")

	var paragraphs []string

	for _, p := range raw {
		p = strings.TrimSpace(p)

		if p != "" {
			paragraphs = append(paragraphs, p)
		}
	}

	return paragraphs
}

func splitIntoTokenBounds(paragraphs []string, maxTokens int, minTokens int, overlap float64, counter tokenizer.Counter) []string {
	if len(paragraphs) == 0 {
		return nil
	}

	var chunks []string

	start := 0

	for start < len(paragraphs) {
		var current []string
		currentTokens := 0

		end := start

		for end < len(paragraphs) {
			paragraph := paragraphs[end]
			tokens := counter.Count(paragraph)
			if tokens > maxTokens {
				if len(current) > 0 {
					chunks = appendChunk(chunks, strings.Join(current, "\n"), minTokens, counter)
					current = nil
					currentTokens = 0
				}
				largeParts := splitLargeText(paragraph, maxTokens, counter)

				for _, part := range largeParts {
					if counter.Count(part) >= minTokens || len(chunks) == 0 {
						chunks = append(chunks, part)
					}
				}

				end++
				start = end
				break
			}

			if currentTokens+tokens > maxTokens && len(current) > 0 {
				break
			}

			current = append(current, paragraph)
			currentTokens += tokens
			end++
		}

		if len(current) == 0 {
			continue
		}

		chunk := strings.Join(current, "\n")

		chunks = appendChunk(chunks, chunk, minTokens, counter)
		overlapCount := int(float64(len(current)) * overlap)

		if overlapCount > 0 && end < len(paragraphs) {
			start = end - overlapCount

			if start <= 0 {
				start = end
			}
		} else {
			start = end
		}
	}

	return chunks
}

func appendChunk(chunks []string, chunk string, minTokens int, counter tokenizer.Counter) []string {
	chunk = strings.TrimSpace(chunk)

	if chunk == "" {
		return chunks
	}

	tokens := counter.Count(chunk)

	if tokens < minTokens && len(chunks) > 0 {
		chunks[len(chunks)-1] += "\n" + chunk
		return chunks
	}

	return append(chunks, chunk)
}

func splitLargeText(text string, maxTokens int, counter tokenizer.Counter) []string {
	words := strings.Fields(text)

	if len(words) == 0 {
		return nil
	}

	var chunks []string
	var current []string
	currentTokens := 0

	for _, word := range words {
		tokens := counter.Count(word)

		if currentTokens+tokens > maxTokens && len(current) > 0 {
			chunks = append(
				chunks,
				strings.Join(current, " "),
			)

			current = nil
			currentTokens = 0
		}

		current = append(current, word)
		currentTokens += tokens
	}

	if len(current) > 0 {
		chunks = append(
			chunks,
			strings.Join(current, " "),
		)
	}

	return chunks
}

func hashContent(content string) string {
	normalized := normalizeContent(content)

	sum := sha256.Sum256([]byte(normalized))

	return hex.EncodeToString(sum[:])
}

func normalizeContent(content string) string {
	return strings.Join(
		strings.Fields(content),
		" ",
	)
}
