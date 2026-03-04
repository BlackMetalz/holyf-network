package tui

import (
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (a *App) topConnectionsDisplayLimit() int {
	if a.zoomed {
		return 100
	}

	// Use panel height so Top Connections (normal + group view) scales with layout.
	if len(a.panels) <= 2 || a.panels[2] == nil {
		return 20
	}

	_, _, _, height := a.panels[2].GetInnerRect()
	if height <= 0 {
		return 20
	}

	// Reserve lines for chips/help text, table headers, and footer.
	limit := height - 7
	if limit < 5 {
		return 5
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func (a *App) visibleTopConnections() []collector.Connection {
	if len(a.latestTalkers) == 0 {
		return nil
	}

	filtered := a.applyTopConnectionFilters(a.latestTalkers)
	if len(filtered) == 0 {
		return nil
	}
	// Copy before sorting to avoid mutating latestTalkers backing array.
	items := append([]collector.Connection(nil), filtered...)
	sortConnections(items, a.sortMode)

	limit := a.topConnectionsDisplayLimit()
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (a *App) visiblePeerGroups() []PeerGroup {
	if len(a.latestTalkers) == 0 {
		return nil
	}

	filtered := applyGroupConnectionFilters(a.latestTalkers, a.portFilter, a.textFilter)
	if len(filtered) == 0 {
		return nil
	}

	groups := buildPeerGroups(filtered)
	limit := a.topConnectionsDisplayLimit()
	if len(groups) > limit {
		groups = groups[:limit]
	}
	return groups
}

func (a *App) visibleTopConnectionCount() int {
	if a.groupView {
		return len(a.visiblePeerGroups())
	}
	return len(a.visibleTopConnections())
}

func (a *App) clampTopConnectionSelection() {
	count := a.visibleTopConnectionCount()
	if count == 0 {
		a.selectedTalkerIndex = 0
		return
	}
	if a.selectedTalkerIndex < 0 {
		a.selectedTalkerIndex = 0
		return
	}
	if a.selectedTalkerIndex >= count {
		a.selectedTalkerIndex = count - 1
	}
}

func (a *App) renderTopConnectionsPanel() {
	a.clampTopConnectionSelection()
	if a.groupView {
		a.panels[2].SetText(renderPeerGroupPanel(
			a.latestTalkers,
			a.portFilter,
			a.textFilter,
			a.topConnectionsDisplayLimit(),
			a.sensitiveIP,
			a.selectedTalkerIndex,
		))
		return
	}
	a.panels[2].SetText(renderTalkersPanel(
		a.latestTalkers,
		a.portFilter,
		a.textFilter,
		a.topConnectionsDisplayLimit(),
		a.sensitiveIP,
		a.selectedTalkerIndex,
		a.sortMode,
	))
}

func (a *App) moveTopConnectionSelection(delta int) bool {
	count := a.visibleTopConnectionCount()
	if count == 0 {
		return false
	}
	a.clampTopConnectionSelection()

	next := a.selectedTalkerIndex + delta
	if next < 0 {
		next = 0
	}
	if next >= count {
		next = count - 1
	}
	if next == a.selectedTalkerIndex {
		return true
	}

	a.selectedTalkerIndex = next
	a.renderTopConnectionsPanel()
	a.updateStatusBar()
	return true
}

func (a *App) applyTopConnectionFilters(conns []collector.Connection) []collector.Connection {
	filtered := conns
	if a.portFilter != "" {
		filtered = filterByPort(filtered, a.portFilter)
	}
	if a.textFilter != "" {
		filtered = filterByText(filtered, a.textFilter)
	}
	return filtered
}

// promptPortFilter shows a simple input dialog for port filtering.
// Uses tview.InputField as a modal overlay.
func (a *App) promptPortFilter() {
	// If any filter is active, clear all filters.
	if a.portFilter != "" || a.textFilter != "" {
		a.portFilter = ""
		a.textFilter = ""
		a.selectedTalkerIndex = 0
		a.refreshData()
		return
	}

	// Create input field
	input := tview.NewInputField()
	input.SetLabel("Filter by port: ")
	input.SetFieldWidth(10)
	input.SetBorder(true)
	input.SetTitle(" Port Filter ")

	// Accept only numbers
	input.SetAcceptanceFunc(tview.InputFieldInteger)

	// On Enter: set filter, close dialog, refresh
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			a.portFilter = input.GetText()
			a.selectedTalkerIndex = 0
		}
		// On Enter or Escape: close the dialog
		a.pages.RemovePage("filter")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
		a.refreshData()
	})

	// Center the input field using Flex spacers
	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 30, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("filter", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(input)
}

func (a *App) promptTextFilter() {
	if a.focusIndex != 2 {
		a.setStatusNote("Focus Top Connections before search", 4*time.Second)
		return
	}

	input := tview.NewInputField()
	input.SetLabel("Search (contains): ")
	input.SetFieldWidth(36)
	input.SetText(a.textFilter)
	input.SetBorder(true)
	input.SetTitle(" Search Filter ")

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			entered := strings.TrimSpace(input.GetText())
			if entered == "" {
				a.portFilter = ""
				a.textFilter = ""
			} else {
				a.textFilter = entered
			}
			a.selectedTalkerIndex = 0
			a.refreshData()
		}
		a.pages.RemovePage("search")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 54, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("search", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(input)
}
