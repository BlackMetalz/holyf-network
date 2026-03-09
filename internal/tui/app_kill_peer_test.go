package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/actions"
)

func TestParseBlockMinutesBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{name: "zero valid", raw: "0", want: 0, wantErr: false},
		{name: "one valid", raw: "1", want: 1, wantErr: false},
		{name: "max valid", raw: "1440", want: 1440, wantErr: false},
		{name: "negative invalid", raw: "-1", wantErr: true},
		{name: "above max invalid", raw: "1441", wantErr: true},
		{name: "non numeric invalid", raw: "abc", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseBlockMinutes(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for raw=%q, got nil", tc.raw)
				}
				if !strings.Contains(err.Error(), "0-1440") {
					t.Fatalf("error should mention allowed range, got: %q", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for raw=%q: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("minutes mismatch for raw=%q: got=%d want=%d", tc.raw, got, tc.want)
			}
		})
	}
}

func TestIsKillOnlyMinutes(t *testing.T) {
	t.Parallel()

	if !isKillOnlyMinutes(0) {
		t.Fatalf("0 minutes should be kill-only mode")
	}
	if isKillOnlyMinutes(1) {
		t.Fatalf("1 minute should not be kill-only mode")
	}
	if isKillOnlyMinutes(1440) {
		t.Fatalf("1440 minutes should not be kill-only mode")
	}
}

func TestBuildKillOnlyActionSummaryHasNoExpiryArtifact(t *testing.T) {
	t.Parallel()

	spec := actions.PeerBlockSpec{PeerIP: "203.0.113.10", LocalPort: 443}
	summary := buildKillOnlyActionSummary(spec, actions.KillConvergeReport{
		BeforeActiveCount: 3,
		AfterActiveCount:  1,
	})

	if !strings.Contains(summary, "Killed connections for 203.0.113.10:443") {
		t.Fatalf("kill-only summary should describe kill-only action, got: %q", summary)
	}
	if !strings.Contains(summary, "killed 2/3 flows") {
		t.Fatalf("kill-only summary should include kill ratio, got: %q", summary)
	}
	if strings.Contains(summary, "expires in") {
		t.Fatalf("kill-only summary must not include expiration artifact, got: %q", summary)
	}
}

func TestBuildBlockActionSummaryIncludesExpiryArtifact(t *testing.T) {
	t.Parallel()

	spec := actions.PeerBlockSpec{PeerIP: "203.0.113.10", LocalPort: 443}
	summary := buildBlockActionSummary(spec, 5*time.Minute, actions.KillConvergeReport{
		BeforeActiveCount: 4,
		AfterActiveCount:  1,
	})

	if !strings.Contains(summary, "Blocked 203.0.113.10:443") {
		t.Fatalf("timed-block summary should mention block action, got: %q", summary)
	}
	if !strings.Contains(summary, "expires in") {
		t.Fatalf("timed-block summary should include expiry text, got: %q", summary)
	}
}

func TestBuildKillOnlyActionSummaryAddsRemainingOnPartial(t *testing.T) {
	t.Parallel()

	spec := actions.PeerBlockSpec{PeerIP: "203.0.113.10", LocalPort: 443}
	summary := buildKillOnlyActionSummary(spec, actions.KillConvergeReport{
		BeforeActiveCount: 5,
		AfterActiveCount:  2,
		Converged:         false,
	})
	if !strings.Contains(summary, "killed 3/5 flows") {
		t.Fatalf("summary should include kill ratio, got: %q", summary)
	}
	if !strings.Contains(summary, "remaining 2 (storm/race)") {
		t.Fatalf("summary should include remaining hint for partial storm, got: %q", summary)
	}
}

func TestBuildKillOnlyActionSummaryIgnoresTimeWaitAsFailure(t *testing.T) {
	t.Parallel()

	spec := actions.PeerBlockSpec{PeerIP: "203.0.113.10", LocalPort: 443}
	summary := buildKillOnlyActionSummary(spec, actions.KillConvergeReport{
		BeforeActiveCount:   1,
		AfterActiveCount:    0,
		BeforeTimeWaitCount: 0,
		AfterTimeWaitCount:  7,
		Converged:           true,
	})
	if strings.Contains(summary, "remaining") {
		t.Fatalf("time_wait-only leftovers must not be treated as kill failure: %q", summary)
	}
	if !strings.Contains(summary, "killed 1/1 flows") {
		t.Fatalf("summary should still be full success, got: %q", summary)
	}
}

func TestSanitizeActionLogMessageBlockedRemovesNoisyParts(t *testing.T) {
	t.Parallel()

	raw := "Blocked 172.25.110.137:3306 | killed 0/0 flows | drop ok | expires in 10:00"
	got := sanitizeActionLogMessage(raw)
	want := "Blocked 172.25.110.137:3306 | drop ok"
	if got != want {
		t.Fatalf("sanitize mismatch: got=%q want=%q", got, want)
	}
}

func TestSanitizeActionLogMessageBlockedKeepsUsefulKillRatio(t *testing.T) {
	t.Parallel()

	raw := "Blocked 172.25.110.137:3306 | killed 3/5 flows | drop ok | expires in 10:00"
	got := sanitizeActionLogMessage(raw)
	want := "Blocked 172.25.110.137:3306 | killed 3/5 flows | drop ok"
	if got != want {
		t.Fatalf("sanitize mismatch: got=%q want=%q", got, want)
	}
}

func TestSanitizeActionLogMessageNonBlockedUnchanged(t *testing.T) {
	t.Parallel()

	raw := "Killed connections for 172.25.110.137:3306 | killed 0/0 flows | drop ok"
	got := sanitizeActionLogMessage(raw)
	if got != raw {
		t.Fatalf("non-blocked messages should stay unchanged: got=%q want=%q", got, raw)
	}
}
