package overlays

import (
	"fmt"
	"strings"

	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

type LiveHelpContext struct {
	FocusIndex int
	Direction  tuishared.TopConnectionDirection
	GroupView  bool
}

type liveHelpEntry struct {
	label string
	desc  string
}

func LiveMainStatusHotkeys(focusIndex int, direction tuishared.TopConnectionDirection) (styled string, plain string) {
	switch focusIndex {
	case 2:
		if direction == tuishared.TopConnectionOutgoing {
			return "[dim]Up/Down[white]=select [dim]pg[white]=page [dim]o[white]=IN [dim]g[white]=group [dim]/[white]=search [dim]f[white]=filter [dim]T[white]=trace [dim]t[white]=traces [dim]Enter/k[white]=disabled [dim]Tab[white]=panel [dim]?[white]=help", "Up/Down=select [ ]=page o=IN g=group /=search f=filter T=trace t=traces Enter/k=disabled Tab=panel ?=help"
		}
		return "[dim]Up/Down[white]=select [dim]pg[white]=page [dim]o[white]=OUT [dim]g[white]=group [dim]/[white]=search [dim]f[white]=filter [dim]T[white]=trace [dim]t[white]=traces [dim]Enter/k[white]=act [dim]Tab[white]=panel [dim]?[white]=help", "Up/Down=select [ ]=page o=OUT g=group /=search f=filter T=trace t=traces Enter/k=act Tab=panel ?=help"
	case 0:
		return "[dim]s[white]=sort [dim]Shift+I[white]=explain [dim]Tab[white]=panel [dim]Ctrl+1..3[white]=focus [dim]?[white]=help", "s=sort Shift+I=explain Tab=panel Ctrl+1..3=focus ?=help"
	case 1:
		return "[dim]d[white]=history [dim]Tab[white]=panel [dim]Ctrl+1..3[white]=focus [dim]?[white]=help", "d=history Tab=panel Ctrl+1..3=focus ?=help"
	default:
		return "[dim]Tab[white]=panel [dim]Ctrl+1..3[white]=focus [dim]r[white]=refresh [dim]?[white]=help", "Tab=panel Ctrl+1..3=focus r=refresh ?=help"
	}
}

func BuildLiveHelpText(ctx LiveHelpContext) string {
	currentTitle, currentEntries := currentPanelHelpSection(ctx)
	globalEntries := []liveHelpEntry{
		{label: "Tab / Shift+Tab", desc: "Move focus between panels"},
		{label: "Ctrl+1..3", desc: "Focus 1=Top 2=System Health 3=Diagnosis"},
		{label: "r", desc: "Refresh now"},
		{label: "p", desc: "Pause / resume auto-refresh"},
		{label: "m", desc: "Toggle sensitive IP mask"},
		{label: "t", desc: "Open trace packet history"},
		{label: "?", desc: "Close help"},
		{label: "q", desc: "Quit"},
	}
	otherEntries := otherPanelHelpEntries(ctx)

	sections := []string{
		renderLiveHelpSection("Current Panel", currentTitle, currentEntries),
		renderLiveHelpSection("Global Navigation", "", globalEntries),
		renderLiveHelpSection("Other Panels", "", otherEntries),
		"[dim]Press Esc or any key to close[white]",
	}
	return strings.Join(sections, "\n\n")
}

func currentPanelHelpSection(ctx LiveHelpContext) (string, []liveHelpEntry) {
	switch ctx.FocusIndex {
	case 2:
		title := fmt.Sprintf("Top Connections (%s, %s view)", ctx.Direction.Label(), topViewLabel(ctx.GroupView))
		entries := []liveHelpEntry{
			{label: "Up/Down", desc: "Select row"},
			{label: "[ / ]", desc: "Previous / next page"},
			{label: "o", desc: fmt.Sprintf("Toggle to %s mode", oppositeDirectionLabel(ctx.Direction))},
			{label: "g", desc: topGroupToggleLabel(ctx.GroupView)},
			{label: "/", desc: fmt.Sprintf("Search current %s list", topViewSearchLabel(ctx.GroupView))},
			{label: "f", desc: "Filter by shown port / clear"},
			{label: "T", desc: "Trace packet for selected peer/port"},
			{label: "t", desc: "Open trace packet history"},
		}
		if ctx.GroupView {
			entries = append(entries, liveHelpEntry{label: "Shift+C", desc: "Sort by connection count"})
		} else {
			entries = append(entries, liveHelpEntry{label: "Shift+B/C/P", desc: "Sort by bandwidth / conns / port"})
		}
		if ctx.Direction == tuishared.TopConnectionOutgoing {
			entries = append(entries, liveHelpEntry{label: "Enter / k", desc: "Disabled in OUT mode"})
		} else {
			entries = append(entries, liveHelpEntry{label: "Enter / k", desc: "Block selected target"})
		}
		entries = append(entries, liveHelpEntry{label: "z", desc: "Zoom Top Connections"})
		return title, entries
	case 0:
		return "System Health", []liveHelpEntry{
			{label: "s", desc: "Sort state rows by count (DESC/ASC)"},
			{label: "Shift+I", desc: "Explain RX/TX, packet rate, app CPU/mem, errors, and drops"},
		}
	case 1:
		return "Diagnosis", []liveHelpEntry{{label: "d", desc: "Show diagnosis history"}}
	default:
		return "Dashboard", nil
	}
}

func otherPanelHelpEntries(ctx LiveHelpContext) []liveHelpEntry {
	entries := make([]liveHelpEntry, 0, 4)
	if ctx.FocusIndex != 2 {
		entries = append(entries, liveHelpEntry{label: "Top Connections", desc: "Up/Down rows, [ ] pages, o IN/OUT, g group, / search, f filter, T trace packet, t trace history, Enter/k actions (IN only)"})
	}
	if ctx.FocusIndex != 0 {
		entries = append(entries, liveHelpEntry{label: "System Health", desc: "s sort states, Shift+I explain metrics"})
	}
	if ctx.FocusIndex != 1 {
		entries = append(entries, liveHelpEntry{label: "Diagnosis", desc: "d show diagnosis history"})
	}
	entries = append(entries, liveHelpEntry{label: "Logs / Blocks", desc: "h action log, t trace history, b blocked peers"})
	return entries
}

func renderLiveHelpSection(title, subtitle string, entries []liveHelpEntry) string {
	var b strings.Builder
	b.WriteString("[yellow]")
	b.WriteString(title)
	b.WriteString("[white]")
	if strings.TrimSpace(subtitle) != "" {
		b.WriteString(" [dim](")
		b.WriteString(subtitle)
		b.WriteString(")[white]")
	}
	b.WriteString("\n")
	b.WriteString("──────────────────────────")
	for _, entry := range entries {
		b.WriteString("\n")
		fmt.Fprintf(&b, "[yellow]%-18s[white] %s", entry.label, entry.desc)
	}
	return b.String()
}

func topViewLabel(groupView bool) string {
	if groupView {
		return "group"
	}
	return "connections"
}

func topViewSearchLabel(groupView bool) string {
	if groupView {
		return "group"
	}
	return "connection"
}

func topGroupToggleLabel(groupView bool) string {
	if groupView {
		return "Switch to connections view"
	}
	return "Group rows by peer"
}

func oppositeDirectionLabel(direction tuishared.TopConnectionDirection) string {
	if direction == tuishared.TopConnectionOutgoing {
		return "IN"
	}
	return "OUT"
}
