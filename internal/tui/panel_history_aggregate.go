package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/BlackMetalz/holyf-network/internal/history"
)

func renderHistoryAggregatePanel(rows []history.SnapshotGroup, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool, direction topConnectionDirection, skipEmpty bool, thresholds config.HealthThresholds, bandwidthAvailable bool) string {
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
		"  [dim]Chips:[white] [yellow]%s[white] | [yellow]MaskIP=%s[white] | [yellow]Search=%s[white] | [yellow]Sort=%s[white] | [yellow]SkipEmpty=%s[white] | [aqua]Dir=%s[white] | [aqua]View=AGG[white]\n",
		portChip,
		maskChip,
		searchChip,
		sortLabelWithDirection(sortMode, sortDesc),
		skipChip,
		direction.Label(),
	))
	sb.WriteString(historyAggregateHintLine(skipEmpty))
	if !bandwidthAvailable {
		sb.WriteString("\n  [yellow]Bandwidth counters unavailable in this snapshot[white]")
	}
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
		connsColWidth = 6
		queueColWidth = 7
		bwColWidth    = 9
	)
	portHeader := "LPORT"
	if direction == topConnectionOutgoing {
		portHeader = "RPORT"
	}
	sb.WriteString(fmt.Sprintf(
		"  [dim]%-*s %*s %-*s %*s %*s %*s %*s %*s %s[white]\n",
		peerColWidth, "PEER",
		portColWidth, portHeader,
		procColWidth, "PROC",
		connsColWidth, "CONNS",
		queueColWidth, "SEND-Q",
		queueColWidth, "RECV-Q",
		bwColWidth, "TX/s",
		bwColWidth, "RX/s",
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
		port := strconv.Itoa(row.Port)
		proc := truncateRight(row.ProcName, procColWidth)
		conns := strconv.Itoa(row.ConnCount)
		sendQ := formatBytes(row.TxQueue)
		recvQ := formatBytes(row.RxQueue)
		txRate := formatBytesRateCompact(row.TxBytesPerSec)
		rxRate := formatBytesRateCompact(row.RxBytesPerSec)
		states := truncateRight(formatStateSummary(row.States), 34)
		sendQColor := "dim"
		if row.TxQueue > 0 {
			sendQColor = "yellow"
		}
		recvQColor := "dim"
		if row.RxQueue > 0 {
			recvQColor = "yellow"
		}
		sendQField := fmt.Sprintf("[%s]%*s[white]", sendQColor, queueColWidth, sendQ)
		recvQField := fmt.Sprintf("[%s]%*s[white]", recvQColor, queueColWidth, recvQ)
		txRateField := fmt.Sprintf("[%s]%*s[white]", bandwidthColor(row.TxBytesPerSec, thresholds.BandwidthPerSec), bwColWidth, txRate)
		rxRateField := fmt.Sprintf("[%s]%*s[white]", bandwidthColor(row.RxBytesPerSec, thresholds.BandwidthPerSec), bwColWidth, rxRate)

		prefix := rowSelectionPrefix(i == selectedIndex)

		sb.WriteString(fmt.Sprintf(
			"%s[aqua]%-*s[white] %*s %-*s %*s %s %s %s %s [green]%s[white]\n",
			prefix,
			peerColWidth, peer,
			portColWidth, port,
			procColWidth, proc,
			connsColWidth, conns,
			sendQField,
			recvQField,
			txRateField,
			rxRateField,
			states,
		))
	}

	sb.WriteString(fmt.Sprintf("\n  [dim]Showing %d of %d aggregate rows[white]", min(len(items), maxRows), len(items)))
	return sb.String()
}

func historyAggregateHintLine(skipEmpty bool) string {
	base := "  [dim]Use ↑/↓ select, o=toggle IN/OUT, t=jump-time, /=snapshot search, Shift+S=timeline search, f=port/clear, Shift+B/C/P sort (toggle DESC/ASC), i/Shift+I=explain qcols, L=follow, "
	if skipEmpty {
		return base + "]=next active snapshot, [=previous active snapshot, x=show all snapshots[white]"
	}
	return base + "]=next snapshot, [=previous snapshot, x=skip empty snapshots[white]"
}

func sortHistoryGroups(rows []history.SnapshotGroup, mode SortMode, desc bool) {
	switch mode {
	case SortByBandwidth:
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].TotalBytesDelta != rows[j].TotalBytesDelta {
				return compareInt64(rows[i].TotalBytesDelta, rows[j].TotalBytesDelta, desc)
			}
			if rows[i].ConnCount != rows[j].ConnCount {
				return compareInt(rows[i].ConnCount, rows[j].ConnCount, desc)
			}
			if rows[i].Port != rows[j].Port {
				return compareInt(rows[i].Port, rows[j].Port, desc)
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
			if rows[i].TotalBytesDelta != rows[j].TotalBytesDelta {
				return compareInt64(rows[i].TotalBytesDelta, rows[j].TotalBytesDelta, desc)
			}
			if rows[i].Port != rows[j].Port {
				return compareInt(rows[i].Port, rows[j].Port, desc)
			}
			if rows[i].PeerIP != rows[j].PeerIP {
				return rows[i].PeerIP < rows[j].PeerIP
			}
			return rows[i].ProcName < rows[j].ProcName
		})
	case SortByPort:
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Port != rows[j].Port {
				return compareInt(rows[i].Port, rows[j].Port, desc)
			}
			if rows[i].ConnCount != rows[j].ConnCount {
				return compareInt(rows[i].ConnCount, rows[j].ConnCount, desc)
			}
			if rows[i].TotalBytesDelta != rows[j].TotalBytesDelta {
				return compareInt64(rows[i].TotalBytesDelta, rows[j].TotalBytesDelta, desc)
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
		if row.Port == port {
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
			strconv.Itoa(row.Port),
			row.ProcName,
			strconv.Itoa(row.ConnCount),
			formatBytes(row.TxQueue),
			formatBytes(row.RxQueue),
			formatBytes(row.TotalQueue),
			formatBytes(row.TxBytesDelta),
			formatBytes(row.RxBytesDelta),
			formatBytes(row.TotalBytesDelta),
			formatBytesRateCompact(row.TxBytesPerSec),
			formatBytesRateCompact(row.RxBytesPerSec),
			formatBytesRateCompact(row.TotalBytesPerSec),
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
