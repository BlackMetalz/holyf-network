package tui

import (
	"fmt"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

// panel_talkers.go — Renders the Top Connections panel content.

// renderTalkersPanel formats the top connections for the TUI panel.
// If portFilter is set, only connections matching that port are shown.
func renderTalkersPanel(conns []collector.Connection, portFilter string) string {
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

	// Render each connection
	for i, conn := range filtered {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("\n  [dim]... and %d more[white]", len(filtered)-20))
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

		// Format: "  1234/nginx  127.0.0.1:3306 ↔ 10.0.0.5:45123  ESTABLISHED  256"
		local := fmt.Sprintf("%s:%d", conn.LocalIP, conn.LocalPort)
		remote := fmt.Sprintf("%s:%d", conn.RemoteIP, conn.RemotePort)

		// Truncate long IPs for display
		if len(local) > 21 {
			local = local[:18] + "..."
		}
		if len(remote) > 21 {
			remote = remote[:18] + "..."
		}

		queueStr := ""
		if conn.Activity > 0 {
			queueStr = fmt.Sprintf(" [yellow]%s[white]", formatBytes(conn.Activity))
		}

		sb.WriteString(fmt.Sprintf("  [aqua]%-14s[white] %-21s ↔ %-21s [%s]%-11s[white]%s\n",
			procInfo, local, remote, stateColor, conn.State, queueStr,
		))
	}

	// Show total count
	sb.WriteString(fmt.Sprintf("\n  [dim]Showing %d of %d connections[white]", min(len(filtered), 20), len(filtered)))

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

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
