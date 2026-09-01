package dto

import (
	"net/http"
	"strconv"

	apperror "analytics-service/lib/errors"
)

// RestaurantFailuresQuery is the validated input for
// GET /restaurants/:restaurantId/failures?from=&to=.
type RestaurantFailuresQuery struct {
	RestaurantID int64  `validate:"required,gt=0"`
	From         string `validate:"required,datetime=2006-01-02"`
	To           string `validate:"required,datetime=2006-01-02"`
}

func ParseRestaurantFailuresQuery(r *http.Request, restaurantIDParam string) (RestaurantFailuresQuery, error) {
	id, err := strconv.ParseInt(restaurantIDParam, 10, 64)
	if err != nil {
		id = 0
	}

	q := RestaurantFailuresQuery{
		RestaurantID: id,
		From:         r.URL.Query().Get("from"),
		To:           r.URL.Query().Get("to"),
	}
	if err := validate.Struct(q); err != nil {
		return RestaurantFailuresQuery{}, apperror.FromValidation(err)
	}
	return q, nil
}
