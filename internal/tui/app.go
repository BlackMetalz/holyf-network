package tui

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/actions"
	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
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
