package errs

import "errors"

var (
	// ErrLLMUnavailable indicates that the configured LLM provider
	// cannot currently serve requests.
	ErrLLMUnavailable = errors.New("LLM service unavailable")

	// ErrLLMTimeout indicates that the LLM request exceeded its timeout.
	ErrLLMTimeout = errors.New("LLM request timed out")

	// ErrLLMRateLimited indicates that the application has exceeded
	// the provider's rate limit.
	ErrLLMRateLimited = errors.New("LLM rate limit exceeded")

	// ErrLLMAuthentication indicates that authentication with the
	// LLM provider failed.
	ErrLLMAuthentication = errors.New("LLM provider authentication failed")

	// ErrLLMInvalidRequest indicates that the request sent to the
	// LLM provider was invalid.
	ErrLLMInvalidRequest = errors.New("invalid LLM request")

	// ErrLLMGenerationFailed indicates that the LLM failed to generate
	// a response.
	ErrLLMGenerationFailed = errors.New("failed to generate response")

	// ErrEmbeddingUnavailable indicates that the embedding provider
	// is unavailable.
	ErrEmbeddingUnavailable = errors.New("embedding service unavailable")

	// ErrEmbeddingRateLimited indicates that the embedding provider
	// has rate-limited the application.
	ErrEmbeddingRateLimited = errors.New("embedding rate limit exceeded")

	// ErrEmbeddingAuthentication indicates that authentication with
	// the embedding provider failed.
	ErrEmbeddingAuthentication = errors.New("embedding provider authentication failed")

	// ErrEmbeddingInvalidRequest indicates that the embedding request
	// is invalid.
	ErrEmbeddingInvalidRequest = errors.New("invalid embedding request")

	// ErrVectorStoreUnavailable indicates that the vector store cannot
	// currently process requests.
	ErrVectorStoreUnavailable = errors.New("vector store unavailable")

	// ErrVectorSearchFailed indicates that vector similarity search failed.
	ErrVectorSearchFailed = errors.New("vector search failed")
)
