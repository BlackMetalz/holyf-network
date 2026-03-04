package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

// panel_top_connections.go — Renders the Top Connections panel content.

// SortMode controls how Top Connections are sorted.
type SortMode int

const (
	SortByQueue SortMode = iota // tx_queue + rx_queue descending
	SortByConns                 // peer connection count descending
	SortByPort                  // local port ascending
)

const (
	defaultTalkersHintLine  = "  [dim]Use ↑/↓ select, Enter/k block, /=search, f=port/clear, Shift+Q/C/P sort (toggle DESC/ASC)[white]"
	readOnlyTalkersHintLine = "  [dim]Use ↑/↓ select, [=prev, ]=next snapshot, a=oldest, e=latest, t=jump-time, /=search, f=port/clear, Shift+Q/C/P sort (toggle DESC/ASC), g=group, L=follow[white]"
	defaultGroupHintLine    = "  [dim]Use ↑/↓ select, g=connections view, /=search, f=port/clear[white]"
	readOnlyGroupHintLine   = "  [dim]Use ↑/↓ select, [=prev, ]=next snapshot, a=oldest, e=latest, t=jump-time, g=connections view, /=search, f=port/clear, L=follow[white]"
)

// Label returns a short display name for the status bar chip.
func (m SortMode) Label() string {
	switch m {
	case SortByQueue:
		return "QUEUE"
	case SortByConns:
		return "CONNS"
	case SortByPort:
		return "PORT"
	default:
		return "QUEUE"
	}
}

func sortLabelWithDirection(mode SortMode, desc bool) string {
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	return mode.Label() + ":" + dir
}

func compareInt(a, b int, desc bool) bool {
	if desc {
		return a > b
	}
	return a < b
}

func compareInt64(a, b int64, desc bool) bool {
	if desc {
		return a > b
	}
	return a < b
}

// sortConnections sorts a slice in-place according to the given mode.
func sortConnections(conns []collector.Connection, mode SortMode, desc bool) {
	switch mode {
	case SortByQueue:
		sort.SliceStable(conns, func(i, j int) bool {
			if conns[i].Activity != conns[j].Activity {
				return compareInt64(conns[i].Activity, conns[j].Activity, desc)
			}
			if conns[i].LocalPort != conns[j].LocalPort {
				return compareInt(conns[i].LocalPort, conns[j].LocalPort, desc)
			}
			return normalizeIP(conns[i].RemoteIP) < normalizeIP(conns[j].RemoteIP)
		})
	case SortByConns:
		// Count connections per remote IP, then sort by count desc.
		counts := make(map[string]int)
		for _, c := range conns {
			counts[normalizeIP(c.RemoteIP)]++
		}
		sort.SliceStable(conns, func(i, j int) bool {
			ci := counts[normalizeIP(conns[i].RemoteIP)]
			cj := counts[normalizeIP(conns[j].RemoteIP)]
			if ci != cj {
				return compareInt(ci, cj, desc)
			}
			if conns[i].Activity != conns[j].Activity {
				return compareInt64(conns[i].Activity, conns[j].Activity, desc)
			}
			if conns[i].LocalPort != conns[j].LocalPort {
				return compareInt(conns[i].LocalPort, conns[j].LocalPort, desc)
			}
			return normalizeIP(conns[i].RemoteIP) < normalizeIP(conns[j].RemoteIP)
		})
	case SortByPort:
		sort.SliceStable(conns, func(i, j int) bool {
			if conns[i].LocalPort != conns[j].LocalPort {
				return compareInt(conns[i].LocalPort, conns[j].LocalPort, desc)
			}
			if conns[i].Activity != conns[j].Activity {
				return compareInt64(conns[i].Activity, conns[j].Activity, desc)
			}
			return normalizeIP(conns[i].RemoteIP) < normalizeIP(conns[j].RemoteIP)
		})
	}
}

// renderTalkersPanel formats the top connections for the TUI panel.
// If portFilter is set, only connections matching that port are shown.
// maxRows controls how many connections to display (use more when zoomed).
func renderTalkersPanel(conns []collector.Connection, portFilter string, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool) string {
	return renderTalkersPanelWithHint(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortMode, sortDesc, defaultTalkersHintLine)
}

func renderTalkersPanelReadOnly(conns []collector.Connection, portFilter string, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool) string {
	return renderTalkersPanelWithHint(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortMode, sortDesc, readOnlyTalkersHintLine)
}

