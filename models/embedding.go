package models

// EmbeddingModelMetadata tracks which model/dimension produced a set of
// vectors. Persist this alongside stored embeddings — changing models or
// dimensions requires re-indexing (README §20), and this is what lets you
// detect that mismatch instead of silently corrupting retrieval.
type EmbeddingModelMetadata struct {
	Provider  string
	Model     string
	Dimension int
}

type EmbeddingRequest struct {
	Texts []string
}

type EmbeddingResponse struct {
	Vectors  [][]float32
	Metadata EmbeddingModelMetadata
	Usage    TokenUsage
}
