package errs

import "errors"

var (
	ErrLLMUnavailable          = errors.New("LLM service unavailable")
	ErrLLMTimeout              = errors.New("LLM request timed out")
	ErrLLMRateLimited          = errors.New("LLM rate limit exceeded")
	ErrLLMAuthentication       = errors.New("LLM provider authentication failed")
	ErrLLMInvalidRequest       = errors.New("invalid LLM request")
	ErrLLMGenerationFailed     = errors.New("failed to generate response")
	ErrEmbeddingUnavailable    = errors.New("embedding service unavailable")
	ErrEmbeddingRateLimited    = errors.New("embedding rate limit exceeded")
	ErrEmbeddingAuthentication = errors.New("embedding provider authentication failed")
	ErrEmbeddingInvalidRequest = errors.New("invalid embedding request")
	ErrVectorStoreUnavailable  = errors.New("vector store unavailable")
	ErrVectorSearchFailed      = errors.New("vector search failed")
	ErrEmptyInput              = errors.New("embedding: no texts provided")
	ErrDimensionMismatch       = errors.New("embedding: returned vector count does not match input count")
)
