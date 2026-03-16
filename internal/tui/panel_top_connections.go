package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
)

// panel_top_connections.go — Renders the Top Connections panel content.

// SortMode controls how Top Connections are sorted.
type SortMode int

const (
	SortByBandwidth SortMode = iota // total bytes delta descending
	SortByConns                     // peer connection count descending
	SortByPort                      // local port ascending
)

const (
	defaultTalkersHintLine  = "  [dim]Use ↑/↓ select, Enter/k block, /=search, f=port/clear, Shift+B/C/P sort (toggle DESC/ASC), i=explain qcols, Shift+I=explain iface[white]"
	readOnlyTalkersHintLine = "  [dim]Use ↑/↓ select, [=prev, ]=next snapshot, a=oldest, e=latest, t=jump-time, /=search, f=port/clear, Shift+B/C/P sort (toggle DESC/ASC), i/Shift+I=explain qcols, g=group, L=follow[white]"
	defaultGroupHintLine    = "  [dim]Use ↑/↓ select, g=connections view, /=search, f=port/clear, Shift+C conns sort (toggle DESC/ASC), i=explain qcols, Shift+I=explain iface[white]"
	readOnlyGroupHintLine   = "  [dim]Use ↑/↓ select, [=prev, ]=next snapshot, a=oldest, e=latest, t=jump-time, g=connections view, /=search, f=port/clear, Shift+C conns sort (toggle DESC/ASC), i/Shift+I=explain qcols, L=follow[white]"
)

// Label returns a short display name for the status bar chip.
func (m SortMode) Label() string {
	switch m {
	case SortByBandwidth:
		return "BW"
	case SortByConns:
		return "CONNS"
	case SortByPort:
		return "PORT"
	default:
		return "BW"
	}
}

func sortLabelWithDirection(mode SortMode, desc bool) string {
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	return mode.Label() + ":" + dir
}

func rowSelectionPrefix(selected bool) string {
	if selected {
		return "[yellow]>[white] "
	}
	return "  "
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
	case SortByBandwidth:
		sort.SliceStable(conns, func(i, j int) bool {
			if conns[i].TotalBytesDelta != conns[j].TotalBytesDelta {
				return compareInt64(conns[i].TotalBytesDelta, conns[j].TotalBytesDelta, desc)
			}
			if conns[i].TotalBytesPerSec != conns[j].TotalBytesPerSec {
				if desc {
					return conns[i].TotalBytesPerSec > conns[j].TotalBytesPerSec
				}
				return conns[i].TotalBytesPerSec < conns[j].TotalBytesPerSec
			}
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
			if conns[i].TotalBytesDelta != conns[j].TotalBytesDelta {
				return compareInt64(conns[i].TotalBytesDelta, conns[j].TotalBytesDelta, desc)
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
			if conns[i].TotalBytesDelta != conns[j].TotalBytesDelta {
				return compareInt64(conns[i].TotalBytesDelta, conns[j].TotalBytesDelta, desc)
			}
			return normalizeIP(conns[i].RemoteIP) < normalizeIP(conns[j].RemoteIP)
		})
	}
}

// renderTalkersPanel formats the top connections for the TUI panel.
// If portFilter is set, only connections matching that port are shown.
// maxRows controls how many connections to display (use more when zoomed).
func renderTalkersPanel(conns []collector.Connection, portFilter string, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string) string {
	return renderTalkersPanelWithHint(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortMode, sortDesc, thresholds, bandwidthNote, defaultTalkersHintLine)
}

func renderTalkersPanelReadOnly(conns []collector.Connection, portFilter string, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string) string {
	return renderTalkersPanelWithHint(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortMode, sortDesc, thresholds, bandwidthNote, readOnlyTalkersHintLine)
}

