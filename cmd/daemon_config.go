package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/history"
	"github.com/spf13/cobra"
)

const defaultDaemonConfigPath = "/etc/holyf-network/daemon.json"

type daemonConfigFile struct {
	DataDir        *string `json:"data-dir"`
	Interface      *string `json:"interface"`
	Interval       *int    `json:"interval"`
	TopLimit       *int    `json:"top-limit"`
	RetentionHours *int    `json:"retention-hours"`
}

func defaultDaemonCaptureOptions() daemonCaptureOptions {
	return daemonCaptureOptions{
		ifaceName:      "",
		intervalSec:    history.DefaultIntervalSeconds(),
		topLimit:       history.DefaultTopLimit(),
		dataDir:        history.DefaultDataDir(),
		retentionHours: history.DefaultRetentionHours(),
	}
}

func defaultDaemonConfigPathValue() string {
	if override := strings.TrimSpace(os.Getenv("HOLYF_NETWORK_DAEMON_CONFIG_FILE")); override != "" {
		return history.ExpandPath(override)
	}
	return defaultDaemonConfigPath
}

func loadDaemonConfigFile() (daemonConfigFile, bool, error) {
	configPath := strings.TrimSpace(defaultDaemonConfigPathValue())
	if configPath == "" {
		return daemonConfigFile{}, false, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return daemonConfigFile{}, false, nil
		}
		return daemonConfigFile{}, false, fmt.Errorf("read daemon config %s: %w", configPath, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var cfg daemonConfigFile
	if err := decoder.Decode(&cfg); err != nil {
		return daemonConfigFile{}, false, fmt.Errorf("decode daemon config %s: %w", configPath, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return daemonConfigFile{}, false, fmt.Errorf("decode daemon config %s: trailing content", configPath)
		}
		return daemonConfigFile{}, false, fmt.Errorf("decode daemon config %s: %w", configPath, err)
	}
	if err := validateDaemonConfigFile(cfg, configPath); err != nil {
		return daemonConfigFile{}, false, err
	}
	return cfg, true, nil
}

func validateDaemonConfigFile(cfg daemonConfigFile, path string) error {
	if cfg.DataDir != nil && strings.TrimSpace(history.ExpandPath(*cfg.DataDir)) == "" {
		return fmt.Errorf("invalid daemon config %s: data-dir must not be empty", path)
	}
	if cfg.Interval != nil && (*cfg.Interval < 1 || *cfg.Interval > 300) {
		return fmt.Errorf("invalid daemon config %s: interval must be 1-300 seconds, got %d", path, *cfg.Interval)
	}
	if cfg.TopLimit != nil && *cfg.TopLimit < 1 {
		return fmt.Errorf("invalid daemon config %s: top-limit must be >= 1, got %d", path, *cfg.TopLimit)
	}
	if cfg.RetentionHours != nil && *cfg.RetentionHours < 1 {
		return fmt.Errorf("invalid daemon config %s: retention-hours must be >= 1, got %d", path, *cfg.RetentionHours)
	}
	return nil
}

func applyDaemonConfigDefaults(opts *daemonCaptureOptions, cfg daemonConfigFile) {
	if cfg.Interface != nil {
		opts.ifaceName = strings.TrimSpace(*cfg.Interface)
	}
	if cfg.Interval != nil {
		opts.intervalSec = *cfg.Interval
	}
	if cfg.TopLimit != nil {
		opts.topLimit = *cfg.TopLimit
	}
	if cfg.DataDir != nil {
		opts.dataDir = history.ExpandPath(*cfg.DataDir)
	}
	if cfg.RetentionHours != nil {
		opts.retentionHours = *cfg.RetentionHours
	}
}

func resolveDaemonConfigDefaults() (daemonCaptureOptions, bool, error) {
	opts := defaultDaemonCaptureOptions()
	cfg, loaded, err := loadDaemonConfigFile()
	if err != nil {
		return daemonCaptureOptions{}, false, err
	}
	if loaded {
		applyDaemonConfigDefaults(&opts, cfg)
	}
	return opts, loaded, nil
}

func resolveDaemonCaptureOptionsForCommand(cmd *cobra.Command, raw daemonCaptureOptions) (daemonCaptureOptions, bool, error) {
	effective := defaultDaemonCaptureOptions()
	configUsed := false
	if !daemonCaptureFlagsAllExplicit(cmd) {
		cfg, loaded, err := loadDaemonConfigFile()
		if err != nil {
			return daemonCaptureOptions{}, false, err
		}
		if loaded {
			applyDaemonConfigDefaults(&effective, cfg)
			configUsed = true
		}
	}

	if cmd.Flags().Changed("interface") {
		effective.ifaceName = raw.ifaceName
	}
	if cmd.Flags().Changed("interval") {
		effective.intervalSec = raw.intervalSec
	}
	if cmd.Flags().Changed("top-limit") {
		effective.topLimit = raw.topLimit
	}
	if cmd.Flags().Changed("data-dir") {
		effective.dataDir = raw.dataDir
	}
	if cmd.Flags().Changed("retention-hours") {
		effective.retentionHours = raw.retentionHours
	}
	return effective, configUsed, nil
}

func daemonCaptureFlagsAllExplicit(cmd *cobra.Command) bool {
	for _, flagName := range []string{"interface", "interval", "top-limit", "data-dir", "retention-hours"} {
		if !cmd.Flags().Changed(flagName) {
			return false
		}
	}
	return true
}

func resolveConfiguredDefaultDataDir() (string, error) {
	defaults, _, err := resolveDaemonConfigDefaults()
	if err != nil {
		return "", err
	}
	dataDir := strings.TrimSpace(history.ExpandPath(defaults.dataDir))
	if dataDir == "" {
		return "", fmt.Errorf("cannot determine default data dir")
	}
	return dataDir, nil
}

func resolveConfiguredDefaultRetentionHours() (int, error) {
	defaults, _, err := resolveDaemonConfigDefaults()
	if err != nil {
		return 0, err
	}
	return defaults.retentionHours, nil
}
