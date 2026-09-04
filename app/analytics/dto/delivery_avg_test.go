package dto

import "testing"

func TestParseRestaurantDeliveryAvgQuery(t *testing.T) {
	cases := []struct {
		name         string
		restaurantID string
		query        string
		wantErr      bool
	}{
		{"valid", "10", "from=2026-03-01&to=2026-03-31", false},
		{"missing to", "10", "from=2026-03-01", true},
		{"malformed to date", "10", "from=2026-03-01&to=31-03-2026", true},
		{"non-numeric restaurantId", "abc", "from=2026-03-01&to=2026-03-31", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRestaurantDeliveryAvgQuery(mustRequest(t, tc.query), tc.restaurantID)
			if tc.wantErr && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
