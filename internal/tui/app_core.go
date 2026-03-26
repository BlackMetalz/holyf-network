package tui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/BlackMetalz/holyf-network/internal/tui/actionlog"
	"github.com/BlackMetalz/holyf-network/internal/tui/blocking"
	"github.com/BlackMetalz/holyf-network/internal/tui/diagnosis"
	tuilayout "github.com/BlackMetalz/holyf-network/internal/tui/layout"
	"github.com/BlackMetalz/holyf-network/internal/tui/livetrace"
	tuioverlays "github.com/BlackMetalz/holyf-network/internal/tui/overlays"
	tuipanels "github.com/BlackMetalz/holyf-network/internal/tui/panels"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	"github.com/BlackMetalz/holyf-network/internal/tui/traffic"
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
	helpView  *tview.TextView
	statusBar *tview.TextView
	grid      *tview.Grid // Store grid for zoom toggle

	focusIndex int    // Which panel is currently focused in the live layout.
	ifaceName  string // Network interface being monitored
	refreshSec int    // Refresh interval in seconds
	appVersion string
	// Non-empty when a newer GitHub release tag is detected at startup.
	updateLatestTag string

	// Previous snapshots for rate calculation (need 2 readings for delta)
	prevIfaceStats    *collector.InterfaceStats
	prevCPUStats      *collector.CPUStats
	prevConntrack     *collector.ConntrackData
	latestSystemUsage collector.SystemUsage
	systemUsageReady  bool
	systemUsageErr    string
	prevRetransmit    *collector.RetransmitData
	bwTracker         *collector.BandwidthTracker
	ssBWTracker       *collector.SocketBandwidthTracker

	// Port filter for Top Connections panel. Empty = show all.
	portFilter string
	// Text contains-filter for Top Connections ("/" search). Empty = no grep filter.
	textFilter string
	// Hide sensitive IP prefixes in Top Connections.
	sensitiveIP bool
	// Latest top connections snapshot used by actions (kill peer, etc.).
	latestTalkers []collector.Connection
	// Local listener ports used to classify incoming vs outgoing flows.
	listenPorts      map[int]struct{}
	listenPortsKnown bool
	topDirection     tuishared.TopConnectionDirection
	// Current top-connection bandwidth sample metadata.
	topSampleSeconds float64
	topBandwidthNote string
	topDiagnosis     *tuishared.Diagnosis
	diagnosisEngine  *diagnosis.Engine

	// Selected row in Top Connections (within currently visible rows).
	selectedTalkerIndex int
	// Zero-based page index for Top Connections/Groups.
	topPageIndex int
	// Sort mode for Top Connections. Default: tuishared.SortByBandwidth.
	sortMode tuishared.SortMode
	// Sort direction for Top Connections. true=DESC, false=ASC.
	sortDesc bool
	// Connection States panel sort direction by count. true=DESC, false=ASC.
	connStateSortDesc bool
	// Group view mode: when true, show per-peer aggregates instead of individual connections.
	groupView bool

	// Short-lived status note shown in status bar.
	statusNote      string
	statusNoteUntil time.Time
	lastStatusNote  string

	// Action log manager (session history and persistence)
	actionLogger *actionlog.Logger


	// Live trace engine (captures, history, and pause state)
	traceEngine *livetrace.Engine

	// Blocking and firewall state
	blockManager *blocking.Manager

	// Auto-refresh state
	stopChan    chan struct{}
	refreshChan chan struct{}
	paused      atomic.Bool
	lastRefresh time.Time

	// Zoom state
	zoomed bool // Whether a panel is zoomed to fullscreen

	healthThresholds config.HealthThresholds
	trafficManager   *traffic.Manager
}