func renderTalkersPanelWithHint(conns []collector.Connection, portFilter string, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string, hintLine string) string {
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
	if strings.TrimSpace(bandwidthNote) != "" {
		sb.WriteString("\n  [yellow]")
		sb.WriteString(truncateRight(strings.TrimSpace(bandwidthNote), 160))
		sb.WriteString("[white]")
	}
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
		processColWidth  = 16
		endpointColWidth = 22
		stateColWidth    = 11
		queueColWidth    = 7
		bwColWidth       = 9
	)
	// Header row
	sb.WriteString(fmt.Sprintf("  [dim]%-*s %-*s %-*s %-*s %*s %*s %*s %*s[white]\n",
		processColWidth, "PROCESS",
		endpointColWidth, "SRC",
		endpointColWidth, "PEER",
		stateColWidth, "STATE",
		queueColWidth, "SEND-Q",
		queueColWidth, "RECV-Q",
		bwColWidth, "TX/s",
		bwColWidth, "RX/s",
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

		// Format process info: "1234/nginx", "1234", "ct/nat", or "-"
		procInfo := formatProcessInfo(conn)
		// Truncate to fit column width
		procInfo = truncateRight(procInfo, processColWidth)
		src := formatEndpoint(conn.LocalIP, conn.LocalPort, endpointColWidth, sensitiveIP)
		peer := formatEndpoint(conn.RemoteIP, conn.RemotePort, endpointColWidth, sensitiveIP)

		sendQColor := "dim"
		if conn.TxQueue > 0 {
			sendQColor = "yellow"
		}
		recvQColor := "dim"
		if conn.RxQueue > 0 {
			recvQColor = "yellow"
		}
		sendQField := fmt.Sprintf("[%s]%*s[white]", sendQColor, queueColWidth, formatBytes(conn.TxQueue))
		recvQField := fmt.Sprintf("[%s]%*s[white]", recvQColor, queueColWidth, formatBytes(conn.RxQueue))
		txRateColor := bandwidthColor(conn.TxBytesPerSec, thresholds.BandwidthPerSec)
		rxRateColor := bandwidthColor(conn.RxBytesPerSec, thresholds.BandwidthPerSec)
		txRateField := fmt.Sprintf("[%s]%*s[white]", txRateColor, bwColWidth, formatBytesRateCompact(conn.TxBytesPerSec))
		rxRateField := fmt.Sprintf("[%s]%*s[white]", rxRateColor, bwColWidth, formatBytesRateCompact(conn.RxBytesPerSec))

		prefix := rowSelectionPrefix(i == selectedIndex)

		sb.WriteString(fmt.Sprintf("%s[aqua]%-*s[white] %-*s %-*s [%s]%-*s[white] %s %s %s %s\n",
			prefix,
			processColWidth, procInfo,
			endpointColWidth, src,
			endpointColWidth, peer,
			stateColor, stateColWidth, conn.State,
			sendQField,
			recvQField,
			txRateField,
			rxRateField,
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
		procInfo := formatProcessInfo(conn)
		haystack := strings.ToLower(strings.Join([]string{
			procInfo,
			normalizeIP(conn.LocalIP),
			normalizeIP(conn.RemoteIP),
			fmt.Sprintf("%s:%d", normalizeIP(conn.LocalIP), conn.LocalPort),
			fmt.Sprintf("%s:%d", normalizeIP(conn.RemoteIP), conn.RemotePort),
			conn.State,
			formatBytes(conn.Activity),
			formatBytes(conn.TxQueue),
			formatBytes(conn.RxQueue),
			formatBytes(conn.TxBytesDelta),
			formatBytes(conn.RxBytesDelta),
			formatBytes(conn.TotalBytesDelta),
			formatBytesRateCompact(conn.TxBytesPerSec),
			formatBytesRateCompact(conn.RxBytesPerSec),
			formatBytesRateCompact(conn.TotalBytesPerSec),
		}, " "))

		if strings.Contains(haystack, needle) {
			result = append(result, conn)
		}
	}
	return result
}

func formatProcessInfo(conn collector.Connection) string {
	if conn.PID > 0 && conn.ProcName != "" {
		return fmt.Sprintf("%d/%s", conn.PID, conn.ProcName)
	}
	if conn.PID > 0 {
		return fmt.Sprintf("%d", conn.PID)
	}
	if strings.TrimSpace(conn.ProcName) != "" {
		return strings.TrimSpace(conn.ProcName)
	}
	return "-"
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

func formatBytesRateCompact(rate float64) string {
	if rate < 0 {
		rate = 0
	}
	return formatBytes(int64(rate)) + "/s"
}

func bandwidthColor(value float64, band config.ThresholdBand) string {
	if band.Crit > 0 && value >= band.Crit {
		return "red"
	}
	if band.Warn > 0 && value >= band.Warn {
		return "yellow"
	}
	if value > 0 {
		return "green"
	}
	return "dim"
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

// PeerGroup represents aggregated connections for a remote peer + process.
type PeerGroup struct {
	PeerIP           string
	ProcName         string
	Count            int
	TxQueue          int64
	RxQueue          int64
	TotalQueue       int64
	TxBytesDelta     int64
	RxBytesDelta     int64
	TotalBytesDelta  int64
	TxBytesPerSec    float64
	RxBytesPerSec    float64
	TotalBytesPerSec float64
	LocalPorts       map[int]struct{}
	States           map[string]int
}

type stateSummaryEntry struct {
	Name  string
	Count int
}

var peerGroupStateOrder = map[string]int{
	"ESTABLISHED": 0,
	"TIME_WAIT":   1,
	"CLOSE_WAIT":  2,
	"LISTEN":      3,
	"SYN_SENT":    4,
	"SYN_RECV":    5,
	"FIN_WAIT1":   6,
	"FIN_WAIT2":   7,
	"LAST_ACK":    8,
	"CLOSING":     9,
	"CLOSE":       10,
}

// buildPeerGroups aggregates connections by remote IP + process.
func buildPeerGroups(conns []collector.Connection, sortDesc bool) []PeerGroup {
	byKey := make(map[string]*PeerGroup)

	for _, conn := range conns {
		peer := normalizeIP(conn.RemoteIP)
		proc := strings.TrimSpace(conn.ProcName)
		if proc == "" {
			proc = "-"
		}
		key := peer + "|" + proc

		g, exists := byKey[key]
		if !exists {
			g = &PeerGroup{
				PeerIP:     peer,
				ProcName:   proc,
				LocalPorts: make(map[int]struct{}),
				States:     make(map[string]int),
			}
			byKey[key] = g
		}
		g.Count++
		g.TxQueue += conn.TxQueue
		g.RxQueue += conn.RxQueue
		g.TotalQueue += conn.Activity
		g.TxBytesDelta += conn.TxBytesDelta
		g.RxBytesDelta += conn.RxBytesDelta
		g.TotalBytesDelta += conn.TotalBytesDelta
		g.TxBytesPerSec += conn.TxBytesPerSec
		g.RxBytesPerSec += conn.RxBytesPerSec
		g.TotalBytesPerSec += conn.TotalBytesPerSec
		g.LocalPorts[conn.LocalPort] = struct{}{}
		g.States[conn.State]++
	}

	groups := make([]PeerGroup, 0, len(byKey))
	for _, g := range byKey {
		groups = append(groups, *g)
	}

	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Count != groups[j].Count {
			return compareInt(groups[i].Count, groups[j].Count, sortDesc)
		}
		if groups[i].TotalBytesDelta != groups[j].TotalBytesDelta {
			return groups[i].TotalBytesDelta > groups[j].TotalBytesDelta
		}
		if groups[i].TotalQueue != groups[j].TotalQueue {
			return groups[i].TotalQueue > groups[j].TotalQueue
		}
		if groups[i].PeerIP != groups[j].PeerIP {
			return groups[i].PeerIP < groups[j].PeerIP
		}
		return groups[i].ProcName < groups[j].ProcName
	})

	return groups
}

// renderPeerGroupPanel renders the per-peer aggregate view.
func renderPeerGroupPanel(conns []collector.Connection, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string) string {
	return renderPeerGroupPanelWithHint(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortDesc, thresholds, bandwidthNote, defaultGroupHintLine)
}

func renderPeerGroupPanelReadOnly(conns []collector.Connection, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string) string {
	return renderPeerGroupPanelWithHint(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortDesc, thresholds, bandwidthNote, readOnlyGroupHintLine)
}

func renderPeerGroupPanelWithHint(conns []collector.Connection, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string, hintLine string) string {
	var sb strings.Builder

	portChip := "Port Filter = ALL"
	if strings.TrimSpace(portFilter) != "" {
		portChip = "Port Filter = " + strings.TrimSpace(portFilter)
	}
	searchChip := "ALL"
	if strings.TrimSpace(textFilter) != "" {
		searchChip = truncateRight(strings.TrimSpace(textFilter), 20)
	}
	sortChip := "CONNS:ASC"
	if sortDesc {
		sortChip = "CONNS:DESC"
	}

	sb.WriteString(fmt.Sprintf(
		"  [dim]Chips:[white] [yellow]%s[white] | [yellow]Search=%s[white] | [yellow]Sort=%s[white] | [aqua]View=GROUP[white]\n",
		portChip,
		searchChip,
		sortChip,
	))
	sb.WriteString(hintLine)
	if strings.TrimSpace(bandwidthNote) != "" {
		sb.WriteString("\n  [yellow]")
		sb.WriteString(truncateRight(strings.TrimSpace(bandwidthNote), 160))
		sb.WriteString("[white]")
	}
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

	groups := buildPeerGroups(filtered, sortDesc)
	if selectedIndex < 0 {
		selectedIndex = 0
	}
	if selectedIndex >= len(groups) {
		selectedIndex = len(groups) - 1
	}

	const (
		peerColWidth    = 20
		countColWidth   = 6
		queueColWidth   = 6
		bwColWidth      = 8
		processColWidth = 9
		portsColWidth   = 10
		stateColWidth   = 31
	)

	sb.WriteString(fmt.Sprintf("  [dim]%-*s %*s %*s %*s %*s %*s %-*s %-*s %-*s[white]\n",
		peerColWidth, "PEER",
		countColWidth, "CONNS",
		queueColWidth, "SEND-Q",
		queueColWidth, "RECV-Q",
		bwColWidth, "TX/s",
		bwColWidth, "RX/s",
		processColWidth, "PROCESS",
		portsColWidth, "PORTS",
		stateColWidth, "STATE %",
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

		sendQColor := "dim"
		if g.TxQueue > 0 {
			sendQColor = "yellow"
		}
		recvQColor := "dim"
		if g.RxQueue > 0 {
			recvQColor = "yellow"
		}
		sendQField := fmt.Sprintf("[%s]%*s[white]", sendQColor, queueColWidth, formatBytes(g.TxQueue))
		recvQField := fmt.Sprintf("[%s]%*s[white]", recvQColor, queueColWidth, formatBytes(g.RxQueue))
		txRateColor := bandwidthColor(g.TxBytesPerSec, thresholds.BandwidthPerSec)
		rxRateColor := bandwidthColor(g.RxBytesPerSec, thresholds.BandwidthPerSec)
		txRateField := fmt.Sprintf("[%s]%*s[white]", txRateColor, bwColWidth, formatBytesRateCompact(g.TxBytesPerSec))
		rxRateField := fmt.Sprintf("[%s]%*s[white]", rxRateColor, bwColWidth, formatBytesRateCompact(g.RxBytesPerSec))

		procText := truncateRight(g.ProcName, processColWidth)
		procColor := "white"
		if g.ProcName == "-" {
			procColor = "dim"
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
		stateDisplay := truncateRight(formatPeerGroupStatePercent(g.States, g.Count), stateColWidth)

		// Color count by severity.
		countColor := "white"
		if g.Count >= 50 {
			countColor = "red"
		} else if g.Count >= 10 {
			countColor = "yellow"
		}

		prefix := rowSelectionPrefix(i == selectedIndex)

		sb.WriteString(fmt.Sprintf("%s%-*s [%s]%*d[white] %s %s %s %s %s %-*s %-*s\n",
			prefix,
			peerColWidth, peer,
			countColor, countColWidth, g.Count,
			sendQField,
			recvQField,
			txRateField,
			rxRateField,
			procField,
			portsColWidth, portsDisplay,
			stateColWidth, stateDisplay,
		))
	}

	sb.WriteString(fmt.Sprintf("\n  [dim]%d groups, %d peers, %d total connections[white]", len(groups), uniquePeerCount(groups), len(filtered)))

	return sb.String()
}

func uniquePeerCount(groups []PeerGroup) int {
	if len(groups) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		seen[g.PeerIP] = struct{}{}
	}
	return len(seen)
}

func formatPeerGroupStatePercent(states map[string]int, total int) string {
	if total <= 0 || len(states) == 0 {
		return "-"
	}

	items := make([]stateSummaryEntry, 0, len(states))
	for name, count := range states {
		if count <= 0 {
			continue
		}
		items = append(items, stateSummaryEntry{Name: name, Count: count})
	}
	if len(items) == 0 {
		return "-"
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		iRank, iKnown := peerGroupStateOrder[items[i].Name]
		jRank, jKnown := peerGroupStateOrder[items[j].Name]
		switch {
		case iKnown && jKnown && iRank != jRank:
			return iRank < jRank
		case iKnown != jKnown:
			return iKnown
		default:
			return items[i].Name < items[j].Name
		}
	})

	if len(items) > 3 {
		items = items[:3]
	}

	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s %s", shortStateName(item.Name), formatStatePercent(item.Count, total)))
	}
	return strings.Join(parts, " - ")
}

func formatStatePercent(count, total int) string {
	if total <= 0 || count <= 0 {
		return "0%"
	}
	if count == total {
		return "100%"
	}
	if count*100 < total {
		return "<1%"
	}
	percent := (count*100 + total/2) / total
	if percent >= 100 {
		percent = 99
	}
	return fmt.Sprintf("%d%%", percent)
}

func shortStateName(state string) string {
	switch state {
	case "ESTABLISHED":
		return "EST"
	case "TIME_WAIT":
		return "TW"
	case "CLOSE_WAIT":
		return "CW"
	case "LISTEN":
		return "LS"
	case "SYN_SENT":
		return "SS"
	case "SYN_RECV":
		return "SR"
	case "FIN_WAIT1":
		return "FW1"
	case "FIN_WAIT2":
		return "FW2"
	case "LAST_ACK":
		return "LA"
	case "CLOSING":
		return "CLG"
	case "CLOSE":
		return "CLS"
	default:
		return state
	}
}
