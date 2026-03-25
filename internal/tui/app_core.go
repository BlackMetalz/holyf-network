package tui

import (
	"context"
	"fmt"
	"net/http"
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
	topDirection     topConnectionDirection
	// Current top-connection bandwidth sample metadata.
	topSampleSeconds float64
	topBandwidthNote string
	topDiagnosis     *topDiagnosis
	diagnosisHistory []diagnosisHistoryEntry
	// Selected row in Top Connections (within currently visible rows).
	selectedTalkerIndex int
	// Zero-based page index for Top Connections/Groups.
	topPageIndex int
	// Sort mode for Top Connections. Default: SortByBandwidth.
	sortMode SortMode
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

	// Recent action history (for modal "h").
	actionLogMu sync.Mutex
	actionLogs  []string
	// Persistent action history location (~/.holyf-network/history.log).
	actionHistoryPath string

	// Recent trace-packet history (for modal "t"), persisted as NDJSON.
	traceHistoryMu      sync.Mutex
	traceHistory        []traceHistoryEntry
	traceHistoryLoaded  bool
	traceHistoryDataDir string

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
	// Tracks temporary auto-pause while the trace-packet flow is open.
	traceFlowAutoPaused bool
	// Whether a trace-packet capture is currently running.
	traceCaptureRunning bool
	// Active cancel function for running trace-packet capture (if any).
	traceCaptureCancel context.CancelFunc

	// Zoom state
	zoomed bool // Whether a panel is zoomed to fullscreen

	healthThresholds config.HealthThresholds
	alertProfile     config.AlertProfile
	alertProfileSpec config.AlertProfileSpec
	ifaceSpikeEMA    float64
	ifaceSpikeCount  int
	ifaceSpeedMbps   float64
	ifaceSpeedKnown  bool
	ifaceSpeedSample bool
}

var livePanelFocusOrder = []int{2, 0, 1, 3, 4} // 1=Top, 2=States, 3=Interface, 4=Conntrack, 5=Diagnosis

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
	alertProfile config.AlertProfile,
) *App {
	version := strings.TrimSpace(appVersion)
	if version == "" {
		version = "dev"
	}
	healthThresholds.Normalize()
	alertProfileSpec := config.AlertProfileSpecFor(alertProfile)

	return &App{
		app:                 tview.NewApplication(),
		ifaceName:           ifaceName,
		refreshSec:          refreshSec,
		appVersion:          version,
		focusIndex:          2, // Top Connections panel is default active focus.
		sensitiveIP:         sensitiveIP,
		activeBlocks:        make(map[string]activeBlockEntry),
		stopChan:            make(chan struct{}),
		refreshChan:         make(chan struct{}, 1), // Buffered: so send never blocks
		healthThresholds:    healthThresholds,
		alertProfile:        alertProfileSpec.Name,
		alertProfileSpec:    alertProfileSpec,
		actionLogs:          make([]string, 0, 32),
		actionHistoryPath:   defaultActionHistoryPath(),
		traceHistory:        make([]traceHistoryEntry, 0, 32),
		traceHistoryDataDir: defaultTraceHistoryDataDir(),
		sortDesc:            true,
		connStateSortDesc:   true,
		bwTracker:           collector.NewBandwidthTracker(),
		ssBWTracker:         collector.NewSocketBandwidthTracker(),
	}
}