var livePanelFocusOrder = []int{2, 0, 1, 3, 4} // 1=Top, 2=States, 3=Interface, 4=Conntrack, 5=Diagnosis

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
		app:                 tview.NewApplication(),
		ifaceName:           ifaceName,
		refreshSec:          refreshSec,
		appVersion:          version,
		focusIndex:          2, // Top Connections panel is default active focus.
		sensitiveIP:         sensitiveIP,
		blockManager:        blocking.NewManager(),
		trafficManager:      traffic.NewManager(healthThresholds),
		diagnosisEngine:     diagnosis.NewEngine(),
		traceEngine:         livetrace.NewEngine(defaultTraceHistoryDataDir()),
		stopChan:            make(chan struct{}),
		refreshChan:         make(chan struct{}, 1), // Buffered: so send never blocks
		healthThresholds:    healthThresholds,
		actionLogger:        actionlog.NewLogger(defaultActionHistoryPath()),
		sortDesc:            true,
		connStateSortDesc:   true,
		bwTracker:           collector.NewBandwidthTracker(),
		ssBWTracker:         collector.NewSocketBandwidthTracker(),
	}
}

// Run starts the TUI event loop. This blocks until the user quits.
func (a *App) Run() error {
	// Build UI components
	a.panels = tuilayout.CreatePanels()
	a.statusBar = tuilayout.CreateStatusBar(a.ifaceName)
	a.grid = tuilayout.CreateGrid(a.panels, a.statusBar)
	helpModal, helpView := tuioverlays.CreateCenteredTextViewModal(" Help ", "")
	a.helpView = helpView
	if a.helpView != nil {
		a.helpView.SetText(tuioverlays.BuildLiveHelpText(tuioverlays.LiveHelpContext{FocusIndex: a.focusIndex, Direction: a.topDirection, GroupView: a.groupView}))
	}

	// tview.Pages lets us stack "pages" (layers) on top of each other.
	// "main" is always visible, "help" is shown/hidden on top.
	a.pages = tview.NewPages()
	a.pages.AddPage("main", a.grid, true, true)
	a.pages.AddPage("help", helpModal, true, false) // resize=true, visible=false

	// Set initial focus highlight
	tuilayout.HighlightPanel(a.panels, a.focusIndex)

	// Load initial data into panels
	a.refreshData()

	// Register global key handler
	a.app.SetInputCapture(a.handleKeyEvent)

	// Start background goroutines
	go a.startRefreshLoop()
	go a.startInterfaceRefreshLoop()
	go a.startWarmupRefresh()
	go a.startStatusTicker()
	a.startUpdateCheck()

	// Start the application (blocks until app.Stop() is called)
	a.app.SetRoot(a.pages, true)
	return a.app.Run()
}

// startUpdateCheck checks GitHub releases once in background and updates footer-right when ready.
func (a *App) startUpdateCheck() {
	currentVersion := strings.TrimSpace(a.appVersion)

	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		latestTag, ok := tuishared.CheckForUpdate(ctx, client, currentVersion)
		if !ok {
			return
		}

		a.app.QueueUpdateDraw(func() {
			a.updateLatestTag = latestTag
			a.updateStatusBar()
		})
	}()
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

// startWarmupRefresh performs one early full refresh shortly after startup.
// This helps stabilize highly volatile first-sample panels (especially Top Connections)
// without changing the configured steady-state refresh interval.
func (a *App) startWarmupRefresh() {
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()

	select {
	case <-timer.C:
		if a.paused.Load() {
			return
		}
		a.app.QueueUpdateDraw(func() {
			a.refreshData()
		})
	case <-a.stopChan:
		return
	}
}

// startInterfaceRefreshLoop runs a dedicated 1-second refresh lane
// for Interface Stats so operators can track bandwidth spikes in near real time.
// This does not change the main refresh interval used by other panels.
func (a *App) startInterfaceRefreshLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if a.paused.Load() {
				continue
			}
			a.app.QueueUpdateDraw(func() {
				a.refreshInterfacePanel()
			})
		case <-a.stopChan:
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

