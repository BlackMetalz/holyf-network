package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
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

func TestReplayDataDirFlagHidden(t *testing.T) {
	t.Parallel()

	cmd := newReplayCmd()
	flag := cmd.Flags().Lookup("data-dir")
	if flag == nil {
		t.Fatalf("expected replay command to have --data-dir flag")
	}
	if !flag.Hidden {
		t.Fatalf("expected replay --data-dir to be hidden")
	}
}

func TestResolveReplayDataDirUsesActiveStateWhenImplicit(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "daemon.state")
	t.Setenv("HOLYF_NETWORK_DAEMON_STATE_FILE", stateFile)

	stateDir := filepath.Join(t.TempDir(), "state-data")
	state := daemonActiveState{
		PID:      os.Getpid(),
		DataDir:  stateDir,
		PIDFile:  filepath.Join(stateDir, "daemon.pid"),
		LogFile:  filepath.Join(stateDir, "daemon.log"),
		LockFile: filepath.Join(stateDir, ".daemon.lock"),
	}
	if err := writeActiveState(stateFile, state); err != nil {
		t.Fatalf("write active-state: %v", err)
	}

	got, err := resolveReplayDataDir("", false)
	if err != nil {
		t.Fatalf("resolveReplayDataDir: %v", err)
	}
	if got != stateDir {
		t.Fatalf("expected data-dir from active-state, got=%q want=%q", got, stateDir)
	}
}

func TestResolveReplayDataDirExplicitOverridesActiveState(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "daemon.state")
	t.Setenv("HOLYF_NETWORK_DAEMON_STATE_FILE", stateFile)

	stateDir := filepath.Join(t.TempDir(), "state-data")
	state := daemonActiveState{
		PID:      os.Getpid(),
		DataDir:  stateDir,
		PIDFile:  filepath.Join(stateDir, "daemon.pid"),
		LogFile:  filepath.Join(stateDir, "daemon.log"),
		LockFile: filepath.Join(stateDir, ".daemon.lock"),
	}
	if err := writeActiveState(stateFile, state); err != nil {
		t.Fatalf("write active-state: %v", err)
	}

	explicitDir := filepath.Join(t.TempDir(), "explicit-data")
	got, err := resolveReplayDataDir(explicitDir, true)
	if err != nil {
		t.Fatalf("resolveReplayDataDir: %v", err)
	}
	if got != explicitDir {
		t.Fatalf("expected explicit data-dir to win, got=%q want=%q", got, explicitDir)
	}
}

func TestResolveReplayDataDirFallsBackToDefault(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "missing-daemon.state")
	t.Setenv("HOLYF_NETWORK_DAEMON_STATE_FILE", stateFile)

	got, err := resolveReplayDataDir("", false)
	if err != nil {
		t.Fatalf("resolveReplayDataDir: %v", err)
	}
	want := history.ExpandPath(history.DefaultDataDir())
	if got != want {
		t.Fatalf("expected fallback default data-dir, got=%q want=%q", got, want)
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

func TestDefaultReplayWindowIfImplicitNoFileUsesToday(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+7", 7*3600)
	now := time.Date(2026, 3, 8, 14, 20, 0, 0, loc)
	gotBegin, gotEnd := defaultReplayWindowIfImplicit(nil, nil, "", now)
	if gotBegin == nil || gotEnd == nil {
		t.Fatalf("expected implicit replay to default to today window")
	}
	wantBegin := time.Date(2026, 3, 8, 0, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 3, 8, 23, 59, 59, int(time.Second-time.Nanosecond), loc)
	if !gotBegin.Equal(wantBegin) || !gotEnd.Equal(wantEnd) {
		t.Fatalf("unexpected default day window: got=%s..%s want=%s..%s",
			gotBegin.Format(time.RFC3339), gotEnd.Format(time.RFC3339Nano),
			wantBegin.Format(time.RFC3339), wantEnd.Format(time.RFC3339Nano),
		)
	}
}

func TestDefaultReplayWindowIfImplicitWithFileKeepsUnbounded(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+7", 7*3600)
	now := time.Date(2026, 3, 8, 14, 20, 0, 0, loc)
	gotBegin, gotEnd := defaultReplayWindowIfImplicit(nil, nil, "connections-20260308.jsonl", now)
	if gotBegin != nil || gotEnd != nil {
		t.Fatalf("expected file-scope replay without bounds to stay unbounded")
	}
}
