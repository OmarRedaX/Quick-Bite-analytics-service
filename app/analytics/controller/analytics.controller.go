// Package controller: validate -> call service -> DTO -> respond. No
// business logic. Methods have the errors.HandlerFunc signature
// (w, r) error — see routes.go for how they're wrapped.
package controller

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"analytics-service/app/analytics/dto"
	"analytics-service/app/analytics/service"
	response "analytics-service/lib/http"
)

type Controller struct {
	service *service.Service
}

func New(service *service.Service) *Controller {
	return &Controller{service: service}
}

// GetRestaurantDays handles GET /restaurants/{restaurantId}/days?from=&to=.
func (c *Controller) GetRestaurantDays(w http.ResponseWriter, r *http.Request) error {
	q, err := dto.ParseRestaurantDaysQuery(r, chi.URLParam(r, "restaurantId"))
	if err != nil {
		return err
	}

	rows, err := c.service.GetRestaurantDays(r.Context(), q.RestaurantID, q.From, q.To)
	if err != nil {
		return err
	}

	response.SendSuccess(w, http.StatusOK, dto.RestaurantDaysResponseFrom(rows))
	return nil
}
