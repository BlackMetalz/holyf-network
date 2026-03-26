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
	{title: " 2. Connection States ", text: "  Loading..."},
	{title: " 3. Interface Stats ", text: "  Loading..."},
	{title: " 1. Top Incoming ", text: "  Loading..."},
	{title: " 4. Conntrack ", text: "  Loading..."},
	{title: " 5. Diagnosis ", text: "  Loading..."},
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
	grid.SetRows(-4, -3, -2, -4, 1)
	grid.SetColumns(-3, -2)
	grid.AddItem(panels[2], 0, 0, 4, 1, 0, 0, false)
	grid.AddItem(panels[0], 0, 1, 1, 1, 0, 0, false)
	grid.AddItem(panels[1], 1, 1, 1, 1, 0, 0, false)
	grid.AddItem(panels[3], 2, 1, 1, 1, 0, 0, false)
	grid.AddItem(panels[4], 3, 1, 1, 1, 0, 0, false)
	grid.AddItem(statusBar, 4, 0, 1, 2, 0, 0, false)
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
