package tui

import "github.com/rivo/tview"

// help.go — Help overlay showing keyboard shortcuts.

// helpText contains all keyboard shortcuts displayed in the help overlay.
const helpText = `[yellow]Keyboard Shortcuts[white]
──────────────────────────
[yellow]Tab[white]         Next panel
[yellow]Shift+Tab[white]   Previous panel
[yellow]r[white]           Refresh now
[yellow]p[white]           Pause/Resume auto-refresh
[yellow]s[white]           Toggle sensitive IP mask
[yellow]f[white]           Port filter / clear all filters
[yellow]/[white]           Search (contains text in Top Connections)
[yellow]Up/Down[white]     Select row (Top Connections)
[yellow]Enter[white]       Block selected row
[yellow]k[white]           Block peer (selected/default)
[yellow]o[white]           Cycle sort mode
[yellow]Shift+Q[white]     Sort by Queue
[yellow]Shift+S[white]     Sort by State
[yellow]Shift+P[white]     Sort by Peer
[yellow]Shift+R[white]     Sort by Process
[yellow]g[white]           Toggle group-by-peer view
[yellow]b[white]           Show blocked peers
[yellow]h[white]           Show action log (latest 20)
[yellow]z[white]           Toggle zoom (focused panel)
[yellow]?[white]           Toggle this help
[yellow]q[white]           Quit

[dim]Press Esc or any key to close[white]`

const historyHelpText = `[yellow]Replay Shortcuts[white]
──────────────────────────
[yellow]left bracket[white]  Previous snapshot
[yellow]right bracket[white] Next snapshot
[yellow]a / e[white]         Jump to oldest / latest snapshot
[yellow]t[white]             Jump to specific time
[yellow]L[white]           Follow latest snapshots
[yellow]Up/Down[white]     Select row
[yellow]f[white]           Port filter / clear all filters
[yellow]/[white]           Search (contains text in current snapshot)
[yellow]o[white]           Cycle sort mode
[yellow]Shift+Q[white]     Sort by Queue
[yellow]Shift+S[white]     Sort by State
[yellow]Shift+P[white]     Sort by Peer
[yellow]Shift+R[white]     Sort by Process
[yellow]g[white]           Toggle group-by-peer view
[yellow]s[white]           Toggle sensitive IP mask
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
		AddItem(nil, 0, 1, false).      // Top spacer
		AddItem(helpView, 20, 0, true). // Help content (fixed height)
		AddItem(nil, 0, 1, false)       // Bottom spacer

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).   // Left spacer
		AddItem(inner, 40, 0, true). // Center column (fixed width)
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
		AddItem(nil, 0, 1, false).
		AddItem(helpView, 22, 0, true).
		AddItem(nil, 0, 1, false)

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(inner, 54, 0, true).
		AddItem(nil, 0, 1, false)

	return modal
}