// Run starts the TUI event loop. This blocks until the user quits.
func (a *App) Run() error {
	// Build UI components
	a.panels = createPanels()
	a.statusBar = createStatusBar(a.ifaceName)
	a.grid = createGrid(a.panels, a.statusBar)
	helpModal, helpView := createHelpModal()
	a.helpView = helpView
	if a.helpView != nil {
		a.helpView.SetText(buildLiveHelpText(a))
	}

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

		latestTag, ok := checkForUpdate(ctx, client, currentVersion)
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
	a.ifaceSpeedSample = true
	a.ifaceSpeedKnown = linkSpeedKnown
	if linkSpeedKnown {
		a.ifaceSpeedMbps = linkSpeedMbps
	} else {
		a.ifaceSpeedMbps = 0
	}
	spike := a.evaluateInterfaceSpike(rates, linkSpeedBps, linkSpeedKnown)
	a.panels[1].SetText(renderInterfacePanel(rates, spike, interfaceSystemSnapshot{
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
	activeThresholds := a.activeHealthThresholds()
	profileSpec := a.currentAlertProfileSpec()

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
		a.panels[3].SetText(renderConntrackPanel(rates, profileSpec.Thresholds.ConntrackPercent, profileSpec.Label))
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
		a.panels[0].SetText(renderConnectionsPanelWithStateSort(connData, retransRates, conntrackRates, activeThresholds, a.connStateSortDesc))
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
			a.topDiagnosis = a.buildTopDiagnosis(connData, retransRates, conntrackRates)
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
			if a.topDirection == topConnectionOutgoing {
				a.setStatusNote("Enter/k is disabled in OUT mode", 4*time.Second)
				return nil
			}
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
		if event.Modifiers()&tcell.ModCtrl != 0 {
			if a.handleCtrlPanelShortcut(event.Rune()) {
				return nil
			}
		}
		// tcell.KeyRune means a regular character key (not special key)
		switch event.Rune() {
		case 'q':
			a.cancelTracePacketCapture()
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
		case 'y':
			a.cycleAlertProfile()
			return nil
		case 'Y':
			a.promptAlertProfileExplain()
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
			if a.focusIndex == 2 && a.topDirection == topConnectionOutgoing {
				a.setStatusNote("Enter/k is disabled in OUT mode", 4*time.Second)
				return nil
			}
			a.promptKillPeer()
			return nil
		case 'b':
			a.promptBlockedPeers()
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
		highlightPanel(a.panels, a.focusIndex)
		return
	}
	nextPos := (orderPos + 1) % len(livePanelFocusOrder)
	a.focusIndex = livePanelFocusOrder[nextPos]
	highlightPanel(a.panels, a.focusIndex)
}

// focusPrev moves focus to the previous panel (wraps around).
func (a *App) focusPrev() {
	orderPos := indexInOrder(livePanelFocusOrder, a.focusIndex)
	if orderPos < 0 {
		a.focusIndex = livePanelFocusOrder[0]
		highlightPanel(a.panels, a.focusIndex)
		return
	}
	prevPos := (orderPos - 1 + len(livePanelFocusOrder)) % len(livePanelFocusOrder)
	a.focusIndex = livePanelFocusOrder[prevPos]
	highlightPanel(a.panels, a.focusIndex)
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
	highlightPanel(a.panels, a.focusIndex)
}

func indexInOrder(order []int, target int) int {
	for i, item := range order {
		if item == target {
			return i
		}
	}
	return -1
}

func directSortModeForRune(r rune) (SortMode, bool) {
	switch r {
	case 'B':
		return SortByBandwidth, true
	case 'C':
		return SortByConns, true
	case 'P':
		return SortByPort, true
	default:
		return SortByBandwidth, false
	}
}

func (a *App) applyTopConnectionSortInput(mode SortMode) {
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
		a.helpView.SetText(buildLiveHelpText(a))
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
	a.grid = createGrid(a.panels, a.statusBar)

	a.pages.RemovePage("main")
	a.pages.AddPage("main", a.grid, true, true)

	a.zoomed = false
	highlightPanel(a.panels, a.focusIndex)
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
	stateText += fmt.Sprintf(" [aqua]PROFILE-%s[white] |", a.currentAlertProfileSpec().Label)
	stateText += a.linkSpeedStatusIndicator()
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

func (a *App) linkSpeedStatusIndicator() string {
	if !a.ifaceSpeedSample {
		return " [dim]LINK(sysfs):warming[white] |"
	}
	if a.ifaceSpeedKnown && a.ifaceSpeedMbps > 0 {
		return fmt.Sprintf(" [aqua]LINK(sysfs):%.0fMb/s[white] |", a.ifaceSpeedMbps)
	}
	return " [yellow]LINK(sysfs):UNKNOWN[white] |"
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
	case "alert-profile-explain":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	case "blocked-peers-remove-result", "block-summary":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	default:
		return liveMainStatusHotkeys(a.focusIndex, a.topDirection)
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
