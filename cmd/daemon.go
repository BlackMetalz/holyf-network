package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/history"
	"github.com/spf13/cobra"
)

const (
	defaultDaemonPIDFileName = "daemon.pid"
	defaultDaemonLogFileName = "daemon.log"
	defaultDaemonStateName   = "daemon.state"
)

type daemonCaptureOptions struct {
	ifaceName      string
	intervalSec    int
	topLimit       int
	dataDir        string
	retentionHours int
}

type daemonStartOptions struct {
	daemonCaptureOptions
	pidFile string
	logFile string
}

type daemonControlOptions struct {
	dataDir string
	pidFile string
}

type daemonRuntimePaths struct {
	dataDir              string
	pidFile              string
	logFile              string
	lockFile             string
	stateFile            string
	allowLogFileFallback bool
}

type daemonActiveState struct {
	PID       int       `json:"pid"`
	DataDir   string    `json:"data_dir"`
	PIDFile   string    `json:"pid_file"`
	LogFile   string    `json:"log_file"`
	LockFile  string    `json:"lock_file"`
	StartedAt time.Time `json:"started_at"`
}

func newDaemonCmd() *cobra.Command {
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage snapshot collector daemon",
	}
	daemonCmd.AddCommand(newDaemonStartCmd())
	daemonCmd.AddCommand(newDaemonStopCmd())
	daemonCmd.AddCommand(newDaemonStatusCmd())
	daemonCmd.AddCommand(newDaemonRunCmd())
	return daemonCmd
}

func newDaemonStartCmd() *cobra.Command {
	opts := daemonStartOptions{}
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start connection snapshot daemon in background",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCaptureOptions(opts.daemonCaptureOptions); err != nil {
				return err
			}
			ifaceName, err := resolveInterface(opts.ifaceName)
			if err != nil {
				return err
			}
			opts.ifaceName = ifaceName

			paths, err := resolveDaemonPaths(opts.dataDir, opts.pidFile, opts.logFile)
			if err != nil {
				return err
			}
			if err := ensureRuntimePaths(&paths); err != nil {
				return err
			}

			stateExists, state, stateRunning, err := readActiveStateStatus(paths.stateFile)
			if err != nil {
				return err
			}
			if stateExists && stateRunning {
				return fmt.Errorf("daemon already running (pid=%d, data-dir=%s)", state.PID, state.DataDir)
			}
			if stateExists && !stateRunning {
				_ = cleanupStaleActiveState(paths.stateFile, state)
			}

			running, pid, err := readDaemonStatus(paths.pidFile)
			if err != nil {
				return err
			}
			if running {
				return fmt.Errorf("daemon already running (pid=%d)", pid)
			}
			if pid > 0 {
				_ = os.Remove(paths.pidFile)
			}

			exePath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve executable: %w", err)
			}
			logFile, err := os.OpenFile(paths.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return fmt.Errorf("open daemon log file: %w", err)
			}
			defer logFile.Close()

			childArgs := []string{
				"daemon", "run",
				"--internal",
				"--interface", opts.ifaceName,
				"--interval", strconv.Itoa(opts.intervalSec),
				"--top-limit", strconv.Itoa(opts.topLimit),
				"--data-dir", paths.dataDir,
				"--retention-hours", strconv.Itoa(opts.retentionHours),
				"--pid-file", paths.pidFile,
			}
			child := exec.Command(exePath, childArgs...)
			child.Stdout = logFile
			child.Stderr = logFile
			child.Stdin = nil
			child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

			if err := child.Start(); err != nil {
				return fmt.Errorf("start daemon process: %w", err)
			}
			pid = child.Process.Pid

			if err := writePIDFile(paths.pidFile, pid); err != nil {
				_ = child.Process.Kill()
				return err
			}

			// Give child a brief moment to fail fast (e.g. lock contention).
			time.Sleep(250 * time.Millisecond)
			if !isProcessRunning(pid) {
				_ = os.Remove(paths.pidFile)
				return fmt.Errorf("daemon failed to stay running; check log: %s", paths.logFile)
			}

			activeState := daemonActiveState{
				PID:       pid,
				DataDir:   paths.dataDir,
				PIDFile:   paths.pidFile,
				LogFile:   paths.logFile,
				LockFile:  paths.lockFile,
				StartedAt: time.Now().Local(),
			}
			if err := writeActiveState(paths.stateFile, activeState); err != nil {
				_ = child.Process.Kill()
				_ = os.Remove(paths.pidFile)
				return err
			}

			fmt.Printf("daemon started (pid=%d)\n", pid)
			fmt.Printf("data-dir: %s\n", paths.dataDir)
			fmt.Printf("pid-file: %s\n", paths.pidFile)
			fmt.Printf("log-file: %s\n", paths.logFile)
			fmt.Printf("state-file: %s\n", paths.stateFile)
			fmt.Printf("interval=%ds top-limit=%d retention=%dh\n",
				opts.intervalSec,
				opts.topLimit,
				opts.retentionHours,
			)
			return nil
		},
	}

	addCaptureFlags(cmd, &opts.daemonCaptureOptions)
	cmd.Flags().StringVar(&opts.pidFile, "pid-file", "", "Daemon PID file path (default: <data-dir>/daemon.pid)")
	cmd.Flags().StringVar(&opts.logFile, "log-file", "", "Daemon log file path (default: <data-dir>/daemon.log)")
	return cmd
}

