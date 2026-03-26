package layout

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type panelInfo struct {
	title string
	text  string
}

var defaultPanels = []panelInfo{
	{title: " 2. System Health ", text: "  Loading..."},
	{title: " 3. Bandwidth ", text: "  Collecting samples..."},
	{title: " 1. Top Incoming ", text: "  Loading..."},
}

func CreatePanels() []*tview.TextView {
	panels := make([]*tview.TextView, len(defaultPanels))
	for i, info := range defaultPanels {
		panel := tview.NewTextView()
		panel.SetBorder(true)
		panel.SetTitle(info.title)
		panel.SetText(info.text)
		panel.SetDynamicColors(true)
		panel.SetScrollable(true)
		panels[i] = panel
	}
	return panels
}

func CreateStatusBar(interfaceName string) *tview.TextView {
	status := tview.NewTextView()
	status.SetDynamicColors(true)
	status.SetTextAlign(tview.AlignLeft)
	status.SetText(fmt.Sprintf(
		" [yellow]%s[white] | Updated: [green]just now[white] | Press [yellow]?[white] for help, [yellow]q[white] to quit",
		interfaceName,
	))
	return status
}

func CreateGrid(panels []*tview.TextView, statusBar *tview.TextView) *tview.Grid {
	grid := tview.NewGrid()
	grid.SetRows(-3, -2, 1)
	grid.SetColumns(-3, -2)
	grid.AddItem(panels[2], 0, 0, 2, 1, 0, 0, false) // Top Connections spans 2 rows
	grid.AddItem(panels[0], 0, 1, 1, 1, 0, 0, false) // System Health
	grid.AddItem(panels[1], 1, 1, 1, 1, 0, 0, false) // Bandwidth
	grid.AddItem(statusBar, 2, 0, 1, 2, 0, 0, false) // Status bar
	return grid
}

func HighlightPanel(panels []*tview.TextView, focusIndex int) {
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
