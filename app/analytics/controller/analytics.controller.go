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

// GetRestaurantFailures handles GET /restaurants/{restaurantId}/failures?from=&to=.
func (c *Controller) GetRestaurantFailures(w http.ResponseWriter, r *http.Request) error {
	q, err := dto.ParseRestaurantFailuresQuery(r, chi.URLParam(r, "restaurantId"))
	if err != nil {
		return err
	}

	rows, err := c.service.GetRestaurantFailures(r.Context(), q.RestaurantID, q.From, q.To)
	if err != nil {
		return err
	}

	response.SendSuccess(w, http.StatusOK, dto.FailuresResponseFrom(rows))
	return nil
}

// GetRestaurantDeliveryAvg handles GET /restaurants/{restaurantId}/delivery-avg?from=&to=.
func (c *Controller) GetRestaurantDeliveryAvg(w http.ResponseWriter, r *http.Request) error {
	q, err := dto.ParseRestaurantDeliveryAvgQuery(r, chi.URLParam(r, "restaurantId"))
	if err != nil {
		return err
	}

	rows, err := c.service.GetRestaurantDeliveryAvg(r.Context(), q.RestaurantID, q.From, q.To)
	if err != nil {
		return err
	}

	response.SendSuccess(w, http.StatusOK, dto.DeliveryAvgResponseFrom(rows))
	return nil
}

// GetActiveRestaurants handles GET /restaurants/active?from=&to=.
func (c *Controller) GetActiveRestaurants(w http.ResponseWriter, r *http.Request) error {
	q, err := dto.ParseActiveRestaurantsQuery(r)
	if err != nil {
		return err
	}

	count, err := c.service.GetActiveRestaurants(r.Context(), q.From, q.To)
	if err != nil {
		return err
	}

	response.SendSuccess(w, http.StatusOK, dto.ActiveRestaurantsResponseFrom(count))
	return nil
}

// GetBranchDays handles GET /branches/{branchId}/days?from=&to=.
func (c *Controller) GetBranchDays(w http.ResponseWriter, r *http.Request) error {
	q, err := dto.ParseBranchDaysQuery(r, chi.URLParam(r, "branchId"))
	if err != nil {
		return err
	}

	rows, err := c.service.GetBranchDays(r.Context(), q.BranchID, q.From, q.To)
	if err != nil {
		return err
	}

	response.SendSuccess(w, http.StatusOK, dto.BranchDaysResponseFrom(rows))
	return nil
}

// GetProductDays handles GET /branches/{branchId}/products/{productId}/days?from=&to=.
func (c *Controller) GetProductDays(w http.ResponseWriter, r *http.Request) error {
	q, err := dto.ParseProductDaysQuery(r, chi.URLParam(r, "branchId"), chi.URLParam(r, "productId"))
	if err != nil {
		return err
	}

	rows, err := c.service.GetProductDays(r.Context(), q.BranchID, q.ProductID, q.From, q.To)
	if err != nil {
		return err
	}

	response.SendSuccess(w, http.StatusOK, dto.ProductDaysResponseFrom(rows))
	return nil
}

// GetPlatformDays handles GET /platform/days?from=&to=.
func (c *Controller) GetPlatformDays(w http.ResponseWriter, r *http.Request) error {
	q, err := dto.ParsePlatformDaysQuery(r)
	if err != nil {
		return err
	}

	rows, err := c.service.GetPlatformDays(r.Context(), q.From, q.To)
	if err != nil {
		return err
	}

	response.SendSuccess(w, http.StatusOK, dto.PlatformDaysResponseFrom(rows))
	return nil
}

// GetPlatformSummary handles GET /platform/summary?from=&to=.
func (c *Controller) GetPlatformSummary(w http.ResponseWriter, r *http.Request) error {
	q, err := dto.ParsePlatformSummaryQuery(r)
	if err != nil {
		return err
	}

	rows, err := c.service.GetPlatformSummary(r.Context(), q.From, q.To)
	if err != nil {
		return err
	}

	response.SendSuccess(w, http.StatusOK, dto.PlatformSummaryResponseFrom(rows))
	return nil
}
