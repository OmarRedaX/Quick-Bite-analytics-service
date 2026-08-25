package analytics

import (
	"net/http"

	apperror "analytics-service/lib/errors"
)

// Module-level, stable-named AppErrors — never construct an ad-hoc
// apperror.New(...) at a call site for a case that has a name here (see
// CLAUDE.md §Error handling).
var (
	ErrInvalidDateRange = apperror.New(
		"ANALYTICS_INVALID_DATE_RANGE",
		http.StatusBadRequest,
		"from must not be after to",
	)
)
