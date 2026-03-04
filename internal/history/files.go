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
	segmentPrefix = "connections-"
	segmentSuffix = ".jsonl"
	segmentLayout = "20060102-15"
	lockFileName  = ".daemon.lock"
)

type segmentFile struct {
	Path      string
	Name      string
	Timestamp time.Time
}

func segmentFileName(t time.Time) string {
	return segmentPrefix + t.UTC().Format(segmentLayout) + segmentSuffix
}

func parseSegmentTime(name string) (time.Time, bool) {
	if !strings.HasPrefix(name, segmentPrefix) || !strings.HasSuffix(name, segmentSuffix) {
		return time.Time{}, false
	}
	stamp := strings.TrimSuffix(strings.TrimPrefix(name, segmentPrefix), segmentSuffix)
	ts, err := time.ParseInLocation(segmentLayout, stamp, time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
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
		ts, ok := parseSegmentTime(entry.Name())
		if !ok {
			continue
		}
		items = append(items, segmentFile{
			Path:      filepath.Join(dataDir, entry.Name()),
			Name:      entry.Name(),
			Timestamp: ts,
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
