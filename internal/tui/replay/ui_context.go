package replay

import (
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
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

	SnapshotRefs() []history.SnapshotRef
	CurrentIndex() int
}