func (a *App) refreshInterfacePanel() {
	ifaceStats, err := collector.CollectInterfaceStats(a.ifaceName)
	if err != nil {
		a.panels[1].SetText(fmt.Sprintf("  [red]%v[white]", err))
		return
	}

	rates := collector.CalculateRates(ifaceStats, a.prevIfaceStats)
	linkSpeedMbps, linkSpeedKnown := collector.CollectInterfaceSpeedMbps(a.ifaceName)
	linkSpeedBps := 0.0
	if linkSpeedKnown && linkSpeedMbps > 0 {
		linkSpeedBps = linkSpeedMbps * 1_000_000 / 8.0
	}
	a.trafficManager.SetSpeed(linkSpeedMbps, linkSpeedKnown)
	spike := a.trafficManager.EvaluateInterfaceSpike(rates, linkSpeedBps, linkSpeedKnown)
	a.panels[1].SetText(tuipanels.RenderInterfacePanel(rates, spike, tuishared.InterfaceSystemSnapshot{
		Usage:      a.latestSystemUsage,
		Ready:      a.systemUsageReady,
		Err:        a.systemUsageErr,
		RefreshSec: a.refreshSec,
	}))
	a.prevIfaceStats = &ifaceStats
}

// refreshData collects data from system and updates all panels.
func (a *App) refreshData() {
	a.lastRefresh = time.Now()
	activeThresholds := a.trafficManager.ActiveHealthThresholds()

	if usage, cpuStats, usageErr := collector.CollectSystemUsage(a.prevCPUStats); usageErr != nil {
		a.systemUsageErr = shortStatus(usageErr.Error(), 96)
	} else {
		a.latestSystemUsage = usage
		a.systemUsageReady = true
		a.systemUsageErr = ""
		a.prevCPUStats = cpuStats
	}

	// Collect conntrack early so panel 0 health strip can use it too.
	var conntrackRates *collector.ConntrackRates
	ctData, err := collector.CollectConntrack()
	if err != nil {
		a.panels[3].SetText(fmt.Sprintf("  [red]%v[white]", err))
	} else {
		rates := collector.CalculateConntrackRates(ctData, a.prevConntrack)
		conntrackRates = &rates
		a.panels[3].SetText(tuipanels.RenderConntrackPanel(rates, activeThresholds.ConntrackPercent))
		a.prevConntrack = &ctData
	}

	// Panel 0: Connection States + Retransmits
	var retransRates *collector.RetransmitRates
	var connData collector.ConnectionData
	connDataAvailable := false
	retransData, retransErr := collector.CollectRetransmits()
	if retransErr == nil {
		r := collector.CalculateRetransmitRates(retransData, a.prevRetransmit)
		retransRates = &r
		a.prevRetransmit = &retransData
	}

	connData, err = collector.CollectConnections()
	if err != nil {
		a.panels[0].SetText(fmt.Sprintf("  [red]%v[white]", err))
	} else {
		connDataAvailable = true
		a.panels[0].SetText(tuipanels.RenderConnectionsPanelWithStateSort(connData, retransRates, conntrackRates, activeThresholds, a.connStateSortDesc))
	}

	a.ensureListenPortsKnown()

	// Panel 1: Interface Stats
	// Kept here for initial/manual/full refresh coherence; a separate 1s lane
	// also updates this panel for near-real-time bandwidth visibility.
	a.refreshInterfacePanel()

	// Panel 2: Top Connections
	// Use full socket set (no collector cap) so group view/counts stay
	// consistent with large-state scenarios (e.g., CLOSE_WAIT pressure).
	// Rendering is still capped by panel height.
	talkers, err := collector.CollectTopTalkers(0)
	if err != nil {
		a.latestTalkers = nil
		a.topSampleSeconds = 0
		a.topBandwidthNote = ""
		a.topDiagnosis = nil
		a.resetTopConnectionsCursor()
		a.updateTopConnectionsPanelTitle()
		a.panels[2].SetText(fmt.Sprintf("  [red]%v[white]", err))
	} else {
		bwSample := collector.BandwidthSnapshot{}
		a.topBandwidthNote = ""
		flows, flowErr := collector.CollectConntrackFlowsTCP()
		if len(flows) > 0 {
			talkers = collector.MergeConntrackHostFlows(talkers, flows)
		}
		if flowErr == nil && a.bwTracker != nil {
			bwSample = a.bwTracker.BuildSnapshot(flows, time.Now())
			talkers = collector.EnrichConnectionsWithBandwidth(talkers, bwSample)
			if !bwSample.Available {
				a.topBandwidthNote = "Bandwidth baseline is warming up (first sample has no delta yet; press r or wait next refresh)."
			}
		} else if flowErr != nil {
			a.topBandwidthNote = "Bandwidth unavailable: " + shortStatus(flowErr.Error(), 140)
		}

		// Fallback path: socket tcp_info counters from ss (helps when conntrack mapping is incomplete).
		ssCounters, ssErr := collector.CollectSocketTCPCounters()
		if ssErr == nil && a.ssBWTracker != nil {
			ssSample := a.ssBWTracker.BuildSnapshot(ssCounters, time.Now())
			if ssSample.Available {
				talkers = collector.OverlayMissingBandwidth(talkers, ssSample)
			}
		} else if a.topBandwidthNote == "" && ssErr != nil {
			a.topBandwidthNote = "Bandwidth unavailable: " + shortStatus(ssErr.Error(), 140)
		}
		a.topSampleSeconds = bwSample.SampleSeconds

		a.latestTalkers = talkers
		if connDataAvailable {
			a.topDiagnosis = diagnosis.BuildTopDiagnosis(connData, retransRates, conntrackRates, activeThresholds, talkers, a.sensitiveIP)
			a.appendDiagnosisHistory(a.lastRefresh, a.topDiagnosis)
		} else {
			a.topDiagnosis = nil
		}
		a.renderTopConnectionsPanel()
	}
	a.renderDiagnosisPanel()

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
		return nil

	case tcell.KeyDown:
		if a.focusIndex == 2 && a.moveTopConnectionSelection(1) {
			return nil
		}
		return nil

	case tcell.KeyLeft, tcell.KeyRight:
		// Disable horizontal panning in panel text views to keep layout stable.
		return nil

	case tcell.KeyEnter:
		if a.focusIndex == 2 {
			if a.topDirection == tuishared.TopConnectionOutgoing {
				a.setStatusNote("Enter/k is disabled in OUT mode", 4*time.Second)
				return nil
			}
			blocking.PromptKillPeer(a, a.blockManager, nil)
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
		if event.Modifiers()&tcell.ModCtrl != 0 {
			if a.handleCtrlPanelShortcut(event.Rune()) {
				return nil
			}
		}
		// tcell.KeyRune means a regular character key (not special key)
		switch event.Rune() {
		case 'q':
			a.cancelTracePacketCapture()
			a.blockManager.CleanupActiveBlocks()
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
			if a.focusIndex != 0 {
				a.setStatusNote("s=sort states (focus Connection States)", 4*time.Second)
				return nil
			}
			a.connStateSortDesc = !a.connStateSortDesc
			dir := "ASC"
			if a.connStateSortDesc {
				dir = "DESC"
			}
			a.setStatusNote("Connection States sort: count "+dir, 4*time.Second)
			a.refreshData()
			return nil
		case 'm':
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
			if a.focusIndex == 2 && a.topDirection == tuishared.TopConnectionOutgoing {
				a.setStatusNote("Enter/k is disabled in OUT mode", 4*time.Second)
				return nil
			}
			blocking.PromptKillPeer(a, a.blockManager, nil)
			return nil
		case 'b':
			blocking.PromptBlockedPeers(a, a.blockManager)
			return nil
		case 'h':
			a.promptActionLog()
			return nil
		case 'd':
			a.promptDiagnosisHistory()
			return nil
		case 't':
			a.promptTraceHistory()
			return nil
		case 'i':
			a.promptSocketQueueExplain()
			return nil
		case 'I':
			a.promptInterfaceStatsExplain()
			return nil
		case 'T':
			a.promptTracePacket()
			return nil
		case 'B', 'C', 'P':
			mode, ok := directSortModeForRune(event.Rune())
			if !ok {
				return event
			}
			a.applyTopConnectionSortInput(mode)
			return nil
		case '[':
			if a.focusIndex != 2 {
				a.setStatusNote("Focus Top Connections before changing page", 4*time.Second)
				return nil
			}
			if !a.moveTopConnectionPage(-1) {
				a.setStatusNote("Top Connections: already at first page", 4*time.Second)
			}
			return nil
		case ']':
			if a.focusIndex != 2 {
				a.setStatusNote("Focus Top Connections before changing page", 4*time.Second)
				return nil
			}
			if !a.moveTopConnectionPage(1) {
				a.setStatusNote("Top Connections: already at last page", 4*time.Second)
			}
			return nil
		case 'g':
			a.groupView = !a.groupView
			a.resetTopConnectionsCursor()
			a.renderTopConnectionsPanel()
			return nil
		case 'o':
			if a.focusIndex != 2 {
				a.setStatusNote("Focus Top Connections before toggling IN/OUT", 4*time.Second)
				return nil
			}
			a.toggleTopConnectionsDirection()
			return nil
		case 'z':
			if !a.zoomed && a.focusIndex != 2 {
				a.setStatusNote("Zoom is only available for Top Connections", 4*time.Second)
				return nil
			}
			a.toggleZoom()
			return nil
		}
	}

	// Let tview handle other keys (arrow keys for scrolling, etc.)
	return event
}

