package config

import "testing"

func TestParseAlertProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in     string
		want   AlertProfile
		wantOK bool
	}{
		{in: "web", want: AlertProfileWeb, wantOK: true},
		{in: "WEB", want: AlertProfileWeb, wantOK: true},
		{in: "db", want: AlertProfileDB, wantOK: true},
		{in: "cache", want: AlertProfileCache, wantOK: true},
		{in: "nope", want: "", wantOK: false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseAlertProfile(tc.in)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("ParseAlertProfile(%q): got=(%q,%v) want=(%q,%v)", tc.in, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestNextAlertProfileCycles(t *testing.T) {
	t.Parallel()

	if got := NextAlertProfile(AlertProfileWeb); got != AlertProfileDB {
		t.Fatalf("next(web) mismatch: got=%q want=%q", got, AlertProfileDB)
	}
	if got := NextAlertProfile(AlertProfileDB); got != AlertProfileCache {
		t.Fatalf("next(db) mismatch: got=%q want=%q", got, AlertProfileCache)
	}
	if got := NextAlertProfile(AlertProfileCache); got != AlertProfileWeb {
		t.Fatalf("next(cache) mismatch: got=%q want=%q", got, AlertProfileWeb)
	}
}

func TestAlertProfileSpecForFallbacksToDefault(t *testing.T) {
	t.Parallel()

	got := AlertProfileSpecFor(AlertProfile("unknown"))
	if got.Name != DefaultAlertProfile() {
		t.Fatalf("fallback name mismatch: got=%q want=%q", got.Name, DefaultAlertProfile())
	}
	if got.Thresholds.ConntrackPercent.Warn <= 0 || got.Thresholds.ConntrackPercent.Crit <= 0 {
		t.Fatalf("expected positive conntrack thresholds, got=%+v", got.Thresholds.ConntrackPercent)
	}
}
