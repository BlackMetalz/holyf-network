package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

// help.go — Help overlay showing keyboard shortcuts.

const historyHelpText = `[yellow]Replay Shortcuts[white]
──────────────────────────
[yellow]][white]             Next snapshot
[yellow][[white]             Previous snapshot
[yellow]a / e[white]         Jump to oldest / latest snapshot
[yellow]t[white]             Jump to specific time
[yellow]Shift+S[white]       Search timeline (all loaded snapshots)
[yellow]L[white]             Follow latest snapshots
[yellow]o[white]             Toggle replay IN/OUT direction
[yellow]Up/Down[white]     Select row
[yellow]f[white]           Port filter / clear all filters
[yellow]/[white]           Search (contains text in current snapshot)
[yellow]Shift+B[white]     Sort by Bandwidth (press again: DESC/ASC)
[yellow]Shift+C[white]     Sort by Conns (press again: DESC/ASC)
[yellow]Shift+P[white]     Sort by Port (press again: DESC/ASC)
[yellow]i[white]           Explain Send-Q / Recv-Q / TX/s / RX/s
[yellow]Shift+I[white]     Alias of i (Explain queue/bandwidth columns)
[yellow]m[white]           Toggle sensitive IP mask
[yellow]x[white]           Toggle skip-empty snapshots
[yellow]?[white]           Toggle this help
[yellow]q[white]           Quit

[dim]Replay is read-only (no kill/block actions)[white]
[dim]Press Esc or any key to close[white]`

type liveHelpEntry struct {
	label string
	desc  string
}

func liveMainStatusHotkeys(focusIndex int, direction topConnectionDirection) (styled string, plain string) {
	switch focusIndex {
	case 2:
		if direction == topConnectionOutgoing {
			return "[dim]Up/Down[white]=select [dim]pg[white]=page [dim]o[white]=IN [dim]g[white]=group [dim]/[white]=search [dim]f[white]=filter [dim]Enter/k[white]=disabled [dim]Tab[white]=panel [dim]?[white]=help",
				"Up/Down=select [ ]=page o=IN g=group /=search f=filter Enter/k=disabled Tab=panel ?=help"
		}
		return "[dim]Up/Down[white]=select [dim]pg[white]=page [dim]o[white]=OUT [dim]g[white]=group [dim]/[white]=search [dim]f[white]=filter [dim]Enter/k[white]=act [dim]Tab[white]=panel [dim]?[white]=help",
			"Up/Down=select [ ]=page o=OUT g=group /=search f=filter Enter/k=act Tab=panel ?=help"
	case 0:
		return "[dim]s[white]=sort [dim]Tab[white]=panel [dim]Ctrl+1..5[white]=focus [dim]?[white]=help",
			"s=sort Tab=panel Ctrl+1..5=focus ?=help"
	case 1:
		return "[dim]Shift+I[white]=explain [dim]Tab[white]=panel [dim]Ctrl+1..5[white]=focus [dim]?[white]=help",
			"Shift+I=explain Tab=panel Ctrl+1..5=focus ?=help"
	case 4:
		return "[dim]d[white]=history [dim]Tab[white]=panel [dim]Ctrl+1..5[white]=focus [dim]?[white]=help",
			"d=history Tab=panel Ctrl+1..5=focus ?=help"
	default:
		return "[dim]Tab[white]=panel [dim]Ctrl+1..5[white]=focus [dim]r[white]=refresh [dim]?[white]=help",
			"Tab=panel Ctrl+1..5=focus r=refresh ?=help"
	}
}

func buildLiveHelpText(a *App) string {
	currentTitle, currentEntries := currentPanelHelpSection(a)
	globalEntries := []liveHelpEntry{
		{label: "Tab / Shift+Tab", desc: "Move focus between panels"},
		{label: "Ctrl+1..5", desc: "Focus 1=Top 2=States 3=Interface 4=Conntrack 5=Diagnosis"},
		{label: "r", desc: "Refresh now"},
		{label: "p", desc: "Pause / resume auto-refresh"},
		{label: "m", desc: "Toggle sensitive IP mask"},
		{label: "?", desc: "Close help"},
		{label: "q", desc: "Quit"},
	}
	otherEntries := otherPanelHelpEntries(a)

	sections := []string{
		renderLiveHelpSection("Current Panel", currentTitle, currentEntries),
		renderLiveHelpSection("Global Navigation", "", globalEntries),
		renderLiveHelpSection("Other Panels", "", otherEntries),
		"[dim]Press Esc or any key to close[white]",
	}
	return strings.Join(sections, "\n\n")
}

