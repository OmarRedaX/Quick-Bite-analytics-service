package dto

import "analytics-service/app/analytics"

// RestaurantDayResponse is the wire shape for one day row. Money in integer
// minor units, no timestamps to format here (date is already YYYY-MM-DD) —
// per CLAUDE.md §Response DTOs, controllers never return analytics.RestaurantDayRow
// (the internal type) directly.
type RestaurantDayResponse struct {
	Date          string `json:"date"`
	OrdersCount   int64  `json:"ordersCount"`
	RevenueMinor  int64  `json:"revenueMinor"`
	Currency      string `json:"currency"`
	AvgOrderMinor int64  `json:"avgOrderMinor"`
}

func RestaurantDayResponseFrom(row analytics.RestaurantDayRow) RestaurantDayResponse {
	return RestaurantDayResponse{
		Date:          row.Date,
		OrdersCount:   row.OrdersCount,
		RevenueMinor:  row.RevenueSumMinor,
		Currency:      row.Currency,
		AvgOrderMinor: row.AvgOrderMinor,
	}
}

func RestaurantDaysResponseFrom(rows []analytics.RestaurantDayRow) []RestaurantDayResponse {
	out := make([]RestaurantDayResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, RestaurantDayResponseFrom(row))
	}
	return out
}
