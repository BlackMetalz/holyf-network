package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	defaultTopConnectionsPanelWidth  = 120
	defaultTopConnectionsPanelHeight = 27
	topConnectionsBaseReservedLines  = 7
	topConnectionsNoteReservedLines  = 1
	topConnectionsPreviewLines       = 5
	topConnectionsMinRows            = 5
	topConnectionsMinRowsWithPreview = 3
	topConnectionsGroupCap           = 20
)

type topConnectionsPanelLayout struct {
	PanelWidth  int
	PanelHeight int
	RowLimit    int
	ShowPreview bool
}

func topConnectionsPageBounds(totalRows, rowsPerPage, pageIndex int) (page, start, end, totalPages int) {
	if totalRows <= 0 {
		return 0, 0, 0, 0
	}
	if rowsPerPage <= 0 {
		rowsPerPage = 1
	}

	totalPages = (totalRows + rowsPerPage - 1) / rowsPerPage
	page = pageIndex
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start = page * rowsPerPage
	end = start + rowsPerPage
	if end > totalRows {
		end = totalRows
	}
	return page, start, end, totalPages
}

func calculateTopConnectionsDisplayLimit(panelHeight int, noteCount int, wantsPreview bool) (int, bool) {
	reservedLines := topConnectionsBaseReservedLines
	if noteCount > 0 {
		reservedLines += noteCount * topConnectionsNoteReservedLines
	}
	if wantsPreview {
		previewLimit := panelHeight - reservedLines - topConnectionsPreviewLines
		if previewLimit >= topConnectionsMinRowsWithPreview {
			return min(previewLimit, 100), true
		}
	}

	limit := panelHeight - reservedLines
	if limit < topConnectionsMinRows {
		limit = topConnectionsMinRows
	}
	if limit > 100 {
		limit = 100
	}
	return limit, false
}

func topConnectionsNoteCount(bandwidthNote string) int {
	if strings.TrimSpace(bandwidthNote) == "" {
		return 0
	}
	return 1
}

func (a *App) topConnectionsPanelSize() (int, int) {
	if len(a.panels) <= 2 || a.panels[2] == nil {
		return defaultTopConnectionsPanelWidth, defaultTopConnectionsPanelHeight
	}

	_, _, width, height := a.panels[2].GetInnerRect()
	if width <= 0 {
		width = defaultTopConnectionsPanelWidth
	}
	if height <= 0 {
		height = defaultTopConnectionsPanelHeight
	}
	return width, height
}

func (a *App) topConnectionsPanelLayout(hasRows bool) topConnectionsPanelLayout {
	width, height := a.topConnectionsPanelSize()
	rowLimit, showPreview := calculateTopConnectionsDisplayLimit(
		height,
		topConnectionsNoteCount(a.topBandwidthNote),
		hasRows,
	)
	return topConnectionsPanelLayout{
		PanelWidth:  width,
		PanelHeight: height,
		RowLimit:    rowLimit,
		ShowPreview: showPreview,
	}
}

func (a *App) filteredTopConnections() []collector.Connection {
	source := a.topConnectionsSource()
	if len(source) == 0 {
		return nil
	}

	filtered := a.applyTopConnectionFilters(source)
	if len(filtered) == 0 {
		return nil
	}

	items := append([]collector.Connection(nil), filtered...)
	sortConnectionsWithDirection(items, a.sortMode, a.sortDesc, a.topDirection)
	return items
}

func (a *App) filteredPeerGroups() []PeerGroup {
	source := a.topConnectionsSource()
	if len(source) == 0 {
		return nil
	}

	filtered := applyGroupConnectionFiltersByDirection(source, a.portFilter, a.textFilter, a.topDirection)
	if len(filtered) == 0 {
		return nil
	}

	groups := buildPeerGroupsWithDirection(filtered, a.sortDesc, a.topDirection)
	return limitPeerGroups(groups, topConnectionsGroupCap, a.sortDesc)
}

func (a *App) visibleTopConnections() []collector.Connection {
	items := a.filteredTopConnections()
	if len(items) == 0 {
		a.topPageIndex = 0
		return nil
	}

	limit := a.topConnectionsPanelLayout(true).RowLimit
	page, start, end, _ := topConnectionsPageBounds(len(items), limit, a.topPageIndex)
	a.topPageIndex = page
	return items[start:end]
}