func newDaemonStopCmd() *cobra.Command {
	opts := daemonControlOptions{}
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop running snapshot daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			explicit := cmd.Flags().Changed("data-dir") || cmd.Flags().Changed("pid-file")
			paths, err := resolveDaemonPaths(opts.dataDir, opts.pidFile, "")
			if err != nil {
				return err
			}

			if explicit {
				return stopDaemonFromPIDFile(paths, false)
			}

			stateExists, state, stateRunning, err := readActiveStateStatus(paths.stateFile)
			if err != nil {
				return err
			}
			if !stateExists {
				fmt.Println("daemon not running (no active-state)")
				fmt.Printf("state-file: %s\n", paths.stateFile)
				return nil
			}
			if !stateRunning {
				_ = cleanupStaleActiveState(paths.stateFile, state)
				fmt.Printf("daemon not running (stale active-state cleaned, pid=%d)\n", state.PID)
				fmt.Printf("state-file: %s\n", paths.stateFile)
				return nil
			}

			statePaths := daemonRuntimePaths{
				dataDir:   state.DataDir,
				pidFile:   state.PIDFile,
				logFile:   state.LogFile,
				lockFile:  state.LockFile,
				stateFile: paths.stateFile,
			}
			if err := stopDaemonByPID(state.PID, statePaths, true); err != nil {
				return err
			}
			_ = removeActiveState(paths.stateFile)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.dataDir, "data-dir", history.DefaultDataDir(), "Snapshot data directory")
	cmd.Flags().StringVar(&opts.pidFile, "pid-file", "", "Daemon PID file path (default: <data-dir>/daemon.pid)")
	return cmd
}

