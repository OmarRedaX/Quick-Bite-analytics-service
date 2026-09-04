package dto

import "testing"

func TestParsePlatformSummaryQuery(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"valid", "from=2026-03-01&to=2026-03-31", false},
		{"missing to", "from=2026-03-01", true},
		{"malformed date", "from=not-a-date&to=2026-03-31", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParsePlatformSummaryQuery(mustRequest(t, tc.query))
			if tc.wantErr && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
