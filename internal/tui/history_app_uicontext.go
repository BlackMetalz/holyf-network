package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	tuitrace "github.com/BlackMetalz/holyf-network/internal/tui/trace"
	"github.com/rivo/tview"
)

// UIContext Implementation

func (h *HistoryApp) DataDir() string        { return h.dataDir }
func (h *HistoryApp) RangeBegin() *time.Time { return h.rangeBegin }
func (h *HistoryApp) RangeEnd() *time.Time   { return h.rangeEnd }
func (h *HistoryApp) RangeLabel() string     { return h.replayRangeLabel() }
func (h *HistoryApp) SensitiveIP() bool      { return h.sensitiveIP }

func (h *HistoryApp) AddPage(name string, item tview.Primitive, resize, visible bool) {
	h.pages.AddPage(name, item, resize, visible)
}
func (h *HistoryApp) RemovePage(name string)                      { h.pages.RemovePage(name) }
func (h *HistoryApp) SetFocus(p tview.Primitive)                  { h.app.SetFocus(p) }
func (h *HistoryApp) BackFocus()                                  { h.app.SetFocus(h.panel) }
func (h *HistoryApp) UpdateStatusBar()                            { h.updateStatusBar() }
func (h *HistoryApp) SetStatusNote(msg string, ttl time.Duration) { h.setStatusNote(msg, ttl) }
func (h *HistoryApp) TopDisplayLimit() int                        { return h.topDisplayLimit() }

func (h *HistoryApp) TraceHistoryStorageSummary() string {
	dir := strings.TrimSpace(h.dataDir)
	if dir == "" {
		return fmt.Sprintf("trace history storage unavailable | retention=%dh", history.DefaultRetentionHours())
	}
	return fmt.Sprintf(
		"data-dir=%s | file=%sYYYYMMDD%s | retention=%dh",
		dir,
		tuitrace.SegmentPrefix,
		tuitrace.SegmentSuffix,
		history.DefaultRetentionHours(),
	)
}

func (h *HistoryApp) TraceTimelineBySnapshot() map[int][]tuitrace.Entry {
	return h.traceTimelineBySnapshot
}
func (h *HistoryApp) TraceTimelineTotal() int                        { return h.traceTimelineTotal }
func (h *HistoryApp) TraceTimelineAssociated() int                   { return h.traceTimelineAssociated }
func (h *HistoryApp) TraceTimelineWindow() time.Duration             { return h.traceTimelineWindow }
func (h *HistoryApp) TraceOnlyMode() bool                            { return h.traceOnlyMode }
func (h *HistoryApp) TraceReplayEntries() []tuitrace.Entry           { return h.traceReplayEntries }
func (h *HistoryApp) SnapshotRefs() []history.SnapshotRef            { return h.refs }
func (h *HistoryApp) CurrentIndex() int                              { return h.currentIndex }
func (h *HistoryApp) TopDirection() tuishared.TopConnectionDirection { return h.topDirection }

func (h *HistoryApp) SetTraceTimeline(bySnapshot map[int][]tuitrace.Entry, total, associated int, window time.Duration) {
	h.traceTimelineBySnapshot = bySnapshot
	h.traceTimelineTotal = total
	h.traceTimelineAssociated = associated
	h.traceTimelineWindow = window
}
