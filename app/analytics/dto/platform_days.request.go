package dto

import (
	"net/http"

	apperror "analytics-service/lib/errors"
)

// PlatformDaysQuery is the validated input for GET /platform/days?from=&to=.
type PlatformDaysQuery struct {
	From string `validate:"required,datetime=2006-01-02"`
	To   string `validate:"required,datetime=2006-01-02"`
}

func ParsePlatformDaysQuery(r *http.Request) (PlatformDaysQuery, error) {
	q := PlatformDaysQuery{
		From: r.URL.Query().Get("from"),
		To:   r.URL.Query().Get("to"),
	}
	if err := validate.Struct(q); err != nil {
		return PlatformDaysQuery{}, apperror.FromValidation(err)
	}
	return q, nil
}
