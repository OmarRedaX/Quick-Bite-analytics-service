package dto

import "analytics-service/app/analytics"

// ProductDayResponse is the wire shape for one product-day row.
// AvgUnitPriceMinor is derived in the service layer.
type ProductDayResponse struct {
	Date              string `json:"date"`
	QuantitySum       int64  `json:"quantitySum"`
	RevenueMinor      int64  `json:"revenueMinor"`
	Currency          string `json:"currency"`
	AvgUnitPriceMinor int64  `json:"avgUnitPriceMinor"`
}

func ProductDaysResponseFrom(rows []analytics.ProductDayRow) []ProductDayResponse {
	out := make([]ProductDayResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, ProductDayResponse{
			Date:              row.Date,
			QuantitySum:       row.QuantitySum,
			RevenueMinor:      row.RevenueSumMinor,
			Currency:          row.Currency,
			AvgUnitPriceMinor: row.AvgUnitPriceMinor,
		})
	}
	return out
}
