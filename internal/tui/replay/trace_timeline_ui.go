package replay

import (
	"fmt"
	"strings"

	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	tuitrace "github.com/BlackMetalz/holyf-network/internal/tui/trace"
)

const historyTracePreviewLimit = 3

func RebuildTraceTimeline(ctx UIContext) {
	refs := ctx.SnapshotRefs()
	if len(refs) == 0 {
		ctx.SetTraceTimeline(make(map[int][]tuitrace.Entry), 0, 0, 0)
		return
	}
	if ctx.TraceOnlyMode() {
		timeline := BuildTraceOnlyTimeline(ctx.TraceReplayEntries())
		ctx.SetTraceTimeline(timeline.BySnapshot, timeline.Total, timeline.Associated, timeline.Window)
		return
	}

	entries, err := LoadTraceHistoryEntries(ctx.DataDir(), ctx.RangeBegin(), ctx.RangeEnd(), false)
	if err != nil || len(entries) == 0 {
		ctx.SetTraceTimeline(make(map[int][]tuitrace.Entry), 0, 0, 0)
		return
	}

	timeline := BuildTraceTimeline(refs, entries)
	ctx.SetTraceTimeline(timeline.BySnapshot, timeline.Total, timeline.Associated, timeline.Window)
}

func CurrentTraceTimelineEvents(ctx UIContext) []tuitrace.Entry {
	bySnapshot := ctx.TraceTimelineBySnapshot()
	idx := ctx.CurrentIndex()
	if len(bySnapshot) == 0 || idx < 0 {
		return nil
	}
	events := bySnapshot[idx]
	if len(events) == 0 {
		return nil
	}
	return events
}

func CurrentTraceTimelineCount(ctx UIContext) int {
	return len(CurrentTraceTimelineEvents(ctx))
}

func RenderCurrentTraceTimelineSection(ctx UIContext) string {
	associated := ctx.TraceTimelineAssociated()
	if associated <= 0 {
		return ""
	}

	events := CurrentTraceTimelineEvents(ctx)
	window := ctx.TraceTimelineWindow()
	traceOnly := ctx.TraceOnlyMode()
	sensitiveIP := ctx.SensitiveIP()

	if len(events) == 0 {
		if traceOnly {
			return "  [dim]Trace timeline: 0 events at current slot[white]\n\n"
		}
		return fmt.Sprintf(
			"  [dim]Trace timeline: 0 events near this snapshot (mapped window ~%s)[white]\n\n",
			tuishared.FormatApproxDuration(window),
		)
	}

	var b strings.Builder
	if traceOnly {
		b.WriteString(fmt.Sprintf(
			"  [yellow]Trace timeline: %d event(s) at current slot (trace-only fallback)[white]\n",
			len(events),
		))
	} else {
		b.WriteString(fmt.Sprintf(
			"  [yellow]Trace timeline: %d event(s) near this snapshot (window ~%s)[white]\n",
			len(events),
			tuishared.FormatApproxDuration(window),
		))
	}

	limit := historyTracePreviewLimit
	if len(events) < limit {
		limit = len(events)
	}
	for i := 0; i < limit; i++ {
		entry := events[i]
		peer := tuishared.FormatPreviewIP(entry.PeerIP, sensitiveIP)
		issue := tuishared.ShortStatus(tuishared.MaskSensitiveIPsInText(tuishared.BlankIfUnknown(entry.Issue, "n/a"), sensitiveIP), 78)
		severity := tuishared.BlankIfUnknown(strings.ToUpper(strings.TrimSpace(entry.Severity)), "INFO")
		b.WriteString(fmt.Sprintf(
			"  [dim]%s[white] [%s]%s[white] [aqua]%s[white] %s:%s | %s\n",
			entry.CapturedAt.Local().Format("15:04:05"),
			tuishared.TracePacketSeverityColor(severity),
			severity,
			tuishared.TraceHistoryCategory(entry),
			peer,
			tuishared.TraceHistoryPortLabel(entry.Port),
			issue,
		))
	}
	if len(events) > limit {
		b.WriteString(fmt.Sprintf("  [dim]... and %d more trace events[white]\n", len(events)-limit))
	}
	b.WriteString("\n")
	return b.String()
}

func RenderTraceReplayViewBody(ctx UIContext) string {
	events := CurrentTraceTimelineEvents(ctx)
	if len(events) == 0 {
		return "  [yellow]No trace events at current replay position[white]\n\n  [dim]Use [ / ] to move timeline, g=connections view, h=open trace history.[white]"
	}

	var b strings.Builder
	toggleHint := "g=connections view"
	if ctx.TraceOnlyMode() {
		toggleHint = "g=connections view (disabled in trace-only)"
	}
	b.WriteString(fmt.Sprintf("  [dim]Trace View: Up/Down not required | [ / ] timeline | %s | h=open trace history[white]\n\n", toggleHint))
	b.WriteString("  [dim]TIME     SEV   CAT           TARGET                 STATUS               ISSUE[white]\n")

	maxRows := ctx.TopDisplayLimit()
	if maxRows < 5 {
		maxRows = 5
	}

	limit := maxRows
	if len(events) < limit {
		limit = len(events)
	}
	sensitiveIP := ctx.SensitiveIP()
	for i := 0; i < limit; i++ {
		entry := events[i]
		severity := strings.ToUpper(tuishared.BlankIfUnknown(entry.Severity, "INFO"))
		sevColor := tuishared.TracePacketSeverityColor(severity)
		category := tuishared.TruncateRight(tuishared.TraceHistoryCategory(entry), 12)
		peer := tuishared.FormatPreviewIP(entry.PeerIP, sensitiveIP)
		target := tuishared.TruncateRight(fmt.Sprintf("%s:%s", peer, tuishared.TraceHistoryPortLabel(entry.Port)), 22)
		status := tuishared.TruncateRight(tuishared.BlankIfUnknown(entry.Status, "completed"), 20)
		issue := tuishared.TruncateRight(tuishared.MaskSensitiveIPsInText(tuishared.BlankIfUnknown(entry.Issue, "n/a"), sensitiveIP), 72)
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
