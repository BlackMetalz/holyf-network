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
	{title: " Top Talkers ", text: "  Loading..."},
	{title: " Conntrack ", text: "  Loading..."},
}

// createPanels creates 4 tview.TextView panels.
// Returns a slice of panels in order: [topLeft, topRight, bottomLeft, bottomRight].
func createPanels() []*tview.TextView {
	panels := make([]*tview.TextView, len(defaultPanels))

	for i, info := range defaultPanels {
		panel := tview.NewTextView()
		panel.SetBorder(true)
		panel.SetTitle(info.title)
		panel.SetText(info.text)

		// Enable color tags like [red]text[white]
		panel.SetDynamicColors(true)

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
//	┌─ Connection States ──┬─ Interface Stats ────┐
//	│                      │                      │
//	├─ Top Talkers ────────┼─ Conntrack ──────────┤
//	│                      │                      │
//	└──────────────────────┴──────────────────────┘
//	 <status bar>
func createGrid(panels []*tview.TextView, statusBar *tview.TextView) *tview.Grid {
	grid := tview.NewGrid()

	// SetRows: two equal rows for panels (-1 = fill remaining), 1 row for status bar
	// SetColumns: two equal columns (-1 = fill remaining)
	grid.SetRows(0, 0, 1)
	grid.SetColumns(0, 0)

	// AddItem(primitive, row, col, rowSpan, colSpan, minHeight, minWidth, focus)
	grid.AddItem(panels[0], 0, 0, 1, 1, 0, 0, false) // Top-left
	grid.AddItem(panels[1], 0, 1, 1, 1, 0, 0, false) // Top-right
	grid.AddItem(panels[2], 1, 0, 1, 1, 0, 0, false) // Bottom-left
	grid.AddItem(panels[3], 1, 1, 1, 1, 0, 0, false) // Bottom-right

	// Status bar spans both columns at the bottom (row 2)
	grid.AddItem(statusBar, 2, 0, 1, 2, 0, 0, false)

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
