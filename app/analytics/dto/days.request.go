package dto

import (
	"net/http"
	"strconv"

	"github.com/go-playground/validator/v10"

	apperror "analytics-service/lib/errors"
)

var validate = validator.New()

// RestaurantDaysQuery is the validated input for
// GET /restaurants/:restaurantId/days?from=&to=. Path/query params are
// validated inline via struct tags — same convention as request body DTOs,
// just no JSON body to decode.
type RestaurantDaysQuery struct {
	RestaurantID int64  `validate:"required,gt=0"`
	From         string `validate:"required,datetime=2006-01-02"`
	To           string `validate:"required,datetime=2006-01-02"`
}

// ParseRestaurantDaysQuery extracts restaurantId from the path and from/to
// from the query string, then validates the result. A malformed
// restaurantId (non-numeric) is folded into the same VALIDATION_ERROR path
// as a missing/malformed date — one uniform 400 shape for the caller.
func ParseRestaurantDaysQuery(r *http.Request, restaurantIDParam string) (RestaurantDaysQuery, error) {
	id, err := strconv.ParseInt(restaurantIDParam, 10, 64)
	if err != nil {
		id = 0 // fails validate's gt=0 below with a uniform message
	}

	q := RestaurantDaysQuery{
		RestaurantID: id,
		From:         r.URL.Query().Get("from"),
		To:           r.URL.Query().Get("to"),
	}
	if err := validate.Struct(q); err != nil {
		return RestaurantDaysQuery{}, apperror.FromValidation(err)
	}
	return q, nil
}
