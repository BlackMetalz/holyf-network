package tui

import (
	"fmt"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// app.go — Main TUI application. Wires together layout, navigation, and help.

// App holds all TUI state.
type App struct {
	app       *tview.Application
	pages     *tview.Pages
	panels    []*tview.TextView
	statusBar *tview.TextView

	focusIndex int    // Which panel is currently focused (0-3)
	ifaceName  string // Network interface being monitored
	refreshSec int    // Refresh interval in seconds

	// Previous interface stats snapshot for rate calculation.
	prevIfaceStats *collector.InterfaceStats

	// Previous conntrack snapshot for rate calculation.
	prevConntrack *collector.ConntrackData

	// Port filter for Top Talkers panel. Empty = show all.
	portFilter string
}

// NewApp creates a new TUI application.
func NewApp(ifaceName string, refreshSec int) *App {
	return &App{
		app:        tview.NewApplication(),
		ifaceName:  ifaceName,
		refreshSec: refreshSec,
		focusIndex: 0,
	}
}

// Run starts the TUI event loop. This blocks until the user quits.
func (a *App) Run() error {
	// Build UI components
	a.panels = createPanels()
	a.statusBar = createStatusBar(a.ifaceName)
	grid := createGrid(a.panels, a.statusBar)
	helpModal := createHelpModal()

	// tview.Pages lets us stack "pages" (layers) on top of each other.
	// "main" is always visible, "help" is shown/hidden on top.
	a.pages = tview.NewPages()
	a.pages.AddPage("main", grid, true, true)
	a.pages.AddPage("help", helpModal, true, false) // resize=true, visible=false

	// Set initial focus highlight
	highlightPanel(a.panels, a.focusIndex)

	// Load initial data into panels
	a.refreshData()

	// Register global key handler
	a.app.SetInputCapture(a.handleKeyEvent)

	// Start the application
	a.app.SetRoot(a.pages, true)
	return a.app.Run()
}

// handleKeyEvent processes all keyboard input.
// Returning nil means "I handled it, don't pass to focused widget".
// Returning the event means "I didn't handle it, let tview process it".
func (a *App) handleKeyEvent(event *tcell.EventKey) *tcell.EventKey {
	// If help is showing, any key closes it
	if a.isHelpVisible() {
		a.hideHelp()
		return nil
	}

	// Handle key by type
	switch event.Key() {
	case tcell.KeyTab:
		a.focusNext()
		return nil

	case tcell.KeyBacktab: // Shift+Tab
		a.focusPrev()
		return nil

	case tcell.KeyRune:
		// tcell.KeyRune means a regular character key (not special key)
		switch event.Rune() {
		case 'q':
			a.app.Stop()
			return nil
		case '?':
			a.showHelp()
			return nil
		case 'r':
			a.refreshData()
			return nil
		case 'f':
			a.promptPortFilter()
			return nil
		}
	}

	// Let tview handle other keys (arrow keys for scrolling, etc.)
	return event
}

// focusNext moves focus to the next panel (wraps around).
func (a *App) focusNext() {
	a.focusIndex = (a.focusIndex + 1) % len(a.panels)
	highlightPanel(a.panels, a.focusIndex)
}

// focusPrev moves focus to the previous panel (wraps around).
func (a *App) focusPrev() {
	a.focusIndex = (a.focusIndex - 1 + len(a.panels)) % len(a.panels)
	highlightPanel(a.panels, a.focusIndex)
}

// showHelp displays the help overlay.
func (a *App) showHelp() {
	a.pages.ShowPage("help")
}

// hideHelp hides the help overlay.
func (a *App) hideHelp() {
	a.pages.HidePage("help")
}

// isHelpVisible checks if the help overlay is currently shown.
func (a *App) isHelpVisible() bool {
	name, _ := a.pages.GetFrontPage()
	return name == "help"
}

// refreshData collects data from system and updates all panels.
func (a *App) refreshData() {
	// Panel 0: Connection States
	connData, err := collector.CollectConnections()
	if err != nil {
		a.panels[0].SetText(fmt.Sprintf("  [red]%v[white]", err))
	} else {
		a.panels[0].SetText(renderConnectionsPanel(connData))
	}

	// Panel 1: Interface Stats
	ifaceStats, err := collector.CollectInterfaceStats(a.ifaceName)
	if err != nil {
		a.panels[1].SetText(fmt.Sprintf("  [red]%v[white]", err))
	} else {
		rates := collector.CalculateRates(ifaceStats, a.prevIfaceStats)
		a.panels[1].SetText(renderInterfacePanel(rates))
		a.prevIfaceStats = &ifaceStats
	}

	// Panel 2: Top Talkers
	talkers, err := collector.CollectTopTalkers(100)
	if err != nil {
		a.panels[2].SetText(fmt.Sprintf("  [red]%v[white]", err))
	} else {
		a.panels[2].SetText(renderTalkersPanel(talkers, a.portFilter))
	}

	// Panel 3: Conntrack
	ctData, err := collector.CollectConntrack()
	if err != nil {
		a.panels[3].SetText(fmt.Sprintf("  [red]%v[white]", err))
	} else {
		ctRates := collector.CalculateConntrackRates(ctData, a.prevConntrack)
		a.panels[3].SetText(renderConntrackPanel(ctRates))
		a.prevConntrack = &ctData
	}

	// Update status bar
	a.statusBar.SetText(fmt.Sprintf(
		" [yellow]%s[white] | Updated: [green]just now[white] | Press [yellow]?[white] for help, [yellow]q[white] to quit",
		a.ifaceName,
	))
}

// promptPortFilter shows a simple input dialog for port filtering.
// Uses tview.InputField as a modal overlay.
func (a *App) promptPortFilter() {
	// If filter is already set, clear it
	if a.portFilter != "" {
		a.portFilter = ""
		a.refreshData()
		return
	}

	// Create input field
	input := tview.NewInputField()
	input.SetLabel("Filter by port: ")
	input.SetFieldWidth(10)
	input.SetBorder(true)
	input.SetTitle(" Port Filter ")

	// Accept only numbers
	input.SetAcceptanceFunc(tview.InputFieldInteger)

	// On Enter: set filter, close dialog, refresh
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			a.portFilter = input.GetText()
		}
		// On Enter or Escape: close the dialog
		a.pages.RemovePage("filter")
		a.refreshData()
	})

	// Center the input field using Flex spacers
	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 30, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("filter", modal, true, true)
	a.app.SetFocus(input)
}
