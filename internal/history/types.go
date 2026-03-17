package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
	Port             int            `json:"port"`
	LocalPort        int            `json:"-"`
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
	TopLimitPerSide    int             `json:"top_limit_per_side"`
	TopLimit           int             `json:"-"`
	SampleSeconds      float64         `json:"sample_seconds"`
	BandwidthAvailable bool            `json:"bandwidth_available"`
	IncomingGroups     []SnapshotGroup `json:"incoming_groups"`
	OutgoingGroups     []SnapshotGroup `json:"outgoing_groups"`
	Groups             []SnapshotGroup `json:"-"`
	Version            string          `json:"version"`
}

// SnapshotRef points to one snapshot line in a segment file.
type SnapshotRef struct {
	FilePath      string
	Offset        int64
	CapturedAt    time.Time
	IncomingCount int
	OutgoingCount int
	TotalCount    int
	ConnCount     int
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
	if shouldUseSystemDefaultPaths(runtime.GOOS, os.Geteuid()) {
		return "/var/lib/holyf-network/snapshots"
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".holyf-network", snapshotDirName)
}

func shouldUseSystemDefaultPaths(goos string, euid int) bool {
	return goos == "linux" && euid == 0
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

func (g *SnapshotGroup) normalizeAliases() {
	if g == nil {
		return
	}
	if g.Port == 0 && g.LocalPort != 0 {
		g.Port = g.LocalPort
	}
	if g.LocalPort == 0 && g.Port != 0 {
		g.LocalPort = g.Port
	}
	if g.States == nil {
		g.States = make(map[string]int)
	}
}

func cloneSnapshotGroups(groups []SnapshotGroup) []SnapshotGroup {
	if len(groups) == 0 {
		return make([]SnapshotGroup, 0)
	}
	cloned := make([]SnapshotGroup, len(groups))
	copy(cloned, groups)
	for i := range cloned {
		cloned[i].normalizeAliases()
	}
	return cloned
}

func normalizeSnapshotRecord(record *SnapshotRecord) {
	if record == nil {
		return
	}
	if record.TopLimitPerSide == 0 && record.TopLimit > 0 {
		record.TopLimitPerSide = record.TopLimit
	}
	if record.TopLimit == 0 {
		record.TopLimit = record.TopLimitPerSide
	}
	if len(record.IncomingGroups) == 0 && len(record.OutgoingGroups) == 0 && record.Groups != nil {
		record.IncomingGroups = cloneSnapshotGroups(record.Groups)
	}
	record.IncomingGroups = cloneSnapshotGroups(record.IncomingGroups)
	record.OutgoingGroups = cloneSnapshotGroups(record.OutgoingGroups)
	record.Groups = cloneSnapshotGroups(record.IncomingGroups)
}

func NormalizeSnapshotRecord(record *SnapshotRecord) {
	normalizeSnapshotRecord(record)
}

func (g *SnapshotGroup) UnmarshalJSON(data []byte) error {
	type snapshotGroupAlias SnapshotGroup
	var raw snapshotGroupAlias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*g = SnapshotGroup(raw)
	g.normalizeAliases()
	return nil
}

func (r *SnapshotRecord) UnmarshalJSON(data []byte) error {
	type snapshotRecordAlias SnapshotRecord
	var raw snapshotRecordAlias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = SnapshotRecord(raw)
	normalizeSnapshotRecord(r)
	return nil
}