func currentPanelHelpSection(a *App) (string, []liveHelpEntry) {
	switch a.focusIndex {
	case 2:
		title := fmt.Sprintf("Top Connections (%s, %s view)", a.topDirection.Label(), topViewLabel(a.groupView))
		entries := []liveHelpEntry{
			{label: "Up/Down", desc: "Select row"},
			{label: "[ / ]", desc: "Previous / next page"},
			{label: "o", desc: fmt.Sprintf("Toggle to %s mode", oppositeDirectionLabel(a.topDirection))},
			{label: "g", desc: topGroupToggleLabel(a.groupView)},
			{label: "/", desc: fmt.Sprintf("Search current %s list", topViewSearchLabel(a.groupView))},
			{label: "f", desc: "Filter by shown port / clear"},
		}
		if a.groupView {
			entries = append(entries, liveHelpEntry{label: "Shift+C", desc: "Sort by connection count"})
		} else {
			entries = append(entries, liveHelpEntry{label: "Shift+B/C/P", desc: "Sort by bandwidth / conns / port"})
		}
		if a.topDirection == topConnectionOutgoing {
			entries = append(entries, liveHelpEntry{label: "Enter / k", desc: "Disabled in OUT mode"})
		} else {
			entries = append(entries, liveHelpEntry{label: "Enter / k", desc: "Block selected target"})
		}
		entries = append(entries, liveHelpEntry{label: "z", desc: "Zoom Top Connections"})
		return title, entries
	case 0:
		return "Connection States", []liveHelpEntry{
			{label: "s", desc: "Sort state rows by count (DESC/ASC)"},
		}
	case 1:
		return "Interface Stats", []liveHelpEntry{
			{label: "Shift+I", desc: "Explain RX/TX, packets, errors, and drops"},
		}
	case 3:
		return "Conntrack", []liveHelpEntry{
			{label: "Info", desc: "Read-only pressure panel; watch usage and drops"},
		}
	case 4:
		return "Diagnosis", []liveHelpEntry{
			{label: "d", desc: "Show diagnosis history"},
		}
	default:
		return "Dashboard", nil
	}
}

func otherPanelHelpEntries(a *App) []liveHelpEntry {
	entries := make([]liveHelpEntry, 0, 5)
	if a.focusIndex != 2 {
		entries = append(entries, liveHelpEntry{
			label: "Top Connections",
			desc:  "Up/Down rows, [ ] pages, o IN/OUT, g group, / search, f filter, Enter/k actions (IN only)",
		})
	}
	if a.focusIndex != 0 {
		entries = append(entries, liveHelpEntry{label: "Connection States", desc: "s sort states"})
	}
	if a.focusIndex != 1 {
		entries = append(entries, liveHelpEntry{label: "Interface Stats", desc: "Shift+I explain metrics"})
	}
	if a.focusIndex != 4 {
		entries = append(entries, liveHelpEntry{label: "Diagnosis", desc: "d show diagnosis history"})
	}
	entries = append(entries, liveHelpEntry{label: "Logs / Blocks", desc: "h action log, b blocked peers"})
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

func oppositeDirectionLabel(direction topConnectionDirection) string {
	if direction == topConnectionOutgoing {
		return "IN"
	}
	return "OUT"
}

// createHelpModal creates a centered modal overlay for the help screen.
func createHelpModal() (*tview.Flex, *tview.TextView) {
	helpView := tview.NewTextView()
	helpView.SetDynamicColors(true)
	helpView.SetText("")
	helpView.SetTextAlign(tview.AlignLeft)
	helpView.SetBorder(true)
	helpView.SetTitle(" Help ")
	helpView.SetTitleAlign(tview.AlignCenter)

	// Wrap in Flex to center it on screen
	// The trick: add empty spacers around the content
	//
	// Layout:
	//   [spacer] [spacer] [spacer]
	//   [spacer] [HELP  ] [spacer]
	//   [spacer] [spacer] [spacer]
	//
	inner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(helpView, 0, 1, true)

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).   // Left spacer
		AddItem(inner, 88, 0, true). // Center column (fixed width)
		AddItem(nil, 0, 1, false)    // Right spacer

	return modal, helpView
}

func createHistoryHelpModal() *tview.Flex {
	helpView := tview.NewTextView()
	helpView.SetDynamicColors(true)
	helpView.SetText(historyHelpText)
	helpView.SetTextAlign(tview.AlignLeft)
	helpView.SetBorder(true)
	helpView.SetTitle(" Replay Help ")
	helpView.SetTitleAlign(tview.AlignCenter)

	inner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(helpView, 0, 1, true)

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(inner, 78, 0, true).
		AddItem(nil, 0, 1, false)

	return modal
}