func renderTalkersPanelWithHint(conns []collector.Connection, portFilter string, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool, hintLine string) string {
	var sb strings.Builder

	portChip := "Port Filter = ALL"
	if strings.TrimSpace(portFilter) != "" {
		portChip = "Port Filter = " + strings.TrimSpace(portFilter)
	}
	maskChip := "OFF"
	if sensitiveIP {
		maskChip = "ON"
	}
	searchChip := "ALL"
	if strings.TrimSpace(textFilter) != "" {
		searchChip = truncateRight(strings.TrimSpace(textFilter), 20)
	}

	sb.WriteString(fmt.Sprintf(
		"  [dim]Chips:[white] [yellow]%s[white] | [yellow]MaskIP=%s[white] | [yellow]Search=%s[white] | [yellow]Sort=%s[white] | [aqua]View=CONN[white]\n",
		portChip,
		maskChip,
		searchChip,
		sortLabelWithDirection(sortMode, sortDesc),
	))
	sb.WriteString(hintLine)
	sb.WriteString("\n\n")

	if len(conns) == 0 {
		sb.WriteString("  No active connections found")
		return sb.String()
	}

	// Apply filters (port + contains text).
	filtered := conns
	if portFilter != "" {
		filtered = filterByPort(conns, portFilter)
	}
	if textFilter != "" {
		filtered = filterByText(filtered, textFilter)
	}
	if len(filtered) == 0 {
		sb.WriteString("  No connections matching current filters")
		return sb.String()
	}

	// Apply sort on the filtered result set.
	sortConnections(filtered, sortMode, sortDesc)

	if selectedIndex < 0 {
		selectedIndex = 0
	}
	if selectedIndex >= len(filtered) {
		selectedIndex = len(filtered) - 1
	}

	const (
		processColWidth  = 18
		endpointColWidth = 24
		stateColWidth    = 11
	)
	// Header row
	sb.WriteString(fmt.Sprintf("  [dim]%-*s %-*s %-*s %-*s %s[white]\n",
		processColWidth, "PROCESS",
		endpointColWidth, "SRC",
		endpointColWidth, "PEER",
		stateColWidth, "STATE",
		"QUEUE",
	))

	// Render each connection
	for i, conn := range filtered {
		if i >= maxRows {
			sb.WriteString(fmt.Sprintf("\n  [dim]... and %d more[white]", len(filtered)-maxRows))
			break
		}

		// Color by state
		stateColor := "white"
		switch conn.State {
		case "ESTABLISHED":
			stateColor = "green"
		case "TIME_WAIT":
			stateColor = "yellow"
		case "CLOSE_WAIT":
			stateColor = "red"
		}

		// Format PID/process info: "1234/nginx" or "-"
		procInfo := "-"
		if conn.PID > 0 {
			if conn.ProcName != "" {
				procInfo = fmt.Sprintf("%d/%s", conn.PID, conn.ProcName)
			} else {
				procInfo = fmt.Sprintf("%d", conn.PID)
			}
		}
		// Truncate to fit column width
		procInfo = truncateRight(procInfo, processColWidth)
		src := formatEndpoint(conn.LocalIP, conn.LocalPort, endpointColWidth, sensitiveIP)
		peer := formatEndpoint(conn.RemoteIP, conn.RemotePort, endpointColWidth, sensitiveIP)

		queueStr := " [dim]0B[white]"
		if conn.Activity > 0 {
			queueStr = fmt.Sprintf(" [yellow]%s[white]", formatBytes(conn.Activity))
		}

		prefix := "  "
		if i == selectedIndex {
			prefix = " [yellow]>[white]"
		}

		sb.WriteString(fmt.Sprintf("%s[aqua]%-*s[white] %-*s %-*s [%s]%-*s[white]%s\n",
			prefix,
			processColWidth, procInfo,
			endpointColWidth, src,
			endpointColWidth, peer,
			stateColor, stateColWidth, conn.State,
			queueStr,
		))
	}

	// Show total count
	sb.WriteString(fmt.Sprintf("\n  [dim]Showing %d of %d connections[white]", min(len(filtered), maxRows), len(filtered)))

	return sb.String()
}

// filterByPort returns only connections where local or remote port matches.
func filterByPort(conns []collector.Connection, portStr string) []collector.Connection {
	port := parsePortFilter(portStr)
	if port == 0 {
		return conns // Invalid filter, show all
	}

	var result []collector.Connection
	for _, conn := range conns {
		if conn.LocalPort == port || conn.RemotePort == port {
			result = append(result, conn)
		}
	}
	return result
}

func filterByLocalPort(conns []collector.Connection, portStr string) []collector.Connection {
	port := parsePortFilter(portStr)
	if port == 0 {
		return conns
	}

	result := make([]collector.Connection, 0, len(conns))
	for _, conn := range conns {
		if conn.LocalPort == port {
			result = append(result, conn)
		}
	}
	return result
}

func parsePortFilter(portStr string) int {
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || port < 1 || port > 65535 {
		return 0
	}
	return port
}

