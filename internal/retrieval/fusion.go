package retrieval

import (
	"sort"

	"github.com/mtariq99/retrievion/models"
)

type Fusion struct {
	K int
}

func NewFusion() *Fusion {
	return &Fusion{
		K: 60,
	}
}

type ResultFusion interface {
	Fuse(vectorResults, keywordResults []models.RetrievedChunk) []models.RetrievedChunk
}

func (f *Fusion) Fuse(vectorResults, keywordResults []models.RetrievedChunk) []models.RetrievedChunk {
	type candidate struct {
		chunk models.RetrievedChunk
		score float64
	}

	candidates := make(map[string]*candidate)

	for rank, chunk := range vectorResults {
		key := chunk.ChunkID.String()

		item, exists := candidates[key]
		if !exists {
			item = &candidate{
				chunk: chunk,
			}

			candidates[key] = item
		}

		item.score += 1.0 / float64(f.K+rank+1)
	}

	for rank, chunk := range keywordResults {
		key := chunk.ChunkID.String()

		item, exists := candidates[key]
		if !exists {
			item = &candidate{
				chunk: chunk,
			}

			candidates[key] = item
		}

		item.score += 1.0 / float64(f.K+rank+1)
	}

	results := make([]models.RetrievedChunk, 0, len(candidates))

	for _, candidate := range candidates {
		candidate.chunk.Score = candidate.score
		results = append(results, candidate.chunk)
	}

	sort.Slice(
		results,
		func(i, j int) bool {
			return results[i].Score > results[j].Score
		},
	)

	return results
}