func newDaemonStatusCmd() *cobra.Command {
	opts := daemonControlOptions{}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show snapshot daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			explicit := cmd.Flags().Changed("data-dir") || cmd.Flags().Changed("pid-file")
			paths, err := resolveDaemonPaths(opts.dataDir, opts.pidFile, "")
			if err != nil {
				return err
			}

			if explicit {
				running, pid, err := readDaemonStatus(paths.pidFile)
				if err != nil {
					return err
				}
				if !running {
					if pid > 0 {
						fmt.Printf("daemon status: stopped (stale pid=%d in %s)\n", pid, paths.pidFile)
					} else {
						fmt.Println("daemon status: stopped")
					}
					fmt.Printf("data-dir: %s\n", paths.dataDir)
					fmt.Printf("pid-file: %s\n", paths.pidFile)
					fmt.Printf("log-file: %s\n", paths.logFile)
					fmt.Printf("state-file: %s\n", paths.stateFile)
					return nil
				}

				fmt.Printf("daemon status: running (pid=%d)\n", pid)
				fmt.Printf("data-dir: %s\n", paths.dataDir)
				fmt.Printf("pid-file: %s\n", paths.pidFile)
				fmt.Printf("log-file: %s\n", paths.logFile)
				fmt.Printf("lock-file: %s\n", paths.lockFile)
				fmt.Printf("state-file: %s\n", paths.stateFile)
				return nil
			}

			stateExists, state, stateRunning, err := readActiveStateStatus(paths.stateFile)
			if err != nil {
				return err
			}
			if !stateExists {
				fmt.Println("daemon status: stopped")
				fmt.Printf("data-dir: %s\n", paths.dataDir)
				fmt.Printf("pid-file: %s\n", paths.pidFile)
				fmt.Printf("log-file: %s\n", paths.logFile)
				fmt.Printf("state-file: %s\n", paths.stateFile)
				return nil
			}
			if !stateRunning {
				_ = cleanupStaleActiveState(paths.stateFile, state)
				fmt.Printf("daemon status: stopped (stale active-state pid=%d cleaned)\n", state.PID)
				fmt.Printf("data-dir: %s\n", state.DataDir)
				fmt.Printf("pid-file: %s\n", state.PIDFile)
				fmt.Printf("log-file: %s\n", state.LogFile)
				fmt.Printf("state-file: %s\n", paths.stateFile)
				return nil
			}

			fmt.Printf("daemon status: running (pid=%d)\n", state.PID)
			fmt.Printf("data-dir: %s\n", state.DataDir)
			fmt.Printf("pid-file: %s\n", state.PIDFile)
			fmt.Printf("log-file: %s\n", state.LogFile)
			fmt.Printf("lock-file: %s\n", state.LockFile)
			fmt.Printf("state-file: %s\n", paths.stateFile)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.dataDir, "data-dir", history.DefaultDataDir(), "Snapshot data directory")
	cmd.Flags().StringVar(&opts.pidFile, "pid-file", "", "Daemon PID file path (default: <data-dir>/daemon.pid)")
	return cmd
}

