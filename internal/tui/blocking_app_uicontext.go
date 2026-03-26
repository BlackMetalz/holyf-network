package tui

import (
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	"github.com/rivo/tview"
)

func (a *App) AddPage(name string, item tview.Primitive, resize, visible bool) {
	a.pages.AddPage(name, item, resize, visible)
}

func (a *App) RemovePage(name string) {
	a.pages.RemovePage(name)
}

func (a *App) SendToFront(name string) {
	a.pages.SendToFront(name)
}

func (a *App) SetFocus(p tview.Primitive) {
	a.app.SetFocus(p)
}

func (a *App) RestoreFocus() {
	if a.focusIndex >= 0 && a.focusIndex < len(a.panels) {
		a.app.SetFocus(a.panels[a.focusIndex])
	}
}

func (a *App) SetStatusNote(msg string, ttl time.Duration) {
	a.setStatusNote(msg, ttl)
}

func (a *App) AddActionLog(msg string) {
	a.addActionLog(msg)
}

func (a *App) QueueUpdateDraw(f func()) {
	a.app.QueueUpdateDraw(f)
}

func (a *App) UpdateStatusBar() {
	a.updateStatusBar()
}

func (a *App) RefreshData() {
	a.refreshData()
}

func (a *App) IsPaused() bool {
	return a.paused.Load()
}

func (a *App) SetPaused(paused bool) {
	a.paused.Store(paused)
}

func (a *App) TopDirection() tuishared.TopConnectionDirection {
	return a.topDirection
}

func (a *App) PortFilter() string {
	return a.portFilter
}

func (a *App) LatestTalkers() []collector.Connection {
	return a.latestTalkers
}
