package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// layout.go — Builds the 4-panel grid + status bar.
// This file only deals with creating UI components.
// Event handling is in app.go.

// panelInfo describes one panel in the dashboard.
type panelInfo struct {
	title string
	text  string
}

// defaultPanels defines the 4 panels shown in the TUI.
var defaultPanels = []panelInfo{
	{title: " Connection States ", text: "  Loading..."},
	{title: " Interface Stats ", text: "  Loading..."},
	{title: " Top Connections ", text: "  Loading..."},
	{title: " Conntrack ", text: "  Loading..."},
}

// createPanels creates 4 tview.TextView panels.
// Returns a slice in logical order:
// [Connection States, Interface Stats, Top Connections, Conntrack].
func createPanels() []*tview.TextView {
	panels := make([]*tview.TextView, len(defaultPanels))

	for i, info := range defaultPanels {
		panel := tview.NewTextView()
		panel.SetBorder(true)
		panel.SetTitle(info.title)
		panel.SetText(info.text)

		// Enable color tags like [red]text[white]
		panel.SetDynamicColors(true)

		// Enable scrolling with arrow keys when focused
		panel.SetScrollable(true)

		panels[i] = panel
	}

	return panels
}

// createStatusBar creates the bottom status bar.
func createStatusBar(interfaceName string) *tview.TextView {
	status := tview.NewTextView()
	status.SetDynamicColors(true)
	status.SetTextAlign(tview.AlignLeft)

	text := fmt.Sprintf(
		" [yellow]%s[white] | Updated: [green]just now[white] | Press [yellow]?[white] for help, [yellow]q[white] to quit",
		interfaceName,
	)
	status.SetText(text)

	return status
}

// createGrid assembles the 4 panels + status bar into a grid layout.
//
// Layout:
//
//	┌─ Top Connections ────┬─ Connection States ─┐
//	│                      ├─ Interface Stats ───┤
//	│                      ├─ Conntrack ─────────┤
//	│                      │                     │
//	└──────────────────────┴─────────────────────┘
//	 <status bar>
func createGrid(panels []*tview.TextView, statusBar *tview.TextView) *tview.Grid {
	grid := tview.NewGrid()

	// Rows: 3 content rows + 1 status row.
	// Cols: left is wider for Top Connections, right is narrower stack.
	grid.SetRows(0, 0, 0, 1)
	grid.SetColumns(-3, -2)

	// AddItem(primitive, row, col, rowSpan, colSpan, minHeight, minWidth, focus)
	grid.AddItem(panels[2], 0, 0, 3, 1, 0, 0, false) // Left: Top Connections (spans all content rows)
	grid.AddItem(panels[0], 0, 1, 1, 1, 0, 0, false) // Right top: Connection States
	grid.AddItem(panels[1], 1, 1, 1, 1, 0, 0, false) // Right middle: Interface Stats
	grid.AddItem(panels[3], 2, 1, 1, 1, 0, 0, false) // Right bottom: Conntrack

	// Status bar spans both columns at the bottom.
	grid.AddItem(statusBar, 3, 0, 1, 2, 0, 0, false)

	return grid
}

// highlightPanel updates borders to show which panel is focused.
// Focused panel gets a colored border, others get default.
func highlightPanel(panels []*tview.TextView, focusIndex int) {
	for i, panel := range panels {
		if i == focusIndex {
			panel.SetBorderColor(tcell.ColorYellow)
			panel.SetTitleColor(tcell.ColorYellow)
		} else {
			panel.SetBorderColor(tcell.ColorWhite)
			panel.SetTitleColor(tcell.ColorWhite)
		}
	}
}
