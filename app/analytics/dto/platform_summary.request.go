package dto

import (
	"net/http"

	apperror "analytics-service/lib/errors"
)

// PlatformSummaryQuery is the validated input for
// GET /platform/summary?from=&to=.
type PlatformSummaryQuery struct {
	From string `validate:"required,datetime=2006-01-02"`
	To   string `validate:"required,datetime=2006-01-02"`
}

func ParsePlatformSummaryQuery(r *http.Request) (PlatformSummaryQuery, error) {
	q := PlatformSummaryQuery{
		From: r.URL.Query().Get("from"),
		To:   r.URL.Query().Get("to"),
	}
	if err := validate.Struct(q); err != nil {
		return PlatformSummaryQuery{}, apperror.FromValidation(err)
	}
	return q, nil
}
