package dto

import (
	"net/http"
	"strconv"

	apperror "analytics-service/lib/errors"
)

// RestaurantDeliveryAvgQuery is the validated input for
// GET /restaurants/:restaurantId/delivery-avg?from=&to=.
type RestaurantDeliveryAvgQuery struct {
	RestaurantID int64  `validate:"required,gt=0"`
	From         string `validate:"required,datetime=2006-01-02"`
	To           string `validate:"required,datetime=2006-01-02"`
}

func ParseRestaurantDeliveryAvgQuery(r *http.Request, restaurantIDParam string) (RestaurantDeliveryAvgQuery, error) {
	id, err := strconv.ParseInt(restaurantIDParam, 10, 64)
	if err != nil {
		id = 0
	}

	q := RestaurantDeliveryAvgQuery{
		RestaurantID: id,
		From:         r.URL.Query().Get("from"),
		To:           r.URL.Query().Get("to"),
	}
	if err := validate.Struct(q); err != nil {
		return RestaurantDeliveryAvgQuery{}, apperror.FromValidation(err)
	}
	return q, nil
}
