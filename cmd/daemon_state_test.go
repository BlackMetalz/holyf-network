package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultDaemonLogPathByMode(t *testing.T) {
	dataDir := "/tmp/holyf/snapshots"
	if got := defaultDaemonLogPath("linux", 0, dataDir); got != "/var/log/holyf-network/daemon.log" {
		t.Fatalf("linux root log path mismatch: got=%q", got)
	}
	if got := defaultDaemonLogPath("linux", 1000, dataDir); got != filepath.Join(dataDir, "daemon.log") {
		t.Fatalf("linux non-root log path mismatch: got=%q", got)
	}
}

func TestWriteReadActiveStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "daemon.state")
	want := daemonActiveState{
		PID:       os.Getpid(),
		DataDir:   filepath.Join(dir, "snapshots"),
		PIDFile:   filepath.Join(dir, "daemon.pid"),
		LogFile:   filepath.Join(dir, "daemon.log"),
		LockFile:  filepath.Join(dir, ".daemon.lock"),
		Retention: 168,
		StartedAt: time.Now().Local().Truncate(time.Second),
	}

	if err := writeActiveState(stateFile, want); err != nil {
		t.Fatalf("write active-state: %v", err)
	}
	got, err := readActiveState(stateFile)
	if err != nil {
		t.Fatalf("read active-state: %v", err)
	}
	if got.PID != want.PID || got.DataDir != want.DataDir || got.PIDFile != want.PIDFile || got.LogFile != want.LogFile || got.LockFile != want.LockFile || got.Retention != want.Retention {
		t.Fatalf("active-state mismatch: got=%+v want=%+v", got, want)
	}
}

func TestDaemonStatusUsesActiveStateWhenNoExplicitFlags(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "daemon.state")
	t.Setenv("HOLYF_NETWORK_DAEMON_STATE_FILE", stateFile)

	state := daemonActiveState{
		PID:      os.Getpid(),
		DataDir:  filepath.Join(dir, "custom-data"),
		PIDFile:  filepath.Join(dir, "daemon.pid"),
		LogFile:  filepath.Join(dir, "daemon.log"),
		LockFile: filepath.Join(dir, ".daemon.lock"),
	}
	if err := writeActiveState(stateFile, state); err != nil {
		t.Fatalf("write active-state: %v", err)
	}

	cmd := newDaemonStatusCmd()
	out, err := captureCommandStdout(cmd)
	if err != nil {
		t.Fatalf("daemon status execute: %v", err)
	}
	if !strings.Contains(out, "daemon status: running") {
		t.Fatalf("expected running status, got: %q", out)
	}
	if !strings.Contains(out, state.DataDir) {
		t.Fatalf("expected status to include state data-dir, got: %q", out)
	}
}

func TestDaemonStatusExplicitFlagsDoNotUseActiveState(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "daemon.state")
	t.Setenv("HOLYF_NETWORK_DAEMON_STATE_FILE", stateFile)

	state := daemonActiveState{
		PID:      os.Getpid(),
		DataDir:  filepath.Join(dir, "state-data"),
		PIDFile:  filepath.Join(dir, "state.pid"),
		LogFile:  filepath.Join(dir, "state.log"),
		LockFile: filepath.Join(dir, "state.lock"),
	}
	if err := writeActiveState(stateFile, state); err != nil {
		t.Fatalf("write active-state: %v", err)
	}

	explicitDir := filepath.Join(dir, "explicit-data")
	cmd := newDaemonStatusCmd()
	cmd.SetArgs([]string{"--data-dir", explicitDir})
	out, err := captureCommandStdout(cmd)
	if err != nil {
		t.Fatalf("daemon status execute: %v", err)
	}
	if strings.Contains(out, state.DataDir) {
		t.Fatalf("status should not use active-state data-dir when explicit flag is set: %q", out)
	}
}

func TestDaemonStopStaleActiveStateCleansStateAndPidFile(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "daemon.state")
	pidFile := filepath.Join(dir, "daemon.pid")
	t.Setenv("HOLYF_NETWORK_DAEMON_STATE_FILE", stateFile)

	if err := os.WriteFile(pidFile, []byte("999999\n"), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	state := daemonActiveState{
		PID:      999999,
		DataDir:  filepath.Join(dir, "state-data"),
		PIDFile:  pidFile,
		LogFile:  filepath.Join(dir, "state.log"),
		LockFile: filepath.Join(dir, "state.lock"),
	}
	if err := writeActiveState(stateFile, state); err != nil {
		t.Fatalf("write active-state: %v", err)
	}

	cmd := newDaemonStopCmd()
	out, err := captureCommandStdout(cmd)
	if err != nil {
		t.Fatalf("daemon stop execute: %v", err)
	}
	if !strings.Contains(out, "stale active-state cleaned") {
		t.Fatalf("expected stale cleanup output, got: %q", out)
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("state file should be removed after stale cleanup")
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatalf("pid file should be removed after stale cleanup")
	}
}

