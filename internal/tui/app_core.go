package tui

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/BlackMetalz/holyf-network/internal/actions"
	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// app.go — Main TUI application. Wires together layout, navigation, help,
// and auto-refresh via goroutines + channels.

// App holds all TUI state.
type App struct {
	app       *tview.Application
	pages     *tview.Pages
	panels    []*tview.TextView
	statusBar *tview.TextView
	grid      *tview.Grid // Store grid for zoom toggle

	focusIndex int    // Which panel is currently focused (0-3)
	ifaceName  string // Network interface being monitored
	refreshSec int    // Refresh interval in seconds
	appVersion string

	// Previous snapshots for rate calculation (need 2 readings for delta)
	prevIfaceStats *collector.InterfaceStats
	prevConntrack  *collector.ConntrackData
	prevRetransmit *collector.RetransmitData

	// Port filter for Top Connections panel. Empty = show all.
	portFilter string
	// Text contains-filter for Top Connections ("/" search). Empty = no grep filter.
	textFilter string
	// Hide sensitive IP prefixes in Top Connections.
	sensitiveIP bool
	// Latest top connections snapshot used by actions (kill peer, etc.).
	latestTalkers []collector.Connection
	// Selected row in Top Connections (within currently visible rows).
	selectedTalkerIndex int
	// Sort mode for Top Connections. Default: SortByQueue.
	sortMode SortMode
	// Group view mode: when true, show per-peer aggregates instead of individual connections.
	groupView bool

	// Short-lived status note shown in status bar.
	statusNote      string
	statusNoteUntil time.Time

	// Recent action history (for modal "h").
	actionLogMu sync.Mutex
	actionLogs  []string
	// Persistent action history location (~/.holyf-network/history.log).
	actionHistoryPath string

	// Active peer blocks for cleanup on shutdown.
	blockMu      sync.Mutex
	activeBlocks map[string]activeBlockEntry

	// Auto-refresh state
	stopChan    chan struct{}
	refreshChan chan struct{}
	paused      atomic.Bool
	lastRefresh time.Time
	// Tracks temporary auto-pause while the kill-peer flow is open.
	killFlowAutoPaused bool

	// Zoom state
	zoomed bool // Whether a panel is zoomed to fullscreen

	healthThresholds config.HealthThresholds
}

type activeBlockEntry struct {
	Spec      actions.PeerBlockSpec
	StartedAt time.Time
	ExpiresAt time.Time
	Summary   string
}

// NewApp creates a new TUI application.
func NewApp(
	ifaceName string,
	refreshSec int,
	sensitiveIP bool,
	appVersion string,
	healthThresholds config.HealthThresholds,
) *App {
	version := strings.TrimSpace(appVersion)
	if version == "" {
		version = "dev"
	}
	healthThresholds.Normalize()

	return &App{
		app:               tview.NewApplication(),
		ifaceName:         ifaceName,
		refreshSec:        refreshSec,
		appVersion:        version,
		focusIndex:        2, // Top Connections panel is default active focus.
		sensitiveIP:       sensitiveIP,
		activeBlocks:      make(map[string]activeBlockEntry),
		stopChan:          make(chan struct{}),
		refreshChan:       make(chan struct{}, 1), // Buffered: so send never blocks
		healthThresholds:  healthThresholds,
		actionLogs:        make([]string, 0, 32),
		actionHistoryPath: defaultActionHistoryPath(),
	}
}

// Run starts the TUI event loop. This blocks until the user quits.
func (a *App) Run() error {
	// Build UI components
	a.panels = createPanels()
	a.statusBar = createStatusBar(a.ifaceName)
	a.grid = createGrid(a.panels, a.statusBar)
	helpModal := createHelpModal()

	// tview.Pages lets us stack "pages" (layers) on top of each other.
	// "main" is always visible, "help" is shown/hidden on top.
	a.pages = tview.NewPages()
	a.pages.AddPage("main", a.grid, true, true)
	a.pages.AddPage("help", helpModal, true, false) // resize=true, visible=false

	// Set initial focus highlight
	highlightPanel(a.panels, a.focusIndex)

	// Load initial data into panels
	a.refreshData()

	// Register global key handler
	a.app.SetInputCapture(a.handleKeyEvent)

	// Start background goroutines
	go a.startRefreshLoop()
	go a.startStatusTicker()

	// Start the application (blocks until app.Stop() is called)
	a.app.SetRoot(a.pages, true)
	return a.app.Run()
}

