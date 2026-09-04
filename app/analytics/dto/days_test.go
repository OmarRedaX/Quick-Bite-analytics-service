package dto

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Colocated unit tests for every *.request.go query parser in this
// package. Table-driven across the shared validation boundary values
// (missing from/to, malformed date, non-numeric path id) — every parser
// funnels through the same package-level `validate` instance and
// apperror.FromValidation, so one table per parser is enough; this file
// (days_test.go, named after days.request.go where `validate` itself is
// declared) covers RestaurantDaysQuery, and the remaining request files
// each get their own smaller table for the fields unique to them.

func mustRequest(t *testing.T, rawQuery string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
}

func TestParseRestaurantDaysQuery(t *testing.T) {
	cases := []struct {
		name         string
		restaurantID string
		query        string
		wantErr      bool
	}{
		{"valid", "10", "from=2026-03-01&to=2026-03-31", false},
		{"missing from", "10", "to=2026-03-31", true},
		{"missing to", "10", "from=2026-03-01", true},
		{"malformed from date", "10", "from=03-01-2026&to=2026-03-31", true},
		{"malformed to date", "10", "from=2026-03-01&to=not-a-date", true},
		{"non-numeric restaurantId", "abc", "from=2026-03-01&to=2026-03-31", true},
		{"zero restaurantId", "0", "from=2026-03-01&to=2026-03-31", true},
		{"negative restaurantId", "-5", "from=2026-03-01&to=2026-03-31", true},
		{"empty restaurantId", "", "from=2026-03-01&to=2026-03-31", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRestaurantDaysQuery(mustRequest(t, tc.query), tc.restaurantID)
			if tc.wantErr && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
