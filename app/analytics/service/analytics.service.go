// Package service holds JUST the service struct and its methods — business
// logic composing repositories, no types/errors/enums declared here (those
// live in the parent app/analytics package). Go analogue of order-service's
// @injectable() OrderService class, minus the DI container: dependencies
// are passed explicitly to New() by lib/boot.
package service

import (
	"context"
	"time"

	"analytics-service/app/analytics"
	"analytics-service/app/analytics/entity"
	"analytics-service/lib/logger"
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
	ApplyOrderDelivered(ctx context.Context, restaurantID int64, date string, deliveryMs int64) error
	ApplyOrderRejected(ctx context.Context, restaurantID int64, date string) error
	CountActiveInRange(ctx context.Context, from, to string) (int64, error)
	FindByDateRange(ctx context.Context, restaurantID int64, from, to string) ([]entity.RestaurantDay, error)
}

type branchDayRepo interface {
	ApplyOrderPlaced(ctx context.Context, branchID int64, date, currency string, totalMinor int64) error
	ApplyOrderDelivered(ctx context.Context, branchID int64, date string, deliveryMs int64) error
	ApplyOrderRejected(ctx context.Context, branchID int64, date string) error
	FindByDateRange(ctx context.Context, branchID int64, from, to string) ([]entity.BranchDay, error)
}

type productDayRepo interface {
	ApplyOrderPlacedItems(ctx context.Context, branchID int64, date, currency string, items []analytics.OrderPlacedItem) error
	FindByDateRange(ctx context.Context, branchID, productID int64, from, to string) ([]entity.ProductDay, error)
}

type platformDayRepo interface {
	ApplyOrderPlaced(ctx context.Context, date, currency string, totalMinor int64) error
	ApplyOrderDelivered(ctx context.Context, date, currency string, deliveryMs int64) error
	ApplyOrderRejected(ctx context.Context, date, currency string) error
	ApplyPaymentCompleted(ctx context.Context, date, currency string, amountMinor int64) error
	FindByDateRange(ctx context.Context, from, to string) ([]entity.PlatformDay, error)
	SummaryByCurrency(ctx context.Context, from, to string) ([]entity.PlatformDayCurrencyTotals, error)
}

// orderContextRepo is the "have I seen order.placed for this order?"
// lookup order.delivered/order.rejected need — see entity.OrderContext.
type orderContextRepo interface {
	Save(ctx context.Context, orderID, currency string, placedAt time.Time) error
	Find(ctx context.Context, orderID string) (entity.OrderContext, bool, error)
}

type Service struct {
	restaurantDays restaurantDayRepo
	branchDays     branchDayRepo
	productDays    productDayRepo
	platformDays   platformDayRepo
	orderContexts  orderContextRepo
}

func New(restaurantDays restaurantDayRepo, branchDays branchDayRepo, productDays productDayRepo, platformDays platformDayRepo, orderContexts orderContextRepo) *Service {
	return &Service{
		restaurantDays: restaurantDays,
		branchDays:     branchDays,
		productDays:    productDays,
		platformDays:   platformDays,
		orderContexts:  orderContexts,
	}
}

// OnOrderPlaced fans one order.placed event out to every day-grained
// aggregate it feeds, plus the OrderContext lookup order.delivered/
// order.rejected read back later. Idempotency is the caller's job
// (lib/coreevents dedupes by eventId before invoking this).
//
// These writes span five collections with no cross-collection transaction
// (this service's dev Mongo is a standalone instance, not a replica set, so
// multi-document ACID transactions aren't available) — a failure partway
// through and a later DLQ replay can double-count whichever $inc already
// landed. Every write here is independently idempotent-per-call by design
// (CLAUDE.md §9); true cross-collection atomicity would need a replica-set
// Mongo, out of scope for this slice.
func (s *Service) OnOrderPlaced(ctx context.Context, in analytics.OnOrderPlacedInput) error {
	date := in.PlacedAt.UTC().Format("2006-01-02")

	if err := s.restaurantDays.ApplyOrderPlaced(ctx, in); err != nil {
		return err
	}
	if err := s.branchDays.ApplyOrderPlaced(ctx, in.BranchID, date, in.Currency, in.TotalMinor); err != nil {
		return err
	}
	if err := s.productDays.ApplyOrderPlacedItems(ctx, in.BranchID, date, in.Currency, in.Items); err != nil {
		return err
	}
	if err := s.platformDays.ApplyOrderPlaced(ctx, date, in.Currency, in.TotalMinor); err != nil {
		return err
	}
	return s.orderContexts.Save(ctx, in.OrderID, in.Currency, in.PlacedAt)
}