// startRefreshLoop runs in a goroutine. It periodically refreshes data
// using time.Ticker and listens for manual refresh signals.
//
// Go concepts:
//   - time.NewTicker: fires at regular intervals
//   - select: wait on multiple channels simultaneously
//   - chan struct{}: signal-only channel (zero memory, just a notification)
//   - QueueUpdateDraw: thread-safe tview update
func (a *App) startRefreshLoop() {
	ticker := time.NewTicker(time.Duration(a.refreshSec) * time.Second)
	defer ticker.Stop() // Always clean up ticker

	for {
		select {
		case <-ticker.C:
			// Timer fired — refresh if not paused
			if !a.paused.Load() {
				a.app.QueueUpdateDraw(func() {
					a.refreshData()
				})
			}

		case <-a.refreshChan:
			// Manual refresh requested (r key)
			ticker.Reset(time.Duration(a.refreshSec) * time.Second) // Reset countdown
			a.app.QueueUpdateDraw(func() {
				a.refreshData()
			})

		case <-a.stopChan:
			// App is quitting — exit the goroutine
			return
		}
	}
}

// startStatusTicker updates the "Updated: Xs ago" text every second.
// Runs in a separate goroutine.
func (a *App) startStatusTicker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.app.QueueUpdateDraw(func() {
				a.updateStatusBar()
			})
		case <-a.stopChan:
			return
		}
	}
}

