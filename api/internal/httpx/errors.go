// Package httpx holds the HTTP plumbing shared by every handler: one response
// envelope, one error shape, and the middleware stack.
package httpx

import (
	"errors"
	"fmt"
	"net/http"
)

// Error codes. These are stable identifiers the frontend switches on; the
// human-readable message alongside them is Indonesian, matching the UI.
const (
	CodeValidationFailed = "validation_failed"
	CodeUnauthorized     = "unauthorized"
	CodeForbidden        = "forbidden"
	CodeNotFound         = "not_found"
	CodeConflict         = "conflict"
	CodePayloadTooLarge  = "payload_too_large"
	CodeUnsupportedMedia = "unsupported_media_type"
	CodeRateLimited      = "rate_limited"
	CodeInternal         = "internal"
	CodeBadRequest       = "bad_request"
)

// APIError is an error carrying everything needed to render a response.
//
// Cause is logged but never serialised: Go error strings leak table names, query
// fragments and file paths, none of which belong in a client response.
type APIError struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]string
	Cause   error
}

func (e *APIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *APIError) Unwrap() error { return e.Cause }

func BadRequest(message string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: CodeBadRequest, Message: message}
}

// Invalid reports per-field validation problems, keyed by the JSON field name so
// the form can attach each message to its input.
func Invalid(fields map[string]string) *APIError {
	return &APIError{
		Status:  http.StatusUnprocessableEntity,
		Code:    CodeValidationFailed,
		Message: "Data tidak valid.",
		Fields:  fields,
	}
}

func Unauthorized() *APIError {
	return &APIError{
		Status:  http.StatusUnauthorized,
		Code:    CodeUnauthorized,
		Message: "Anda harus masuk terlebih dahulu.",
	}
}

func Forbidden() *APIError {
	return &APIError{
		Status:  http.StatusForbidden,
		Code:    CodeForbidden,
		Message: "Anda tidak memiliki akses untuk tindakan ini.",
	}
}

func NotFound() *APIError {
	return &APIError{
		Status:  http.StatusNotFound,
		Code:    CodeNotFound,
		Message: "Data tidak ditemukan.",
	}
}

func TooLarge(message string) *APIError {
	return &APIError{Status: http.StatusRequestEntityTooLarge, Code: CodePayloadTooLarge, Message: message}
}

func UnsupportedMedia(message string) *APIError {
	return &APIError{Status: http.StatusUnsupportedMediaType, Code: CodeUnsupportedMedia, Message: message}
}

func RateLimited() *APIError {
	return &APIError{
		Status:  http.StatusTooManyRequests,
		Code:    CodeRateLimited,
		Message: "Terlalu banyak percobaan. Coba lagi beberapa saat lagi.",
	}
}

// Internal wraps an unexpected failure. The cause is logged, never returned.
func Internal(cause error) *APIError {
	return &APIError{
		Status:  http.StatusInternalServerError,
		Code:    CodeInternal,
		Message: "Terjadi kesalahan pada server.",
		Cause:   cause,
	}
}

// AsAPIError converts any error into an APIError, defaulting to a 500.
func AsAPIError(err error) *APIError {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return Internal(err)
}
