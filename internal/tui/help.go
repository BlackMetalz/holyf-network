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
[yellow]f[white]           Filter (in Top Connections)
[yellow]k[white]           Block peer (input form)
[yellow]b[white]           Show blocked peers
[yellow]?[white]           Toggle this help
[yellow]q[white]           Quit

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
		AddItem(helpView, 15, 0, true). // Help content (fixed height)
		AddItem(nil, 0, 1, false)       // Bottom spacer

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).   // Left spacer
		AddItem(inner, 40, 0, true). // Center column (fixed width)
		AddItem(nil, 0, 1, false)    // Right spacer

	return modal
}
