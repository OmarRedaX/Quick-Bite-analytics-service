package dto

import "analytics-service/app/analytics"

// BranchDayResponse is the wire shape for one branch-day row. Same shape as
// RestaurantDayResponse.
type BranchDayResponse struct {
	Date          string `json:"date"`
	OrdersCount   int64  `json:"ordersCount"`
	RevenueMinor  int64  `json:"revenueMinor"`
	Currency      string `json:"currency"`
	AvgOrderMinor int64  `json:"avgOrderMinor"`
}

func BranchDaysResponseFrom(rows []analytics.BranchDayRow) []BranchDayResponse {
	out := make([]BranchDayResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, BranchDayResponse{
			Date:          row.Date,
			OrdersCount:   row.OrdersCount,
			RevenueMinor:  row.RevenueSumMinor,
			Currency:      row.Currency,
			AvgOrderMinor: row.AvgOrderMinor,
		})
	}
	return out
}
