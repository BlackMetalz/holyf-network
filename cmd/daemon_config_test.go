package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/history"
	"github.com/spf13/cobra"
)

func TestResolveDaemonCaptureOptionsUsesPartialConfigAndBuiltInDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "daemon.json")
	customDataDir := filepath.Join(t.TempDir(), "snapshots")
	if err := os.WriteFile(configPath, []byte("{\"data-dir\":\""+customDataDir+"\"}\n"), 0o644); err != nil {
		t.Fatalf("write daemon config: %v", err)
	}
	t.Setenv("HOLYF_NETWORK_DAEMON_CONFIG_FILE", configPath)

	cmd := &cobra.Command{Use: "test"}
	opts := daemonCaptureOptions{}
	addCaptureFlags(cmd, &opts)

	got, configUsed, err := resolveDaemonCaptureOptionsForCommand(cmd, opts)
	if err != nil {
		t.Fatalf("resolve daemon capture options: %v", err)
	}
	if !configUsed {
		t.Fatalf("expected config file to be used")
	}
	if got.dataDir != customDataDir {
		t.Fatalf("expected data-dir from config, got=%q want=%q", got.dataDir, customDataDir)
	}
	if got.intervalSec != history.DefaultIntervalSeconds() {
		t.Fatalf("expected built-in interval default, got=%d want=%d", got.intervalSec, history.DefaultIntervalSeconds())
	}
	if got.topLimit != history.DefaultTopLimit() {
		t.Fatalf("expected built-in top-limit default, got=%d want=%d", got.topLimit, history.DefaultTopLimit())
	}
	if got.retentionHours != history.DefaultRetentionHours() {
		t.Fatalf("expected built-in retention default, got=%d want=%d", got.retentionHours, history.DefaultRetentionHours())
	}
}

func TestResolveDaemonCaptureOptionsExplicitFlagsOverrideConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "daemon.json")
	if err := os.WriteFile(configPath, []byte("{\"data-dir\":\"/tmp/from-config\",\"interval\":45}\n"), 0o644); err != nil {
		t.Fatalf("write daemon config: %v", err)
	}
	t.Setenv("HOLYF_NETWORK_DAEMON_CONFIG_FILE", configPath)

	cmd := &cobra.Command{Use: "test"}
	opts := daemonCaptureOptions{}
	addCaptureFlags(cmd, &opts)
	if err := cmd.Flags().Set("interval", "90"); err != nil {
		t.Fatalf("set interval flag: %v", err)
	}
	if err := cmd.Flags().Set("data-dir", "/tmp/from-flag"); err != nil {
		t.Fatalf("set data-dir flag: %v", err)
	}

	got, configUsed, err := resolveDaemonCaptureOptionsForCommand(cmd, opts)
	if err != nil {
		t.Fatalf("resolve daemon capture options: %v", err)
	}
	if !configUsed {
		t.Fatalf("expected config file to be used")
	}
	if got.intervalSec != 90 {
		t.Fatalf("expected interval from explicit flag, got=%d", got.intervalSec)
	}
	if got.dataDir != "/tmp/from-flag" {
		t.Fatalf("expected data-dir from explicit flag, got=%q", got.dataDir)
	}
	if got.topLimit != history.DefaultTopLimit() {
		t.Fatalf("expected untouched top-limit default, got=%d", got.topLimit)
	}
}

func TestResolveDaemonCaptureOptionsSkipsConfigWhenAllFlagsExplicit(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "daemon.json")
	if err := os.WriteFile(configPath, []byte("{not-json}\n"), 0o644); err != nil {
		t.Fatalf("write daemon config: %v", err)
	}
	t.Setenv("HOLYF_NETWORK_DAEMON_CONFIG_FILE", configPath)

	cmd := &cobra.Command{Use: "test"}
	opts := daemonCaptureOptions{}
	addCaptureFlags(cmd, &opts)
	for name, value := range map[string]string{
		"interface":       "eth0",
		"interval":        "15",
		"top-limit":       "42",
		"data-dir":        "/tmp/explicit",
		"retention-hours": "24",
	} {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set %s flag: %v", name, err)
		}
	}

	got, configUsed, err := resolveDaemonCaptureOptionsForCommand(cmd, opts)
	if err != nil {
		t.Fatalf("resolve daemon capture options: %v", err)
	}
	if configUsed {
		t.Fatalf("expected config file to be skipped when all flags are explicit")
	}
	if got.ifaceName != "eth0" || got.intervalSec != 15 || got.topLimit != 42 || got.dataDir != "/tmp/explicit" || got.retentionHours != 24 {
		t.Fatalf("unexpected explicit capture options: %+v", got)
	}
}

