package dto

import "testing"

func TestParseRestaurantFailuresQuery(t *testing.T) {
	cases := []struct {
		name         string
		restaurantID string
		query        string
		wantErr      bool
	}{
		{"valid", "10", "from=2026-03-01&to=2026-03-31", false},
		{"missing from", "10", "to=2026-03-31", true},
		{"malformed from date", "10", "from=20260301&to=2026-03-31", true},
		{"non-numeric restaurantId", "abc", "from=2026-03-01&to=2026-03-31", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRestaurantFailuresQuery(mustRequest(t, tc.query), tc.restaurantID)
			if tc.wantErr && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
