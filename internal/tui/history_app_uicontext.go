package tui

import (
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
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

func (h *HistoryApp) SnapshotRefs() []history.SnapshotRef            { return h.refs }
func (h *HistoryApp) CurrentIndex() int                              { return h.currentIndex }
func (h *HistoryApp) TopDirection() tuishared.TopConnectionDirection { return h.topDirection }
