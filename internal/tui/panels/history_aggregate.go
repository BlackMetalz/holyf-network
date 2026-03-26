package panels

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/BlackMetalz/holyf-network/internal/history"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

func RenderHistoryAggregatePanel(rows []history.SnapshotGroup, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool, direction topConnectionDirection, skipEmpty bool, thresholds config.HealthThresholds, bandwidthAvailable bool) string {
	var sb strings.Builder

	var chips []string
	dirLabel := "Incoming"
	if direction == tuishared.TopConnectionOutgoing {
		dirLabel = "Outgoing"
	}
	chips = append(chips, fmt.Sprintf("[aqua]%s[white]", dirLabel))
	chips = append(chips, fmt.Sprintf("[yellow]Sort=%s[white]", tuishared.SortLabelWithDirection(sortMode, sortDesc)))
	if strings.TrimSpace(portFilter) != "" {
		chips = append(chips, fmt.Sprintf("[yellow]Port=%s[white]", strings.TrimSpace(portFilter)))
	}
	if strings.TrimSpace(textFilter) != "" {
		chips = append(chips, fmt.Sprintf("[yellow]Search=%s[white]", tuishared.TruncateRight(strings.TrimSpace(textFilter), 20)))
	}
	if sensitiveIP {
		chips = append(chips, "[yellow]MaskIP[white]")
	}
	if skipEmpty {
		chips = append(chips, "[yellow]SkipEmpty[white]")
	}
	chips = append(chips, "[aqua]View=AGG[white]")

	sb.WriteString(fmt.Sprintf("  %s\n", strings.Join(chips, " | ")))
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
		filtered = FilterHistoryGroupsByPort(filtered, portFilter)
	}
	if strings.TrimSpace(textFilter) != "" {
		filtered = FilterHistoryGroupsByText(filtered, textFilter)
	}
	if len(filtered) == 0 {
		sb.WriteString("  No rows matching current filters")
		return sb.String()
	}

	items := append([]history.SnapshotGroup(nil), filtered...)
	SortHistoryGroups(items, sortMode, sortDesc)

	if selectedIndex < 0 {
		selectedIndex = 0
	}
	if selectedIndex >= len(items) {
		selectedIndex = len(items) - 1
	}

	const (
		peerColWidth  = 20
		portColWidth  = 6
		procColWidth  = 10
		connsColWidth = 5
		queueColWidth = 6
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
		peer = tuishared.TruncateRight(peer, peerColWidth)
		port := strconv.Itoa(row.Port)
		procName := row.ProcName
		if procName == "unknown" || procName == "" {
			procName = "-"
		}
		proc := tuishared.TruncateRight(procName, procColWidth)
		conns := strconv.Itoa(row.ConnCount)
		sendQ := FormatBytes(row.TxQueue)
		recvQ := FormatBytes(row.RxQueue)
		txRate := FormatBytesRateCompact(row.TxBytesPerSec)
		rxRate := FormatBytesRateCompact(row.RxBytesPerSec)
		states := tuishared.TruncateRight(formatStateSummary(row.States), 34)
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

func SortHistoryGroups(rows []history.SnapshotGroup, mode SortMode, desc bool) {
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

func FilterHistoryGroupsByPort(rows []history.SnapshotGroup, portFilter string) []history.SnapshotGroup {
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

func FilterHistoryGroupsByText(rows []history.SnapshotGroup, query string) []history.SnapshotGroup {
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
			FormatBytes(row.TxQueue),
			FormatBytes(row.RxQueue),
			FormatBytes(row.TotalQueue),
			FormatBytes(row.TxBytesDelta),
			FormatBytes(row.RxBytesDelta),
			FormatBytes(row.TotalBytesDelta),
			FormatBytesRateCompact(row.TxBytesPerSec),
			FormatBytesRateCompact(row.RxBytesPerSec),
			FormatBytesRateCompact(row.TotalBytesPerSec),
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
