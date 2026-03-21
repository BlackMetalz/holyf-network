package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
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
	if h.traceOnlyMode {
		h.traceTimelineTotal = len(h.traceReplayEntries)
		h.traceTimelineAssociated = len(h.traceReplayEntries)
		for i, entry := range h.traceReplayEntries {
			h.traceTimelineBySnapshot[i] = []traceHistoryEntry{entry}
		}
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

func (h *HistoryApp) buildTraceOnlyRefs() ([]history.SnapshotRef, []traceHistoryEntry) {
	entries, err := readTraceHistoryEntriesFromDir(h.dataDir)
	if err != nil || len(entries) == 0 {
		return nil, nil
	}
	entries = h.filterTraceEntriesByReplayRange(entries)
	if len(entries) == 0 {
		return nil, nil
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].CapturedAt.Before(entries[j].CapturedAt)
	})

	refs := make([]history.SnapshotRef, 0, len(entries))
	for i, entry := range entries {
		if entry.CapturedAt.IsZero() {
			continue
		}
		refs = append(refs, history.SnapshotRef{
			FilePath:      "trace-history",
			Offset:        int64(i),
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
	trimmed := make([]traceHistoryEntry, 0, len(refs))
	for _, ref := range refs {
		idx := int(ref.Offset)
		if idx >= 0 && idx < len(entries) {
			trimmed = append(trimmed, entries[idx])
		}
	}
	return refs, trimmed
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
		if h.traceOnlyMode {
			return "  [dim]Trace timeline: 0 events at current slot[white]\n\n"
		}
		return fmt.Sprintf(
			"  [dim]Trace timeline: 0 events near this snapshot (mapped window ~%s)[white]\n\n",
			formatApproxDuration(h.traceTimelineWindow),
		)
	}

	var b strings.Builder
	if h.traceOnlyMode {
		b.WriteString(fmt.Sprintf(
			"  [yellow]Trace timeline: %d event(s) at current slot (trace-only fallback)[white]\n",
			len(events),
		))
	} else {
		b.WriteString(fmt.Sprintf(
			"  [yellow]Trace timeline: %d event(s) near this snapshot (window ~%s)[white]\n",
			len(events),
			formatApproxDuration(h.traceTimelineWindow),
		))
	}

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

func (h *HistoryApp) renderTraceReplayViewBody() string {
	events := h.currentTraceTimelineEvents()
	if len(events) == 0 {
		return "  [yellow]No trace events at current replay position[white]\n\n  [dim]Use [ / ] to move timeline, g=connections view, h=open trace history.[white]"
	}

	var b strings.Builder
	toggleHint := "g=connections view"
	if h.traceOnlyMode {
		toggleHint = "g=connections view (disabled in trace-only)"
	}
	b.WriteString(fmt.Sprintf("  [dim]Trace View: Up/Down not required | [ / ] timeline | %s | h=open trace history[white]\n\n", toggleHint))
	b.WriteString("  [dim]TIME     SEV   CAT           TARGET                 STATUS               ISSUE[white]\n")

	maxRows := h.topDisplayLimit()
	if maxRows < 5 {
		maxRows = 5
	}

	limit := maxRows
	if len(events) < limit {
		limit = len(events)
	}
	for i := 0; i < limit; i++ {
		entry := events[i]
		severity := strings.ToUpper(blankIfUnknown(entry.Severity, traceSeverityInfo))
		sevColor := tracePacketSeverityColor(severity)
		category := truncateRight(traceHistoryCategory(entry), 12)
		peer := formatPreviewIP(entry.PeerIP, h.sensitiveIP)
		target := truncateRight(fmt.Sprintf("%s:%s", peer, traceHistoryPortLabel(entry.Port)), 22)
		status := truncateRight(blankIfUnknown(entry.Status, "completed"), 20)
		issue := truncateRight(maskSensitiveIPsInText(blankIfUnknown(entry.Issue, "n/a"), h.sensitiveIP), 72)
		b.WriteString(fmt.Sprintf(
			"  [dim]%-8s[white] [%s]%-5s[white] [aqua]%-12s[white] %-22s %-20s %s\n",
			entry.CapturedAt.Local().Format("15:04:05"),
			sevColor,
			severity,
			category,
			target,
			status,
			issue,
		))
	}
	if len(events) > limit {
		b.WriteString(fmt.Sprintf("  [dim]... and %d more trace events[white]\n", len(events)-limit))
	}
	return b.String()
}
