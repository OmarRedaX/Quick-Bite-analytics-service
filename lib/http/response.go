// Package response renders the service's two response envelopes:
//
//	success: {"success": true, "data": ...}
//	error:   {"success": false, "error": {"code": "...", "message": "..."}}
//
// Package name is `response`, not `http`, so callers can import both this
// package and stdlib "net/http" without aliasing — mirrors the apperror
// trick in lib/errors.
package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type successEnvelope struct {
	Success bool `json:"success"`
	Data    any  `json:"data"`
	Meta    any  `json:"meta,omitempty"`
}

type errorEnvelope struct {
	Success bool      `json:"success"`
	Error   errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// CodedError is the structural interface lib/errors.AppError satisfies.
// Defined here (not imported from lib/errors) so lib/http and lib/errors
// have no dependency on each other — either can change independently, and
// any future error type just needs these three methods to render correctly.
type CodedError interface {
	error
	Code() string
	StatusCode() int
}

func SendSuccess(w http.ResponseWriter, statusCode int, data any) {
	writeJSON(w, statusCode, successEnvelope{Success: true, Data: data})
}

// SendPaginated is SendSuccess with a meta block alongside data (cursor,
// hasMore, etc.) — the DTO layer decides what meta contains.
func SendPaginated(w http.ResponseWriter, data any, meta any) {
	writeJSON(w, http.StatusOK, successEnvelope{Success: true, Data: data, Meta: meta})
}

// SendError renders err as the error envelope. A CodedError (i.e. an
// *apperror.AppError) renders with its own code/status/message; anything
// else is treated as an unexpected failure and rendered as a generic 500
// so internals never leak to clients.
func SendError(w http.ResponseWriter, logger *slog.Logger, err error) {
	if ce, ok := err.(CodedError); ok {
		writeJSON(w, ce.StatusCode(), errorEnvelope{
			Success: false,
			Error:   errorBody{Code: ce.Code(), Message: ce.Error()},
		})
		return
	}

	logger.Error("unhandled error", "error", err.Error())
	writeJSON(w, http.StatusInternalServerError, errorEnvelope{
		Success: false,
		Error:   errorBody{Code: "INTERNAL_ERROR", Message: "Something went wrong"},
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}