func newDaemonRunCmd() *cobra.Command {
	opts := daemonCaptureOptions{}
	var pidFile string
	var internal bool

	runCmd := &cobra.Command{
		Use:    "run",
		Hidden: true,
		Short:  "Run connection snapshot collector in foreground (internal)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !internal {
				return fmt.Errorf("use `daemon start` instead")
			}
			if err := validateCaptureOptions(opts); err != nil {
				return err
			}

			ifaceName, err := resolveInterface(opts.ifaceName)
			if err != nil {
				return err
			}
			opts.ifaceName = ifaceName

			paths, err := resolveDaemonPaths(opts.dataDir, pidFile, "")
			if err != nil {
				return err
			}
			opts.dataDir = paths.dataDir
			if err := ensureRuntimePaths(&paths); err != nil {
				return err
			}

			if paths.pidFile != "" {
				if err := writePIDFile(paths.pidFile, os.Getpid()); err != nil {
					return err
				}
				defer os.Remove(paths.pidFile)
			}

			writer, err := history.NewSnapshotWriter(history.WriterConfig{
				DataDir:             opts.dataDir,
				RetentionHours:      opts.retentionHours,
				PruneEverySnapshots: 10,
			})
			if err != nil {
				return err
			}
			defer writer.Close()
			bwTracker := collector.NewBandwidthTracker()
			ssBWTracker := collector.NewSocketBandwidthTracker()

			version := resolveBuildVersion(Version)
			fmt.Printf("holyf-network daemon started | iface=%s interval=%ds top-limit=%d data-dir=%s retention=%dh\n",
				opts.ifaceName,
				opts.intervalSec,
				opts.topLimit,
				opts.dataDir,
				opts.retentionHours,
			)

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			capture := func(ts time.Time) {
				conns, err := collector.CollectTopTalkers(0)
				if err != nil {
					fmt.Printf("[%s] capture failed: %v\n", ts.Format(time.RFC3339), err)
					return
				}

				bwSample := collector.BandwidthSnapshot{}
				flows, flowErr := collector.CollectConntrackFlowsTCP()
				if len(flows) > 0 {
					conns = collector.MergeConntrackHostFlows(conns, flows)
				}
				if flowErr == nil {
					bwSample = bwTracker.BuildSnapshot(flows, ts)
					conns = collector.EnrichConnectionsWithBandwidth(conns, bwSample)
				}
				ssCounters, ssErr := collector.CollectSocketTCPCounters()
				if ssErr == nil {
					ssSample := ssBWTracker.BuildSnapshot(ssCounters, ts)
					if ssSample.Available {
						conns = collector.OverlayMissingBandwidth(conns, ssSample)
						if !bwSample.Available {
							bwSample.Available = true
							bwSample.SampleSeconds = ssSample.SampleSeconds
						}
					}
				}

				groups := history.AggregateConnections(conns, opts.topLimit)
				totalDelta := int64(0)
				for _, row := range groups {
					totalDelta += row.TotalBytesDelta
				}
				result, err := writer.Append(history.SnapshotRecord{
					CapturedAt:         ts.Local(),
					Interface:          opts.ifaceName,
					TopLimit:           opts.topLimit,
					SampleSeconds:      bwSample.SampleSeconds,
					BandwidthAvailable: bwSample.Available,
					Groups:             groups,
					Version:            version,
				})
				if err != nil {
					fmt.Printf("[%s] write failed: %v\n", ts.Format(time.RFC3339), err)
					return
				}

				fmt.Printf("[%s] captured %d connections -> %d aggregate rows (cap=%d) | bw_available=%t | total_delta=%s -> %s\n",
					ts.Format(time.RFC3339),
					len(conns),
					len(groups),
					opts.topLimit,
					bwSample.Available,
					humanBytes(totalDelta),
					filepath.Base(result.SegmentPath),
				)
				if flowErr != nil {
					fmt.Printf("[%s] bandwidth capture unavailable: %v\n", ts.Format(time.RFC3339), flowErr)
				}
				if ssErr != nil {
					fmt.Printf("[%s] socket bandwidth fallback unavailable: %v\n", ts.Format(time.RFC3339), ssErr)
				}
				if result.Prune.RemovedByAge > 0 {
					fmt.Printf("[%s] pruned files: age=%d\n",
						ts.Format(time.RFC3339),
						result.Prune.RemovedByAge,
					)
				}
			}

			capture(time.Now())
			ticker := time.NewTicker(time.Duration(opts.intervalSec) * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					fmt.Println("holyf-network daemon stopping")
					return nil
				case tickAt := <-ticker.C:
					capture(tickAt)
				}
			}
		},
	}

	addCaptureFlags(runCmd, &opts)
	runCmd.Flags().StringVar(&pidFile, "pid-file", "", "Daemon PID file path")
	runCmd.Flags().BoolVar(&internal, "internal", false, "Internal daemon worker mode")
	_ = runCmd.Flags().MarkHidden("pid-file")
	_ = runCmd.Flags().MarkHidden("internal")

	return runCmd
}

func addCaptureFlags(cmd *cobra.Command, opts *daemonCaptureOptions) {
	cmd.Flags().StringVarP(&opts.ifaceName, "interface", "i", "", "Network interface to monitor (default: auto-detect)")
	cmd.Flags().IntVar(&opts.intervalSec, "interval", history.DefaultIntervalSeconds(), "Capture interval in seconds (1-300)")
	cmd.Flags().IntVar(&opts.topLimit, "top-limit", history.DefaultTopLimit(), "Max aggregate rows stored per snapshot")
	cmd.Flags().StringVar(&opts.dataDir, "data-dir", history.DefaultDataDir(), "Snapshot data directory")
	cmd.Flags().IntVar(&opts.retentionHours, "retention-hours", history.DefaultRetentionHours(), "Retention window in hours")
}

