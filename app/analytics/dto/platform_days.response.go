package dto

import "analytics-service/app/analytics"

// PlatformDayResponse is the wire shape for one platform-day row. A single
// date can produce more than one row — one per currency active that day.
type PlatformDayResponse struct {
	Date          string `json:"date"`
	Currency      string `json:"currency"`
	OrdersCount   int64  `json:"ordersCount"`
	RevenueMinor  int64  `json:"revenueMinor"`
	AvgOrderMinor int64  `json:"avgOrderMinor"`
}

func PlatformDaysResponseFrom(rows []analytics.PlatformDayRow) []PlatformDayResponse {
	out := make([]PlatformDayResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, PlatformDayResponse{
			Date:          row.Date,
			Currency:      row.Currency,
			OrdersCount:   row.OrdersCount,
			RevenueMinor:  row.RevenueSumMinor,
			AvgOrderMinor: row.AvgOrderMinor,
		})
	}
	return out
}
