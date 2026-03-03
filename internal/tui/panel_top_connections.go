package tui

import (
	"fmt"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

// panel_top_connections.go — Renders the Top Connections panel content.

// renderTalkersPanel formats the top connections for the TUI panel.
// If portFilter is set, only connections matching that port are shown.
// maxRows controls how many connections to display (use more when zoomed).
func renderTalkersPanel(conns []collector.Connection, portFilter string, maxRows int, sensitiveIP bool) string {
	var sb strings.Builder

	// Show active filter
	if portFilter != "" {
		sb.WriteString(fmt.Sprintf("  [yellow]Filter: :%s[white]  [dim](press f to clear)[white]\n", portFilter))
	} else {
		sb.WriteString("  [dim]Press f to filter by port[white]\n")
	}
	sb.WriteString("\n")

	if len(conns) == 0 {
		sb.WriteString("  No active connections found")
		return sb.String()
	}

	// Filter by port if set
	filtered := conns
	if portFilter != "" {
		filtered = filterByPort(conns, portFilter)
		if len(filtered) == 0 {
			sb.WriteString(fmt.Sprintf("  No connections matching port %s", portFilter))
			return sb.String()
		}
	}

	const (
		processColWidth    = 18
		dirColWidth        = 4
		endpointColWidth   = 20
		connectionColWidth = endpointColWidth*2 + 4 // "<local> <-> <remote>"
		stateColWidth      = 11
	)

	// Header row
	sb.WriteString(fmt.Sprintf("  [dim]%-*s %-*s %-*s %-*s %s[white]\n",
		processColWidth, "PROCESS",
		dirColWidth, "DIR",
		connectionColWidth, "CONNECTION",
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
		dir, arrow := connectionDirection(conn)
		local := formatEndpoint(conn.LocalIP, conn.LocalPort, endpointColWidth, sensitiveIP)
		remote := formatEndpoint(conn.RemoteIP, conn.RemotePort, endpointColWidth, sensitiveIP)
		connection := fmt.Sprintf("%-*s %2s %-*s",
			endpointColWidth, local,
			arrow,
			endpointColWidth, remote,
		)

		queueStr := " [dim]0B[white]"
		if conn.Activity > 0 {
			queueStr = fmt.Sprintf(" [yellow]%s[white]", formatBytes(conn.Activity))
		}

		sb.WriteString(fmt.Sprintf("  [aqua]%-*s[white] %-*s %-*s [%s]%-*s[white]%s\n",
			processColWidth, procInfo,
			dirColWidth, dir,
			connectionColWidth, connection,
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

// connectionDirection infers the flow direction from local/remote ports.
func connectionDirection(conn collector.Connection) (dir string, arrow string) {
	const systemPortThreshold = 1024

	switch {
	case conn.LocalPort <= systemPortThreshold && conn.RemotePort > systemPortThreshold:
		return "IN", "<-"
	case conn.RemotePort <= systemPortThreshold && conn.LocalPort > systemPortThreshold:
		return "OUT", "->"
	case conn.LocalPort < conn.RemotePort:
		return "IN", "<-"
	case conn.LocalPort > conn.RemotePort:
		return "OUT", "->"
	default:
		return "PEER", "<>"
	}
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
