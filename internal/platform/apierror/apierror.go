// Package apierror defines the application error type and the stable error
// codes returned to API clients. Codes are part of the public contract: never
// rename one, only add new ones.
package apierror

import "net/http"

// Code is a stable, machine-readable error identifier (UPPER_SNAKE_CASE).
type Code string

const (
	CodeValidationFailed      Code = "VALIDATION_FAILED"
	CodeInvalidDocumentNumber Code = "INVALID_DOCUMENT_NUMBER"
	CodeUnauthorized          Code = "UNAUTHORIZED"
	CodeForbidden             Code = "FORBIDDEN"
	CodeNotFound              Code = "NOT_FOUND"
	CodeDocumentNotFound      Code = "DOCUMENT_NOT_FOUND"
	CodeRateLimited           Code = "RATE_LIMITED"
	CodeUpstreamFailed        Code = "UPSTREAM_FAILED"
	CodeUpstreamTimeout       Code = "UPSTREAM_TIMEOUT"
	CodeInternal              Code = "INTERNAL_ERROR"
)

// Detail points at one field (or parameter) that caused the error.
type Detail struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// Error is returned by handlers and services; the response layer renders it.
type Error struct {
	Code    Code
	Status  int
	Message string
	Details []Detail
	Cause   error // logged, never sent to the client
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }
func (e *Error) Unwrap() error { return e.Cause }

func New(status int, code Code, message string, details ...Detail) *Error {
	return &Error{Code: code, Status: status, Message: message, Details: details}
}

func (e *Error) WithCause(err error) *Error {
	e.Cause = err
	return e
}

func ValidationFailed(details ...Detail) *Error {
	return New(http.StatusBadRequest, CodeValidationFailed, "Request validation failed.", details...)
}

func InvalidDocumentNumber(message string) *Error {
	return New(http.StatusBadRequest, CodeInvalidDocumentNumber, message)
}

func Unauthorized() *Error {
	return New(http.StatusUnauthorized, CodeUnauthorized, "Missing or invalid API key.")
}

func DocumentNotFound() *Error {
	return New(http.StatusNotFound, CodeDocumentNotFound, "Document not found in the database or the upstream provider.")
}

func UpstreamFailed(cause error) *Error {
	return New(http.StatusBadGateway, CodeUpstreamFailed, "The upstream provider failed.").WithCause(cause)
}

func UpstreamTimeout(cause error) *Error {
	return New(http.StatusGatewayTimeout, CodeUpstreamTimeout, "The upstream provider timed out.").WithCause(cause)
}

func Internal(cause error) *Error {
	return New(http.StatusInternalServerError, CodeInternal, "Internal server error.").WithCause(cause)
}
