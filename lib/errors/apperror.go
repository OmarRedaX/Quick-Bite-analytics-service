// Package apperror is the Go analogue of order-service's lib/error/AppError.ts
// — a typed error services/controllers return instead of throwing. Unlike
// the Node version (message + statusCode), AppError also carries a stable
// `code` string, because this service's response envelope requires
// {"error": {"code", "message"}} (see docs/api-contracts.md).
//
// Package name is `apperror`, not `errors`, so call sites can import both
// this package and stdlib "errors" in the same file without aliasing.
package apperror

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// AppError is the only error type services/controllers should return for
// expected, user-facing failures. Fields are unexported and exposed via
// methods so lib/http can render it through a small structural interface
// (see lib/http/response.go's CodedError) without importing this package —
// that keeps lib/http and lib/errors from depending on each other.
type AppError struct {
	code       string
	statusCode int
	message    string
	err        error
}

func New(code string, statusCode int, message string) *AppError {
	return &AppError{code: code, statusCode: statusCode, message: message}
}

// WithCause attaches an AppError code/status/message to an underlying
// error, keeping it inspectable via errors.Unwrap/errors.Is for logging.
// (Named WithCause, not Wrap, to leave `Wrap` free for handler.go's HTTP
// middleware — the two are unrelated concepts that happened to want the
// same verb.)
func WithCause(err error, code string, statusCode int, message string) *AppError {
	return &AppError{code: code, statusCode: statusCode, message: message, err: err}
}

func (e *AppError) Error() string   { return e.message }
func (e *AppError) Code() string    { return e.code }
func (e *AppError) StatusCode() int { return e.statusCode }
func (e *AppError) Unwrap() error   { return e.err }

// FromValidation converts a go-playground/validator error into a single
// AppError with code VALIDATION_ERROR — the DTO-layer equivalent of
// order-service's `validateBody(DTO, req.body)` throwing class-validator
// errors as a 400.
func FromValidation(err error) *AppError {
	var fieldErrs validator.ValidationErrors
	if !AsValidationErrors(err, &fieldErrs) {
		return New("VALIDATION_ERROR", 400, err.Error())
	}

	msgs := make([]string, 0, len(fieldErrs))
	for _, fe := range fieldErrs {
		msgs = append(msgs, fmt.Sprintf("%s: failed on '%s'", fe.Field(), fe.Tag()))
	}
	return New("VALIDATION_ERROR", 400, strings.Join(msgs, "; "))
}

// AsValidationErrors is a tiny errors.As shim kept local so callers of
// FromValidation don't need to import validator themselves.
func AsValidationErrors(err error, target *validator.ValidationErrors) bool {
	ve, ok := err.(validator.ValidationErrors)
	if !ok {
		return false
	}
	*target = ve
	return true
}
