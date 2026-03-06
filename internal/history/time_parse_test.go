package history

import (
	"testing"
	"time"
)

func TestParseReplayTime(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+7", 7*3600)
	now := time.Date(2026, 3, 6, 9, 30, 0, 0, loc)

	tests := []struct {
		name     string
		raw      string
		want     time.Time
		wantFail bool
	}{
		{
			name: "datetime",
			raw:  "2026-03-05 20:00:30",
			want: time.Date(2026, 3, 5, 20, 0, 30, 0, loc),
		},
		{
			name: "clock-only uses today",
			raw:  "20:15",
			want: time.Date(2026, 3, 6, 20, 15, 0, 0, loc),
		},
		{
			name: "yesterday clock",
			raw:  "yesterday 20:10",
			want: time.Date(2026, 3, 5, 20, 10, 0, 0, loc),
		},
		{
			name: "rfc3339",
			raw:  "2026-03-05T20:10:20+07:00",
			want: time.Date(2026, 3, 5, 20, 10, 20, 0, loc),
		},
		{
			name:     "invalid",
			raw:      "tomorrow morning",
			wantFail: true,
		},
		{
			name:     "empty",
			raw:      "   ",
			wantFail: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseReplayTime(tc.raw, now)
			if tc.wantFail {
				if err == nil {
					t.Fatalf("expected parse failure for %q", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse failed for %q: %v", tc.raw, err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("parsed time mismatch for %q: got=%s want=%s", tc.raw, got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}
