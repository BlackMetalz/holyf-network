package history

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	segmentPrefix      = "connections-"
	segmentSuffix      = ".jsonl"
	segmentLayoutDaily = "20060102"
	lockFileName       = ".daemon.lock"
)

type segmentFile struct {
	Path      string
	Name      string
	Timestamp time.Time
	Span      time.Duration
}

func segmentFileName(t time.Time) string {
	// Segment names follow server local time to match operator expectation.
	return segmentPrefix + t.Local().Format(segmentLayoutDaily) + segmentSuffix
}

func parseSegmentTime(name string) (time.Time, bool) {
	ts, _, ok := parseSegmentWindow(name)
	return ts, ok
}

func parseSegmentWindow(name string) (time.Time, time.Duration, bool) {
	if !strings.HasPrefix(name, segmentPrefix) || !strings.HasSuffix(name, segmentSuffix) {
		return time.Time{}, 0, false
	}
	stamp := strings.TrimSuffix(strings.TrimPrefix(name, segmentPrefix), segmentSuffix)
	// Parse in server local timezone because filenames are local-time based.
	ts, err := time.ParseInLocation(segmentLayoutDaily, stamp, time.Local)
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
		return nil, fmt.Errorf("list data dir: %w", err)
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
