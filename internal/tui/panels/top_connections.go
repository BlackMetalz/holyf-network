package panels

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

// panel_top_connections.go — Renders the Top Connections panel content.

// SortMode controls how Top Connections are sorted.
type SortMode = tuishared.SortMode

const (
	SortByBandwidth = tuishared.SortByBandwidth
	SortByConns     = tuishared.SortByConns
	SortByPort      = tuishared.SortByPort
)

const (
	readOnlyTalkersHintLine = "  [dim]Use ↑/↓ select, [=prev, ]=next snapshot, a=oldest, e=latest, t=jump-time, /=search, f=port/clear, Shift+B/C/P sort (toggle DESC/ASC), i/Shift+I=explain qcols, g=group, L=follow[white]"
	readOnlyGroupHintLine   = "  [dim]Use ↑/↓ select, [=prev, ]=next snapshot, a=oldest, e=latest, t=jump-time, g=connections view, /=search, f=port/clear, Shift+C conns sort (toggle DESC/ASC), i/Shift+I=explain qcols, L=follow[white]"
)

type topConnectionDirection = tuishared.TopConnectionDirection

const (
	topConnectionIncoming           = tuishared.TopConnectionIncoming
	topConnectionOutgoing           = tuishared.TopConnectionOutgoing
	defaultTopConnectionsPanelWidth = tuishared.DefaultTopConnectionsPanelWidth
	TopConnectionsGroupCap          = tuishared.TopConnectionsGroupCap
	topConnectionsBaseReservedLines  = 7
	topConnectionsNoteReservedLines  = 1
	topConnectionsPreviewLines       = 5
	topConnectionsMinRows            = 5
	topConnectionsMinRowsWithPreview = 3
)

type TopConnectionsPanelLayout struct {
	PanelWidth  int
	PanelHeight int
	RowLimit    int
	ShowPreview bool
}