func TestDaemonStopExplicitFlagsDoNotCleanupActiveState(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "daemon.state")
	t.Setenv("HOLYF_NETWORK_DAEMON_STATE_FILE", stateFile)

	state := daemonActiveState{
		PID:      999999,
		DataDir:  filepath.Join(dir, "state-data"),
		PIDFile:  filepath.Join(dir, "state.pid"),
		LogFile:  filepath.Join(dir, "state.log"),
		LockFile: filepath.Join(dir, "state.lock"),
	}
	if err := writeActiveState(stateFile, state); err != nil {
		t.Fatalf("write active-state: %v", err)
	}

	explicitDir := filepath.Join(dir, "explicit-data")
	cmd := newDaemonStopCmd()
	cmd.SetArgs([]string{"--data-dir", explicitDir})
	_, err := captureCommandStdout(cmd)
	if err != nil {
		t.Fatalf("daemon stop execute: %v", err)
	}
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("active-state file should remain for explicit stop path, err=%v", err)
	}
}

func TestDaemonPruneExplicitTargetUsesFlags(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "daemon.state")
	t.Setenv("HOLYF_NETWORK_DAEMON_STATE_FILE", stateFile)

	stateDir := filepath.Join(dir, "state-data")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	state := daemonActiveState{
		PID:       os.Getpid(),
		DataDir:   stateDir,
		PIDFile:   filepath.Join(stateDir, "daemon.pid"),
		LogFile:   filepath.Join(stateDir, "daemon.log"),
		LockFile:  filepath.Join(stateDir, ".daemon.lock"),
		Retention: 72,
	}
	if err := writeActiveState(stateFile, state); err != nil {
		t.Fatalf("write active-state: %v", err)
	}

	explicitDir := filepath.Join(dir, "explicit-data")
	if err := os.MkdirAll(explicitDir, 0o755); err != nil {
		t.Fatalf("mkdir explicit dir: %v", err)
	}
	oldSeg := filepath.Join(explicitDir, "connections-20200101.jsonl")
	if err := os.WriteFile(oldSeg, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write old segment: %v", err)
	}

	cmd := newDaemonPruneCmd()
	cmd.SetArgs([]string{"--data-dir", explicitDir, "--retention-hours", "1"})
	out, err := captureCommandStdout(cmd)
	if err != nil {
		t.Fatalf("daemon prune execute: %v", err)
	}
	if !strings.Contains(out, fmt.Sprintf("data-dir: %s (explicit-flags)", explicitDir)) {
		t.Fatalf("expected explicit data-dir in output, got: %q", out)
	}
	if !strings.Contains(out, "retention: 1h (flag)") {
		t.Fatalf("expected flag retention source, got: %q", out)
	}
	if _, err := os.Stat(oldSeg); !os.IsNotExist(err) {
		t.Fatalf("expected old segment to be pruned")
	}
}

func TestDaemonPruneImplicitUsesActiveStateTargetAndRetention(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "daemon.state")
	t.Setenv("HOLYF_NETWORK_DAEMON_STATE_FILE", stateFile)

	activeDir := filepath.Join(dir, "active-data")
	if err := os.MkdirAll(activeDir, 0o755); err != nil {
		t.Fatalf("mkdir active dir: %v", err)
	}
	oldSeg := filepath.Join(activeDir, "connections-20200101.jsonl")
	if err := os.WriteFile(oldSeg, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write old segment: %v", err)
	}
	state := daemonActiveState{
		PID:       os.Getpid(),
		DataDir:   activeDir,
		PIDFile:   filepath.Join(activeDir, "daemon.pid"),
		LogFile:   filepath.Join(activeDir, "daemon.log"),
		LockFile:  filepath.Join(activeDir, ".daemon.lock"),
		Retention: 48,
	}
	if err := writeActiveState(stateFile, state); err != nil {
		t.Fatalf("write active-state: %v", err)
	}

	cmd := newDaemonPruneCmd()
	out, err := captureCommandStdout(cmd)
	if err != nil {
		t.Fatalf("daemon prune execute: %v", err)
	}
	if !strings.Contains(out, fmt.Sprintf("data-dir: %s (active-state)", activeDir)) {
		t.Fatalf("expected active-state data-dir in output, got: %q", out)
	}
	if !strings.Contains(out, "retention: 48h (active-state)") {
		t.Fatalf("expected active-state retention source, got: %q", out)
	}
	if !strings.Contains(out, fmt.Sprintf("daemon-running: yes (pid=%d)", os.Getpid())) {
		t.Fatalf("expected running marker in output, got: %q", out)
	}
	if _, err := os.Stat(oldSeg); !os.IsNotExist(err) {
		t.Fatalf("expected old segment to be pruned")
	}
}

func TestNextLocalMidnight(t *testing.T) {
	loc := time.Now().Local().Location()
	now := time.Date(2026, 3, 8, 14, 45, 0, 0, loc)
	got := nextLocalMidnight(now)
	want := time.Date(2026, 3, 9, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("next midnight mismatch: got=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func captureCommandStdout(cmd interface {
	Execute() error
}) (string, error) {
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = writer

	runErr := cmd.Execute()
	_ = writer.Close()
	os.Stdout = oldStdout

	data, readErr := io.ReadAll(reader)
	if readErr != nil {
		return "", readErr
	}
	return string(data), runErr
}
