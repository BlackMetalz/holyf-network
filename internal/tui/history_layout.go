package tui

import "github.com/rivo/tview"

func createHistoryPanel() *tview.TextView {
	panel := tview.NewTextView()
	panel.SetBorder(true)
	panel.SetTitle(" Connection History ")
	panel.SetDynamicColors(true)
	panel.SetScrollable(true)
	panel.SetWrap(false)
	panel.SetText("  Loading snapshots...")
	return panel
}

func createHistoryStatusBar() *tview.TextView {
	status := tview.NewTextView()
	status.SetDynamicColors(true)
	status.SetTextAlign(tview.AlignLeft)
	status.SetText(" [yellow]history[white] | Loading...")
	return status
}

func createHistoryLayout(panel *tview.TextView, statusBar *tview.TextView) *tview.Flex {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(panel, 0, 1, true).
		AddItem(statusBar, 2, 0, false)
}