// focusNext moves focus to the next panel (wraps around).
func (a *App) focusNext() {
	orderPos := indexInOrder(livePanelFocusOrder, a.focusIndex)
	if orderPos < 0 {
		a.focusIndex = livePanelFocusOrder[0]
		tuilayout.HighlightPanel(a.panels, a.focusIndex)
		return
	}
	nextPos := (orderPos + 1) % len(livePanelFocusOrder)
	a.focusIndex = livePanelFocusOrder[nextPos]
	tuilayout.HighlightPanel(a.panels, a.focusIndex)
}

// focusPrev moves focus to the previous panel (wraps around).
func (a *App) focusPrev() {
	orderPos := indexInOrder(livePanelFocusOrder, a.focusIndex)
	if orderPos < 0 {
		a.focusIndex = livePanelFocusOrder[0]
		tuilayout.HighlightPanel(a.panels, a.focusIndex)
		return
	}
	prevPos := (orderPos - 1 + len(livePanelFocusOrder)) % len(livePanelFocusOrder)
	a.focusIndex = livePanelFocusOrder[prevPos]
	tuilayout.HighlightPanel(a.panels, a.focusIndex)
}

func (a *App) handleCtrlPanelShortcut(r rune) bool {
	switch r {
	case '1':
		a.focusPanel(2)
		return true
	case '2':
		a.focusPanel(0)
		return true
	case '3':
		a.focusPanel(1)
		return true
	case '4':
		a.focusPanel(3)
		return true
	case '5':
		a.focusPanel(4)
		return true
	default:
		return false
	}
}

