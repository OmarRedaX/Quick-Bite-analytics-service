// Package service holds JUST the service struct and its methods — business
// logic composing repositories, no types/errors/enums declared here (those
// live in the parent app/analytics package). Go analogue of order-service's
// @injectable() OrderService class, minus the DI container: dependencies
// are passed explicitly to New() by lib/boot.
package service

import (
	"context"

	"analytics-service/app/analytics"
	"analytics-service/app/analytics/entity"
)

// restaurantDayRepo is the narrow slice of RestaurantDayRepo this service
// needs. Declared here, unexported, and satisfied implicitly by
// *repository.RestaurantDayRepo — this keeps mongo-driver types out of the
// service package entirely (CLAUDE.md: "ONLY place mongo-driver appears" is
// repository/) while still letting the service depend on a concrete-enough
// contract. This is a Go idiom with no direct TS equivalent — see
// docs/node-to-go-mapping.md, "when to break the mapping".
type restaurantDayRepo interface {
	ApplyOrderPlaced(ctx context.Context, in analytics.OnOrderPlacedInput) error
	FindByDateRange(ctx context.Context, restaurantID int64, from, to string) ([]entity.RestaurantDay, error)
}

type Service struct {
	restaurantDays restaurantDayRepo
}

func New(restaurantDays restaurantDayRepo) *Service {
	return &Service{restaurantDays: restaurantDays}
}

// OnOrderPlaced applies one order.placed event to the restaurant's daily
// aggregate. Idempotency is the caller's job (lib/coreevents dedupes by
// eventId before invoking this) — this method itself is safe to call more
// than once for different events; it must never be called twice for the
// *same* event.
func (s *Service) OnOrderPlaced(ctx context.Context, in analytics.OnOrderPlacedInput) error {
	return s.restaurantDays.ApplyOrderPlaced(ctx, in)
}

// GetRestaurantDays returns the day rows for restaurantID in [from, to]
// (both YYYY-MM-DD), with avgOrderMinor derived here — never stored.
func (s *Service) GetRestaurantDays(ctx context.Context, restaurantID int64, from, to string) ([]analytics.RestaurantDayRow, error) {
	if from > to {
		return nil, analytics.ErrInvalidDateRange
	}

	rows, err := s.restaurantDays.FindByDateRange(ctx, restaurantID, from, to)
	if err != nil {
		return nil, err
	}

	out := make([]analytics.RestaurantDayRow, 0, len(rows))
	for _, row := range rows {
		var avg int64
		if row.OrdersCount > 0 {
			avg = row.RevenueSumMinor / row.OrdersCount
		}
		out = append(out, analytics.RestaurantDayRow{
			Date:            row.Date,
			Currency:        row.Currency,
			OrdersCount:     row.OrdersCount,
			RevenueSumMinor: row.RevenueSumMinor,
			AvgOrderMinor:   avg,
		})
	}
	return out, nil
}
