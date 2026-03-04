package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
)

type daemonCaptureOptions struct {
	ifaceName      string
	intervalSec    int
	topLimit       int
	dataDir        string
	retentionHours int
	maxFiles       int
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
	dataDir  string
	pidFile  string
	logFile  string
	lockFile string
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
			if err := ensureRuntimePaths(paths); err != nil {
				return err
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
				"--max-files", strconv.Itoa(opts.maxFiles),
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

			fmt.Printf("daemon started (pid=%d)\n", pid)
			fmt.Printf("data-dir: %s\n", paths.dataDir)
			fmt.Printf("pid-file: %s\n", paths.pidFile)
			fmt.Printf("log-file: %s\n", paths.logFile)
			fmt.Printf("interval=%ds top-limit=%d retention=%dh max-files=%d\n",
				opts.intervalSec,
				opts.topLimit,
				opts.retentionHours,
				opts.maxFiles,
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
			paths, err := resolveDaemonPaths(opts.dataDir, opts.pidFile, "")
			if err != nil {
				return err
			}

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
				return nil
			}

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
			fmt.Printf("daemon stopped (pid=%d)\n", pid)
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
			paths, err := resolveDaemonPaths(opts.dataDir, opts.pidFile, "")
			if err != nil {
				return err
			}

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
				return nil
			}

			fmt.Printf("daemon status: running (pid=%d)\n", pid)
			fmt.Printf("data-dir: %s\n", paths.dataDir)
			fmt.Printf("pid-file: %s\n", paths.pidFile)
			fmt.Printf("log-file: %s\n", paths.logFile)
			fmt.Printf("lock-file: %s\n", paths.lockFile)
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
			if err := ensureRuntimePaths(paths); err != nil {
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
				MaxFiles:            opts.maxFiles,
				PruneEverySnapshots: 10,
			})
			if err != nil {
				return err
			}
			defer writer.Close()

			version := resolveBuildVersion(Version)
			fmt.Printf("holyf-network daemon started | iface=%s interval=%ds top-limit=%d data-dir=%s retention=%dh max-files=%d\n",
				opts.ifaceName,
				opts.intervalSec,
				opts.topLimit,
				opts.dataDir,
				opts.retentionHours,
				opts.maxFiles,
			)

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			capture := func(ts time.Time) {
				conns, err := collector.CollectTopTalkers(opts.topLimit)
				if err != nil {
					fmt.Printf("[%s] capture failed: %v\n", ts.Format(time.RFC3339), err)
					return
				}
				result, err := writer.Append(history.SnapshotRecord{
					CapturedAt:  ts.Local(),
					Interface:   opts.ifaceName,
					TopLimit:    opts.topLimit,
					Connections: conns,
					Version:     version,
				})
				if err != nil {
					fmt.Printf("[%s] write failed: %v\n", ts.Format(time.RFC3339), err)
					return
				}

				fmt.Printf("[%s] captured %d connections -> %s\n",
					ts.Format(time.RFC3339),
					len(conns),
					filepath.Base(result.SegmentPath),
				)
				if result.Prune.RemovedByAge > 0 || result.Prune.RemovedByMaxFiles > 0 {
					fmt.Printf("[%s] pruned files: age=%d max-files=%d\n",
						ts.Format(time.RFC3339),
						result.Prune.RemovedByAge,
						result.Prune.RemovedByMaxFiles,
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
	cmd.Flags().IntVar(&opts.topLimit, "top-limit", history.DefaultTopLimit(), "Max top connections stored per snapshot")
	cmd.Flags().StringVar(&opts.dataDir, "data-dir", history.DefaultDataDir(), "Snapshot data directory")
	cmd.Flags().IntVar(&opts.retentionHours, "retention-hours", history.DefaultRetentionHours(), "Retention window in hours")
	cmd.Flags().IntVar(&opts.maxFiles, "max-files", history.DefaultMaxFiles(), "Maximum segment files retained")
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
	if opts.maxFiles < 1 {
		return fmt.Errorf("max-files must be >= 1, got %d", opts.maxFiles)
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
	if resolvedLog == "" {
		resolvedLog = filepath.Join(resolvedDataDir, defaultDaemonLogFileName)
	} else {
		resolvedLog = history.ExpandPath(resolvedLog)
	}

	return daemonRuntimePaths{
		dataDir:  resolvedDataDir,
		pidFile:  resolvedPID,
		logFile:  resolvedLog,
		lockFile: filepath.Join(resolvedDataDir, ".daemon.lock"),
	}, nil
}

func ensureRuntimePaths(paths daemonRuntimePaths) error {
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
			return fmt.Errorf("create log-file dir: %w", err)
		}
	}
	return nil
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
