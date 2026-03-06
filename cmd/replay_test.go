package cmd

import (
	"strings"
	"testing"
	"time"
)

func TestReplayFileFlagHasShortF(t *testing.T) {
	t.Parallel()

	cmd := newReplayCmd()
	flag := cmd.Flags().Lookup("file")
	if flag == nil {
		t.Fatalf("expected replay command to have --file flag")
	}
	if flag.Shorthand != "f" {
		t.Fatalf("expected --file shorthand to be -f, got %q", flag.Shorthand)
	}
}

func TestReplayBeginParseValidation(t *testing.T) {
	t.Parallel()

	cmd := newReplayCmd()
	cmd.SetArgs([]string{"--begin", "tomorrow morning"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected invalid --begin to return error")
	}
	if !strings.Contains(err.Error(), "invalid --begin") {
		t.Fatalf("unexpected error for invalid --begin: %v", err)
	}
}

func TestReplayEndParseValidation(t *testing.T) {
	t.Parallel()

	cmd := newReplayCmd()
	cmd.SetArgs([]string{"--end", "not-a-time"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected invalid --end to return error")
	}
	if !strings.Contains(err.Error(), "invalid --end") {
		t.Fatalf("unexpected error for invalid --end: %v", err)
	}
}

func TestReplayBeginAfterEndValidation(t *testing.T) {
	t.Parallel()

	cmd := newReplayCmd()
	cmd.SetArgs([]string{"--begin", "2026-03-05 12:00", "--end", "2026-03-05 08:00"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected begin > end to return error")
	}
	if !strings.Contains(err.Error(), "--begin must be <= --end") {
		t.Fatalf("unexpected error for begin>end: %v", err)
	}
}

func TestResolveReplayBoundTimeClockOnlyWithoutFileUsesToday(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+7", 7*3600)
	now := time.Date(2026, 3, 6, 9, 30, 0, 0, loc)
	got, err := resolveReplayBoundTime("20:15", now, "")
	if err != nil {
		t.Fatalf("resolve replay bound time failed: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil parsed time")
	}
	want := time.Date(2026, 3, 6, 20, 15, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("clock-only no-file should use today: got=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestResolveReplayBoundTimeClockOnlyWithFileUsesFileDate(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+7", 7*3600)
	now := time.Date(2026, 3, 6, 9, 30, 0, 0, loc)
	got, err := resolveReplayBoundTime("20:15", now, "connections-20260304.jsonl")
	if err != nil {
		t.Fatalf("resolve replay bound time failed: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil parsed time")
	}
	want := time.Date(2026, 3, 4, 20, 15, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("clock-only with file should use file date: got=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestResolveReplayBoundTimeDateTimeKeepsExplicitDate(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+7", 7*3600)
	now := time.Date(2026, 3, 6, 9, 30, 0, 0, loc)
	got, err := resolveReplayBoundTime("2026-03-05 20:15", now, "connections-20260304.jsonl")
	if err != nil {
		t.Fatalf("resolve replay bound time failed: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil parsed time")
	}
	want := time.Date(2026, 3, 5, 20, 15, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("explicit datetime should keep input date: got=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestCompleteReplayDayWindowBeginOnly(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+7", 7*3600)
	begin := time.Date(2026, 3, 5, 20, 15, 0, 0, loc)

	gotBegin, gotEnd := completeReplayDayWindow(&begin, nil)
	if gotBegin == nil || gotEnd == nil {
		t.Fatalf("expected both bounds to be set")
	}
	wantEnd := time.Date(2026, 3, 5, 23, 59, 59, int(time.Second-time.Nanosecond), loc)
	if !gotBegin.Equal(begin) {
		t.Fatalf("begin should be unchanged: got=%s want=%s", gotBegin.Format(time.RFC3339), begin.Format(time.RFC3339))
	}
	if !gotEnd.Equal(wantEnd) {
		t.Fatalf("end should default to end-of-day: got=%s want=%s", gotEnd.Format(time.RFC3339Nano), wantEnd.Format(time.RFC3339Nano))
	}
}

func TestCompleteReplayDayWindowEndOnly(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+7", 7*3600)
	end := time.Date(2026, 3, 5, 23, 45, 0, 0, loc)

	gotBegin, gotEnd := completeReplayDayWindow(nil, &end)
	if gotBegin == nil || gotEnd == nil {
		t.Fatalf("expected both bounds to be set")
	}
	wantBegin := time.Date(2026, 3, 5, 0, 0, 0, 0, loc)
	if !gotBegin.Equal(wantBegin) {
		t.Fatalf("begin should default to start-of-day: got=%s want=%s", gotBegin.Format(time.RFC3339), wantBegin.Format(time.RFC3339))
	}
	if !gotEnd.Equal(end) {
		t.Fatalf("end should be unchanged: got=%s want=%s", gotEnd.Format(time.RFC3339), end.Format(time.RFC3339))
	}
}
