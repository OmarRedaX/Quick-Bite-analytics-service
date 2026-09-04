package dto

import "testing"

func TestParseBranchDaysQuery(t *testing.T) {
	cases := []struct {
		name     string
		branchID string
		query    string
		wantErr  bool
	}{
		{"valid", "20", "from=2026-03-01&to=2026-03-31", false},
		{"missing from", "20", "to=2026-03-31", true},
		{"malformed to date", "20", "from=2026-03-01&to=2026/03/31", true},
		{"non-numeric branchId", "xyz", "from=2026-03-01&to=2026-03-31", true},
		{"zero branchId", "0", "from=2026-03-01&to=2026-03-31", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseBranchDaysQuery(mustRequest(t, tc.query), tc.branchID)
			if tc.wantErr && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
