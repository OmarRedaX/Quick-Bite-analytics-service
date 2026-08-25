package apperror

import (
	"net/http"

	"analytics-service/lib/http"
	"analytics-service/lib/logger"
)

// HandlerFunc is a controller method that can fail. This is the Go
// equivalent of an Express controller that `throw`s an AppError — Go has no
// exceptions, so the error comes back as a normal return value instead, and
// Wrap is what used to be the centralized `errorHandler` middleware.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Wrap adapts a HandlerFunc into a standard http.HandlerFunc: call the
// handler, and if it returns an error, render it through response.SendError.
// Every controller method in app/analytics/controller is registered via
// Wrap — see controller/routes.go.
func Wrap(h HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			response.SendError(w, logger.FromContext(r.Context()), err)
		}
	}
}