func TestResolveReplayDataDirUsesConfigWhenNoActiveState(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "missing-daemon.state")
	t.Setenv("HOLYF_NETWORK_DAEMON_STATE_FILE", stateFile)

	configPath := filepath.Join(t.TempDir(), "daemon.json")
	customDataDir := filepath.Join(t.TempDir(), "snapshot-data")
	if err := os.WriteFile(configPath, []byte("{\"data-dir\":\""+customDataDir+"\"}\n"), 0o644); err != nil {
		t.Fatalf("write daemon config: %v", err)
	}
	t.Setenv("HOLYF_NETWORK_DAEMON_CONFIG_FILE", configPath)

	got, err := resolveReplayDataDir("", false)
	if err != nil {
		t.Fatalf("resolveReplayDataDir: %v", err)
	}
	if got != customDataDir {
		t.Fatalf("expected replay data-dir from config, got=%q want=%q", got, customDataDir)
	}
}

func TestDaemonStatusUsesConfigWhenNoActiveState(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "missing-daemon.state")
	t.Setenv("HOLYF_NETWORK_DAEMON_STATE_FILE", stateFile)

	configPath := filepath.Join(t.TempDir(), "daemon.json")
	customDataDir := filepath.Join(t.TempDir(), "snapshot-data")
	if err := os.WriteFile(configPath, []byte("{\"data-dir\":\""+customDataDir+"\"}\n"), 0o644); err != nil {
		t.Fatalf("write daemon config: %v", err)
	}
	t.Setenv("HOLYF_NETWORK_DAEMON_CONFIG_FILE", configPath)

	cmd := newDaemonStatusCmd()
	out, err := captureCommandStdout(cmd)
	if err != nil {
		t.Fatalf("daemon status execute: %v", err)
	}
	if !containsLineValue(out, "data-dir: ", customDataDir) {
		t.Fatalf("expected status to use config data-dir, got: %q", out)
	}
}

func TestDaemonPruneUsesConfigRetentionWhenNoActiveState(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "missing-daemon.state")
	t.Setenv("HOLYF_NETWORK_DAEMON_STATE_FILE", stateFile)

	dataDir := filepath.Join(t.TempDir(), "snapshot-data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	oldSeg := filepath.Join(dataDir, "connections-20200101.jsonl")
	if err := os.WriteFile(oldSeg, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write old segment: %v", err)
	}

	configPath := filepath.Join(t.TempDir(), "daemon.json")
	payload := "{\"data-dir\":\"" + dataDir + "\",\"retention-hours\":1}\n"
	if err := os.WriteFile(configPath, []byte(payload), 0o644); err != nil {
		t.Fatalf("write daemon config: %v", err)
	}
	t.Setenv("HOLYF_NETWORK_DAEMON_CONFIG_FILE", configPath)

	cmd := newDaemonPruneCmd()
	out, err := captureCommandStdout(cmd)
	if err != nil {
		t.Fatalf("daemon prune execute: %v", err)
	}
	if !strings.Contains(out, "retention: 1h (default)") {
		t.Fatalf("expected prune to use config retention, got: %q", out)
	}
	if _, err := os.Stat(oldSeg); !os.IsNotExist(err) {
		t.Fatalf("expected old segment to be pruned")
	}
}

func containsLineValue(out, prefix, want string) bool {
	for _, line := range strings.Split(out, "\n") {
		if line == prefix+want {
			return true
		}
	}
	return false
}
