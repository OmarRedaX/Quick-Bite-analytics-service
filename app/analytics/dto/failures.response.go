package dto

import "analytics-service/app/analytics"

// FailureResponse is the wire shape for one day's failure counts.
// FailureRate is a fraction (0..1), computed in the service layer.
type FailureResponse struct {
	Date        string  `json:"date"`
	OrdersCount int64   `json:"ordersCount"`
	FailedCount int64   `json:"failedCount"`
	FailureRate float64 `json:"failureRate"`
}

func FailuresResponseFrom(rows []analytics.FailureRow) []FailureResponse {
	out := make([]FailureResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, FailureResponse{
			Date:        row.Date,
			OrdersCount: row.OrdersCount,
			FailedCount: row.FailedCount,
			FailureRate: row.FailureRate,
		})
	}
	return out
}
