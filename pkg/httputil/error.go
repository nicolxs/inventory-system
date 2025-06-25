package httputil

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// HTTPError represents a structured error response.
type HTTPError struct {
	StatusCode int    `json:"-"` // HTTP status code, not included in JSON response body directly by Echo
	Code       string `json:"code,omitempty"`    // Application-specific error code (optional)
	Message    string `json:"message"`           // Human-readable error message
	Details    any    `json:"details,omitempty"` // More detailed error information (e.g., validation errors)
}

// NewHTTPError creates a new HTTPError instance.
func NewHTTPError(statusCode int, message string) *HTTPError {
	return &HTTPError{
		StatusCode: statusCode,
		Message:    message,
	}
}

// NewHTTPErrorWithCode creates a new HTTPError instance with an application-specific code.
func NewHTTPErrorWithCode(statusCode int, code string, message string) *HTTPError {
	return &HTTPError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
	}
}

// WithDetails adds details to the HTTPError.
func (e *HTTPError) WithDetails(details any) *HTTPError {
	e.Details = details
	return e
}

// Error implements the error interface.
// This allows HTTPError to be returned as an error from handlers,
// and Echo's default error handler can potentially pick it up if configured.
// However, we'll mostly use a dedicated function to send the response.
func (e *HTTPError) Error() string {
	return e.Message
}

// SendErrorResponse sends a standardized JSON error response.
// It logs the error internally for server-side tracking if it's a 5xx error.
func SendErrorResponse(c echo.Context, err *HTTPError) error {
	if err.StatusCode >= 500 {
		// Log server errors for monitoring
		log.Printf("Server Error: Status %d, Message: %s, Details: %v, Path: %s",
			err.StatusCode, err.Message, err.Details, c.Request().URL.Path)
	}

	// The `HTTPError` struct itself will be marshalled to JSON by Echo.
	// We pass err directly. Echo's c.JSON() will use the fields
	// with `json` tags for the response body and err.StatusCode for the HTTP status.
	return c.JSON(err.StatusCode, err)
}

// --- Common Error Constructors ---

func BadRequestError(message string) *HTTPError {
	return NewHTTPErrorWithCode(http.StatusBadRequest, "BAD_REQUEST", message)
}

func ValidationError(message string, details any) *HTTPError {
	err := NewHTTPErrorWithCode(http.StatusUnprocessableEntity, "VALIDATION_ERROR", message)
	return err.WithDetails(details)
}

func NotFoundError(message string) *HTTPError {
	return NewHTTPErrorWithCode(http.StatusNotFound, "NOT_FOUND", message)
}

func UnauthorizedError(message string) *HTTPError {
	return NewHTTPErrorWithCode(http.StatusUnauthorized, "UNAUTHORIZED", message)
}

func ForbiddenError(message string) *HTTPError {
	return NewHTTPErrorWithCode(http.StatusForbidden, "FORBIDDEN", message)
}

func InternalServerError(message string) *HTTPError {
	// For client-facing message, keep it generic. Detailed error is logged.
	if message == "" {
		message = "An unexpected error occurred on the server."
	}
	return NewHTTPErrorWithCode(http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", message)
}

func ConflictError(message string) *HTTPError {
    return NewHTTPErrorWithCode(http.StatusConflict, "CONFLICT", message)
}
