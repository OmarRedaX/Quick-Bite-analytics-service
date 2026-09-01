package dto

import "analytics-service/app/analytics"

// DeliveryAvgResponse is the wire shape for one day's delivery-duration
// rollup. AvgDeliveryMs is computed in the service layer.
type DeliveryAvgResponse struct {
	Date           string `json:"date"`
	DeliveredCount int64  `json:"deliveredCount"`
	AvgDeliveryMs  int64  `json:"avgDeliveryMs"`
}

func DeliveryAvgResponseFrom(rows []analytics.DeliveryAvgRow) []DeliveryAvgResponse {
	out := make([]DeliveryAvgResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, DeliveryAvgResponse{
			Date:           row.Date,
			DeliveredCount: row.DeliveredCount,
			AvgDeliveryMs:  row.AvgDeliveryMs,
		})
	}
	return out
}
