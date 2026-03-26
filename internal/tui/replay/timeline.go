package replay

import (
	"sort"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
	tuitrace "github.com/BlackMetalz/holyf-network/internal/tui/trace"
)

type TraceTimeline struct {
	BySnapshot map[int][]tuitrace.Entry
	Total      int
	Associated int
	Window     time.Duration
}

func FilterSnapshotRefsByRange(refs []history.SnapshotRef, begin, end *time.Time) []history.SnapshotRef {
	if (begin == nil && end == nil) || len(refs) == 0 {
		return refs
	}

	filtered := make([]history.SnapshotRef, 0, len(refs))
	for _, ref := range refs {
		if begin != nil && ref.CapturedAt.Before(*begin) {
			continue
		}
		if end != nil && ref.CapturedAt.After(*end) {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

func FilterTraceEntriesByRange(entries []tuitrace.Entry, begin, end *time.Time) []tuitrace.Entry {
	if (begin == nil && end == nil) || len(entries) == 0 {
		return entries
	}

	filtered := make([]tuitrace.Entry, 0, len(entries))
	for _, entry := range entries {
		ts := entry.CapturedAt
		if ts.IsZero() {
			continue
		}
		if begin != nil && ts.Before(*begin) {
			continue
		}
		if end != nil && ts.After(*end) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func LoadTraceHistoryEntries(dataDir string, begin, end *time.Time, newestFirst bool) ([]tuitrace.Entry, error) {
	entries, err := tuitrace.ReadEntriesFromDir(dataDir)
	if err != nil || len(entries) == 0 {
		return entries, err
	}
	entries = FilterTraceEntriesByRange(entries, begin, end)
	if len(entries) == 0 {
		return nil, nil
	}
	if newestFirst {
		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].CapturedAt.After(entries[j].CapturedAt)
		})
	} else {
		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].CapturedAt.Before(entries[j].CapturedAt)
		})
	}
	return entries, nil
}

func BuildTraceOnlyRefs(entries []tuitrace.Entry) ([]history.SnapshotRef, []tuitrace.Entry) {
	if len(entries) == 0 {
		return nil, nil
	}

	sorted := append([]tuitrace.Entry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CapturedAt.Before(sorted[j].CapturedAt)
	})

	refs := make([]history.SnapshotRef, 0, len(sorted))
	trimmed := make([]tuitrace.Entry, 0, len(sorted))
	for _, entry := range sorted {
		if entry.CapturedAt.IsZero() {
			continue
		}
		trimmed = append(trimmed, entry)
		idx := len(trimmed) - 1
		refs = append(refs, history.SnapshotRef{
			FilePath:      "trace-history",
			Offset:        int64(idx),
			CapturedAt:    entry.CapturedAt,
			IncomingCount: 1,
			OutgoingCount: 1,
			TotalCount:    1,
			ConnCount:     1,
		})
	}
	if len(refs) == 0 {
		return nil, nil
	}
	return refs, trimmed
}

func LoadTraceOnlyRefs(dataDir string, begin, end *time.Time) ([]history.SnapshotRef, []tuitrace.Entry, error) {
	entries, err := LoadTraceHistoryEntries(dataDir, begin, end, false)
	if err != nil || len(entries) == 0 {
		return nil, nil, err
	}
	refs, trimmed := BuildTraceOnlyRefs(entries)
	return refs, trimmed, nil
}

func TraceTimelineAssociationWindow(refs []history.SnapshotRef) time.Duration {
	if len(refs) < 2 {
		return 45 * time.Second
	}
	gaps := make([]time.Duration, 0, len(refs)-1)
	for i := 1; i < len(refs); i++ {
		gap := refs[i].CapturedAt.Sub(refs[i-1].CapturedAt)
		if gap > 0 {
			gaps = append(gaps, gap)
		}
	}
	if len(gaps) == 0 {
		return 45 * time.Second
	}

	sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
	median := gaps[len(gaps)/2]
	window := median * 2
	if window < 15*time.Second {
		window = 15 * time.Second
	}
	if window > 15*time.Minute {
		window = 15 * time.Minute
	}
	return window
}

func BuildTraceOnlyTimeline(entries []tuitrace.Entry) TraceTimeline {
	result := TraceTimeline{BySnapshot: make(map[int][]tuitrace.Entry)}
	if len(entries) == 0 {
		return result
	}
	result.Total = len(entries)
	result.Associated = len(entries)
	for i, entry := range entries {
		result.BySnapshot[i] = []tuitrace.Entry{entry}
	}
	return result
}

func BuildTraceTimeline(refs []history.SnapshotRef, entries []tuitrace.Entry) TraceTimeline {
	result := TraceTimeline{BySnapshot: make(map[int][]tuitrace.Entry)}
	if len(refs) == 0 || len(entries) == 0 {
		return result
	}

	result.Total = len(entries)
	result.Window = TraceTimelineAssociationWindow(refs)
	for _, entry := range entries {
		idx := ClosestSnapshotIndex(refs, entry.CapturedAt)
		if idx < 0 || idx >= len(refs) {
			continue
		}
		if result.Window > 0 {
			delta := refs[idx].CapturedAt.Sub(entry.CapturedAt)
			if delta < 0 {
				delta = -delta
			}
			if delta > result.Window {
				continue
			}
		}
		result.BySnapshot[idx] = append(result.BySnapshot[idx], entry)
		result.Associated++
	}

	for idx := range result.BySnapshot {
		sort.SliceStable(result.BySnapshot[idx], func(i, j int) bool {
			return result.BySnapshot[idx][i].CapturedAt.After(result.BySnapshot[idx][j].CapturedAt)
		})
	}
	return result
}
