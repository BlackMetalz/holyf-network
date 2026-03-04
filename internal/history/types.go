package history

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

const (
	defaultIntervalSeconds  = 30
	defaultTopLimit         = 100
	defaultRetentionHours   = 24
	defaultMaxFiles         = 72
	defaultPruneEveryWrites = 10
	snapshotDirName         = "snapshots"
)

// SnapshotRecord is one persisted capture point.
type SnapshotRecord struct {
	CapturedAt  time.Time              `json:"captured_at"`
	Interface   string                 `json:"interface"`
	TopLimit    int                    `json:"top_limit"`
	Connections []collector.Connection `json:"connections"`
	Version     string                 `json:"version"`
}

// SnapshotRef points to one snapshot line in a segment file.
type SnapshotRef struct {
	FilePath   string
	Offset     int64
	CapturedAt time.Time
	ConnCount  int
}

// IndexStats summarizes index scan results.
type IndexStats struct {
	Files   int
	Corrupt int
}

// PruneResult summarizes retention work.
type PruneResult struct {
	RemovedByAge      int
	RemovedByMaxFiles int
}

// AppendResult summarizes one append operation.
type AppendResult struct {
	SegmentPath string
	Prune       PruneResult
}

// WriterConfig controls segment writing and retention.
type WriterConfig struct {
	DataDir             string
	RetentionHours      int
	MaxFiles            int
	PruneEverySnapshots int
}

func DefaultIntervalSeconds() int { return defaultIntervalSeconds }
func DefaultTopLimit() int        { return defaultTopLimit }
func DefaultRetentionHours() int  { return defaultRetentionHours }
func DefaultMaxFiles() int        { return defaultMaxFiles }

func DefaultWriterConfig(dataDir string) WriterConfig {
	return WriterConfig{
		DataDir:             dataDir,
		RetentionHours:      defaultRetentionHours,
		MaxFiles:            defaultMaxFiles,
		PruneEverySnapshots: defaultPruneEveryWrites,
	}
}

func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".holyf-network", snapshotDirName)
}

func ExpandPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
