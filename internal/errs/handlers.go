package errs

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mtariq99/retrievion/internal/response"
)

// HTTPStatus converts an application error into an HTTP status code.
func HTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}

	switch {
	// ---------------------------------------------------------------------
	// 400 Bad Request
	// ---------------------------------------------------------------------

	case errors.Is(err, ErrInvalidRequest),
		errors.Is(err, ErrValidation),
		errors.Is(err, ErrInvalidID),
		errors.Is(err, ErrMissingID),
		errors.Is(err, ErrMissingUserID),
		errors.Is(err, ErrMissingDocumentID),
		errors.Is(err, ErrMissingName),
		errors.Is(err, ErrMissingPhone),
		errors.Is(err, ErrMissingEmail),
		errors.Is(err, ErrInvalidEmail),
		errors.Is(err, ErrMissingPassword),
		errors.Is(err, ErrInvalidPassword),
		errors.Is(err, ErrInvalidFileExtension),
		errors.Is(err, ErrInvalidFileSize),
		errors.Is(err, ErrMissingQuery),
		errors.Is(err, ErrInvalidQuery),
		errors.Is(err, ErrMissingConversationID),
		errors.Is(err, ErrMissingMessage),
		errors.Is(err, ErrInvalidMessage),
		errors.Is(err, ErrInvalidPage),
		errors.Is(err, ErrInvalidPageSize),
		errors.Is(err, ErrUnsupportedFileType),
		errors.Is(err, ErrDocumentTooLarge):

		return http.StatusBadRequest

	// ---------------------------------------------------------------------
	// 401 Unauthorized
	// ---------------------------------------------------------------------

	case errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrInvalidCredentials),
		errors.Is(err, ErrInvalidToken),
		errors.Is(err, ErrEmailNotVerified),
		errors.Is(err, ErrCurrentPasswordIncorrect),
		errors.Is(err, ErrLLMAuthentication),
		errors.Is(err, ErrEmbeddingAuthentication):

		return http.StatusUnauthorized

	// ---------------------------------------------------------------------
	// 403 Forbidden
	// ---------------------------------------------------------------------

	case errors.Is(err, ErrForbidden),
		errors.Is(err, ErrAccountDisabled),
		errors.Is(err, ErrAccountLocked):

		return http.StatusForbidden

	// ---------------------------------------------------------------------
	// 404 Not Found
	// ---------------------------------------------------------------------

	case errors.Is(err, ErrNotFound):

		return http.StatusNotFound

	// ---------------------------------------------------------------------
	// 409 Conflict
	// ---------------------------------------------------------------------

	case errors.Is(err, ErrAlreadyExists),
		errors.Is(err, ErrConflict),
		errors.Is(err, ErrDocumentProcessing),
		errors.Is(err, ErrDocumentAlreadyProcessed):

		return http.StatusConflict

	// ---------------------------------------------------------------------
	// 408 Request Timeout
	// ---------------------------------------------------------------------

	case errors.Is(err, context.DeadlineExceeded):

		return http.StatusRequestTimeout

	// ---------------------------------------------------------------------
	// 422 Unprocessable Entity
	// ---------------------------------------------------------------------

	case errors.Is(err, ErrDocumentEmpty),
		errors.Is(err, ErrDocumentInaccessible),
		errors.Is(err, ErrChunkingFailed):

		return http.StatusUnprocessableEntity

	// ---------------------------------------------------------------------
	// 429 Too Many Requests
	// ---------------------------------------------------------------------

	case errors.Is(err, ErrLLMRateLimited),
		errors.Is(err, ErrEmbeddingRateLimited):

		return http.StatusTooManyRequests

	// ---------------------------------------------------------------------
	// 502 Bad Gateway
	// ---------------------------------------------------------------------

	case errors.Is(err, ErrLLMGenerationFailed),
		errors.Is(err, ErrEmbeddingFailed),
		errors.Is(err, ErrRetrievalFailed),
		errors.Is(err, ErrIndexingFailed),
		errors.Is(err, ErrVectorSearchFailed):

		return http.StatusBadGateway

	// ---------------------------------------------------------------------
	// 503 Service Unavailable
	// ---------------------------------------------------------------------

	case errors.Is(err, ErrDatabaseUnavailable),
		errors.Is(err, ErrLLMUnavailable),
		errors.Is(err, ErrEmbeddingUnavailable),
		errors.Is(err, ErrVectorStoreUnavailable),
		errors.Is(err, ErrRetryable):

		return http.StatusServiceUnavailable

	// ---------------------------------------------------------------------
	// 504 Gateway Timeout
	// ---------------------------------------------------------------------

	case errors.Is(err, ErrDatabaseTimeout),
		errors.Is(err, ErrLLMTimeout):

		return http.StatusGatewayTimeout

	default:
		return http.StatusInternalServerError
	}
}

// HandlerError converts an application error into a safe HTTP response.
//
// Important:
//   - 4xx errors may expose their safe application-level message.
//   - 5xx errors are logged internally and replaced with ErrInternal.
//   - Raw infrastructure/database/provider errors must never be exposed
//     directly to clients.
func HandlerError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	status := HTTPStatus(err)

	if status >= http.StatusInternalServerError {
		log.Printf("internal error: %+v", err)

		response.WriteErrorResponse(
			c,
			ErrInternal.Error(),
			status,
		)

		return
	}

	response.WriteErrorResponse(c, err.Error(), status)
}