func validateCaptureOptions(opts daemonCaptureOptions) error {
	if opts.intervalSec < 1 || opts.intervalSec > 300 {
		return fmt.Errorf("interval must be 1-300 seconds, got %d", opts.intervalSec)
	}
	if opts.topLimit < 1 {
		return fmt.Errorf("top-limit must be >= 1, got %d", opts.topLimit)
	}
	if opts.retentionHours < 1 {
		return fmt.Errorf("retention-hours must be >= 1, got %d", opts.retentionHours)
	}
	return nil
}

func resolveDaemonPaths(dataDir, pidFile, logFile string) (daemonRuntimePaths, error) {
	resolvedDataDir := history.ExpandPath(dataDir)
	if strings.TrimSpace(resolvedDataDir) == "" {
		resolvedDataDir = history.DefaultDataDir()
	}
	if strings.TrimSpace(resolvedDataDir) == "" {
		return daemonRuntimePaths{}, fmt.Errorf("cannot determine default data dir")
	}

	resolvedPID := strings.TrimSpace(pidFile)
	if resolvedPID == "" {
		resolvedPID = filepath.Join(resolvedDataDir, defaultDaemonPIDFileName)
	} else {
		resolvedPID = history.ExpandPath(resolvedPID)
	}

	resolvedLog := strings.TrimSpace(logFile)
	allowLogFallback := false
	if resolvedLog == "" {
		resolvedLog = defaultDaemonLogPath(runtime.GOOS, os.Geteuid(), resolvedDataDir)
		allowLogFallback = shouldUseSystemDaemonPaths(runtime.GOOS, os.Geteuid())
	} else {
		resolvedLog = history.ExpandPath(resolvedLog)
	}

	stateFile := defaultDaemonStatePath()
	if strings.TrimSpace(stateFile) == "" {
		return daemonRuntimePaths{}, fmt.Errorf("cannot determine daemon state file path")
	}

	return daemonRuntimePaths{
		dataDir:              resolvedDataDir,
		pidFile:              resolvedPID,
		logFile:              resolvedLog,
		lockFile:             filepath.Join(resolvedDataDir, ".daemon.lock"),
		stateFile:            stateFile,
		allowLogFileFallback: allowLogFallback,
	}, nil
}

func ensureRuntimePaths(paths *daemonRuntimePaths) error {
	if err := os.MkdirAll(paths.dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if paths.pidFile != "" {
		if err := os.MkdirAll(filepath.Dir(paths.pidFile), 0o755); err != nil {
			return fmt.Errorf("create pid-file dir: %w", err)
		}
	}
	if paths.logFile != "" {
		if err := os.MkdirAll(filepath.Dir(paths.logFile), 0o755); err != nil {
			if !paths.allowLogFileFallback {
				return fmt.Errorf("create log-file dir: %w", err)
			}
			fallbackLog := filepath.Join(paths.dataDir, defaultDaemonLogFileName)
			if mkErr := os.MkdirAll(filepath.Dir(fallbackLog), 0o755); mkErr != nil {
				return fmt.Errorf("create log-file dir: %w", err)
			}
			paths.logFile = fallbackLog
			paths.allowLogFileFallback = false
		}
	}
	if paths.stateFile != "" {
		if err := os.MkdirAll(filepath.Dir(paths.stateFile), 0o755); err != nil {
			return fmt.Errorf("create state-file dir: %w", err)
		}
	}
	return nil
}

func defaultDaemonLogPath(goos string, euid int, dataDir string) string {
	if shouldUseSystemDaemonPaths(goos, euid) {
		return "/var/log/holyf-network/" + defaultDaemonLogFileName
	}
	return filepath.Join(dataDir, defaultDaemonLogFileName)
}

func shouldUseSystemDaemonPaths(goos string, euid int) bool {
	return goos == "linux" && euid == 0
}

func defaultDaemonStatePath() string {
	if override := strings.TrimSpace(os.Getenv("HOLYF_NETWORK_DAEMON_STATE_FILE")); override != "" {
		return history.ExpandPath(override)
	}
	if shouldUseSystemDaemonPaths(runtime.GOOS, os.Geteuid()) {
		return "/run/holyf-network/" + defaultDaemonStateName
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".holyf-network", defaultDaemonStateName)
}

func writeActiveState(path string, state daemonActiveState) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("active-state path is required")
	}
	if state.PID <= 0 {
		return fmt.Errorf("active-state pid is invalid: %d", state.PID)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create active-state dir: %w", err)
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal active-state: %w", err)
	}
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write active-state temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("rename active-state file: %w", err)
	}
	return nil
}

