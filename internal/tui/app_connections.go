package tui

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/actions"
	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/tui/blocking"
	tuipanels "github.com/BlackMetalz/holyf-network/internal/tui/panels"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// --- Top Connections Panel ---

func (a *App) topConnectionsPanelSize() (int, int) {
	if len(a.panels) <= 2 || a.panels[2] == nil {
		return tuishared.DefaultTopConnectionsPanelWidth, 27 // fallback
	}

	_, _, width, height := a.panels[2].GetInnerRect()
	if width <= 0 {
		width = tuishared.DefaultTopConnectionsPanelWidth
	}
	if height <= 0 {
		height = 27
	}
	return width, height
}

func (a *App) topConnectionsPanelLayout(hasRows bool) tuipanels.TopConnectionsPanelLayout {
	width, height := a.topConnectionsPanelSize()
	rowLimit, showPreview := tuipanels.CalculateTopConnectionsDisplayLimit(
		height,
		tuipanels.TopConnectionsNoteCount(a.topBandwidthNote),
		hasRows,
	)
	return tuipanels.TopConnectionsPanelLayout{
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
	tuipanels.SortConnectionsWithDirection(items, a.sortMode, a.sortDesc, a.topDirection)
	return items
}

func (a *App) filteredPeerGroups() []tuipanels.PeerGroup {
	source := a.topConnectionsSource()
	if len(source) == 0 {
		return nil
	}

	filtered := tuipanels.ApplyGroupConnectionFiltersByDirection(source, a.portFilter, a.textFilter, a.topDirection)
	if len(filtered) == 0 {
		return nil
	}

	groups := tuipanels.BuildPeerGroupsWithDirection(filtered, a.sortDesc, a.topDirection)
	return tuipanels.LimitPeerGroups(groups, tuipanels.TopConnectionsGroupCap, a.sortDesc)
}

func (a *App) visibleTopConnections() []collector.Connection {
	items := a.filteredTopConnections()
	if len(items) == 0 {
		a.topPageIndex = 0
		return nil
	}

	limit := a.topConnectionsPanelLayout(true).RowLimit
	page, start, end, _ := tuipanels.TopConnectionsPageBounds(len(items), limit, a.topPageIndex)
	a.topPageIndex = page
	return items[start:end]
}

func (a *App) visiblePeerGroups() []tuipanels.PeerGroup {
	groups := a.filteredPeerGroups()
	if len(groups) == 0 {
		a.topPageIndex = 0
		return nil
	}

	limit := a.topConnectionsPanelLayout(true).RowLimit
	page, start, end, _ := tuipanels.TopConnectionsPageBounds(len(groups), limit, a.topPageIndex)
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
	currentPage, _, _, totalPages := tuipanels.TopConnectionsPageBounds(total, layout.RowLimit, a.topPageIndex)
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
		page, start, end, _ := tuipanels.TopConnectionsPageBounds(len(groups), layout.RowLimit, a.topPageIndex)
		a.topPageIndex = page
		visibleGroups := groups[start:end]
		if count := len(visibleGroups); count == 0 {
			a.selectedTalkerIndex = 0
		} else if a.selectedTalkerIndex >= count {
			a.selectedTalkerIndex = count - 1
		} else if a.selectedTalkerIndex < 0 {
			a.selectedTalkerIndex = 0
		}

		var preview *tuipanels.SelectedRowPreview
		if layout.ShowPreview {
			preview = a.buildSelectedPeerGroupPreview(visibleGroups)
		}

		a.panels[2].SetText(tuipanels.RenderPeerGroupPanelWithPreviewDirection(
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
	page, start, end, _ := tuipanels.TopConnectionsPageBounds(len(items), layout.RowLimit, a.topPageIndex)
	a.topPageIndex = page
	visibleItems := items[start:end]
	if count := len(visibleItems); count == 0 {
		a.selectedTalkerIndex = 0
	} else if a.selectedTalkerIndex >= count {
		a.selectedTalkerIndex = count - 1
	} else if a.selectedTalkerIndex < 0 {
		a.selectedTalkerIndex = 0
	}

	var preview *tuipanels.SelectedRowPreview
	if layout.ShowPreview {
		preview = a.buildSelectedConnectionPreview(visibleItems)
	}

	a.panels[2].SetText(tuipanels.RenderTalkersPanelWithPreviewDirection(
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
			if a.topDirection == tuishared.TopConnectionOutgoing && isListenerPort {
				continue
			}
			if a.topDirection == tuishared.TopConnectionIncoming && !isListenerPort {
				continue
			}
		}
		filtered = append(filtered, conn)
	}
	return filtered
}

func (a *App) ensureListenPortsKnown() {
	if a.listenPortsKnown {
		return
	}
	listenPorts, err := collector.CollectListenPorts()
	if err != nil {
		return
	}
	a.listenPorts = listenPorts
	a.listenPortsKnown = true
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
		filtered = tuipanels.FilterByPortDirection(filtered, a.portFilter, a.topDirection)
	}
	if a.textFilter != "" {
		filtered = tuipanels.FilterByText(filtered, a.textFilter)
	}
	return filtered
}

func (a *App) buildSelectedConnectionPreview(conns []collector.Connection) *tuipanels.SelectedRowPreview {
	if len(conns) == 0 || a.selectedTalkerIndex < 0 || a.selectedTalkerIndex >= len(conns) {
		return nil
	}

	conn := conns[a.selectedTalkerIndex]
	peerIP := tuishared.NormalizeIP(conn.RemoteIP)
	count := a.countPeerMatches(peerIP, conn.LocalPort)

	return tuipanels.BuildSelectedConnectionPreview(conn, count, a.sensitiveIP, a.topDirection)
}

func (a *App) buildSelectedPeerGroupPreview(groups []tuipanels.PeerGroup) *tuipanels.SelectedRowPreview {
	if len(groups) == 0 || a.selectedTalkerIndex < 0 || a.selectedTalkerIndex >= len(groups) {
		return nil
	}

	group := groups[a.selectedTalkerIndex]
	target, ok := a.selectedPeerPortTarget(group.PeerIP)
	if !ok {
		target = blocking.PeerKillTarget{PeerIP: group.PeerIP}
	}

	return tuipanels.BuildSelectedPeerGroupPreview(group, target.Count, target.LocalPort, a.sensitiveIP, a.topDirection)
}

func (a *App) updateTopConnectionsPanelTitle() {
	if len(a.panels) <= 2 || a.panels[2] == nil {
		return
	}
	a.panels[2].SetTitle(a.topDirection.PanelTitle())
}

func (a *App) toggleTopConnectionsDirection() {
	if a.topDirection == tuishared.TopConnectionOutgoing {
		a.topDirection = tuishared.TopConnectionIncoming
		a.setStatusNote("Top Connections: IN mode (local listener ports, Enter/k enabled)", 4*time.Second)
	} else {
		a.topDirection = tuishared.TopConnectionOutgoing
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
	if a.topDirection == tuishared.TopConnectionOutgoing {
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

// --- Kill Targets ---

// selectPeerKillTarget picks the most frequent peer->localPort tuple.
func (a *App) selectPeerKillTarget() (blocking.PeerKillTarget, bool) {
	if a.topDirection == tuishared.TopConnectionOutgoing {
		return blocking.PeerKillTarget{}, false
	}
	source := a.topConnectionsSource()
	if len(source) == 0 {
		return blocking.PeerKillTarget{}, false
	}

	filtered := a.applyTopConnectionFilters(source)
	if a.groupView {
		filtered = tuipanels.ApplyGroupConnectionFiltersByDirection(source, a.portFilter, a.textFilter, a.topDirection)
	}
	if len(filtered) == 0 {
		return blocking.PeerKillTarget{}, false
	}

	type aggregate struct {
		target   blocking.PeerKillTarget
		activity int64
	}
	aggByKey := make(map[string]aggregate)

	for _, conn := range filtered {
		peer := tuishared.NormalizeIP(conn.RemoteIP)
		key := fmt.Sprintf("%s|%d", peer, conn.LocalPort)

		current := aggByKey[key]
		current.target.PeerIP = peer
		current.target.LocalPort = conn.LocalPort
		current.target.Count++
		current.activity += conn.Activity
		aggByKey[key] = current
	}

	candidates := make([]aggregate, 0, len(aggByKey))
	for _, candidate := range aggByKey {
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return blocking.PeerKillTarget{}, false
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].target.Count != candidates[j].target.Count {
			return candidates[i].target.Count > candidates[j].target.Count
		}
		if candidates[i].activity != candidates[j].activity {
			return candidates[i].activity > candidates[j].activity
		}
		if candidates[i].target.LocalPort != candidates[j].target.LocalPort {
			return candidates[i].target.LocalPort < candidates[j].target.LocalPort
		}
		return candidates[i].target.PeerIP < candidates[j].target.PeerIP
	})

	return candidates[0].target, true
}

func (a *App) selectedPeerKillTarget() (blocking.PeerKillTarget, bool) {
	if a.topDirection == tuishared.TopConnectionOutgoing {
		return blocking.PeerKillTarget{}, false
	}
	if a.groupView {
		groups := a.visiblePeerGroups()
		if len(groups) == 0 {
			return blocking.PeerKillTarget{}, false
		}
		a.clampTopConnectionSelection()

		peerIP := groups[a.selectedTalkerIndex].PeerIP
		return a.selectedPeerPortTarget(peerIP)
	}

	visible := a.visibleTopConnections()
	if len(visible) == 0 {
		return blocking.PeerKillTarget{}, false
	}
	a.clampTopConnectionSelection()

	conn := visible[a.selectedTalkerIndex]
	peerIP := tuishared.NormalizeIP(conn.RemoteIP)
	localPort := conn.LocalPort
	return blocking.PeerKillTarget{
		PeerIP:    peerIP,
		LocalPort: localPort,
		Count:     a.countPeerMatches(peerIP, localPort),
	}, true
}

func (a *App) selectedPeerPortTarget(peerIP string) (blocking.PeerKillTarget, bool) {
	if a.topDirection == tuishared.TopConnectionOutgoing {
		return blocking.PeerKillTarget{}, false
	}
	source := a.topConnectionsSource()
	if len(source) == 0 {
		return blocking.PeerKillTarget{}, false
	}

	filtered := a.applyTopConnectionFilters(source)
	if a.groupView {
		filtered = tuipanels.ApplyGroupConnectionFiltersByDirection(source, a.portFilter, a.textFilter, a.topDirection)
	}
	if len(filtered) == 0 {
		return blocking.PeerKillTarget{}, false
	}

	type portAggregate struct {
		count    int
		activity int64
	}
	byPort := make(map[int]portAggregate)
	for _, conn := range filtered {
		if tuishared.NormalizeIP(conn.RemoteIP) != peerIP {
			continue
		}
		current := byPort[conn.LocalPort]
		current.count++
		current.activity += conn.Activity
		byPort[conn.LocalPort] = current
	}
	if len(byPort) == 0 {
		return blocking.PeerKillTarget{}, false
	}

	bestPort := 0
	best := portAggregate{}
	first := true
	for port, agg := range byPort {
		if first {
			bestPort = port
			best = agg
			first = false
			continue
		}
		if agg.count > best.count ||
			(agg.count == best.count && agg.activity > best.activity) ||
			(agg.count == best.count && agg.activity == best.activity && port < bestPort) {
			bestPort = port
			best = agg
		}
	}

	return blocking.PeerKillTarget{
		PeerIP:    peerIP,
		LocalPort: bestPort,
		Count:     best.count,
	}, true
}

func (a *App) countPeerMatches(peerIP string, localPort int) int {
	source := a.topConnectionsSource()
	if len(source) == 0 {
		return 0
	}

	filtered := a.applyTopConnectionFilters(source)

	count := 0
	for _, conn := range filtered {
		if tuishared.NormalizeIP(conn.RemoteIP) == peerIP && conn.LocalPort == localPort {
			count++
		}
	}
	return count
}

func (a *App) matchingBlockTuples(peerIP string, localPort int) []actions.SocketTuple {
	if len(a.latestTalkers) == 0 {
		return nil
	}

	normalizedPeer := tuishared.NormalizeIP(peerIP)
	seen := make(map[string]struct{})
	tuples := make([]actions.SocketTuple, 0, 8)

	for _, conn := range a.latestTalkers {
		if conn.LocalPort != localPort {
			continue
		}
		if !strings.EqualFold(conn.State, "ESTABLISHED") {
			continue
		}

		remoteIP := tuishared.NormalizeIP(conn.RemoteIP)
		if remoteIP != normalizedPeer {
			continue
		}

		localIP := tuishared.NormalizeIP(conn.LocalIP)
		if localIP == "" || conn.RemotePort < 1 || conn.RemotePort > 65535 {
			continue
		}

		key := fmt.Sprintf("%s|%d|%s|%d", localIP, conn.LocalPort, remoteIP, conn.RemotePort)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		tuples = append(tuples, actions.SocketTuple{
			LocalIP:    localIP,
			LocalPort:  conn.LocalPort,
			RemoteIP:   remoteIP,
			RemotePort: conn.RemotePort,
		})
	}

	return tuples
}

// matchingBlockTuplesFromSnapshot is like matchingBlockTuples but operates on
// a pre-captured snapshot of connections. Safe to call from any goroutine.
func matchingBlockTuplesFromSnapshot(conns []collector.Connection, peerIP string, localPort int) []actions.SocketTuple {
	if len(conns) == 0 {
		return nil
	}

	normalizedPeer := tuishared.NormalizeIP(peerIP)
	seen := make(map[string]struct{})
	tuples := make([]actions.SocketTuple, 0, 8)

	for _, conn := range conns {
		if conn.LocalPort != localPort {
			continue
		}
		if !strings.EqualFold(conn.State, "ESTABLISHED") {
			continue
		}

		remoteIP := tuishared.NormalizeIP(conn.RemoteIP)
		if remoteIP != normalizedPeer {
			continue
		}

		localIP := tuishared.NormalizeIP(conn.LocalIP)
		if localIP == "" || conn.RemotePort < 1 || conn.RemotePort > 65535 {
			continue
		}

		key := fmt.Sprintf("%s|%d|%s|%d", localIP, conn.LocalPort, remoteIP, conn.RemotePort)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		tuples = append(tuples, actions.SocketTuple{
			LocalIP:    localIP,
			LocalPort:  conn.LocalPort,
			RemoteIP:   remoteIP,
			RemotePort: conn.RemotePort,
		})
	}

	return tuples
}

func parsePeerIPInput(raw string) (string, bool) {
	peerIP := strings.TrimSpace(raw)
	peerIP = strings.TrimPrefix(peerIP, "[")
	peerIP = strings.TrimSuffix(peerIP, "]")
	peerIP = strings.TrimPrefix(peerIP, "::ffff:")
	parsed := net.ParseIP(peerIP)
	if parsed == nil {
		return "", false
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String(), true
	}
	return parsed.String(), true
}
