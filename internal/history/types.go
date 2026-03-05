package history

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultIntervalSeconds  = 30
	defaultTopLimit         = 500
	defaultRetentionHours   = 168
	defaultPruneEveryWrites = 10
	snapshotDirName         = "snapshots"
)

// SnapshotGroup is one aggregate row in persisted history.
type SnapshotGroup struct {
	PeerIP           string         `json:"peer_ip"`
	LocalPort        int            `json:"local_port"`
	ProcName         string         `json:"proc_name"`
	ConnCount        int            `json:"conn_count"`
	TxQueue          int64          `json:"tx_queue"`
	RxQueue          int64          `json:"rx_queue"`
	TotalQueue       int64          `json:"total_queue"`
	TxBytesDelta     int64          `json:"tx_bytes_delta"`
	RxBytesDelta     int64          `json:"rx_bytes_delta"`
	TotalBytesDelta  int64          `json:"total_bytes_delta"`
	TxBytesPerSec    float64        `json:"tx_bytes_per_sec"`
	RxBytesPerSec    float64        `json:"rx_bytes_per_sec"`
	TotalBytesPerSec float64        `json:"total_bytes_per_sec"`
	States           map[string]int `json:"states"`
}

// SnapshotRecord is one persisted capture point.
type SnapshotRecord struct {
	CapturedAt         time.Time       `json:"captured_at"`
	Interface          string          `json:"interface"`
	TopLimit           int             `json:"top_limit"`
	SampleSeconds      float64         `json:"sample_seconds"`
	BandwidthAvailable bool            `json:"bandwidth_available"`
	Groups             []SnapshotGroup `json:"groups"`
	Version            string          `json:"version"`
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
	RemovedByAge int
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
	PruneEverySnapshots int
}

func DefaultIntervalSeconds() int { return defaultIntervalSeconds }
func DefaultTopLimit() int        { return defaultTopLimit }
func DefaultRetentionHours() int  { return defaultRetentionHours }

func DefaultWriterConfig(dataDir string) WriterConfig {
	return WriterConfig{
		DataDir:             dataDir,
		RetentionHours:      defaultRetentionHours,
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