func (a *App) visiblePeerGroups() []PeerGroup {
	groups := a.filteredPeerGroups()
	if len(groups) == 0 {
		a.topPageIndex = 0
		return nil
	}

	limit := a.topConnectionsPanelLayout(true).RowLimit
	page, start, end, _ := topConnectionsPageBounds(len(groups), limit, a.topPageIndex)
	a.topPageIndex = page
	return groups[start:end]
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

func (a *App) resetTopConnectionsCursor() {
	a.selectedTalkerIndex = 0
	a.topPageIndex = 0
}

func (a *App) moveTopConnectionPage(delta int) bool {
	if delta == 0 {
		return false
	}

	var total int
	if a.groupView {
		total = len(a.filteredPeerGroups())
	} else {
		total = len(a.filteredTopConnections())
	}
	if total == 0 {
		a.topPageIndex = 0
		return false
	}

	layout := a.topConnectionsPanelLayout(true)
	currentPage, _, _, totalPages := topConnectionsPageBounds(total, layout.RowLimit, a.topPageIndex)
	if totalPages <= 1 {
		a.topPageIndex = currentPage
		return false
	}

	nextPage := currentPage + delta
	if nextPage < 0 {
		nextPage = 0
	}
	if nextPage >= totalPages {
		nextPage = totalPages - 1
	}
	if nextPage == currentPage {
		return false
	}

	a.topPageIndex = nextPage
	a.selectedTalkerIndex = 0
	a.renderTopConnectionsPanel()
	a.updateStatusBar()
	return true
}

func (a *App) renderTopConnectionsPanel() {
	a.updateTopConnectionsPanelTitle()
	source := a.topConnectionsSource()
	if a.groupView {
		groups := a.filteredPeerGroups()
		layout := a.topConnectionsPanelLayout(len(groups) > 0)
		page, start, end, _ := topConnectionsPageBounds(len(groups), layout.RowLimit, a.topPageIndex)
		a.topPageIndex = page
		visibleGroups := groups[start:end]
		if count := len(visibleGroups); count == 0 {
			a.selectedTalkerIndex = 0
		} else if a.selectedTalkerIndex >= count {
			a.selectedTalkerIndex = count - 1
		} else if a.selectedTalkerIndex < 0 {
			a.selectedTalkerIndex = 0
		}

		var preview *selectedRowPreview
		if layout.ShowPreview {
			preview = a.buildSelectedPeerGroupPreview(visibleGroups)
		}

		a.panels[2].SetText(renderPeerGroupPanelWithPreviewDirection(
			source,
			a.portFilter,
			a.textFilter,
			layout.RowLimit,
			a.sensitiveIP,
			a.selectedTalkerIndex,
			a.sortDesc,
			a.healthThresholds,
			a.topBandwidthNote,
			layout.PanelWidth,
			preview,
			a.topPageIndex,
			a.topDirection,
		))
		return
	}

	items := a.filteredTopConnections()
	layout := a.topConnectionsPanelLayout(len(items) > 0)
	page, start, end, _ := topConnectionsPageBounds(len(items), layout.RowLimit, a.topPageIndex)
	a.topPageIndex = page
	visibleItems := items[start:end]
	if count := len(visibleItems); count == 0 {
		a.selectedTalkerIndex = 0
	} else if a.selectedTalkerIndex >= count {
		a.selectedTalkerIndex = count - 1
	} else if a.selectedTalkerIndex < 0 {
		a.selectedTalkerIndex = 0
	}

	var preview *selectedRowPreview
	if layout.ShowPreview {
		preview = a.buildSelectedConnectionPreview(visibleItems)
	}

	a.panels[2].SetText(renderTalkersPanelWithPreviewDirection(
		source,
		a.portFilter,
		a.textFilter,
		layout.RowLimit,
		a.sensitiveIP,
		a.selectedTalkerIndex,
		a.sortMode,
		a.sortDesc,
		a.healthThresholds,
		a.topBandwidthNote,
		layout.PanelWidth,
		preview,
		a.topPageIndex,
		a.topDirection,
	))
}

func (a *App) topConnectionsSource() []collector.Connection {
	if len(a.latestTalkers) == 0 {
		return nil
	}

	selfPID := os.Getpid()
	filtered := make([]collector.Connection, 0, len(a.latestTalkers))
	for _, conn := range a.latestTalkers {
		if selfPID > 0 && conn.PID == selfPID {
			continue
		}
		if a.listenPortsKnown {
			_, isListenerPort := a.listenPorts[conn.LocalPort]
			if a.topDirection == topConnectionOutgoing && isListenerPort {
				continue
			}
			if a.topDirection == topConnectionIncoming && !isListenerPort {
				continue
			}
		}
		filtered = append(filtered, conn)
	}
	return filtered
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
		filtered = filterByPortDirection(filtered, a.portFilter, a.topDirection)
	}
	if a.textFilter != "" {
		filtered = filterByText(filtered, a.textFilter)
	}
	return filtered
}