func readActiveState(path string) (daemonActiveState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return daemonActiveState{}, err
	}
	var state daemonActiveState
	if err := json.Unmarshal(data, &state); err != nil {
		return daemonActiveState{}, fmt.Errorf("decode active-state %s: %w", path, err)
	}
	return state, nil
}

func removeActiveState(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func cleanupStaleActiveState(stateFile string, state daemonActiveState) error {
	if strings.TrimSpace(state.PIDFile) != "" {
		_ = os.Remove(state.PIDFile)
	}
	return removeActiveState(stateFile)
}

func readActiveStateStatus(stateFile string) (exists bool, state daemonActiveState, running bool, err error) {
	state, err = readActiveState(stateFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, daemonActiveState{}, false, nil
		}
		return false, daemonActiveState{}, false, err
	}
	return true, state, isProcessRunning(state.PID), nil
}

func readDaemonStatus(pidFile string) (running bool, pid int, err error) {
	pid, err = readPIDFile(pidFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, 0, nil
		}
		return false, 0, err
	}
	return isProcessRunning(pid), pid, nil
}

func stopDaemonFromPIDFile(paths daemonRuntimePaths, cleanupState bool) error {
	running, pid, err := readDaemonStatus(paths.pidFile)
	if err != nil {
		return err
	}
	if !running {
		if pid > 0 {
			_ = os.Remove(paths.pidFile)
			fmt.Printf("daemon not running (stale pid file removed: %s)\n", paths.pidFile)
		} else {
			fmt.Println("daemon not running")
		}
		if cleanupState {
			_ = removeActiveState(paths.stateFile)
		}
		return nil
	}

	return stopDaemonByPID(pid, paths, cleanupState)
}

func stopDaemonByPID(pid int, paths daemonRuntimePaths, cleanupState bool) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find daemon process %d: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM to pid %d: %w", pid, err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for isProcessRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if isProcessRunning(pid) {
		_ = proc.Signal(syscall.SIGKILL)
		for i := 0; i < 20 && isProcessRunning(pid); i++ {
			time.Sleep(100 * time.Millisecond)
		}
	}

	if isProcessRunning(pid) {
		return fmt.Errorf("failed to stop daemon pid %d", pid)
	}

	_ = os.Remove(paths.pidFile)
	if cleanupState {
		_ = removeActiveState(paths.stateFile)
	}
	fmt.Printf("daemon stopped (pid=%d)\n", pid)
	if paths.dataDir != "" {
		fmt.Printf("data-dir: %s\n", paths.dataDir)
	}
	if paths.pidFile != "" {
		fmt.Printf("pid-file: %s\n", paths.pidFile)
	}
	if paths.logFile != "" {
		fmt.Printf("log-file: %s\n", paths.logFile)
	}
	if paths.stateFile != "" {
		fmt.Printf("state-file: %s\n", paths.stateFile)
	}
	return nil
}

func readPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid pid file %s", path)
	}
	return pid, nil
}

func writePIDFile(path string, pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid: %d", pid)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		return fmt.Errorf("write pid file %s: %w", path, err)
	}
	return nil
}

func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == syscall.EPERM {
		return true
	}
	return false
}

func humanBytes(n int64) string {
	if n >= 1024*1024*1024 {
		return fmt.Sprintf("%.1fGB", float64(n)/(1024*1024*1024))
	}
	if n >= 1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
	if n >= 1024 {
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	return fmt.Sprintf("%dB", n)
}
