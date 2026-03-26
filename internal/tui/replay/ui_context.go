package replay

import (
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
	tuitrace "github.com/BlackMetalz/holyf-network/internal/tui/trace"
	"github.com/rivo/tview"
)

// UIContext provides the necessary application state and UI controls
// for replay components to render and interact with the main application.
type UIContext interface {
	DataDir() string
	RangeBegin() *time.Time
	RangeEnd() *time.Time
	RangeLabel() string
	SensitiveIP() bool

	AddPage(name string, item tview.Primitive, resize, visible bool)
	RemovePage(name string)
	SetFocus(p tview.Primitive)
	BackFocus()
	UpdateStatusBar()
	SetStatusNote(msg string, ttl time.Duration)
	TopDisplayLimit() int
	TraceHistoryStorageSummary() string

	// Trace Timeline State
	TraceTimelineBySnapshot() map[int][]tuitrace.Entry
	TraceTimelineTotal() int
	TraceTimelineAssociated() int
	TraceTimelineWindow() time.Duration
	TraceOnlyMode() bool
	TraceReplayEntries() []tuitrace.Entry
	SnapshotRefs() []history.SnapshotRef
	CurrentIndex() int
	SetTraceTimeline(bySnapshot map[int][]tuitrace.Entry, total, associated int, window time.Duration)
}
