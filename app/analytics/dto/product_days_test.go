package dto

import "testing"

func TestParseProductDaysQuery(t *testing.T) {
	cases := []struct {
		name      string
		branchID  string
		productID string
		query     string
		wantErr   bool
	}{
		{"valid", "20", "5", "from=2026-03-01&to=2026-03-31", false},
		{"non-numeric branchId", "abc", "5", "from=2026-03-01&to=2026-03-31", true},
		{"non-numeric productId", "20", "abc", "from=2026-03-01&to=2026-03-31", true},
		{"zero productId", "20", "0", "from=2026-03-01&to=2026-03-31", true},
		{"missing to", "20", "5", "from=2026-03-01", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseProductDaysQuery(mustRequest(t, tc.query), tc.branchID, tc.productID)
			if tc.wantErr && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
