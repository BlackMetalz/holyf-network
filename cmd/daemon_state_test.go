package cmd

import (
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
		StartedAt: time.Now().Local().Truncate(time.Second),
	}

	if err := writeActiveState(stateFile, want); err != nil {
		t.Fatalf("write active-state: %v", err)
	}
	got, err := readActiveState(stateFile)
	if err != nil {
		t.Fatalf("read active-state: %v", err)
	}
	if got.PID != want.PID || got.DataDir != want.DataDir || got.PIDFile != want.PIDFile || got.LogFile != want.LogFile || got.LockFile != want.LockFile {
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