// refreshData collects data from system and updates all panels.
func (a *App) refreshData() {
	a.lastRefresh = time.Now()

	// Collect conntrack early so panel 0 health strip can use it too.
	var conntrackRates *collector.ConntrackRates
	ctData, err := collector.CollectConntrack()
	if err != nil {
		a.panels[3].SetText(fmt.Sprintf("  [red]%v[white]", err))
	} else {
		rates := collector.CalculateConntrackRates(ctData, a.prevConntrack)
		conntrackRates = &rates
		a.panels[3].SetText(renderConntrackPanel(rates))
		a.prevConntrack = &ctData
	}

	// Panel 0: Connection States + Retransmits
	var retransRates *collector.RetransmitRates
	retransData, retransErr := collector.CollectRetransmits()
	if retransErr == nil {
		r := collector.CalculateRetransmitRates(retransData, a.prevRetransmit)
		retransRates = &r
		a.prevRetransmit = &retransData
	}

	connData, err := collector.CollectConnections()
	if err != nil {
		a.panels[0].SetText(fmt.Sprintf("  [red]%v[white]", err))
	} else {
		a.panels[0].SetText(renderConnectionsPanel(connData, retransRates, conntrackRates, a.healthThresholds))
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

	// Panel 2: Top Connections
	talkers, err := collector.CollectTopTalkers(100)
	if err != nil {
		a.latestTalkers = nil
		a.selectedTalkerIndex = 0
		a.panels[2].SetText(fmt.Sprintf("  [red]%v[white]", err))
	} else {
		a.latestTalkers = talkers
		a.renderTopConnectionsPanel()
	}

	a.updateStatusBar()
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
	// When non-help overlays are visible (forms/modals), let focused widget handle keys.
	if a.isOverlayVisible() {
		return event
	}

	// Handle key by type
	switch event.Key() {
	case tcell.KeyUp:
		if a.focusIndex == 2 && a.moveTopConnectionSelection(-1) {
			return nil
		}
		return event

	case tcell.KeyDown:
		if a.focusIndex == 2 && a.moveTopConnectionSelection(1) {
			return nil
		}
		return event

	case tcell.KeyEnter:
		if a.focusIndex == 2 {
			a.promptKillPeer()
			return nil
		}
		return event

	case tcell.KeyTab:
		if !a.zoomed {
			a.focusNext()
		}
		return nil

	case tcell.KeyBacktab: // Shift+Tab
		if !a.zoomed {
			a.focusPrev()
		}
		return nil

	case tcell.KeyEsc:
		if a.zoomed {
			a.exitZoom()
		}
		return nil

	case tcell.KeyRune:
		// tcell.KeyRune means a regular character key (not special key)
		switch event.Rune() {
		case 'q':
			a.cleanupActiveBlocks()
			close(a.stopChan) // Signal goroutines to stop
			a.app.Stop()
			return nil
		case '?':
			a.showHelp()
			return nil
		case 'r':
			// Send signal to refresh goroutine (non-blocking)
			select {
			case a.refreshChan <- struct{}{}:
			default: // Channel full, refresh already pending
			}
			return nil
		case 'p':
			a.paused.Store(!a.paused.Load())
			a.updateStatusBar()
			return nil
		case 's':
			a.sensitiveIP = !a.sensitiveIP
			a.refreshData()
			return nil
		case 'f':
			a.promptPortFilter()
			return nil
		case '/':
			a.promptTextFilter()
			return nil
		case 'k':
			a.promptKillPeer()
			return nil
		case 'b':
			a.promptBlockedPeers()
			return nil
		case 'h':
			a.promptActionLog()
			return nil
		case 'o':
			a.applyTopConnectionSortMode(NextSortMode(a.sortMode))
			return nil
		case 'Q', 'S', 'P', 'R':
			mode, _ := directSortModeForRune(event.Rune())
			a.applyTopConnectionSortMode(mode)
			return nil
		case 'g':
			a.groupView = !a.groupView
			a.selectedTalkerIndex = 0
			a.renderTopConnectionsPanel()
			return nil
		case 'z':
			a.toggleZoom()
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

func directSortModeForRune(r rune) (SortMode, bool) {
	switch r {
	case 'Q':
		return SortByQueue, true
	case 'S':
		return SortByState, true
	case 'P':
		return SortByPeer, true
	case 'R':
		return SortByProcess, true
	default:
		return SortByQueue, false
	}
}

func (a *App) applyTopConnectionSortMode(mode SortMode) {
	a.sortMode = mode
	a.selectedTalkerIndex = 0
	a.renderTopConnectionsPanel()
}

func (a *App) showHelp() {
	a.pages.SendToFront("help") // Ensure help renders above main after zoom reorder
	a.pages.ShowPage("help")
	a.updateStatusBar()
}
func (a *App) hideHelp() {
	a.pages.HidePage("help")
	a.updateStatusBar()
}
func (a *App) isHelpVisible() bool {
	name, _ := a.pages.GetFrontPage()
	return name == "help"
}
func (a *App) isOverlayVisible() bool {
	name, _ := a.pages.GetFrontPage()
	return name != "main" && name != "help"
}

// toggleZoom switches between grid view and fullscreen focused panel.
func (a *App) toggleZoom() {
	if a.zoomed {
		a.exitZoom()
		return
	}

	// Create fullscreen layout: just the focused panel + status bar
	zoomLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.panels[a.focusIndex], 0, 1, true).
		AddItem(a.statusBar, 1, 0, false)

	a.pages.RemovePage("main")
	a.pages.AddPage("main", zoomLayout, true, true)

	a.zoomed = true
	a.updateStatusBar()
}

// exitZoom returns to the normal 4-panel grid view.
func (a *App) exitZoom() {
	// Rebuild grid (panels were removed from old grid)
	a.grid = createGrid(a.panels, a.statusBar)

	a.pages.RemovePage("main")
	a.pages.AddPage("main", a.grid, true, true)

	a.zoomed = false
	highlightPanel(a.panels, a.focusIndex)
	a.updateStatusBar()
}

// updateStatusBar updates the bottom status bar with current state.
func (a *App) updateStatusBar() {
	// Calculate time since last refresh
	ago := "never"
	if !a.lastRefresh.IsZero() {
		elapsed := time.Since(a.lastRefresh).Truncate(time.Second)
		if elapsed < 1*time.Second {
			ago = "just now"
		} else {
			ago = elapsed.String() + " ago"
		}
	}

	// Build state indicators
	stateText := ""
	if a.paused.Load() {
		stateText += " [red]PAUSED[white] |"
	}
	if a.zoomed {
		stateText += " [aqua]ZOOMED[white] |"
	}
	if a.sensitiveIP {
		stateText += " [yellow]IP MASK[white] |"
	}
	if time.Now().Before(a.statusNoteUntil) && a.statusNote != "" {
		stateText += fmt.Sprintf(" [yellow]%s[white] |", a.statusNote)
	}

	page := a.frontPageName()
	hotkeysStyled, hotkeysPlain := statusHotkeysForPage(page)
	leftStyled := fmt.Sprintf(
		" [yellow]%s[white] |%s Updated: [green]%s[white] | Refresh: [green]%ds[white] | %s",
		a.ifaceName,
		stateText,
		ago,
		a.refreshSec,
		hotkeysStyled,
	)
	leftPlain := fmt.Sprintf(
		" %s |%s Updated: %s | Refresh: %ds | %s",
		a.ifaceName,
		stripStatusColors(stateText),
		ago,
		a.refreshSec,
		hotkeysPlain,
	)
	versionLabel := "holyf-network " + a.appVersion
	rightStyled := " [dim]" + versionLabel + "[white]"
	rightPlain := " " + versionLabel

	text := leftStyled
	_, _, width, _ := a.statusBar.GetInnerRect()
	if width > 0 {
		pad := width - utf8.RuneCountInString(leftPlain) - utf8.RuneCountInString(rightPlain)
		if pad > 0 {
			text = leftStyled + strings.Repeat(" ", pad) + rightStyled
		}
	}

	a.statusBar.SetText(text)
}

func (a *App) frontPageName() string {
	if a.pages == nil {
		return "main"
	}
	name, _ := a.pages.GetFrontPage()
	if strings.TrimSpace(name) == "" {
		return "main"
	}
	return name
}

func statusHotkeysForPage(page string) (styled string, plain string) {
	switch page {
	case "help":
		return "[dim]any key[white]=close", "any key=close"
	case "filter":
		return "[dim]Enter[white]=apply [dim]Esc[white]=cancel", "Enter=apply Esc=cancel"
	case "search":
		return "[dim]Enter[white]=apply [dim]Esc[white]=cancel", "Enter=apply Esc=cancel"
	case "kill-peer-form":
		return "[dim]Tab[white]=field [dim]Enter[white]=next [dim]Esc[white]=cancel", "Tab=field Enter=next Esc=cancel"
	case "kill-peer":
		return "[dim]<-/->[white]=choose [dim]Enter[white]=confirm [dim]Esc[white]=cancel", "<-/->=choose Enter=confirm Esc=cancel"
	case "blocked-peers":
		return "[dim]Up/Down[white]=select [dim]Enter[white]=remove [dim]Del[white]=remove [dim]Tab[white]=buttons [dim]Esc[white]=close",
			"Up/Down=select Enter=remove Del=remove Tab=buttons Esc=close"
	case "action-log":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	case "blocked-peers-remove-result", "block-summary":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	default:
		return "[dim]r p f k o Q/S/P/R g b h z ? q[white]", "r p f k o Q/S/P/R g b h z ? q"
	}
}

func stripStatusColors(s string) string {
	replacer := strings.NewReplacer(
		"[red]", "",
		"[green]", "",
		"[yellow]", "",
		"[aqua]", "",
		"[white]", "",
		"[dim]", "",
	)
	return replacer.Replace(s)
}

func (a *App) setStatusNote(note string, ttl time.Duration) {
	a.statusNote = strings.TrimSpace(note)
	a.statusNoteUntil = time.Now().Add(ttl)
	a.updateStatusBar()
}
