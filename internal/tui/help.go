package tui

import "github.com/rivo/tview"

// help.go — Help overlay showing keyboard shortcuts.

// helpText contains all keyboard shortcuts displayed in the help overlay.
const helpText = `[yellow]Keyboard Shortcuts[white]
──────────────────────────
[yellow]Tab[white]         Next panel
[yellow]Shift+Tab[white]   Previous panel
[yellow]Ctrl+1..5[white]   Focus panel (1=Top, 2=States, 3=Interface, 4=Conntrack, 5=Diagnosis)
[yellow]r[white]           Refresh now
[yellow]p[white]           Pause/Resume auto-refresh
[yellow]m[white]           Toggle sensitive IP mask
[yellow]s[white]           Sort Connection States by count (toggle DESC/ASC)
[yellow]f[white]           Port filter / clear all filters
[yellow]/[white]           Search (contains text in Top Connections)
[yellow]Up/Down[white]     Select row (Top Connections)
[yellow]o[white]           Toggle Top Connections IN/OUT mode
[yellow]Enter[white]       Block selected row (IN mode only)
[yellow]k[white]           Block peer (selected/default, IN mode only)
[yellow]Shift+B[white]     Sort by Bandwidth (press again: DESC/ASC)
[yellow]Shift+C[white]     Sort by Conns (press again: DESC/ASC)
[yellow]Shift+P[white]     Sort by Port (press again: DESC/ASC)
[yellow]i[white]           Explain Send-Q / Recv-Q / TX/s / RX/s
[yellow]Shift+I[white]     Explain Interface Stats (RX/TX, Packets, Errors, Drops)
[yellow]g[white]           Toggle group-by-peer view
[yellow]b[white]           Show blocked peers
[yellow]h[white]           Show action log (latest 20)
[yellow]z[white]           Toggle zoom (Top Connections only)
[yellow]?[white]           Toggle this help
[yellow]q[white]           Quit

[dim]Press Esc or any key to close[white]`

const historyHelpText = `[yellow]Replay Shortcuts[white]
──────────────────────────
[yellow]][white]             Next snapshot
[yellow][[white]             Previous snapshot
[yellow]a / e[white]         Jump to oldest / latest snapshot
[yellow]t[white]             Jump to specific time
[yellow]Shift+S[white]       Search timeline (all loaded snapshots)
[yellow]L[white]             Follow latest snapshots
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

// createHelpModal creates a centered modal overlay for the help screen.
func createHelpModal() *tview.Flex {
	// Create the text view for help content
	helpView := tview.NewTextView()
	helpView.SetDynamicColors(true)
	helpView.SetText(helpText)
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
		AddItem(inner, 72, 0, true). // Center column (fixed width)
		AddItem(nil, 0, 1, false)    // Right spacer

	return modal
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
