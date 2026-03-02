package tui

import (
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
			// Placeholder for manual refresh (Sprint 2)
			a.statusBar.SetText(" [yellow]Refreshing...[white] (not implemented yet)")
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