func applyGroupConnectionFilters(conns []collector.Connection, portFilter, textFilter string) []collector.Connection {
	filtered := conns
	if strings.TrimSpace(portFilter) != "" {
		filtered = filterByLocalPort(filtered, portFilter)
	}
	if strings.TrimSpace(textFilter) != "" {
		filtered = filterByText(filtered, textFilter)
	}
	return filtered
}

// filterByText returns only connections whose rendered fields contain query.
func filterByText(conns []collector.Connection, query string) []collector.Connection {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return conns
	}

	result := make([]collector.Connection, 0, len(conns))
	for _, conn := range conns {
		procInfo := "-"
		if conn.PID > 0 {
			if conn.ProcName != "" {
				procInfo = fmt.Sprintf("%d/%s", conn.PID, conn.ProcName)
			} else {
				procInfo = fmt.Sprintf("%d", conn.PID)
			}
		}
		haystack := strings.ToLower(strings.Join([]string{
			procInfo,
			normalizeIP(conn.LocalIP),
			normalizeIP(conn.RemoteIP),
			fmt.Sprintf("%s:%d", normalizeIP(conn.LocalIP), conn.LocalPort),
			fmt.Sprintf("%s:%d", normalizeIP(conn.RemoteIP), conn.RemotePort),
			conn.State,
			formatBytes(conn.Activity),
		}, " "))

		if strings.Contains(haystack, needle) {
			result = append(result, conn)
		}
	}
	return result
}

// formatBytes converts bytes to human-readable format (no "/s" suffix).
func formatBytes(bytes int64) string {
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
	if bytes >= 1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%dB", bytes)
}

// formatEndpoint normalizes, masks (optional), and truncates endpoint text.
func formatEndpoint(ip string, port int, width int, sensitiveIP bool) string {
	displayIP := normalizeIP(ip)
	if sensitiveIP {
		displayIP = maskIP(displayIP)
	}

	if strings.Contains(displayIP, ":") && !strings.Contains(displayIP, ".") {
		return truncateEndpoint(fmt.Sprintf("[%s]:%d", displayIP, port), width)
	}

	return truncateEndpoint(fmt.Sprintf("%s:%d", displayIP, port), width)
}

// normalizeIP removes noisy IPv4-mapped IPv6 prefix for readability.
func normalizeIP(ip string) string {
	return strings.TrimPrefix(ip, "::ffff:")
}

// maskIP hides the first 2 IPv4 octets (or 2 IPv6 groups).
func maskIP(ip string) string {
	if strings.Count(ip, ".") == 3 {
		parts := strings.Split(ip, ".")
		return fmt.Sprintf("xxx.xxx.%s.%s", parts[2], parts[3])
	}

	if strings.Contains(ip, ":") {
		parts := strings.Split(ip, ":")
		masked := 0
		for i := 0; i < len(parts) && masked < 2; i++ {
			if parts[i] == "" {
				continue
			}
			parts[i] = "xxxx"
			masked++
		}
		return strings.Join(parts, ":")
	}

	return ip
}

// truncateEndpoint keeps the endpoint suffix (including :port) visible.
func truncateEndpoint(endpoint string, width int) string {
	if len(endpoint) <= width {
		return endpoint
	}
	if width <= 3 {
		return endpoint[:width]
	}

	suffix := ""
	if idx := strings.LastIndex(endpoint, ":"); idx >= 0 {
		suffix = endpoint[idx:]
	}
	if suffix == "" || len(suffix) >= width-3 {
		return endpoint[:width-3] + "..."
	}

	prefixLen := width - len(suffix) - 3
	if prefixLen < 1 {
		prefixLen = 1
	}

	return endpoint[:prefixLen] + "..." + suffix
}

// truncateRight trims text to width with a trailing ellipsis.
func truncateRight(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

// PeerGroup represents aggregated connections for a single remote IP.
type PeerGroup struct {
	PeerIP     string
	Count      int
	TotalQueue int64
	LocalPorts map[int]struct{}
	States     map[string]int
	TopProc    string // most common process name
}

// buildPeerGroups aggregates connections by remote IP.
func buildPeerGroups(conns []collector.Connection) []PeerGroup {
	byPeer := make(map[string]*PeerGroup)
	procCount := make(map[string]map[string]int) // peerIP -> procName -> count

	for _, conn := range conns {
		peer := normalizeIP(conn.RemoteIP)
		g, exists := byPeer[peer]
		if !exists {
			g = &PeerGroup{
				PeerIP:     peer,
				LocalPorts: make(map[int]struct{}),
				States:     make(map[string]int),
			}
			byPeer[peer] = g
			procCount[peer] = make(map[string]int)
		}
		g.Count++
		g.TotalQueue += conn.Activity
		g.LocalPorts[conn.LocalPort] = struct{}{}
		g.States[conn.State]++
		if conn.ProcName != "" {
			procCount[peer][conn.ProcName]++
		}
	}

	groups := make([]PeerGroup, 0, len(byPeer))
	for _, g := range byPeer {
		// Find most common process.
		bestCount := 0
		for proc, cnt := range procCount[g.PeerIP] {
			if cnt > bestCount {
				bestCount = cnt
				g.TopProc = proc
			}
		}
		groups = append(groups, *g)
	}

	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}
		return groups[i].TotalQueue > groups[j].TotalQueue
	})

	return groups
}