func (a *App) focusPanel(index int) {
	if index < 0 || index >= len(a.panels) {
		return
	}
	a.focusIndex = index
	tuilayout.HighlightPanel(a.panels, a.focusIndex)
}

func indexInOrder(order []int, target int) int {
	for i, item := range order {
		if item == target {
			return i
		}
	}
	return -1
}

func directSortModeForRune(r rune) (tuishared.SortMode, bool) {
	switch r {
	case 'B':
		return tuishared.SortByBandwidth, true
	case 'C':
		return tuishared.SortByConns, true
	case 'P':
		return tuishared.SortByPort, true
	default:
		return tuishared.SortByBandwidth, false
	}
}

func (a *App) applyTopConnectionSortInput(mode tuishared.SortMode) {
	if a.sortMode == mode {
		a.sortDesc = !a.sortDesc
	} else {
		a.sortMode = mode
		a.sortDesc = true // first hit on mode starts DESC
	}
	a.resetTopConnectionsCursor()
	a.renderTopConnectionsPanel()
}

func (a *App) showHelp() {
	if a.helpView != nil {
		a.helpView.SetText(tuioverlays.BuildLiveHelpText(tuioverlays.LiveHelpContext{FocusIndex: a.focusIndex, Direction: a.topDirection, GroupView: a.groupView}))
	}
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
	a.grid = tuilayout.CreateGrid(a.panels, a.statusBar)

	a.pages.RemovePage("main")
	a.pages.AddPage("main", a.grid, true, true)

	a.zoomed = false
	tuilayout.HighlightPanel(a.panels, a.focusIndex)
	a.updateStatusBar()
}

