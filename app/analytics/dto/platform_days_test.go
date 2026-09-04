package dto

import "testing"

func TestParsePlatformDaysQuery(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"valid", "from=2026-03-01&to=2026-03-31", false},
		{"missing from", "to=2026-03-31", true},
		{"malformed date", "from=2026-03-01&to=03/31/2026", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParsePlatformDaysQuery(mustRequest(t, tc.query))
			if tc.wantErr && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