func CalculateTopConnectionsDisplayLimit(panelHeight int, noteCount int, wantsPreview bool) (int, bool) {
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

func TopConnectionsNoteCount(bandwidthNote string) int {
	if strings.TrimSpace(bandwidthNote) == "" {
		return 0
	}
	return 1
}

func TopConnectionsPageBounds(totalRows, rowsPerPage, pageIndex int) (page, start, end, totalPages int) {
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

func talkersHintLine(direction topConnectionDirection) string {
	if direction == topConnectionOutgoing {
		return "  [dim]Use ↑/↓ select, [=prev page, ]=next page, o=toggle IN/OUT, /=search, f=port/clear, T=trace packet, t=trace history, Shift+B/C/P sort (toggle DESC/ASC), i=explain qcols, Shift+I=explain iface[white]"
	}
	return "  [dim]Use ↑/↓ select, [=prev page, ]=next page, Enter/k block, o=toggle IN/OUT, /=search, f=port/clear, T=trace packet, t=trace history, Shift+B/C/P sort (toggle DESC/ASC), i=explain qcols, Shift+I=explain iface[white]"
}

func groupHintLine(direction topConnectionDirection) string {
	if direction == topConnectionOutgoing {
		return "  [dim]Use ↑/↓ select, [=prev page, ]=next page, g=connections view, o=toggle IN/OUT, /=search, f=port/clear, T=trace packet, t=trace history, Shift+C conns sort (toggle DESC/ASC), i=explain qcols, Shift+I=explain iface[white]"
	}
	return "  [dim]Use ↑/↓ select, [=prev page, ]=next page, g=connections view, o=toggle IN/OUT, /=search, f=port/clear, T=trace packet, t=trace history, Shift+C conns sort (toggle DESC/ASC), i=explain qcols, Shift+I=explain iface[white]"
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

func BuildSelectedConnectionPreview(
	conn collector.Connection,
	peerMatchesCount int,
	sensitiveIP bool,
	topDirection tuishared.TopConnectionDirection,
) *SelectedRowPreview {
	peerIP := tuishared.NormalizeIP(conn.RemoteIP)

	actionLine := fmt.Sprintf(
		"Action: Enter/k => peer %s -> local %d (%d matches; peer+port scope)",
		tuishared.FormatPreviewIP(peerIP, sensitiveIP),
		conn.LocalPort,
		peerMatchesCount,
	)
	if topDirection == tuishared.TopConnectionOutgoing {
		actionLine = "Action: Enter/k disabled in OUT mode (incoming-only mitigation)"
	}

	return &SelectedRowPreview{
		Title: "Selected Detail",
		Lines: []string{
			fmt.Sprintf(
				"Local: %s -> Peer: %s | State: %s",
				tuishared.FormatPreviewEndpoint(conn.LocalIP, conn.LocalPort, sensitiveIP),
				tuishared.FormatPreviewEndpoint(conn.RemoteIP, conn.RemotePort, sensitiveIP),
				conn.State,
			),
			fmt.Sprintf(
				"Proc: %s | Queue: send %s recv %s | BW: tx %s rx %s",
				FormatProcessInfo(conn),
				FormatBytes(conn.TxQueue),
				FormatBytes(conn.RxQueue),
				FormatBytesRateCompact(conn.TxBytesPerSec),
				FormatBytesRateCompact(conn.RxBytesPerSec),
			),
			actionLine,
		},
	}
}

func BuildSelectedPeerGroupPreview(
	group PeerGroup,
	peerMatchesCount int,
	bestLocalPort int,
	sensitiveIP bool,
	topDirection tuishared.TopConnectionDirection,
) *SelectedRowPreview {
	portSet := group.LocalPorts
	portLabel := "Ports"
	actionLine := fmt.Sprintf(
		"%s: %s | Action: Enter/k => local %d (%d matches)",
		portLabel,
		FormatAllPeerGroupPorts(portSet),
		bestLocalPort,
		peerMatchesCount,
	)
	if topDirection == tuishared.TopConnectionOutgoing {
		portSet = group.RemotePorts
		portLabel = "RPorts"
		actionLine = fmt.Sprintf(
			"%s: %s | Action: Enter/k disabled in OUT mode",
			portLabel,
			FormatAllPeerGroupPorts(portSet),
		)
	}

	return &SelectedRowPreview{
		Title: "Selected Detail",
		Lines: []string{
			fmt.Sprintf(
				"Peer: %s | Proc: %s | Conns: %d",
				tuishared.FormatPreviewIP(group.PeerIP, sensitiveIP),
				group.ProcName,
				group.Count,
			),
			fmt.Sprintf(
				"States: %s",
				FormatPeerGroupStatePreview(group.States, group.Count),
			),
			actionLine,
		},
	}
}

type SelectedRowPreview struct {
	Title string
	Lines []string
}

// sortConnections sorts a slice in-place according to the given mode.
func sortConnections(conns []collector.Connection, mode SortMode, desc bool) {
	SortConnectionsWithDirection(conns, mode, desc, topConnectionIncoming)
}

func SortConnectionsWithDirection(conns []collector.Connection, mode SortMode, desc bool, direction topConnectionDirection) {
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
			if direction.SortPort(conns[i]) != direction.SortPort(conns[j]) {
				return compareInt(direction.SortPort(conns[i]), direction.SortPort(conns[j]), desc)
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
			if direction.SortPort(conns[i]) != direction.SortPort(conns[j]) {
				return compareInt(direction.SortPort(conns[i]), direction.SortPort(conns[j]), desc)
			}
			return normalizeIP(conns[i].RemoteIP) < normalizeIP(conns[j].RemoteIP)
		})
	case SortByPort:
		sort.SliceStable(conns, func(i, j int) bool {
			if direction.SortPort(conns[i]) != direction.SortPort(conns[j]) {
				return compareInt(direction.SortPort(conns[i]), direction.SortPort(conns[j]), desc)
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
func RenderTalkersPanel(conns []collector.Connection, portFilter string, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string) string {
	return renderTalkersPanelWithHintAndPreview(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortMode, sortDesc, thresholds, bandwidthNote, talkersHintLine(topConnectionIncoming), defaultTopConnectionsPanelWidth, nil, 0, topConnectionIncoming)
}

func RenderTalkersPanelWithPreview(conns []collector.Connection, portFilter string, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string, panelWidth int, preview *SelectedRowPreview) string {
	return renderTalkersPanelWithHintAndPreview(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortMode, sortDesc, thresholds, bandwidthNote, talkersHintLine(topConnectionIncoming), panelWidth, preview, 0, topConnectionIncoming)
}

func RenderTalkersPanelReadOnly(conns []collector.Connection, portFilter string, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string) string {
	return renderTalkersPanelWithHintAndPreview(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortMode, sortDesc, thresholds, bandwidthNote, readOnlyTalkersHintLine, defaultTopConnectionsPanelWidth, nil, 0, topConnectionIncoming)
}

func RenderTalkersPanelWithPreviewDirection(conns []collector.Connection, portFilter string, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string, panelWidth int, preview *SelectedRowPreview, pageIndex int, direction topConnectionDirection) string {
	return renderTalkersPanelWithHintAndPreview(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortMode, sortDesc, thresholds, bandwidthNote, talkersHintLine(direction), panelWidth, preview, pageIndex, direction)
}

func renderTalkersPanelWithHintAndPreview(conns []collector.Connection, portFilter string, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortMode SortMode, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string, hintLine string, panelWidth int, preview *SelectedRowPreview, pageIndex int, direction topConnectionDirection) string {
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
		"  [dim]Chips:[white] [yellow]%s[white] | [yellow]MaskIP=%s[white] | [yellow]Search=%s[white] | [yellow]Sort=%s[white] | [aqua]Dir=%s[white] | [aqua]View=CONN[white]\n",
		portChip,
		maskChip,
		searchChip,
		sortLabelWithDirection(sortMode, sortDesc),
		direction.Label(),
	))
	sb.WriteString(hintLine)
	renderTopConnectionsNotes(&sb, bandwidthNote, panelWidth)

	if len(conns) == 0 {
		sb.WriteString("  No active connections found")
		return sb.String()
	}

	// Apply filters (port + contains text).
	filtered := conns
	if portFilter != "" {
		filtered = FilterByPortDirection(conns, portFilter, direction)
	}
	if textFilter != "" {
		filtered = FilterByText(filtered, textFilter)
	}
	if len(filtered) == 0 {
		sb.WriteString("  No connections matching current filters")
		return sb.String()
	}

	// Apply sort on the filtered result set.
	SortConnectionsWithDirection(filtered, sortMode, sortDesc, direction)
	page, start, end, totalPages := TopConnectionsPageBounds(len(filtered), maxRows, pageIndex)
	visible := filtered[start:end]

	if selectedIndex < 0 {
		selectedIndex = 0
	}
	if selectedIndex >= len(visible) {
		selectedIndex = len(visible) - 1
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

	// Render current page connections.
	for i, conn := range visible {

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
		procInfo := FormatProcessInfo(conn)
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
		sendQField := fmt.Sprintf("[%s]%*s[white]", sendQColor, queueColWidth, FormatBytes(conn.TxQueue))
		recvQField := fmt.Sprintf("[%s]%*s[white]", recvQColor, queueColWidth, FormatBytes(conn.RxQueue))
		txRateColor := bandwidthColor(conn.TxBytesPerSec, thresholds.BandwidthPerSec)
		rxRateColor := bandwidthColor(conn.RxBytesPerSec, thresholds.BandwidthPerSec)
		txRateField := fmt.Sprintf("[%s]%*s[white]", txRateColor, bwColWidth, FormatBytesRateCompact(conn.TxBytesPerSec))
		rxRateField := fmt.Sprintf("[%s]%*s[white]", rxRateColor, bwColWidth, FormatBytesRateCompact(conn.RxBytesPerSec))

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

	renderSelectedRowPreview(&sb, preview, panelWidth)

	// Show total count
	if totalPages <= 1 {
		sb.WriteString(fmt.Sprintf("\n  [dim]Showing %d of %d connections[white]", len(visible), len(filtered)))
	} else {
		sb.WriteString(fmt.Sprintf("\n  [dim]Showing %d-%d of %d connections | Page %d/%d[white]", start+1, end, len(filtered), page+1, totalPages))
	}

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

func FilterByPortDirection(conns []collector.Connection, portStr string, direction topConnectionDirection) []collector.Connection {
	port := parsePortFilter(portStr)
	if port == 0 {
		return conns
	}

	result := make([]collector.Connection, 0, len(conns))
	for _, conn := range conns {
		if direction.FilterMatchesPort(conn, port) {
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

func filterByRemotePort(conns []collector.Connection, portStr string) []collector.Connection {
	port := parsePortFilter(portStr)
	if port == 0 {
		return conns
	}

	result := make([]collector.Connection, 0, len(conns))
	for _, conn := range conns {
		if conn.RemotePort == port {
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
	return ApplyGroupConnectionFiltersByDirection(conns, portFilter, textFilter, topConnectionIncoming)
}

func ApplyGroupConnectionFiltersByDirection(conns []collector.Connection, portFilter, textFilter string, direction topConnectionDirection) []collector.Connection {
	filtered := conns
	if strings.TrimSpace(portFilter) != "" {
		if direction == topConnectionOutgoing {
			filtered = filterByRemotePort(filtered, portFilter)
		} else {
			filtered = filterByLocalPort(filtered, portFilter)
		}
	}
	if strings.TrimSpace(textFilter) != "" {
		filtered = FilterByText(filtered, textFilter)
	}
	return filtered
}

// filterByText returns only connections whose rendered fields contain query.
func FilterByText(conns []collector.Connection, query string) []collector.Connection {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return conns
	}

	result := make([]collector.Connection, 0, len(conns))
	for _, conn := range conns {
		procInfo := FormatProcessInfo(conn)
		haystack := strings.ToLower(strings.Join([]string{
			procInfo,
			normalizeIP(conn.LocalIP),
			normalizeIP(conn.RemoteIP),
			fmt.Sprintf("%s:%d", normalizeIP(conn.LocalIP), conn.LocalPort),
			fmt.Sprintf("%s:%d", normalizeIP(conn.RemoteIP), conn.RemotePort),
			conn.State,
			FormatBytes(conn.Activity),
			FormatBytes(conn.TxQueue),
			FormatBytes(conn.RxQueue),
			FormatBytes(conn.TxBytesDelta),
			FormatBytes(conn.RxBytesDelta),
			FormatBytes(conn.TotalBytesDelta),
			FormatBytesRateCompact(conn.TxBytesPerSec),
			FormatBytesRateCompact(conn.RxBytesPerSec),
			FormatBytesRateCompact(conn.TotalBytesPerSec),
		}, " "))

		if strings.Contains(haystack, needle) {
			result = append(result, conn)
		}
	}
	return result
}

func FormatProcessInfo(conn collector.Connection) string {
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
func FormatBytes(bytes int64) string {
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
	if bytes >= 1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%dB", bytes)
}

func FormatBytesRateCompact(rate float64) string {
	if rate < 0 {
		rate = 0
	}
	return FormatBytes(int64(rate)) + "/s"
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
	return truncateEndpoint(formatPreviewEndpoint(ip, port, sensitiveIP), width)
}

func formatPreviewEndpoint(ip string, port int, sensitiveIP bool) string {
	displayIP := formatPreviewIP(ip, sensitiveIP)
	if strings.Contains(displayIP, ":") && !strings.Contains(displayIP, ".") {
		return fmt.Sprintf("[%s]:%d", displayIP, port)
	}
	return fmt.Sprintf("%s:%d", displayIP, port)
}

func formatPreviewIP(ip string, sensitiveIP bool) string {
	displayIP := normalizeIP(ip)
	if sensitiveIP {
		displayIP = maskIP(displayIP)
	}
	return displayIP
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
	RemotePorts      map[int]struct{}
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

func sortedStateSummaryEntries(states map[string]int) []stateSummaryEntry {
	items := make([]stateSummaryEntry, 0, len(states))
	for name, count := range states {
		if count <= 0 {
			continue
		}
		items = append(items, stateSummaryEntry{Name: name, Count: count})
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
	return items
}

func SortedPeerGroupPorts(ports map[int]struct{}) []int {
	portList := make([]int, 0, len(ports))
	for p := range ports {
		portList = append(portList, p)
	}
	sort.Ints(portList)
	return portList
}

func formatPeerGroupPorts(ports map[int]struct{}, maxItems int) string {
	portList := SortedPeerGroupPorts(ports)
	if len(portList) == 0 {
		return "-"
	}

	limit := len(portList)
	if maxItems >= 0 && maxItems < limit {
		limit = maxItems
	}

	portStrs := make([]string, 0, limit+1)
	for i, port := range portList {
		if i >= limit {
			portStrs = append(portStrs, fmt.Sprintf("+%d", len(portList)-limit))
			break
		}
		portStrs = append(portStrs, fmt.Sprintf("%d", port))
	}
	return strings.Join(portStrs, ",")
}

func FormatAllPeerGroupPorts(ports map[int]struct{}) string {
	return formatPeerGroupPorts(ports, -1)
}

// buildPeerGroups aggregates connections by remote IP + process.
func BuildPeerGroups(conns []collector.Connection, sortDesc bool) []PeerGroup {
	return BuildPeerGroupsWithDirection(conns, sortDesc, topConnectionIncoming)
}

func BuildPeerGroupsWithDirection(conns []collector.Connection, sortDesc bool, direction topConnectionDirection) []PeerGroup {
	_ = direction
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
				PeerIP:      peer,
				ProcName:    proc,
				LocalPorts:  make(map[int]struct{}),
				RemotePorts: make(map[int]struct{}),
				States:      make(map[string]int),
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
		g.RemotePorts[conn.RemotePort] = struct{}{}
		g.States[conn.State]++
	}

	groups := make([]PeerGroup, 0, len(byKey))
	for _, g := range byKey {
		groups = append(groups, *g)
	}

	sortPeerGroups(groups, sortDesc)
	return groups
}

func sortPeerGroups(groups []PeerGroup, sortDesc bool) {
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
}

func LimitPeerGroups(groups []PeerGroup, maxGroups int, displayDesc bool) []PeerGroup {
	if maxGroups <= 0 || len(groups) <= maxGroups {
		return groups
	}

	ranked := append([]PeerGroup(nil), groups...)
	sortPeerGroups(ranked, true)
	ranked = append([]PeerGroup(nil), ranked[:maxGroups]...)
	sortPeerGroups(ranked, displayDesc)
	return ranked
}

// renderPeerGroupPanel renders the per-peer aggregate view.
func RenderPeerGroupPanel(conns []collector.Connection, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string) string {
	return renderPeerGroupPanelWithHintAndPreview(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortDesc, thresholds, bandwidthNote, groupHintLine(topConnectionIncoming), defaultTopConnectionsPanelWidth, nil, 0, topConnectionIncoming)
}

func RenderPeerGroupPanelWithPreview(conns []collector.Connection, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string, panelWidth int, preview *SelectedRowPreview) string {
	return renderPeerGroupPanelWithHintAndPreview(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortDesc, thresholds, bandwidthNote, groupHintLine(topConnectionIncoming), panelWidth, preview, 0, topConnectionIncoming)
}

func RenderPeerGroupPanelReadOnly(conns []collector.Connection, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string) string {
	return renderPeerGroupPanelWithHintAndPreview(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortDesc, thresholds, bandwidthNote, readOnlyGroupHintLine, defaultTopConnectionsPanelWidth, nil, 0, topConnectionIncoming)
}

func RenderPeerGroupPanelWithPreviewDirection(conns []collector.Connection, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string, panelWidth int, preview *SelectedRowPreview, pageIndex int, direction topConnectionDirection) string {
	return renderPeerGroupPanelWithHintAndPreview(conns, portFilter, textFilter, maxRows, sensitiveIP, selectedIndex, sortDesc, thresholds, bandwidthNote, groupHintLine(direction), panelWidth, preview, pageIndex, direction)
}

func renderPeerGroupPanelWithHintAndPreview(conns []collector.Connection, portFilter, textFilter string, maxRows int, sensitiveIP bool, selectedIndex int, sortDesc bool, thresholds config.HealthThresholds, bandwidthNote string, hintLine string, panelWidth int, preview *SelectedRowPreview, pageIndex int, direction topConnectionDirection) string {
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
		"  [dim]Chips:[white] [yellow]%s[white] | [yellow]Search=%s[white] | [yellow]Sort=%s[white] | [aqua]Dir=%s[white] | [aqua]View=GROUP[white]\n",
		portChip,
		searchChip,
		sortChip,
		direction.Label(),
	))
	sb.WriteString(hintLine)
	renderTopConnectionsNotes(&sb, bandwidthNote, panelWidth)

	if len(conns) == 0 {
		sb.WriteString("  No active connections found")
		return sb.String()
	}

	// Apply filters first, then group.
	filtered := ApplyGroupConnectionFiltersByDirection(conns, portFilter, textFilter, direction)
	if len(filtered) == 0 {
		sb.WriteString("  No connections matching current filters")
		return sb.String()
	}

	allGroups := BuildPeerGroupsWithDirection(filtered, sortDesc, direction)
	groups := LimitPeerGroups(allGroups, TopConnectionsGroupCap, sortDesc)
	page, start, end, totalPages := TopConnectionsPageBounds(len(groups), maxRows, pageIndex)
	visibleGroups := groups[start:end]
	if selectedIndex < 0 {
		selectedIndex = 0
	}
	if selectedIndex >= len(visibleGroups) {
		selectedIndex = len(visibleGroups) - 1
	}

	const (
		peerColWidth    = 20
		countColWidth   = 6
		queueColWidth   = 6
		bwColWidth      = 8
		processColWidth = 12
		portsColWidth   = 18
	)

	sb.WriteString(fmt.Sprintf("  [dim]%-*s %*s %*s %*s %*s %*s %-*s %-*s[white]\n",
		peerColWidth, "PEER",
		countColWidth, "CONNS",
		queueColWidth, "SEND-Q",
		queueColWidth, "RECV-Q",
		bwColWidth, "TX/s",
		bwColWidth, "RX/s",
		processColWidth, "PROCESS",
		portsColWidth, direction.GroupPortHeader(),
	))

	for i, g := range visibleGroups {

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
		sendQField := fmt.Sprintf("[%s]%*s[white]", sendQColor, queueColWidth, FormatBytes(g.TxQueue))
		recvQField := fmt.Sprintf("[%s]%*s[white]", recvQColor, queueColWidth, FormatBytes(g.RxQueue))
		txRateColor := bandwidthColor(g.TxBytesPerSec, thresholds.BandwidthPerSec)
		rxRateColor := bandwidthColor(g.RxBytesPerSec, thresholds.BandwidthPerSec)
		txRateField := fmt.Sprintf("[%s]%*s[white]", txRateColor, bwColWidth, FormatBytesRateCompact(g.TxBytesPerSec))
		rxRateField := fmt.Sprintf("[%s]%*s[white]", rxRateColor, bwColWidth, FormatBytesRateCompact(g.RxBytesPerSec))

		procText := truncateRight(g.ProcName, processColWidth)
		procColor := "white"
		if g.ProcName == "-" {
			procColor = "dim"
		}
		procField := fmt.Sprintf("[%s]%-*s[white]", procColor, processColWidth, procText)

		portSet := g.LocalPorts
		if direction == topConnectionOutgoing {
			portSet = g.RemotePorts
		}
		portsDisplay := truncateRight(formatPeerGroupPorts(portSet, 6), portsColWidth)

		// Color count by severity.
		countColor := "white"
		if g.Count >= 50 {
			countColor = "red"
		} else if g.Count >= 10 {
			countColor = "yellow"
		}

		prefix := rowSelectionPrefix(i == selectedIndex)

		sb.WriteString(fmt.Sprintf("%s%-*s [%s]%*d[white] %s %s %s %s %s %-*s\n",
			prefix,
			peerColWidth, peer,
			countColor, countColWidth, g.Count,
			sendQField,
			recvQField,
			txRateField,
			rxRateField,
			procField,
			portsColWidth, portsDisplay,
		))
	}

	renderSelectedRowPreview(&sb, preview, panelWidth)

	if len(allGroups) > len(groups) {
		sb.WriteString(fmt.Sprintf(
			"\n  [dim]%d shown / %d groups, %d peers, %d total connections[white]",
			len(groups),
			len(allGroups),
			uniquePeerCount(allGroups),
			len(filtered),
		))
	} else {
		sb.WriteString(fmt.Sprintf("\n  [dim]%d groups, %d peers, %d total connections[white]", len(groups), uniquePeerCount(groups), len(filtered)))
	}
	if totalPages > 1 {
		sb.WriteString(fmt.Sprintf(" [dim]| Page %d/%d[white]", page+1, totalPages))
	}

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

func FormatPeerGroupStatePreview(states map[string]int, total int) string {
	if total <= 0 || len(states) == 0 {
		return "-"
	}

	items := sortedStateSummaryEntries(states)
	if len(items) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s %s (%d)", tuishared.ShortStateName(item.Name), FormatStatePercent(item.Count, total), item.Count))
	}
	return strings.Join(parts, " - ")
}

func FormatStatePercent(count, total int) string {
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


func renderSelectedRowPreview(sb *strings.Builder, preview *SelectedRowPreview, panelWidth int) {
	if preview == nil || len(preview.Lines) == 0 {
		return
	}

	contentWidth := previewContentWidth(panelWidth)
	title := strings.TrimSpace(preview.Title)
	if title == "" {
		title = "Selected Detail"
	}

	sb.WriteString("\n\n  [bold]")
	sb.WriteString(truncateRight("── "+title+" ──", contentWidth))
	sb.WriteString("[white]\n")

	for i, line := range preview.Lines {
		sb.WriteString("  ")
		sb.WriteString(truncateRight(strings.TrimSpace(line), contentWidth))
		if i < len(preview.Lines)-1 {
			sb.WriteString("\n")
		}
	}
}

func renderTopConnectionsNotes(sb *strings.Builder, bandwidthNote string, panelWidth int) {
	contentWidth := previewContentWidth(panelWidth)

	if strings.TrimSpace(bandwidthNote) != "" {
		sb.WriteString("\n  [yellow]")
		sb.WriteString(truncateRight(strings.TrimSpace(bandwidthNote), contentWidth))
		sb.WriteString("[white]")
	}
	sb.WriteString("\n\n")
}

func previewContentWidth(panelWidth int) int {
	if panelWidth <= 0 {
		panelWidth = defaultTopConnectionsPanelWidth
	}
	if panelWidth <= 4 {
		return panelWidth
	}
	return panelWidth - 2
}
