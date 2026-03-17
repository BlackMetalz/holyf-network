package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// layout.go — Builds the live dashboard grid + status bar.
// This file only deals with creating UI components.
// Event handling is in app.go.

// panelInfo describes one panel in the dashboard.
type panelInfo struct {
	title string
	text  string
}

// defaultPanels defines the live panels shown in the TUI.
var defaultPanels = []panelInfo{
	{title: " 2. Connection States ", text: "  Loading..."},
	{title: " 3. Interface Stats ", text: "  Loading..."},
	{title: " 1. Top Connections ", text: "  Loading..."},
	{title: " 4. Conntrack ", text: "  Loading..."},
	{title: " 5. Diagnosis ", text: "  Loading..."},
}

// createPanels creates the live dashboard panels.
// Returns a slice in logical order:
// [Connection States, Interface Stats, Top Connections, Conntrack, Diagnosis].
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

// createGrid assembles the live panels + status bar into a grid layout.
//
// Layout:
//
//	┌─ Top Connections ────┬─ Connection States ─┐
//	│                      ├─ Interface Stats ───┤
//	│                      ├─ Conntrack ─────────┤
//	│                      ├─ Diagnosis ─────────┤
//	│                      │                     │
//	└──────────────────────┴─────────────────────┘
//	 <status bar>
func createGrid(panels []*tview.TextView, statusBar *tview.TextView) *tview.Grid {
	grid := tview.NewGrid()

	// Rows: 4 weighted content rows + 1 status row.
	// Cols: left is wider for Top Connections, right is narrower stack.
	grid.SetRows(-4, -3, -2, -4, 1)
	grid.SetColumns(-3, -2)

	// AddItem(primitive, row, col, rowSpan, colSpan, minHeight, minWidth, focus)
	grid.AddItem(panels[2], 0, 0, 4, 1, 0, 0, false) // Left: Top Connections (spans all content rows)
	grid.AddItem(panels[0], 0, 1, 1, 1, 0, 0, false) // Right top: Connection States
	grid.AddItem(panels[1], 1, 1, 1, 1, 0, 0, false) // Right upper-middle: Interface Stats
	grid.AddItem(panels[3], 2, 1, 1, 1, 0, 0, false) // Right lower-middle: Conntrack
	grid.AddItem(panels[4], 3, 1, 1, 1, 0, 0, false) // Right bottom: Diagnosis

	// Status bar spans both columns at the bottom.
	grid.AddItem(statusBar, 4, 0, 1, 2, 0, 0, false)

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
