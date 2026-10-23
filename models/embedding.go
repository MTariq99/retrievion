package models

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
