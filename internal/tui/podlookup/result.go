package podlookup

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/podlookup"
	"github.com/BlackMetalz/holyf-network/internal/tui/blocking"
	tuioverlays "github.com/BlackMetalz/holyf-network/internal/tui/overlays"
	"github.com/gdamore/tcell/v2"
)

const resultPageName = "pod-lookup-result"

// maxPeersShown caps the per-peer-IP rows rendered per pod block.
const maxPeersShown = 12

// ShowPodLookupResults renders one block per matching pod and shows them in a
// scrollable modal. The first block is the "primary" result; subsequent blocks
// are numbered so the user can tell how many pods on this node touch the port.
func ShowPodLookupResults(ctx blocking.UIContext, port int, results []*podlookup.PodLookupResult) {
	var sb strings.Builder

	if len(results) > 1 {
		fmt.Fprintf(&sb, "\n  [aqua]%d pods on this node touch port %d[white]\n", len(results), port)
		sb.WriteString("  [dim]Use ↑/↓, j/k, PgUp/PgDn to scroll. Esc or q to close.[white]\n")
	}

	for i, r := range results {
		if len(results) > 1 {
			fmt.Fprintf(&sb, "\n  [yellow::b]── #%d ──[-:-:-]\n", i+1)
		} else {
			sb.WriteString("\n")
		}
		writePodBlock(&sb, r)
	}

	sb.WriteString("\n  [dim]Press Esc to close[white]\n")

	showResultModal(ctx, fmt.Sprintf(" K8s Pod Lookup: port %d ", port), sb.String())
}

func writePodBlock(sb *strings.Builder, result *podlookup.PodLookupResult) {
	writeField := func(label, value string) {
		if value == "" {
			value = "[dim]-[white]"
		}
		fmt.Fprintf(sb, "  [yellow]%-16s[white] %s\n", label+":", value)
	}

	writeField("PID", fmt.Sprintf("%d", result.PID))
	writeField("Process", result.ProcName)
	writeField("Container ID", result.ContainerID)
	writeField("Pod", result.PodName)
	writeField("Namespace", result.PodNamespace)
	writeField("Deployment", result.Deployment)
	writeField("Network NS", result.NetNS)
	writeField("Local IP", result.LocalIP)
	writeField("State", result.State)

	writePeers(sb, result.Port, result.Peers)
}

// writePeers renders aggregated peer connections grouped by (remote IP, remote port).
// For server-side matches (LocalPort == targetPort), the remote port is
// ephemeral and uninteresting — we group by IP only.
// For client-side matches (RemotePort == targetPort), we keep the remote port
// so distinct upstream servers (e.g. 10.0.0.1:27017 vs 10.0.0.2:27017) are
// rendered as separate rows.
func writePeers(sb *strings.Builder, targetPort int, peers []podlookup.PeerConnection) {
	fmt.Fprintf(sb, "\n  [yellow]Peers (%d):[white]\n", len(peers))

	if len(peers) == 0 {
		fmt.Fprintf(sb, "    [dim]No established connections on this port.[white]\n")
		return
	}

	type peerKey struct {
		ip   string
		port int // 0 when the remote port is ephemeral (server-side connection)
	}
	type peerAgg struct {
		key   peerKey
		state string
		count int
	}

	groups := make(map[peerKey]*peerAgg)
	for _, p := range peers {
		k := peerKey{ip: p.RemoteIP}
		if p.RemotePort == targetPort && p.LocalPort != targetPort {
			k.port = p.RemotePort
		}
		agg, ok := groups[k]
		if !ok {
			agg = &peerAgg{key: k, state: p.State}
			groups[k] = agg
		}
		agg.count++
	}

	rows := make([]*peerAgg, 0, len(groups))
	for _, g := range groups {
		rows = append(rows, g)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count != rows[j].count {
			return rows[i].count > rows[j].count
		}
		return rows[i].key.ip < rows[j].key.ip
	})

	shown := rows
	hidden := 0
	if len(shown) > maxPeersShown {
		hidden = len(shown) - maxPeersShown
		shown = shown[:maxPeersShown]
	}

	for _, r := range shown {
		peer := r.key.ip
		if r.key.port != 0 {
			peer = fmt.Sprintf("%s:%d", r.key.ip, r.key.port)
		}
		conn := "conn"
		if r.count > 1 {
			conn = "conns"
		}
		fmt.Fprintf(sb, "    [aqua]%-30s[white] [dim]%-12s[white] %d %s\n",
			peer, r.state, r.count, conn)
	}
	if hidden > 0 {
		fmt.Fprintf(sb, "    [dim]… %d more peer(s) hidden[white]\n", hidden)
	}
}

// ShowPodLookupNotFound displays a not-found message.
func ShowPodLookupNotFound(ctx blocking.UIContext, port int, nsCount int, elapsed time.Duration) {
	text := fmt.Sprintf(
		"\n  Port [yellow]%d[white] not found in any network namespace.\n"+
			"  Scanned [aqua]%d[white] namespaces in %s.\n\n"+
			"  [dim]Press Esc to close[white]\n",
		port, nsCount, elapsed.Truncate(time.Millisecond))

	showResultModal(ctx, fmt.Sprintf(" K8s Pod Lookup: port %d ", port), text)
}

func showResultModal(ctx blocking.UIContext, title, text string) {
	modal, view := tuioverlays.CreateCenteredTextViewModal(title, text)

	closeFunc := func() {
		ctx.RemovePage(resultPageName)
		ctx.UpdateStatusBar()
		ctx.RestoreFocus()
	}

	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeFunc()
			return nil
		case tcell.KeyEnter,
			tcell.KeyUp, tcell.KeyDown,
			tcell.KeyPgUp, tcell.KeyPgDn,
			tcell.KeyHome, tcell.KeyEnd:
			return event // let TextView scroll
		}
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'q', 'Q':
				closeFunc()
				return nil
			case 'j', 'k', 'g', 'G', ' ':
				return event // vim-style scrolling
			}
			closeFunc()
			return nil
		}
		return event
	})

	ctx.RemovePage(resultPageName)
	ctx.AddPage(resultPageName, modal, true, true)
	ctx.SetFocus(view)
}