// updateStatusBar updates the bottom status bar with current state.
func (a *App) updateStatusBar() {
	if a.statusBar == nil {
		return
	}
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
	if a.trafficManager.IfaceSpeedSample() && a.trafficManager.IfaceSpeedKnown() && a.trafficManager.IfaceSpeedMbps() > 0 {
		stateText += fmt.Sprintf(" [aqua]LINK:%.0fMb/s[white] |", a.trafficManager.IfaceSpeedMbps())
	}
	if time.Now().Before(a.statusNoteUntil) && a.statusNote != "" {
		stateText += fmt.Sprintf(" [yellow]%s[white] |", a.statusNote)
	} else if strings.TrimSpace(a.lastStatusNote) != "" {
		stateText += fmt.Sprintf(" [dim]Last:%s[white] |", shortStatus(a.lastStatusNote, 72))
	}

	page := a.frontPageName()
	hotkeysStyled, hotkeysPlain := a.statusHotkeysForPage(page)
	leftStyled := fmt.Sprintf(
		" [yellow]%s[white] |%s Updated: [green]%s[white] | Refresh Intervals: [green]%ds[white] | %s",
		a.ifaceName,
		stateText,
		ago,
		a.refreshSec,
		hotkeysStyled,
	)
	leftPlain := fmt.Sprintf(
		" %s |%s Updated: %s | Refresh Intervals: %ds | %s",
		a.ifaceName,
		stripStatusColors(stateText),
		ago,
		a.refreshSec,
		hotkeysPlain,
	)
	versionPlain := "holyf-network " + a.appVersion
	rightStyled := " [dim]" + versionPlain + "[white]"
	rightPlain := " " + versionPlain
	if latest := strings.TrimSpace(a.updateLatestTag); latest != "" {
		rightStyled += " [yellow](new " + latest + ")[white]"
		rightPlain += " (new " + latest + ")"
	}

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

func (a *App) statusHotkeysForPage(page string) (styled string, plain string) {
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
	case tracePacketPageForm:
		return "[dim]Tab[white]=field [dim]Enter[white]=start [dim]Esc[white]=cancel", "Tab=field Enter=start Esc=cancel"
	case tracePacketPageProgress:
		return "[dim]Esc[white]=abort [dim]q[white]=abort", "Esc=abort q=abort"
	case tracePacketPageResult:
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	case "blocked-peers":
		return "[dim]Up/Down[white]=select [dim]Enter[white]=remove [dim]Del[white]=remove [dim]Tab[white]=buttons [dim]Esc[white]=close",
			"Up/Down=select Enter=remove Del=remove Tab=buttons Esc=close"
	case "action-log":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	case "diagnosis-history":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	case traceHistoryPage:
		return "[dim]Up/Down[white]=select [dim]Enter[white]=detail [dim]c[white]=compare [dim]Esc[white]=close", "Up/Down=select Enter=detail c=compare Esc=close"
	case traceHistoryDetailPage:
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	case traceHistoryComparePage:
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	case "socket-queue-explain":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	case "interface-stats-explain":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	case "blocked-peers-remove-result", "block-summary":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	default:
		return tuioverlays.LiveMainStatusHotkeys(a.focusIndex, a.topDirection)
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
	if a.statusNote != "" {
		a.lastStatusNote = a.statusNote
	}
	a.statusNoteUntil = time.Now().Add(ttl)
	a.updateStatusBar()
}

func (a *App) promptSocketQueueExplain() {
	closeModal := func() {
		a.pages.RemovePage("socket-queue-explain")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}

	modal := tview.NewModal().
		SetText(tuioverlays.BuildSocketQueueExplainText(false)).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(_ int, _ string) {
			closeModal()
		})
	modal.SetTitle(" Send-Q / Recv-Q Explain ")
	modal.SetBorder(true)
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeModal()
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			closeModal()
			return nil
		}
		return event
	})

	a.pages.RemovePage("socket-queue-explain")
	a.pages.AddPage("socket-queue-explain", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(modal)
}

