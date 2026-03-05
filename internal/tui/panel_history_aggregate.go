package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/history"
)

const historyAggregateHintLine = "  [dim]Use ↑/↓ select, [=prev, ]=next snapshot, t=jump-time, /=search, f=port/clear, Shift+Q/C/P sort (toggle DESC/ASC), x=skip-empty, L=follow[white]"

func renderHistoryAggregatePanel(rows []history.SnapshotGroup, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool, skipEmpty bool) string {
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
	skipChip := "OFF"
	if skipEmpty {
		skipChip = "ON"
	}

	sb.WriteString(fmt.Sprintf(
		"  [dim]Chips:[white] [yellow]%s[white] | [yellow]MaskIP=%s[white] | [yellow]Search=%s[white] | [yellow]Sort=%s[white] | [yellow]SkipEmpty=%s[white] | [aqua]View=AGG[white]\n",
		portChip,
		maskChip,
		searchChip,
		sortLabelWithDirection(sortMode, sortDesc),
		skipChip,
	))
	sb.WriteString(historyAggregateHintLine)
	sb.WriteString("\n\n")

	if len(rows) == 0 {
		sb.WriteString("  No aggregate rows found")
		return sb.String()
	}

	filtered := rows
	if strings.TrimSpace(portFilter) != "" {
		filtered = filterHistoryGroupsByPort(filtered, portFilter)
	}
	if strings.TrimSpace(textFilter) != "" {
		filtered = filterHistoryGroupsByText(filtered, textFilter)
	}
	if len(filtered) == 0 {
		sb.WriteString("  No rows matching current filters")
		return sb.String()
	}

	items := append([]history.SnapshotGroup(nil), filtered...)
	sortHistoryGroups(items, sortMode, sortDesc)

	if selectedIndex < 0 {
		selectedIndex = 0
	}
	if selectedIndex >= len(items) {
		selectedIndex = len(items) - 1
	}

	const (
		peerColWidth  = 24
		portColWidth  = 6
		procColWidth  = 14
		connsColWidth = 7
		queueColWidth = 10
	)
	sb.WriteString(fmt.Sprintf(
		"  [dim]%-*s %*s %-*s %*s %*s %s[white]\n",
		peerColWidth, "PEER",
		portColWidth, "PORT",
		procColWidth, "PROC",
		connsColWidth, "CONNS",
		queueColWidth, "QUEUE",
		"STATES",
	))

	for i, row := range items {
		if i >= maxRows {
			sb.WriteString(fmt.Sprintf("\n  [dim]... and %d more[white]", len(items)-maxRows))
			break
		}

		peer := row.PeerIP
		if sensitiveIP {
			peer = maskIP(peer)
		}
		peer = truncateRight(peer, peerColWidth)
		port := strconv.Itoa(row.LocalPort)
		proc := truncateRight(row.ProcName, procColWidth)
		conns := strconv.Itoa(row.ConnCount)
		queue := formatBytes(row.TotalQueue)
		states := truncateRight(formatStateSummary(row.States), 34)

		prefix := "  "
		if i == selectedIndex {
			prefix = " [yellow]>[white]"
		}

		sb.WriteString(fmt.Sprintf(
			"%s[aqua]%-*s[white] %*s %-*s %*s %*s [green]%s[white]\n",
			prefix,
			peerColWidth, peer,
			portColWidth, port,
			procColWidth, proc,
			connsColWidth, conns,
			queueColWidth, queue,
			states,
		))
	}

	sb.WriteString(fmt.Sprintf("\n  [dim]Showing %d of %d aggregate rows[white]", min(len(items), maxRows), len(items)))
	return sb.String()
}

func sortHistoryGroups(rows []history.SnapshotGroup, mode SortMode, desc bool) {
	switch mode {
	case SortByQueue:
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].TotalQueue != rows[j].TotalQueue {
				return compareInt64(rows[i].TotalQueue, rows[j].TotalQueue, desc)
			}
			if rows[i].ConnCount != rows[j].ConnCount {
				return compareInt(rows[i].ConnCount, rows[j].ConnCount, desc)
			}
			if rows[i].LocalPort != rows[j].LocalPort {
				return compareInt(rows[i].LocalPort, rows[j].LocalPort, desc)
			}
			if rows[i].PeerIP != rows[j].PeerIP {
				return rows[i].PeerIP < rows[j].PeerIP
			}
			return rows[i].ProcName < rows[j].ProcName
		})
	case SortByConns:
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].ConnCount != rows[j].ConnCount {
				return compareInt(rows[i].ConnCount, rows[j].ConnCount, desc)
			}
			if rows[i].TotalQueue != rows[j].TotalQueue {
				return compareInt64(rows[i].TotalQueue, rows[j].TotalQueue, desc)
			}
			if rows[i].LocalPort != rows[j].LocalPort {
				return compareInt(rows[i].LocalPort, rows[j].LocalPort, desc)
			}
			if rows[i].PeerIP != rows[j].PeerIP {
				return rows[i].PeerIP < rows[j].PeerIP
			}
			return rows[i].ProcName < rows[j].ProcName
		})
	case SortByPort:
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].LocalPort != rows[j].LocalPort {
				return compareInt(rows[i].LocalPort, rows[j].LocalPort, desc)
			}
			if rows[i].ConnCount != rows[j].ConnCount {
				return compareInt(rows[i].ConnCount, rows[j].ConnCount, desc)
			}
			if rows[i].TotalQueue != rows[j].TotalQueue {
				return compareInt64(rows[i].TotalQueue, rows[j].TotalQueue, desc)
			}
			if rows[i].PeerIP != rows[j].PeerIP {
				return rows[i].PeerIP < rows[j].PeerIP
			}
			return rows[i].ProcName < rows[j].ProcName
		})
	}
}

func filterHistoryGroupsByPort(rows []history.SnapshotGroup, portFilter string) []history.SnapshotGroup {
	port := parsePortFilter(portFilter)
	if port == 0 {
		return rows
	}

	result := make([]history.SnapshotGroup, 0, len(rows))
	for _, row := range rows {
		if row.LocalPort == port {
			result = append(result, row)
		}
	}
	return result
}

func filterHistoryGroupsByText(rows []history.SnapshotGroup, query string) []history.SnapshotGroup {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return rows
	}

	result := make([]history.SnapshotGroup, 0, len(rows))
	for _, row := range rows {
		haystack := strings.ToLower(strings.Join([]string{
			row.PeerIP,
			strconv.Itoa(row.LocalPort),
			row.ProcName,
			strconv.Itoa(row.ConnCount),
			formatBytes(row.TotalQueue),
			formatStateSummary(row.States),
		}, " "))
		if strings.Contains(haystack, needle) {
			result = append(result, row)
		}
	}
	return result
}

func dominantState(states map[string]int) string {
	if len(states) == 0 {
		return "UNKNOWN"
	}
	bestState := "UNKNOWN"
	bestCount := -1
	for state, count := range states {
		if count > bestCount || (count == bestCount && state < bestState) {
			bestState = state
			bestCount = count
		}
	}
	return bestState
}

func formatStateSummary(states map[string]int) string {
	if len(states) == 0 {
		return "UNKNOWN:0"
	}

	type stateCount struct {
		state string
		count int
	}
	items := make([]stateCount, 0, len(states))
	for state, count := range states {
		items = append(items, stateCount{state: state, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].state < items[j].state
	})

	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s:%d", item.state, item.count))
	}
	return strings.Join(parts, ",")
}
