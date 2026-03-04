package tui

import (
	"github.com/BlackMetalz/holyf-network/internal/collector"
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
