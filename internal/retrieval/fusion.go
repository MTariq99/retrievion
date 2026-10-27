package retrieval

import (
	"sort"

	"github.com/mtariq99/retrievion/models"
)

type Fusion struct {
	K int
}

func NewFusion() *Fusion {
	return &Fusion{K: 60}
}

type ResultFusion interface {
	Fuse(vectorResults, keywordResults []models.RetrievedChunk) []models.RetrievedChunk
}

func (f *Fusion) Fuse(vectorResults, keywordResults []models.RetrievedChunk) []models.RetrievedChunk {
	type candidate struct {
		chunk models.RetrievedChunk
		score float64
	}

	k := f.K
	if k <= 0 {
		k = 60
	}

	candidates := make(map[string]*candidate)

	addResults := func(chunks []models.RetrievedChunk) {
		for rank, chunk := range chunks {
			key := chunk.ChunkID.String()

			item, exists := candidates[key]
			if !exists {
				item = &candidate{chunk: chunk}
				candidates[key] = item
			}

			item.score += 1.0 / float64(k+rank+1)
		}
	}

	addResults(vectorResults)
	addResults(keywordResults)

	results := make([]models.RetrievedChunk, 0, len(candidates))
	for _, item := range candidates {
		item.chunk.Score = item.score
		results = append(results, item.chunk)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}
