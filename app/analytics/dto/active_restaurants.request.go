package dto

import (
	"net/http"

	apperror "analytics-service/lib/errors"
)

// ActiveRestaurantsQuery is the validated input for
// GET /restaurants/active?from=&to=.
type ActiveRestaurantsQuery struct {
	From string `validate:"required,datetime=2006-01-02"`
	To   string `validate:"required,datetime=2006-01-02"`
}

func ParseActiveRestaurantsQuery(r *http.Request) (ActiveRestaurantsQuery, error) {
	q := ActiveRestaurantsQuery{
		From: r.URL.Query().Get("from"),
		To:   r.URL.Query().Get("to"),
	}
	if err := validate.Struct(q); err != nil {
		return ActiveRestaurantsQuery{}, apperror.FromValidation(err)
	}
	return q, nil
}