func (a *App) buildSelectedConnectionPreview(conns []collector.Connection) *selectedRowPreview {
	if len(conns) == 0 || a.selectedTalkerIndex < 0 || a.selectedTalkerIndex >= len(conns) {
		return nil
	}

	conn := conns[a.selectedTalkerIndex]
	peerIP := normalizeIP(conn.RemoteIP)
	target := peerKillTarget{
		PeerIP:    peerIP,
		LocalPort: conn.LocalPort,
		Count:     a.countPeerMatches(peerIP, conn.LocalPort),
	}

	actionLine := fmt.Sprintf(
		"Action: Enter/k => peer %s -> local %d (%d matches; peer+port scope)",
		formatPreviewIP(target.PeerIP, a.sensitiveIP),
		target.LocalPort,
		target.Count,
	)
	if a.topDirection == topConnectionOutgoing {
		actionLine = "Action: Enter/k disabled in OUT mode (incoming-only mitigation)"
	}

	return &selectedRowPreview{
		Title: "Selected Detail",
		Lines: []string{
			fmt.Sprintf(
				"Local: %s -> Peer: %s | State: %s",
				formatPreviewEndpoint(conn.LocalIP, conn.LocalPort, a.sensitiveIP),
				formatPreviewEndpoint(conn.RemoteIP, conn.RemotePort, a.sensitiveIP),
				conn.State,
			),
			fmt.Sprintf(
				"Proc: %s | Queue: send %s recv %s | BW: tx %s rx %s",
				formatProcessInfo(conn),
				formatBytes(conn.TxQueue),
				formatBytes(conn.RxQueue),
				formatBytesRateCompact(conn.TxBytesPerSec),
				formatBytesRateCompact(conn.RxBytesPerSec),
			),
			actionLine,
		},
	}
}

func (a *App) buildSelectedPeerGroupPreview(groups []PeerGroup) *selectedRowPreview {
	if len(groups) == 0 || a.selectedTalkerIndex < 0 || a.selectedTalkerIndex >= len(groups) {
		return nil
	}

	group := groups[a.selectedTalkerIndex]
	target, ok := a.selectedPeerPortTarget(group.PeerIP)
	if !ok {
		target = peerKillTarget{PeerIP: group.PeerIP}
	}

	portSet := group.LocalPorts
	portLabel := "Ports"
	actionLine := fmt.Sprintf(
		"%s: %s | Action: Enter/k => local %d (%d matches)",
		portLabel,
		formatAllPeerGroupPorts(portSet),
		target.LocalPort,
		target.Count,
	)
	if a.topDirection == topConnectionOutgoing {
		portSet = group.RemotePorts
		portLabel = "RPorts"
		actionLine = fmt.Sprintf(
			"%s: %s | Action: Enter/k disabled in OUT mode",
			portLabel,
			formatAllPeerGroupPorts(portSet),
		)
	}

	return &selectedRowPreview{
		Title: "Selected Detail",
		Lines: []string{
			fmt.Sprintf(
				"Peer: %s | Proc: %s | Conns: %d",
				formatPreviewIP(group.PeerIP, a.sensitiveIP),
				group.ProcName,
				group.Count,
			),
			fmt.Sprintf(
				"States: %s",
				formatPeerGroupStatePreview(group.States, group.Count),
			),
			actionLine,
		},
	}
}

func (a *App) updateTopConnectionsPanelTitle() {
	if len(a.panels) <= 2 || a.panels[2] == nil {
		return
	}
	a.panels[2].SetTitle(a.topDirection.PanelTitle())
}

func (a *App) toggleTopConnectionsDirection() {
	if a.topDirection == topConnectionOutgoing {
		a.topDirection = topConnectionIncoming
		a.setStatusNote("Top Connections: IN mode (local listener ports, Enter/k enabled)", 4*time.Second)
	} else {
		a.topDirection = topConnectionOutgoing
		a.setStatusNote("Top Connections: OUT mode (remote service ports, Enter/k disabled)", 4*time.Second)
	}
	a.resetTopConnectionsCursor()
	a.renderTopConnectionsPanel()
}

// promptPortFilter shows a simple input dialog for port filtering.
// Uses tview.InputField as a modal overlay.
func (a *App) promptPortFilter() {
	// If any filter is active, clear all filters.
	if a.portFilter != "" || a.textFilter != "" {
		a.portFilter = ""
		a.textFilter = ""
		a.resetTopConnectionsCursor()
		a.refreshData()
		return
	}

	// Create input field
	input := tview.NewInputField()
	if a.topDirection == topConnectionOutgoing {
		input.SetLabel("Filter by remote port: ")
		input.SetTitle(" Remote Port Filter ")
	} else {
		input.SetLabel("Filter by local port: ")
		input.SetTitle(" Local Port Filter ")
	}
	input.SetFieldWidth(10)
	input.SetBorder(true)

	// Accept only numbers
	input.SetAcceptanceFunc(tview.InputFieldInteger)

	// On Enter: set filter, close dialog, refresh
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			a.portFilter = input.GetText()
			a.resetTopConnectionsCursor()
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
			a.resetTopConnectionsCursor()
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