func (a *App) promptInterfaceStatsExplain() {
	closeModal := func() {
		a.pages.RemovePage("interface-stats-explain")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}

	modal := tview.NewModal().
		SetText(tuioverlays.BuildInterfaceStatsExplainText()).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(_ int, _ string) {
			closeModal()
		})
	modal.SetTitle(" Interface Stats Explain ")
	modal.SetBorder(true)
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeModal()
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			closeModal()
			return nil
		}
		return event
	})

	a.pages.RemovePage("interface-stats-explain")
	a.pages.AddPage("interface-stats-explain", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(modal)
}

func (a *App) renderDiagnosisPanel() {
	if len(a.panels) <= 4 || a.panels[4] == nil {
		return
	}

	_, _, width, _ := a.panels[4].GetInnerRect()
	a.panels[4].SetText(tuipanels.RenderDiagnosisPanel(a.topDiagnosis, width))
}

// --- UIContext interface implementation for blocking package ---

func (a *App) AddPage(name string, item tview.Primitive, resize, visible bool) {
	a.pages.AddPage(name, item, resize, visible)
}

func (a *App) RemovePage(name string) {
	a.pages.RemovePage(name)
}

func (a *App) SendToFront(name string) {
	a.pages.SendToFront(name)
}

func (a *App) SetFocus(p tview.Primitive) {
	a.app.SetFocus(p)
}

func (a *App) RestoreFocus() {
	if a.focusIndex >= 0 && a.focusIndex < len(a.panels) {
		a.app.SetFocus(a.panels[a.focusIndex])
	}
}

func (a *App) SetStatusNote(msg string, ttl time.Duration) {
	a.setStatusNote(msg, ttl)
}

func (a *App) AddActionLog(msg string) {
	a.addActionLog(msg)
}

func (a *App) QueueUpdateDraw(f func()) {
	a.app.QueueUpdateDraw(f)
}

func (a *App) UpdateStatusBar() {
	a.updateStatusBar()
}

func (a *App) RefreshData() {
	a.refreshData()
}

func (a *App) IsPaused() bool {
	return a.paused.Load()
}

func (a *App) SetPaused(paused bool) {
	a.paused.Store(paused)
}

func (a *App) TopDirection() tuishared.TopConnectionDirection {
	return a.topDirection
}

func (a *App) PortFilter() string {
	return a.portFilter
}

func (a *App) LatestTalkers() []collector.Connection {
	return a.latestTalkers
}

// --- Shared Constants & Utilities ---

const (
	defaultBlockMinutes = 10
	maxBlockMinutes     = 1440

	actionLogModalLimit      = 20
	inMemoryActionLogMax     = 500
	actionLogRotateLimit     = 500
	traceHistoryModalLimit   = 20
	diagnosisHistoryLimit    = 20
	actionHistoryDirName     = ".holyf-network"
	actionHistoryFileName    = "history.log"
	actionHistoryDisplayPath = "~/.holyf-network/history.log"
)

func shortStatus(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func formatBlockDuration(duration time.Duration) string {
	minutes := int(duration / time.Minute)
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	seconds := int(duration / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprintf("%ds", seconds)
}

func formatRemainingDuration(duration time.Duration) string {
	if duration <= 0 {
		return "00:00"
	}

	totalSeconds := int(duration.Round(time.Second) / time.Second)
	if totalSeconds < 0 {
		totalSeconds = 0
	}

	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%dh%02dm", hours, minutes)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func sanitizeActionLogMessage(message string) string {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return ""
	}
	if !strings.HasPrefix(msg, "Blocked ") {
		return msg
	}

	parts := strings.Split(msg, " | ")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "expires in ") {
			continue
		}
		if p == "killed 0/0 flows" {
			continue
		}
		filtered = append(filtered, p)
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, " | ")
}

// --- Diagnosis History Modal ---

func (a *App) appendDiagnosisHistory(now time.Time, diag *tuishared.Diagnosis) {
	a.diagnosisEngine.Append(now, diag)
}

