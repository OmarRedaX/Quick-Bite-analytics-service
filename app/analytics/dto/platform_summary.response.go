package dto

import "analytics-service/app/analytics"

// PlatformSummaryResponse is the wire shape for one currency's range
// totals. FailureRate is a fraction (0..1); every derived field is
// computed in the service layer.
type PlatformSummaryResponse struct {
	Currency                  string  `json:"currency"`
	OrdersCount               int64   `json:"ordersCount"`
	RevenueMinor              int64   `json:"revenueMinor"`
	AvgOrderMinor             int64   `json:"avgOrderMinor"`
	FailedCount               int64   `json:"failedCount"`
	FailureRate               float64 `json:"failureRate"`
	DeliveredCount            int64   `json:"deliveredCount"`
	AvgDeliveryMs             int64   `json:"avgDeliveryMs"`
	OnlinePaymentsCount       int64   `json:"onlinePaymentsCount"`
	OnlinePaymentsAmountMinor int64   `json:"onlinePaymentsAmountMinor"`
}

func PlatformSummaryResponseFrom(rows []analytics.PlatformSummaryRow) []PlatformSummaryResponse {
	out := make([]PlatformSummaryResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, PlatformSummaryResponse{
			Currency:                  row.Currency,
			OrdersCount:               row.OrdersCount,
			RevenueMinor:              row.RevenueSumMinor,
			AvgOrderMinor:             row.AvgOrderMinor,
			FailedCount:               row.FailedCount,
			FailureRate:               row.FailureRate,
			DeliveredCount:            row.DeliveredCount,
			AvgDeliveryMs:             row.AvgDeliveryMs,
			OnlinePaymentsCount:       row.OnlinePaymentsCount,
			OnlinePaymentsAmountMinor: row.OnlinePaymentsAmountSumMinor,
		})
	}
	return out
}
