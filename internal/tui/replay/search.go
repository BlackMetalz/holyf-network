package replay

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
)

type SearchResult struct {
	SnapshotIndex int
	CapturedAt    time.Time
	MatchCount    int
}

func ScanTimelineMatches(refs []history.SnapshotRef, matchCount func(history.SnapshotRef) (int, error)) []SearchResult {
	if len(refs) == 0 || matchCount == nil {
		return nil
	}

	results := make([]SearchResult, 0, 16)
	for i, ref := range refs {
		count, err := matchCount(ref)
		if err != nil || count <= 0 {
			continue
		}
		results = append(results, SearchResult{
			SnapshotIndex: i,
			CapturedAt:    ref.CapturedAt,
			MatchCount:    count,
		})
	}
	return results
}

func ClosestSnapshotIndex(refs []history.SnapshotRef, target time.Time) int {
	if len(refs) == 0 {
		return -1
	}

	targetUTC := target.UTC()
	idx := sort.Search(len(refs), func(i int) bool {
		return !refs[i].CapturedAt.Before(targetUTC)
	})

	if idx <= 0 {
		return 0
	}
	if idx >= len(refs) {
		return len(refs) - 1
	}

	before := refs[idx-1].CapturedAt
	after := refs[idx].CapturedAt
	if targetUTC.Sub(before) <= after.Sub(targetUTC) {
		return idx - 1
	}
	return idx
}

func ParsePortFilter(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	port, err := strconv.Atoi(trimmed)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid port filter")
	}
	return strconv.Itoa(port), nil
}
