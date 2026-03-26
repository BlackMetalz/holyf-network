package blocking

import (
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	"github.com/rivo/tview"
)

type UIContext interface {
	AddPage(name string, item tview.Primitive, resize, visible bool)
	RemovePage(name string)
	SendToFront(name string)
	SetFocus(p tview.Primitive)
	RestoreFocus()

	SetStatusNote(msg string, ttl time.Duration)
	AddActionLog(msg string)
	QueueUpdateDraw(f func())
	UpdateStatusBar()
	RefreshData()

	IsPaused() bool
	SetPaused(paused bool)

	TopDirection() tuishared.TopConnectionDirection
	PortFilter() string
	LatestTalkers() []collector.Connection
}
