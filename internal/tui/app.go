package tui

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/actions"
	"github.com/BlackMetalz/holyf-network/internal/collector"
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

	// Previous snapshots for rate calculation (need 2 readings for delta)
	prevIfaceStats *collector.InterfaceStats
	prevConntrack  *collector.ConntrackData
	prevRetransmit *collector.RetransmitData

	// Port filter for Top Connections panel. Empty = show all.
	portFilter string
	// Hide sensitive IP prefixes in Top Connections.
	sensitiveIP bool
	// Latest top connections snapshot used by actions (kill peer, etc.).
	latestTalkers []collector.Connection

	// Short-lived status note shown in status bar.
	statusNote      string
	statusNoteUntil time.Time

	// Active peer blocks for cleanup on shutdown.
	blockMu      sync.Mutex
	activeBlocks map[string]actions.PeerBlockSpec

	// Auto-refresh state (Epic 7)
	stopChan    chan struct{}
	refreshChan chan struct{}
	paused      bool
	lastRefresh time.Time

	// Zoom state (V2-4.3)
	zoomed bool // Whether a panel is zoomed to fullscreen
}

// NewApp creates a new TUI application.
func NewApp(ifaceName string, refreshSec int, sensitiveIP bool) *App {
	return &App{
		app:          tview.NewApplication(),
		ifaceName:    ifaceName,
		refreshSec:   refreshSec,
		focusIndex:   0,
		sensitiveIP:  sensitiveIP,
		activeBlocks: make(map[string]actions.PeerBlockSpec),
		stopChan:     make(chan struct{}),
		refreshChan:  make(chan struct{}, 1), // Buffered: so send never blocks
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
			if !a.paused {
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
			a.paused = !a.paused
			a.updateStatusBar()
			return nil
		case 's':
			a.sensitiveIP = !a.sensitiveIP
			a.refreshData()
			return nil
		case 'f':
			a.promptPortFilter()
			return nil
		case 'k':
			a.promptKillPeer()
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

func (a *App) showHelp() {
	a.pages.SendToFront("help") // Ensure help renders above main after zoom reorder
	a.pages.ShowPage("help")
}
func (a *App) hideHelp() { a.pages.HidePage("help") }
func (a *App) isHelpVisible() bool {
	name, _ := a.pages.GetFrontPage()
	return name == "help"
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

// refreshData collects data from system and updates all panels.
func (a *App) refreshData() {
	a.lastRefresh = time.Now()

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
		a.panels[0].SetText(renderConnectionsPanel(connData, retransRates))
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
		a.panels[2].SetText(fmt.Sprintf("  [red]%v[white]", err))
	} else {
		a.latestTalkers = talkers
		displayLimit := 20
		if a.zoomed {
			displayLimit = 100
		}
		a.panels[2].SetText(renderTalkersPanel(talkers, a.portFilter, displayLimit, a.sensitiveIP))
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
	if a.paused {
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

	a.statusBar.SetText(fmt.Sprintf(
		" [yellow]%s[white] |%s Updated: [green]%s[white] | Refresh: [green]%ds[white] | [dim]r[white]=refresh [dim]p[white]=pause [dim]s[white]=mask-ip [dim]f[white]=filter [dim]k[white]=kill-peer [dim]z[white]=zoom [dim]?[white]=help [dim]q[white]=quit",
		a.ifaceName,
		stateText,
		ago,
		a.refreshSec,
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

type peerKillTarget struct {
	PeerIP    string
	LocalPort int
	Count     int
}

// promptKillPeer confirms and applies a temporary firewall block for a peer.
func (a *App) promptKillPeer() {
	if a.focusIndex != 2 {
		a.setStatusNote("Focus Top Connections before kill-peer", 5*time.Second)
		return
	}

	target, ok := a.selectPeerKillTarget()
	if !ok {
		a.setStatusNote("No peer candidate in current Top Connections view", 5*time.Second)
		return
	}
	spec := actions.PeerBlockSpec{PeerIP: target.PeerIP, LocalPort: target.LocalPort}
	if a.hasActiveBlock(spec) {
		a.setStatusNote(fmt.Sprintf("Already blocked %s:%d", target.PeerIP, target.LocalPort), 5*time.Second)
		return
	}

	duration := 10 * time.Minute
	label := "Block 10m"
	text := fmt.Sprintf(
		"Block peer %s -> local port %d for 10 minutes?\n\nMatches in current view: %d\nThis inserts a DROP rule and deletes active conntrack flows.",
		target.PeerIP,
		target.LocalPort,
		target.Count,
	)

	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{label, "Cancel"}).
		SetDoneFunc(func(_ int, button string) {
			a.pages.RemovePage("kill-peer")
			a.app.SetFocus(a.panels[a.focusIndex])
			if button != label {
				return
			}

			a.setStatusNote(fmt.Sprintf("Blocking %s:%d...", target.PeerIP, target.LocalPort), 4*time.Second)
			go a.blockPeerForDuration(target, duration)
		})
	modal.SetTitle(" Kill Peer ")
	modal.SetBorder(true)

	a.pages.AddPage("kill-peer", modal, true, true)
	a.app.SetFocus(modal)
}

// selectPeerKillTarget picks the most frequent peer->localPort tuple.
func (a *App) selectPeerKillTarget() (peerKillTarget, bool) {
	if len(a.latestTalkers) == 0 {
		return peerKillTarget{}, false
	}

	filtered := a.latestTalkers
	if a.portFilter != "" {
		filtered = filterByPort(filtered, a.portFilter)
	}
	if len(filtered) == 0 {
		return peerKillTarget{}, false
	}

	type aggregate struct {
		target   peerKillTarget
		activity int64
	}
	aggByKey := make(map[string]aggregate)

	for _, conn := range filtered {
		peer := normalizeIP(conn.RemoteIP)
		key := fmt.Sprintf("%s|%d", peer, conn.LocalPort)

		current := aggByKey[key]
		current.target.PeerIP = peer
		current.target.LocalPort = conn.LocalPort
		current.target.Count++
		current.activity += conn.Activity
		aggByKey[key] = current
	}

	candidates := make([]aggregate, 0, len(aggByKey))
	for _, candidate := range aggByKey {
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return peerKillTarget{}, false
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].target.Count != candidates[j].target.Count {
			return candidates[i].target.Count > candidates[j].target.Count
		}
		if candidates[i].activity != candidates[j].activity {
			return candidates[i].activity > candidates[j].activity
		}
		if candidates[i].target.LocalPort != candidates[j].target.LocalPort {
			return candidates[i].target.LocalPort < candidates[j].target.LocalPort
		}
		return candidates[i].target.PeerIP < candidates[j].target.PeerIP
	})

	return candidates[0].target, true
}

func (a *App) blockPeerForDuration(target peerKillTarget, duration time.Duration) {
	spec := actions.PeerBlockSpec{
		PeerIP:    target.PeerIP,
		LocalPort: target.LocalPort,
	}

	if err := actions.BlockPeer(spec); err != nil {
		a.app.QueueUpdateDraw(func() {
			a.setStatusNote("Block failed: "+shortStatus(err.Error(), 64), 8*time.Second)
		})
		return
	}
	a.addActiveBlock(spec)

	dropWarning := ""
	if err := actions.DropPeerConnections(spec); err != nil {
		dropWarning = " (flow-drop skipped)"
	}

	a.app.QueueUpdateDraw(func() {
		a.setStatusNote(fmt.Sprintf("Blocked %s:%d for 10m%s", target.PeerIP, target.LocalPort, dropWarning), 8*time.Second)
		a.refreshData()
	})

	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-a.stopChan:
		return
	}

	if err := actions.UnblockPeer(spec); err != nil {
		a.app.QueueUpdateDraw(func() {
			a.setStatusNote("Auto-unblock failed: "+shortStatus(err.Error(), 64), 8*time.Second)
		})
		return
	}
	a.removeActiveBlock(spec)

	a.app.QueueUpdateDraw(func() {
		a.setStatusNote(fmt.Sprintf("Unblocked %s:%d", target.PeerIP, target.LocalPort), 6*time.Second)
		a.refreshData()
	})
}

func (a *App) setStatusNote(note string, ttl time.Duration) {
	a.statusNote = strings.TrimSpace(note)
	a.statusNoteUntil = time.Now().Add(ttl)
	a.updateStatusBar()
}

func shortStatus(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func blockKey(spec actions.PeerBlockSpec) string {
	return fmt.Sprintf("%s|%d", spec.PeerIP, spec.LocalPort)
}

func (a *App) addActiveBlock(spec actions.PeerBlockSpec) {
	a.blockMu.Lock()
	defer a.blockMu.Unlock()
	a.activeBlocks[blockKey(spec)] = spec
}

func (a *App) removeActiveBlock(spec actions.PeerBlockSpec) {
	a.blockMu.Lock()
	defer a.blockMu.Unlock()
	delete(a.activeBlocks, blockKey(spec))
}

func (a *App) cleanupActiveBlocks() {
	a.blockMu.Lock()
	pending := make([]actions.PeerBlockSpec, 0, len(a.activeBlocks))
	for _, spec := range a.activeBlocks {
		pending = append(pending, spec)
	}
	a.blockMu.Unlock()

	for _, spec := range pending {
		_ = actions.UnblockPeer(spec)
		a.removeActiveBlock(spec)
	}
}

func (a *App) hasActiveBlock(spec actions.PeerBlockSpec) bool {
	a.blockMu.Lock()
	defer a.blockMu.Unlock()
	_, exists := a.activeBlocks[blockKey(spec)]
	return exists
}
