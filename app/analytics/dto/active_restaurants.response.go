package dto

// ActiveRestaurantsResponse is the wire shape for the active-restaurants
// count over a range.
type ActiveRestaurantsResponse struct {
	Count int64 `json:"count"`
}

func ActiveRestaurantsResponseFrom(count int64) ActiveRestaurantsResponse {
	return ActiveRestaurantsResponse{Count: count}
}
