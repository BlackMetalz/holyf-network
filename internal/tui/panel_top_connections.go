package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

// panel_top_connections.go — Renders the Top Connections panel content.

// SortMode controls how Top Connections are sorted.
type SortMode int

const (
	SortByQueue   SortMode = iota // tx_queue + rx_queue descending
	SortByState                   // state name alphabetical, then activity desc
	SortByPeer                    // group by remote IP, highest connection count first
	SortByProcess                 // process name alphabetical, then activity desc
)

// Label returns a short display name for the status bar chip.
func (m SortMode) Label() string {
	switch m {
	case SortByQueue:
		return "QUEUE"
	case SortByState:
		return "STATE"
	case SortByPeer:
		return "PEER"
	case SortByProcess:
		return "PROCESS"
	default:
		return "QUEUE"
	}
}

// NextSortMode cycles to the next sort mode.
func NextSortMode(m SortMode) SortMode {
	return (m + 1) % 4
}

// sortConnections sorts a slice in-place according to the given mode.
func sortConnections(conns []collector.Connection, mode SortMode) {
	switch mode {
	case SortByQueue:
		sort.SliceStable(conns, func(i, j int) bool {
			return conns[i].Activity > conns[j].Activity
		})
	case SortByState:
		sort.SliceStable(conns, func(i, j int) bool {
			if conns[i].State != conns[j].State {
				return conns[i].State < conns[j].State
			}
			return conns[i].Activity > conns[j].Activity
		})
	case SortByPeer:
		// Count connections per remote IP, then sort by count desc.
		counts := make(map[string]int)
		for _, c := range conns {
			counts[normalizeIP(c.RemoteIP)]++
		}
		sort.SliceStable(conns, func(i, j int) bool {
			ci := counts[normalizeIP(conns[i].RemoteIP)]
			cj := counts[normalizeIP(conns[j].RemoteIP)]
			if ci != cj {
				return ci > cj
			}
			if normalizeIP(conns[i].RemoteIP) != normalizeIP(conns[j].RemoteIP) {
				return normalizeIP(conns[i].RemoteIP) < normalizeIP(conns[j].RemoteIP)
			}
			return conns[i].Activity > conns[j].Activity
		})
	case SortByProcess:
		sort.SliceStable(conns, func(i, j int) bool {
			pi, pj := conns[i].ProcName, conns[j].ProcName
			if pi != pj {
				return pi < pj
			}
			return conns[i].Activity > conns[j].Activity
		})
	}
}

// renderTalkersPanel formats the top connections for the TUI panel.
// If portFilter is set, only connections matching that port are shown.
// maxRows controls how many connections to display (use more when zoomed).
func renderTalkersPanel(conns []collector.Connection, portFilter string, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode) string {
	var sb strings.Builder

	filterChip := "ALL"
	if strings.TrimSpace(portFilter) != "" {
		filterChip = ":" + strings.TrimSpace(portFilter)
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
		"  [dim]Chips:[white] [yellow]Filter=%s[white] | [yellow]MaskIP=%s[white] | [yellow]Search=%s[white] | [yellow]Sort=%s[white]\n",
		filterChip,
		maskChip,
		searchChip,
		sortMode.Label(),
	))
	sb.WriteString("  [dim]Use ↑/↓ select, Enter/k block, /=search, f=port/clear, o=sort[white]\n\n")

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
	sortConnections(filtered, sortMode)

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
	var port int
	fmt.Sscanf(portStr, "%d", &port)
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