// OnOrderDelivered applies one order.delivered event's delivery duration to
// the restaurant/branch/platform day the order was *placed* on — looked up
// via OrderContext, since neither the delivery duration nor that date is on
// the event's own payload. A lookup miss (order.delivered arrived before
// this service ever processed order.placed for the order, or the
// OrderContext row aged past its TTL) is logged and treated as a no-op,
// never an error — see entity.OrderContext's doc comment.
func (s *Service) OnOrderDelivered(ctx context.Context, in analytics.OnOrderDeliveredInput) error {
	orderCtx, found, err := s.orderContexts.Find(ctx, in.OrderID)
	if err != nil {
		return err
	}
	if !found {
		logger.FromContext(ctx).Warn("order.delivered: no order_context row, skipping delivery-duration aggregation",
			"orderId", in.OrderID)
		return nil
	}

	deliveryMs := in.DeliveredAt.Sub(orderCtx.PlacedAt).Milliseconds()
	if deliveryMs < 0 {
		deliveryMs = 0
	}
	date := orderCtx.PlacedAt.UTC().Format("2006-01-02")

	if err := s.restaurantDays.ApplyOrderDelivered(ctx, in.RestaurantID, date, deliveryMs); err != nil {
		return err
	}
	if err := s.branchDays.ApplyOrderDelivered(ctx, in.BranchID, date, deliveryMs); err != nil {
		return err
	}
	return s.platformDays.ApplyOrderDelivered(ctx, date, in.Currency, deliveryMs)
}

// OnOrderRejected applies one order.rejected event to the restaurant/branch
// failed-order counters (bucketed by the rejection's own OccurredAt — see
// analytics.OnOrderRejectedInput). The platform-wide counter additionally
// needs a currency, which this event's payload doesn't carry — that step is
// best-effort via OrderContext and is skipped (logged, not errored) on a
// miss, independent of the restaurant/branch updates above.
func (s *Service) OnOrderRejected(ctx context.Context, in analytics.OnOrderRejectedInput) error {
	date := in.OccurredAt.UTC().Format("2006-01-02")

	if err := s.restaurantDays.ApplyOrderRejected(ctx, in.RestaurantID, date); err != nil {
		return err
	}
	if err := s.branchDays.ApplyOrderRejected(ctx, in.BranchID, date); err != nil {
		return err
	}

	orderCtx, found, err := s.orderContexts.Find(ctx, in.OrderID)
	if err != nil {
		return err
	}
	if !found {
		logger.FromContext(ctx).Warn("order.rejected: no order_context row, skipping platform-day aggregation",
			"orderId", in.OrderID)
		return nil
	}
	return s.platformDays.ApplyOrderRejected(ctx, date, orderCtx.Currency)
}