func (a *App) recentDiagnosisHistory(limit int) []diagnosis.HistoryEntry {
	return a.diagnosisEngine.Recent(limit)
}

func (a *App) promptDiagnosisHistory() {
	entries := a.recentDiagnosisHistory(diagnosisHistoryLimit)

	var body strings.Builder
	if len(entries) == 0 {
		body.WriteString("  No diagnosis changes yet")
	} else {
		for i, entry := range entries {
			body.WriteString("  ")
			body.WriteString(formatDiagnosisHistoryEntry(entry))
			if i < len(entries)-1 {
				body.WriteString("\n")
			}
		}
	}
	body.WriteString("\n\n")
	body.WriteString(fmt.Sprintf(
		"  [dim]Showing latest %d diagnosis changes in this live session[white]",
		diagnosisHistoryLimit,
	))

	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetTextAlign(tview.AlignLeft).
		SetText(body.String())
	view.SetBorder(true)
	view.SetTitle(fmt.Sprintf(" Diagnosis History (latest %d) ", diagnosisHistoryLimit))

	closeModal := func() {
		a.pages.RemovePage("diagnosis-history")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}

	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc, tcell.KeyEnter:
			closeModal()
			return nil
		}
		return event
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(view, 116, 0, true).
			AddItem(nil, 0, 1, false),
			16, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage("diagnosis-history")
	a.pages.AddPage("diagnosis-history", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(view)
}

func formatDiagnosisHistoryEntry(entry diagnosis.HistoryEntry) string {
	color := tuishared.ColorForHealthLevel(entry.Diagnosis.Severity)
	conf := shortStatus(tuipanels.DiagnosisConfidenceValue(&entry.Diagnosis), 8)
	issue := shortStatus(tuipanels.DiagnosisIssueValue(&entry.Diagnosis), 36)
	scope := shortStatus(tuipanels.DiagnosisScopeValue(&entry.Diagnosis), 36)
	signal := shortStatus(tuipanels.DiagnosisSignalValue(&entry.Diagnosis), 96)

	return fmt.Sprintf(
		"[%s]%s[white] | [%s]%s[white] | %s | %s | %s",
		color,
		formatDiagnosisHistoryRange(entry),
		color,
		issue,
		conf,
		scope,
		signal,
	)
}

func formatDiagnosisHistoryRange(entry diagnosis.HistoryEntry) string {
	first := entry.FirstSeen.Format("15:04:05")
	last := entry.LastSeen.Format("15:04:05")
	if first == last {
		return first
	}
	return first + "-" + last
}

// --- Action Log Modal ---

func (a *App) promptActionLog() {
	logs := a.actionLogger.Recent(actionLogModalLimit)

	var body strings.Builder
	if len(logs) == 0 {
		body.WriteString("  No actions yet")
	} else {
		for i, entry := range logs {
			body.WriteString("  ")
			body.WriteString(entry)
			if i < len(logs)-1 {
				body.WriteString("\n")
			}
		}
	}
	body.WriteString("\n\n")
	body.WriteString(fmt.Sprintf(
		"  [dim]Showing latest %d. Full history: %s (rolling %d events)[white]",
		actionLogModalLimit,
		actionHistoryDisplayPath,
		actionLogRotateLimit,
	))

	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetTextAlign(tview.AlignLeft).
		SetText(body.String())
	view.SetBorder(true)
	view.SetTitle(fmt.Sprintf(" Action Log (latest %d) ", actionLogModalLimit))

	closeModal := func() {
		a.pages.RemovePage("action-log")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}

	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc, tcell.KeyEnter:
			closeModal()
			return nil
		}
		return event
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(view, 100, 0, true).
			AddItem(nil, 0, 1, false),
			16, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage("action-log")
	a.pages.AddPage("action-log", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(view)
}

func (a *App) addActionLog(message string) {
	a.actionLogger.Add(message)
}

func (a *App) recentActionLogs(limit int) []string {
	if limit <= 0 {
		limit = actionLogModalLimit
	}
	return a.actionLogger.Recent(limit)
}

func defaultActionHistoryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(strings.TrimSpace(home), actionHistoryDirName, actionHistoryFileName)
}
