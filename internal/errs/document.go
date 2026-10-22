package errs

import "errors"

var (
	ErrDocumentProcessing       = errors.New("document is already being processed")
	ErrDocumentTooLarge         = errors.New("document is too large")
	ErrUnsupportedFileType      = errors.New("unsupported file type")
	ErrDocumentEmpty            = errors.New("document contains no usable text")
	ErrDocumentInaccessible     = errors.New("document is inaccessible")
	ErrDocumentAlreadyProcessed = errors.New("document has already been processed")
	ErrChunkingFailed           = errors.New("failed to chunk document")
	ErrEmbeddingFailed          = errors.New("failed to generate document embeddings")
	ErrIndexingFailed           = errors.New("failed to index document")
	ErrRetrievalFailed          = errors.New("failed to retrieve relevant documents")
	ErrMissingDocument          = errors.New("document is required")
	ErrFileTooLarge             = errors.New("file is too large")
	ErrEmptyFile                = errors.New("document is empty")
	ErrInvalidFilename          = errors.New("invalid file name")
	ErrDuplicateDocument        = errors.New("duplicate document")
	ErrEmptyInput               = errors.New("embedding: no texts provided")
	ErrDimensionMismatch        = errors.New("embedding: returned vector count does not match input count")
	ErrDocumentCorrupt          = errors.New("document is corrupt or malformed")
)
