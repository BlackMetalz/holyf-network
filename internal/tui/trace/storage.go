package trace

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	SegmentPrefix = "trace-history-"
	SegmentSuffix = ".jsonl"
	SegmentLayout = "20060102"
)

type Entry struct {
	CapturedAt time.Time `json:"captured_at"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	EndedAt    time.Time `json:"ended_at,omitempty"`

	Interface   string `json:"interface"`
	PeerIP      string `json:"peer_ip"`
	Port        int    `json:"port"`
	Mode        string `json:"mode,omitempty"`
	Scope       string `json:"scope"`
	Preset      string `json:"preset,omitempty"`
	Direction   string `json:"direction"`
	DurationSec int    `json:"duration_sec"`
	PacketCap   int    `json:"packet_cap"`
	Filter      string `json:"filter"`

	Status   string `json:"status"`
	Saved    bool   `json:"saved"`
	PCAPPath string `json:"pcap_path,omitempty"`

	Captured          int  `json:"captured"`
	CapturedEstimated bool `json:"captured_estimated,omitempty"`
	ReceivedByFilter  int  `json:"received_by_filter"`
	DroppedByKernel   int  `json:"dropped_by_kernel"`
	DecodedPackets    int  `json:"decoded_packets"`
	SynCount          int  `json:"syn_count"`
	SynAckCount       int  `json:"syn_ack_count"`
	RstCount          int  `json:"rst_count"`

	Severity   string `json:"severity"`
	Confidence string `json:"confidence"`
	Issue      string `json:"issue"`
	Signal     string `json:"signal"`
	Likely     string `json:"likely"`
	Check      string `json:"check_next"`

	CaptureErr string   `json:"capture_err,omitempty"`
	ReadErr    string   `json:"read_err,omitempty"`
	Sample     []string `json:"sample_packets,omitempty"`
}

func (e *Entry) UnmarshalJSON(data []byte) error {
	type entryAlias Entry
	var raw struct {
		entryAlias
		LegacyProfile string `json:"profile,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*e = Entry(raw.entryAlias)
	if strings.TrimSpace(e.Mode) == "" {
		e.Mode = strings.TrimSpace(raw.LegacyProfile)
	}
	return nil
}

func SegmentFileName(t time.Time) string {
	stamp := t.Local().Format(SegmentLayout)
	return SegmentPrefix + stamp + SegmentSuffix
}

type segmentFile struct {
	Path      string
	Name      string
	Timestamp time.Time
	Span      time.Duration
}

func parseSegmentWindow(name string) (time.Time, time.Duration, bool) {
	if !strings.HasPrefix(name, SegmentPrefix) || !strings.HasSuffix(name, SegmentSuffix) {
		return time.Time{}, 0, false
	}
	stamp := strings.TrimSuffix(strings.TrimPrefix(name, SegmentPrefix), SegmentSuffix)
	ts, err := time.ParseInLocation(SegmentLayout, stamp, time.Local)
	if err != nil {
		return time.Time{}, 0, false
	}
	return ts, 24 * time.Hour, true
}

func listSegmentFiles(dataDir string) ([]segmentFile, error) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	items := make([]segmentFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ts, span, ok := parseSegmentWindow(entry.Name())
		if !ok {
			continue
		}
		items = append(items, segmentFile{
			Path:      filepath.Join(dataDir, entry.Name()),
			Name:      entry.Name(),
			Timestamp: ts,
			Span:      span,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if !items[i].Timestamp.Equal(items[j].Timestamp) {
			return items[i].Timestamp.Before(items[j].Timestamp)
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func ReadEntriesFromDir(dataDir string) ([]Entry, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, nil
	}
	files, err := listSegmentFiles(dataDir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}

	all := make([]Entry, 0, 128)
	for _, file := range files {
		entries, err := ReadEntries(file.Path)
		if err != nil {
			continue
		}
		all = append(all, entries...)
	}
	if len(all) == 0 {
		return nil, nil
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].CapturedAt.Before(all[j].CapturedAt)
	})
	return all, nil
}

func PruneDataDirByAge(dataDir string, retentionHours int, now time.Time) error {
	if retentionHours < 1 {
		return nil
	}
	files, err := listSegmentFiles(dataDir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	now = now.Local()
	cutoff := now.Add(-time.Duration(retentionHours) * time.Hour)
	currentFile := SegmentFileName(now)

	for _, file := range files {
		if file.Name == currentFile {
			continue
		}
		segmentEnd := file.Timestamp
		if file.Span > 0 {
			segmentEnd = segmentEnd.Add(file.Span)
		}
		if segmentEnd.Before(cutoff) {
			_ = os.Remove(file.Path)
		}
	}
	return nil
}

func ReadEntries(path string) ([]Entry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	entries := make([]Entry, 0, 64)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.CapturedAt.IsZero() {
			if !entry.EndedAt.IsZero() {
				entry.CapturedAt = entry.EndedAt
			} else {
				entry.CapturedAt = entry.StartedAt
			}
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
