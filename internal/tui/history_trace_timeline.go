package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const historyTracePreviewLimit = 3

func (h *HistoryApp) rebuildTraceTimeline() {
	h.traceTimelineBySnapshot = make(map[int][]traceHistoryEntry)
	h.traceTimelineTotal = 0
	h.traceTimelineAssociated = 0
	h.traceTimelineWindow = 0

	if len(h.refs) == 0 {
		return
	}

	entries, err := readTraceHistoryEntriesFromDir(h.dataDir)
	if err != nil || len(entries) == 0 {
		return
	}

	entries = h.filterTraceEntriesByReplayRange(entries)
	if len(entries) == 0 {
		return
	}
	h.traceTimelineTotal = len(entries)
	h.traceTimelineWindow = h.traceTimelineAssociationWindow()

	for _, entry := range entries {
		idx := h.closestSnapshotIndex(entry.CapturedAt)
		if idx < 0 || idx >= len(h.refs) {
			continue
		}
		if h.traceTimelineWindow > 0 {
			delta := h.refs[idx].CapturedAt.Sub(entry.CapturedAt)
			if delta < 0 {
				delta = -delta
			}
			if delta > h.traceTimelineWindow {
				continue
			}
		}
		h.traceTimelineBySnapshot[idx] = append(h.traceTimelineBySnapshot[idx], entry)
		h.traceTimelineAssociated++
	}

	for idx := range h.traceTimelineBySnapshot {
		sort.SliceStable(h.traceTimelineBySnapshot[idx], func(i, j int) bool {
			return h.traceTimelineBySnapshot[idx][i].CapturedAt.After(h.traceTimelineBySnapshot[idx][j].CapturedAt)
		})
	}
}

func (h *HistoryApp) filterTraceEntriesByReplayRange(entries []traceHistoryEntry) []traceHistoryEntry {
	if !h.hasReplayRange() || len(entries) == 0 {
		return entries
	}

	filtered := make([]traceHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		ts := entry.CapturedAt
		if ts.IsZero() {
			continue
		}
		if h.rangeBegin != nil && ts.Before(*h.rangeBegin) {
			continue
		}
		if h.rangeEnd != nil && ts.After(*h.rangeEnd) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func (h *HistoryApp) traceTimelineAssociationWindow() time.Duration {
	if len(h.refs) < 2 {
		return 45 * time.Second
	}
	gaps := make([]time.Duration, 0, len(h.refs)-1)
	for i := 1; i < len(h.refs); i++ {
		gap := h.refs[i].CapturedAt.Sub(h.refs[i-1].CapturedAt)
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

func (h *HistoryApp) currentTraceTimelineEvents() []traceHistoryEntry {
	if len(h.traceTimelineBySnapshot) == 0 || h.currentIndex < 0 {
		return nil
	}
	events := h.traceTimelineBySnapshot[h.currentIndex]
	if len(events) == 0 {
		return nil
	}
	return events
}

func (h *HistoryApp) currentTraceTimelineCount() int {
	return len(h.currentTraceTimelineEvents())
}

func (h *HistoryApp) renderCurrentTraceTimelineSection() string {
	if h.traceTimelineAssociated <= 0 {
		return ""
	}

	events := h.currentTraceTimelineEvents()
	if len(events) == 0 {
		return fmt.Sprintf(
			"  [dim]Trace timeline: 0 events near this snapshot (mapped window ~%s)[white]\n\n",
			formatApproxDuration(h.traceTimelineWindow),
		)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		"  [yellow]Trace timeline: %d event(s) near this snapshot (window ~%s)[white]\n",
		len(events),
		formatApproxDuration(h.traceTimelineWindow),
	))

	limit := historyTracePreviewLimit
	if len(events) < limit {
		limit = len(events)
	}
	for i := 0; i < limit; i++ {
		entry := events[i]
		peer := formatPreviewIP(entry.PeerIP, h.sensitiveIP)
		issue := shortStatus(maskSensitiveIPsInText(blankIfUnknown(entry.Issue, "n/a"), h.sensitiveIP), 78)
		severity := blankIfUnknown(strings.ToUpper(strings.TrimSpace(entry.Severity)), "INFO")
		b.WriteString(fmt.Sprintf(
			"  [dim]%s[white] [%s]%s[white] [aqua]%s[white] %s:%s | %s\n",
			entry.CapturedAt.Local().Format("15:04:05"),
			tracePacketSeverityColor(severity),
			severity,
			traceHistoryCategory(entry),
			peer,
			traceHistoryPortLabel(entry.Port),
			issue,
		))
	}
	if len(events) > limit {
		b.WriteString(fmt.Sprintf("  [dim]... and %d more trace events[white]\n", len(events)-limit))
	}
	b.WriteString("\n")
	return b.String()
}