// OnPaymentCompleted applies one payment.completed event to the
// platform-wide online-payment counters only — this is deliberately scoped
// narrower than order.placed/delivered/rejected. Revenue is already counted
// once by order.placed regardless of payment method (COD or online); this
// event tracks a distinct KPI — online gateway capture volume, for
// reconciliation — not a second count of the same orders. See plan.md.
func (s *Service) OnPaymentCompleted(ctx context.Context, in analytics.OnPaymentCompletedInput) error {
	date := in.CompletedAt.UTC().Format("2006-01-02")
	return s.platformDays.ApplyPaymentCompleted(ctx, date, in.Currency, in.AmountMinor)
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

// GetBranchDays mirrors GetRestaurantDays for the branch-scoped endpoint.
func (s *Service) GetBranchDays(ctx context.Context, branchID int64, from, to string) ([]analytics.BranchDayRow, error) {
	if from > to {
		return nil, analytics.ErrInvalidDateRange
	}

	rows, err := s.branchDays.FindByDateRange(ctx, branchID, from, to)
	if err != nil {
		return nil, err
	}

	out := make([]analytics.BranchDayRow, 0, len(rows))
	for _, row := range rows {
		var avg int64
		if row.OrdersCount > 0 {
			avg = row.RevenueSumMinor / row.OrdersCount
		}
		out = append(out, analytics.BranchDayRow{
			Date:            row.Date,
			Currency:        row.Currency,
			OrdersCount:     row.OrdersCount,
			RevenueSumMinor: row.RevenueSumMinor,
			AvgOrderMinor:   avg,
		})
	}
	return out, nil
}

// GetProductDays returns the day rows for one (branchID, productID) in
// [from, to], with avgUnitPriceMinor derived here — never stored.
func (s *Service) GetProductDays(ctx context.Context, branchID, productID int64, from, to string) ([]analytics.ProductDayRow, error) {
	if from > to {
		return nil, analytics.ErrInvalidDateRange
	}

	rows, err := s.productDays.FindByDateRange(ctx, branchID, productID, from, to)
	if err != nil {
		return nil, err
	}

	out := make([]analytics.ProductDayRow, 0, len(rows))
	for _, row := range rows {
		var avg int64
		if row.QuantitySum > 0 {
			avg = row.RevenueSumMinor / row.QuantitySum
		}
		out = append(out, analytics.ProductDayRow{
			Date:              row.Date,
			Currency:          row.Currency,
			QuantitySum:       row.QuantitySum,
			RevenueSumMinor:   row.RevenueSumMinor,
			AvgUnitPriceMinor: avg,
		})
	}
	return out, nil
}

// GetRestaurantFailures returns the day rows for restaurantID in [from,
// to], with failureRate derived here (FailedCount/OrdersCount) — never
// stored.
func (s *Service) GetRestaurantFailures(ctx context.Context, restaurantID int64, from, to string) ([]analytics.FailureRow, error) {
	if from > to {
		return nil, analytics.ErrInvalidDateRange
	}

	rows, err := s.restaurantDays.FindByDateRange(ctx, restaurantID, from, to)
	if err != nil {
		return nil, err
	}
	return failureRowsFrom(rows), nil
}

// GetRestaurantDeliveryAvg returns the day rows for restaurantID in [from,
// to], with avgDeliveryMs derived here — never stored.
func (s *Service) GetRestaurantDeliveryAvg(ctx context.Context, restaurantID int64, from, to string) ([]analytics.DeliveryAvgRow, error) {
	if from > to {
		return nil, analytics.ErrInvalidDateRange
	}

	rows, err := s.restaurantDays.FindByDateRange(ctx, restaurantID, from, to)
	if err != nil {
		return nil, err
	}
	return deliveryAvgRowsFrom(rows), nil
}

// GetActiveRestaurants counts distinct restaurants with at least one order
// in [from, to].
func (s *Service) GetActiveRestaurants(ctx context.Context, from, to string) (int64, error) {
	if from > to {
		return 0, analytics.ErrInvalidDateRange
	}
	return s.restaurantDays.CountActiveInRange(ctx, from, to)
}

// GetPlatformDays returns platform-wide day rows in [from, to]. A single
// date can produce more than one row — agg_platform_day is keyed by (date,
// currency).
func (s *Service) GetPlatformDays(ctx context.Context, from, to string) ([]analytics.PlatformDayRow, error) {
	if from > to {
		return nil, analytics.ErrInvalidDateRange
	}

	rows, err := s.platformDays.FindByDateRange(ctx, from, to)
	if err != nil {
		return nil, err
	}

	out := make([]analytics.PlatformDayRow, 0, len(rows))
	for _, row := range rows {
		var avg int64
		if row.OrdersCount > 0 {
			avg = row.RevenueSumMinor / row.OrdersCount
		}
		out = append(out, analytics.PlatformDayRow{
			Date:            row.Date,
			Currency:        row.Currency,
			OrdersCount:     row.OrdersCount,
			RevenueSumMinor: row.RevenueSumMinor,
			AvgOrderMinor:   avg,
		})
	}
	return out, nil
}

// GetPlatformSummary totals every platform-wide counter across [from, to],
// one row per currency — every derived field computed here from a single
// $group aggregation's sums/counts.
func (s *Service) GetPlatformSummary(ctx context.Context, from, to string) ([]analytics.PlatformSummaryRow, error) {
	if from > to {
		return nil, analytics.ErrInvalidDateRange
	}

	totals, err := s.platformDays.SummaryByCurrency(ctx, from, to)
	if err != nil {
		return nil, err
	}

	out := make([]analytics.PlatformSummaryRow, 0, len(totals))
	for _, t := range totals {
		var avgOrder int64
		if t.OrdersCount > 0 {
			avgOrder = t.RevenueSumMinor / t.OrdersCount
		}
		var failureRate float64
		if t.OrdersCount > 0 {
			failureRate = float64(t.FailedCount) / float64(t.OrdersCount)
		}
		var avgDelivery int64
		if t.DeliveryMsCount > 0 {
			avgDelivery = t.DeliveryMsSum / t.DeliveryMsCount
		}
		out = append(out, analytics.PlatformSummaryRow{
			Currency:                     t.Currency,
			OrdersCount:                  t.OrdersCount,
			RevenueSumMinor:              t.RevenueSumMinor,
			AvgOrderMinor:                avgOrder,
			FailedCount:                  t.FailedCount,
			FailureRate:                  failureRate,
			DeliveredCount:               t.DeliveryMsCount,
			AvgDeliveryMs:                avgDelivery,
			OnlinePaymentsCount:          t.OnlinePaymentsCount,
			OnlinePaymentsAmountSumMinor: t.OnlinePaymentsAmountSumMinor,
		})
	}
	return out, nil
}

func failureRowsFrom(rows []entity.RestaurantDay) []analytics.FailureRow {
	out := make([]analytics.FailureRow, 0, len(rows))
	for _, row := range rows {
		var rate float64
		if row.OrdersCount > 0 {
			rate = float64(row.FailedCount) / float64(row.OrdersCount)
		}
		out = append(out, analytics.FailureRow{
			Date:        row.Date,
			OrdersCount: row.OrdersCount,
			FailedCount: row.FailedCount,
			FailureRate: rate,
		})
	}
	return out
}

func deliveryAvgRowsFrom(rows []entity.RestaurantDay) []analytics.DeliveryAvgRow {
	out := make([]analytics.DeliveryAvgRow, 0, len(rows))
	for _, row := range rows {
		var avg int64
		if row.DeliveryMsCount > 0 {
			avg = row.DeliveryMsSum / row.DeliveryMsCount
		}
		out = append(out, analytics.DeliveryAvgRow{
			Date:           row.Date,
			DeliveredCount: row.DeliveryMsCount,
			AvgDeliveryMs:  avg,
		})
	}
	return out
}