// renderPeerGroupPanel renders the per-peer aggregate view.
func renderPeerGroupPanel(conns []collector.Connection, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int) string {
	return renderPeerGroupPanelWithHint(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, defaultGroupHintLine)
}

func renderPeerGroupPanelReadOnly(conns []collector.Connection, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int) string {
	return renderPeerGroupPanelWithHint(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, readOnlyGroupHintLine)
}

func renderPeerGroupPanelWithHint(conns []collector.Connection, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, hintLine string) string {
	var sb strings.Builder

	portChip := "Port Filter = ALL"
	if strings.TrimSpace(portFilter) != "" {
		portChip = "Port Filter = " + strings.TrimSpace(portFilter)
	}
	searchChip := "ALL"
	if strings.TrimSpace(textFilter) != "" {
		searchChip = truncateRight(strings.TrimSpace(textFilter), 20)
	}

	sb.WriteString(fmt.Sprintf(
		"  [dim]Chips:[white] [yellow]%s[white] | [yellow]Search=%s[white] | [aqua]View=GROUP[white]\n",
		portChip,
		searchChip,
	))
	sb.WriteString(hintLine)
	sb.WriteString("\n\n")

	if len(conns) == 0 {
		sb.WriteString("  No active connections found")
		return sb.String()
	}

	// Apply filters first, then group.
	filtered := applyGroupConnectionFilters(conns, portFilter, textFilter)
	if len(filtered) == 0 {
		sb.WriteString("  No connections matching current filters")
		return sb.String()
	}

	groups := buildPeerGroups(filtered)
	if selectedIndex < 0 {
		selectedIndex = 0
	}
	if selectedIndex >= len(groups) {
		selectedIndex = len(groups) - 1
	}

	const (
		peerColWidth    = 24
		countColWidth   = 7
		queueColWidth   = 10
		processColWidth = 14
		portsColWidth   = 16
	)

	sb.WriteString(fmt.Sprintf("  [dim]%-*s %*s %*s %-*s %-*s[white]\n",
		peerColWidth, "PEER",
		countColWidth, "CONNS",
		queueColWidth, "QUEUE",
		processColWidth, "PROCESS",
		portsColWidth, "PORTS",
	))

	for i, g := range groups {
		if i >= maxRows {
			sb.WriteString(fmt.Sprintf("\n  [dim]... and %d more peers[white]", len(groups)-maxRows))
			break
		}

		peer := g.PeerIP
		if sensitiveIP {
			peer = maskIP(peer)
		}
		peer = truncateRight(peer, peerColWidth)

		queueText := "0B"
		queueColor := "dim"
		if g.TotalQueue > 0 {
			queueText = formatBytes(g.TotalQueue)
			queueColor = "yellow"
		}
		queueField := fmt.Sprintf("[%s]%*s[white]", queueColor, queueColWidth, queueText)

		procText := "-"
		procColor := "dim"
		if g.TopProc != "" {
			procText = truncateRight(g.TopProc, processColWidth)
			procColor = "white"
		}
		procField := fmt.Sprintf("[%s]%-*s[white]", procColor, processColWidth, procText)

		// Show up to 4 unique local ports.
		portList := make([]int, 0, len(g.LocalPorts))
		for p := range g.LocalPorts {
			portList = append(portList, p)
		}
		sort.Ints(portList)
		portStrs := make([]string, 0, 4)
		for j, p := range portList {
			if j >= 4 {
				portStrs = append(portStrs, fmt.Sprintf("+%d", len(portList)-4))
				break
			}
			portStrs = append(portStrs, fmt.Sprintf("%d", p))
		}
		portsDisplay := truncateRight(strings.Join(portStrs, ","), portsColWidth)

		// Color count by severity.
		countColor := "white"
		if g.Count >= 50 {
			countColor = "red"
		} else if g.Count >= 10 {
			countColor = "yellow"
		}

		prefix := "  "
		if i == selectedIndex {
			prefix = " [yellow]>[white]"
		}

		sb.WriteString(fmt.Sprintf("%s%-*s [%s]%*d[white] %s %s %-*s\n",
			prefix,
			peerColWidth, peer,
			countColor, countColWidth, g.Count,
			queueField,
			procField,
			portsColWidth, portsDisplay,
		))
	}

	sb.WriteString(fmt.Sprintf("\n  [dim]%d peers, %d total connections[white]", len(groups), len(filtered)))

	return sb.String()
}
