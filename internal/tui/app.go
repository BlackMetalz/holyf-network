package tui

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
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

	// Short-lived status note shown in status bar.
	statusNote      string
	statusNoteUntil time.Time

	// Active peer blocks for cleanup on shutdown.
	blockMu      sync.Mutex
	activeBlocks map[string]activeBlockEntry

	// Auto-refresh state (Epic 7)
	stopChan    chan struct{}
	refreshChan chan struct{}
	paused      bool
	lastRefresh time.Time
	// Tracks temporary auto-pause while the kill-peer flow is open.
	killFlowAutoPaused bool

	// Zoom state (V2-4.3)
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
		app:              tview.NewApplication(),
		ifaceName:        ifaceName,
		refreshSec:       refreshSec,
		appVersion:       version,
		focusIndex:       0,
		sensitiveIP:      sensitiveIP,
		activeBlocks:     make(map[string]activeBlockEntry),
		stopChan:         make(chan struct{}),
		refreshChan:      make(chan struct{}, 1), // Buffered: so send never blocks
		healthThresholds: healthThresholds,
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
		case '/':
			a.promptTextFilter()
			return nil
		case 'k':
			a.promptKillPeer()
			return nil
		case 'b':
			a.promptBlockedPeers()
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

func (a *App) topConnectionsDisplayLimit() int {
	if a.zoomed {
		return 100
	}
	return 20
}

func (a *App) visibleTopConnections() []collector.Connection {
	if len(a.latestTalkers) == 0 {
		return nil
	}

	filtered := a.applyTopConnectionFilters(a.latestTalkers)

	limit := a.topConnectionsDisplayLimit()
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

func (a *App) clampTopConnectionSelection() {
	visible := a.visibleTopConnections()
	if len(visible) == 0 {
		a.selectedTalkerIndex = 0
		return
	}
	if a.selectedTalkerIndex < 0 {
		a.selectedTalkerIndex = 0
		return
	}
	if a.selectedTalkerIndex >= len(visible) {
		a.selectedTalkerIndex = len(visible) - 1
	}
}

func (a *App) renderTopConnectionsPanel() {
	a.clampTopConnectionSelection()
	a.panels[2].SetText(renderTalkersPanel(
		a.latestTalkers,
		a.portFilter,
		a.textFilter,
		a.topConnectionsDisplayLimit(),
		a.sensitiveIP,
		a.selectedTalkerIndex,
	))
}

func (a *App) moveTopConnectionSelection(delta int) bool {
	visible := a.visibleTopConnections()
	if len(visible) == 0 {
		return false
	}
	a.clampTopConnectionSelection()

	next := a.selectedTalkerIndex + delta
	if next < 0 {
		next = 0
	}
	if next >= len(visible) {
		next = len(visible) - 1
	}
	if next == a.selectedTalkerIndex {
		return true
	}

	a.selectedTalkerIndex = next
	a.renderTopConnectionsPanel()
	a.updateStatusBar()
	return true
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
	case "blocked-peers-remove-result", "block-summary":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	default:
		return "[dim]r p f k b z ? q[white]", "r p f k b z ? q"
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

// promptPortFilter shows a simple input dialog for port filtering.
// Uses tview.InputField as a modal overlay.
func (a *App) promptPortFilter() {
	// If any filter is active, clear all filters.
	if a.portFilter != "" || a.textFilter != "" {
		a.portFilter = ""
		a.textFilter = ""
		a.selectedTalkerIndex = 0
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
			a.selectedTalkerIndex = 0
		}
		// On Enter or Escape: close the dialog
		a.pages.RemovePage("filter")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
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
	a.updateStatusBar()
	a.app.SetFocus(input)
}

func (a *App) promptTextFilter() {
	if a.focusIndex != 2 {
		a.setStatusNote("Focus Top Connections before search", 4*time.Second)
		return
	}

	input := tview.NewInputField()
	input.SetLabel("Search (contains): ")
	input.SetFieldWidth(36)
	input.SetText(a.textFilter)
	input.SetBorder(true)
	input.SetTitle(" Search Filter ")

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			entered := strings.TrimSpace(input.GetText())
			if entered == "" {
				a.portFilter = ""
				a.textFilter = ""
			} else {
				a.textFilter = entered
			}
			a.selectedTalkerIndex = 0
			a.refreshData()
		}
		a.pages.RemovePage("search")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 54, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("search", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(input)
}

func (a *App) applyTopConnectionFilters(conns []collector.Connection) []collector.Connection {
	filtered := conns
	if a.portFilter != "" {
		filtered = filterByPort(filtered, a.portFilter)
	}
	if a.textFilter != "" {
		filtered = filterByText(filtered, a.textFilter)
	}
	return filtered
}

type peerKillTarget struct {
	PeerIP    string
	LocalPort int
	Count     int
}

const (
	defaultBlockMinutes = 10
	maxBlockMinutes     = 1440
)

// promptKillPeer confirms and applies a temporary firewall block for a peer.
func (a *App) promptKillPeer() {
	if a.focusIndex != 2 {
		a.setStatusNote("Focus Top Connections before kill-peer", 5*time.Second)
		return
	}

	filteredPort := 0
	if a.portFilter != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(a.portFilter))
		if err != nil || parsed < 1 || parsed > 65535 {
			a.setStatusNote("Current port filter must be 1-65535", 5*time.Second)
			return
		}
		filteredPort = parsed
	}
	a.enterKillFlowPause()

	suggested, hasSuggested := a.selectedPeerKillTarget()
	if !hasSuggested {
		suggested, hasSuggested = a.selectPeerKillTarget()
	}
	defaultPeer := ""
	defaultPort := ""
	helpText := fmt.Sprintf("Enter peer IP + local port + block minutes (default %d).", defaultBlockMinutes)
	if hasSuggested {
		defaultPeer = suggested.PeerIP
		defaultPort = strconv.Itoa(suggested.LocalPort)
		helpText = fmt.Sprintf("Suggested: %s -> local port %d (%d matches in view).", suggested.PeerIP, suggested.LocalPort, suggested.Count)
	}
	if filteredPort > 0 {
		defaultPort = strconv.Itoa(filteredPort)
		if hasSuggested {
			helpText = fmt.Sprintf("Port filter active: local port %d. Suggested peer: %s (%d matches).", filteredPort, suggested.PeerIP, suggested.Count)
		} else {
			helpText = fmt.Sprintf("Port filter active: local port %d. Enter peer IP to block.", filteredPort)
		}
	}

	peerInput := tview.NewInputField().
		SetLabel("Peer IP: ").
		SetFieldWidth(30).
		SetText(defaultPeer)

	form := tview.NewForm().AddFormItem(peerInput)
	form.SetItemPadding(0)
	form.SetButtonsAlign(tview.AlignRight)

	var portInput *tview.InputField
	if filteredPort == 0 {
		portInput = tview.NewInputField().
			SetLabel("Local port: ").
			SetFieldWidth(8).
			SetText(defaultPort)
		portInput.SetAcceptanceFunc(tview.InputFieldInteger)
		form.AddFormItem(portInput)
	}

	minutesInput := tview.NewInputField().
		SetLabel("Block minutes: ").
		SetFieldWidth(6).
		SetText(strconv.Itoa(defaultBlockMinutes))
	minutesInput.SetAcceptanceFunc(tview.InputFieldInteger)
	form.AddFormItem(minutesInput)

	submit := func() {
		peerIP, ok := parsePeerIPInput(peerInput.GetText())
		if !ok {
			a.setStatusNote("Invalid peer IP", 5*time.Second)
			return
		}

		port := filteredPort
		if filteredPort == 0 {
			parsedPort, err := strconv.Atoi(strings.TrimSpace(portInput.GetText()))
			if err != nil || parsedPort < 1 || parsedPort > 65535 {
				a.setStatusNote("Invalid local port", 5*time.Second)
				return
			}
			port = parsedPort
		}

		minutes, err := strconv.Atoi(strings.TrimSpace(minutesInput.GetText()))
		if err != nil || minutes < 1 || minutes > maxBlockMinutes {
			a.setStatusNote(fmt.Sprintf("Block minutes must be 1-%d", maxBlockMinutes), 5*time.Second)
			return
		}
		duration := time.Duration(minutes) * time.Minute

		target := peerKillTarget{
			PeerIP:    peerIP,
			LocalPort: port,
			Count:     a.countPeerMatches(peerIP, port),
		}
		spec := actions.PeerBlockSpec{PeerIP: target.PeerIP, LocalPort: target.LocalPort}
		if a.hasActiveBlock(spec) {
			a.setStatusNote(fmt.Sprintf("Already blocked %s:%d", target.PeerIP, target.LocalPort), 5*time.Second)
			return
		}

		a.pages.RemovePage("kill-peer-form")
		a.updateStatusBar()
		a.promptKillPeerConfirm(target, duration)
	}

	form.AddButton("Next", func() {
		submit()
	})
	form.AddButton("Cancel", func() {
		a.pages.RemovePage("kill-peer-form")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.exitKillFlowPause()
		a.updateStatusBar()
	})
	form.SetCancelFunc(func() {
		a.pages.RemovePage("kill-peer-form")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.exitKillFlowPause()
		a.updateStatusBar()
	})
	inputCount := 2
	if filteredPort == 0 {
		inputCount = 3
	}
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			a.pages.RemovePage("kill-peer-form")
			a.app.SetFocus(a.panels[a.focusIndex])
			a.exitKillFlowPause()
			a.updateStatusBar()
			return nil
		case tcell.KeyTab:
			currentItem, _ := form.GetFocusedItemIndex()
			if currentItem < 0 || currentItem >= inputCount {
				currentItem = -1
			}
			next := currentItem + 1
			if next >= inputCount {
				next = 0
			}
			form.SetFocus(next)
			a.app.SetFocus(form)
			return nil
		case tcell.KeyEnter:
			submit()
			return nil
		}
		return event
	})
	form.SetBorder(true)
	form.SetTitle(" Kill Peer ")

	helpLine := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetText("  [dim]" + helpText + "[white]")

	modalHeight := 11
	if filteredPort == 0 {
		modalHeight = 12
	}

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(helpLine, 1, 0, false).
				AddItem(form, 0, 1, true),
				84, 0, true).
			AddItem(nil, 0, 1, false),
			modalHeight, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("kill-peer-form", modal, true, true)
	a.updateStatusBar()
	form.SetFocus(0)
	a.app.SetFocus(form)
}

func (a *App) promptKillPeerConfirm(target peerKillTarget, duration time.Duration) {
	label := "Block " + formatBlockDuration(duration)
	minutes := int(duration / time.Minute)
	text := fmt.Sprintf(
		"Block peer %s -> local port %d for %d minutes?\n\nMatches in current view: %d\nThis inserts a firewall block rule and attempts to terminate active flows.",
		target.PeerIP,
		target.LocalPort,
		minutes,
		target.Count,
	)
	if target.Count == 0 {
		text = fmt.Sprintf(
			"Block peer %s -> local port %d for %d minutes?\n\nMatches in current view: 0 (manual target)\nThis inserts a firewall block rule and attempts to terminate active flows.",
			target.PeerIP,
			target.LocalPort,
			minutes,
		)
	}

	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{label, "Cancel"}).
		SetDoneFunc(func(_ int, button string) {
			a.pages.RemovePage("kill-peer")
			a.app.SetFocus(a.panels[a.focusIndex])
			a.exitKillFlowPause()
			a.updateStatusBar()
			if button != label {
				return
			}

			a.setStatusNote(fmt.Sprintf("Blocking %s:%d...", target.PeerIP, target.LocalPort), 4*time.Second)
			go a.blockPeerForDuration(target, duration)
		})
	modal.SetTitle(" Kill Peer ")
	modal.SetBorder(true)

	a.pages.AddPage("kill-peer", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(modal)
}

func (a *App) promptBlockedPeers() {
	blocks := a.snapshotDisplayActiveBlocks()
	if len(blocks) == 0 {
		a.setStatusNote("No active blocked peers", 4*time.Second)
		return
	}

	selectedIndex := 0
	list := tview.NewList().
		ShowSecondaryText(true)
	list.SetBorder(false)
	list.SetMainTextColor(tcell.ColorWhite)
	list.SetSecondaryTextColor(tcell.ColorGreen)

	closeModal := func() {
		a.pages.RemovePage("blocked-peers")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}

	showRemoveResultPopup := func(message string, onClose func()) {
		shownAt := time.Now()
		closePopup := func() {
			a.pages.RemovePage("blocked-peers-remove-result")
			a.updateStatusBar()
			if onClose != nil {
				onClose()
			}
		}

		modal := tview.NewModal().
			SetText(message).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(_ int, _ string) {
				// Ignore the first Enter right after opening to avoid
				// accidentally closing immediately from the Remove button keypress.
				if time.Since(shownAt) < 200*time.Millisecond {
					return
				}
				closePopup()
			})
		modal.SetTitle(" Remove Block ")
		modal.SetBorder(true)
		modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if time.Since(shownAt) < 200*time.Millisecond {
				return nil
			}
			switch event.Key() {
			case tcell.KeyEsc:
				closePopup()
				return nil
			}
			if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
				closePopup()
				return nil
			}
			return event
		})

		a.pages.RemovePage("blocked-peers-remove-result")
		a.pages.AddPage("blocked-peers-remove-result", modal, true, true)
		a.pages.SendToFront("blocked-peers-remove-result")
		a.updateStatusBar()
		a.app.SetFocus(modal)
	}

	var refreshList func(nextIndex int)
	removeSelected := func() {
		if selectedIndex < 0 || selectedIndex >= len(blocks) {
			a.setStatusNote("No blocked peer selected", 4*time.Second)
			return
		}
		spec := blocks[selectedIndex].Spec
		if err := actions.UnblockPeer(spec); err != nil {
			a.setStatusNote("Unblock failed: "+shortStatus(err.Error(), 64), 8*time.Second)
			return
		}
		a.removeActiveBlock(spec)
		a.setStatusNote(fmt.Sprintf("Unblocked %s:%d", spec.PeerIP, spec.LocalPort), 6*time.Second)

		remaining := a.snapshotDisplayActiveBlocks()
		if len(remaining) == 0 {
			closeModal()
			showRemoveResultPopup(
				fmt.Sprintf("Removed block %s:%d", spec.PeerIP, spec.LocalPort),
				func() {
					a.app.SetFocus(a.panels[a.focusIndex])
				},
			)
			return
		}

		refreshList(selectedIndex)
		showRemoveResultPopup(
			fmt.Sprintf("Removed block %s:%d", spec.PeerIP, spec.LocalPort),
			func() {
				a.app.SetFocus(list)
			},
		)
	}

	list.SetChangedFunc(func(index int, _ string, _ string, _ rune) {
		selectedIndex = index
	})

	refreshList = func(nextIndex int) {
		blocks = a.snapshotDisplayActiveBlocks()
		if len(blocks) == 0 {
			closeModal()
			a.setStatusNote("No active blocked peers", 4*time.Second)
			a.refreshData()
			return
		}

		if nextIndex < 0 {
			nextIndex = 0
		}
		if nextIndex >= len(blocks) {
			nextIndex = len(blocks) - 1
		}

		list.Clear()
		for _, entry := range blocks {
			main := formatBlockedSpec(entry.Spec)
			secondary := formatBlockedListSecondary(entry)
			list.AddItem(main, secondary, 0, nil)
		}
		selectedIndex = nextIndex
		list.SetCurrentItem(nextIndex)
	}
	refreshList(0)

	form := tview.NewForm().
		AddButton("Remove", func() {
			removeSelected()
		}).
		AddButton("Close", func() {
			closeModal()
		})
	form.SetBorder(false)
	form.SetButtonsAlign(tview.AlignRight)
	form.SetItemPadding(1)
	form.SetCancelFunc(func() {
		closeModal()
	})

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeModal()
			return nil
		case tcell.KeyEnter:
			removeSelected()
			return nil
		case tcell.KeyTab:
			a.app.SetFocus(form)
			return nil
		case tcell.KeyDelete, tcell.KeyBackspace, tcell.KeyBackspace2:
			removeSelected()
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			closeModal()
			return nil
		}
		return event
	})
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeModal()
			return nil
		case tcell.KeyTab:
			a.app.SetFocus(list)
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			closeModal()
			return nil
		}
		return event
	})

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(list, 0, 1, true).
		AddItem(form, 3, 0, false)
	content.SetBorder(true)
	content.SetTitle(" Blocked Peers ")

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(content, 98, 0, true).
			AddItem(nil, 0, 1, false),
			16, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("blocked-peers", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(list)
}

// selectPeerKillTarget picks the most frequent peer->localPort tuple.
func (a *App) selectPeerKillTarget() (peerKillTarget, bool) {
	if len(a.latestTalkers) == 0 {
		return peerKillTarget{}, false
	}

	filtered := a.applyTopConnectionFilters(a.latestTalkers)
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

func (a *App) selectedPeerKillTarget() (peerKillTarget, bool) {
	visible := a.visibleTopConnections()
	if len(visible) == 0 {
		return peerKillTarget{}, false
	}
	a.clampTopConnectionSelection()

	conn := visible[a.selectedTalkerIndex]
	peerIP := normalizeIP(conn.RemoteIP)
	localPort := conn.LocalPort
	return peerKillTarget{
		PeerIP:    peerIP,
		LocalPort: localPort,
		Count:     a.countPeerMatches(peerIP, localPort),
	}, true
}

func (a *App) countPeerMatches(peerIP string, localPort int) int {
	if len(a.latestTalkers) == 0 {
		return 0
	}

	filtered := a.applyTopConnectionFilters(a.latestTalkers)

	count := 0
	for _, conn := range filtered {
		if normalizeIP(conn.RemoteIP) == peerIP && conn.LocalPort == localPort {
			count++
		}
	}
	return count
}

func (a *App) matchingBlockTuples(peerIP string, localPort int) []actions.SocketTuple {
	if len(a.latestTalkers) == 0 {
		return nil
	}

	normalizedPeer := normalizeIP(peerIP)
	seen := make(map[string]struct{})
	tuples := make([]actions.SocketTuple, 0, 8)

	for _, conn := range a.latestTalkers {
		if conn.LocalPort != localPort {
			continue
		}
		if !strings.EqualFold(conn.State, "ESTABLISHED") {
			continue
		}

		remoteIP := normalizeIP(conn.RemoteIP)
		if remoteIP != normalizedPeer {
			continue
		}

		localIP := normalizeIP(conn.LocalIP)
		if localIP == "" || conn.RemotePort < 1 || conn.RemotePort > 65535 {
			continue
		}

		key := fmt.Sprintf("%s|%d|%s|%d", localIP, conn.LocalPort, remoteIP, conn.RemotePort)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		tuples = append(tuples, actions.SocketTuple{
			LocalIP:    localIP,
			LocalPort:  conn.LocalPort,
			RemoteIP:   remoteIP,
			RemotePort: conn.RemotePort,
		})
	}

	return tuples
}

func (a *App) blockPeerForDuration(target peerKillTarget, duration time.Duration) {
	spec := actions.PeerBlockSpec{
		PeerIP:    target.PeerIP,
		LocalPort: target.LocalPort,
	}
	blockedAt := time.Now()
	expiresAt := blockedAt.Add(duration)

	if err := actions.BlockPeer(spec); err != nil {
		a.app.QueueUpdateDraw(func() {
			a.setStatusNote("Block failed: "+shortStatus(err.Error(), 64), 8*time.Second)
		})
		return
	}

	a.addActiveBlock(activeBlockEntry{
		Spec:      spec,
		StartedAt: blockedAt,
		ExpiresAt: expiresAt,
		Summary:   fmt.Sprintf("Blocked %s:%d | processing...", spec.PeerIP, spec.LocalPort),
	})

	tuples := a.matchingBlockTuples(target.PeerIP, target.LocalPort)
	beforeCount, beforeCountErr := actions.CountEstablishedPeerSockets(spec)
	socketErr := actions.QueryAndKillPeerSockets(spec)
	if socketErr != nil {
		// Fallback: try killing with cached tuples from the TUI snapshot.
		socketErr = actions.KillSockets(tuples)
	}
	flowErr := actions.DropPeerConnections(spec)
	afterCount, afterCountErr := actions.CountEstablishedPeerSockets(spec)

	dropWarningParts := make([]string, 0, 2)
	if socketErr != nil {
		dropWarningParts = append(dropWarningParts, "socket "+shortStatus(socketErr.Error(), 28))
	}
	if flowErr != nil {
		dropWarningParts = append(dropWarningParts, "flow "+shortStatus(flowErr.Error(), 28))
	}
	dropWarning := ""
	if len(dropWarningParts) > 0 {
		dropWarning = " (drop partial: " + strings.Join(dropWarningParts, "; ") + ")"
	}
	actionSummary := buildBlockActionSummary(spec, duration, beforeCount, beforeCountErr, afterCount, afterCountErr, socketErr, flowErr)
	a.updateActiveBlockSummary(spec, actionSummary)

	a.app.QueueUpdateDraw(func() {
		a.setStatusNote(fmt.Sprintf("Blocked %s:%d for %s%s", target.PeerIP, target.LocalPort, formatBlockDuration(duration), dropWarning), 8*time.Second)
		a.showBlockSummaryPopup(actionSummary)
		a.refreshData()
	})

	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-a.stopChan:
		return
	}
	if !a.hasActiveBlock(spec) {
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

func buildBlockActionSummary(
	spec actions.PeerBlockSpec,
	duration time.Duration,
	beforeCount int,
	beforeErr error,
	afterCount int,
	afterErr error,
	socketErr error,
	flowErr error,
) string {
	killPart := "killed ?/? flows"
	if beforeErr == nil && afterErr == nil {
		if beforeCount < 0 {
			beforeCount = 0
		}
		if afterCount < 0 {
			afterCount = 0
		}
		if afterCount > beforeCount {
			afterCount = beforeCount
		}
		killPart = fmt.Sprintf("killed %d/%d flows", beforeCount-afterCount, beforeCount)
	} else if beforeErr == nil {
		killPart = fmt.Sprintf("killed ?/%d flows", beforeCount)
	}

	dropPart := "drop ok"
	if socketErr != nil && flowErr != nil {
		dropPart = "drop partial"
	}

	return fmt.Sprintf(
		"Blocked %s:%d | %s | %s | expires in %s",
		spec.PeerIP,
		spec.LocalPort,
		killPart,
		dropPart,
		formatRemainingDuration(duration),
	)
}

func formatActiveBlockDetail(entry activeBlockEntry) string {
	summary := strings.TrimSpace(entry.Summary)
	if summary == "" {
		summary = fmt.Sprintf("Blocked %s:%d", entry.Spec.PeerIP, entry.Spec.LocalPort)
	}
	expiresText := formatRemainingDuration(time.Until(entry.ExpiresAt))
	if entry.ExpiresAt.IsZero() {
		expiresText = "n/a (unmanaged)"
	}
	return fmt.Sprintf("[dim]Summary:[white] %s\n[dim]Expires in:[white] %s", summary, expiresText)
}

func (a *App) showBlockSummaryPopup(summary string) {
	modal := tview.NewModal().
		SetText(summary).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(_ int, _ string) {
			a.pages.RemovePage("block-summary")
			a.updateStatusBar()
			a.app.SetFocus(a.panels[a.focusIndex])
		})
	modal.SetTitle(" Block Summary ")
	modal.SetBorder(true)

	a.pages.RemovePage("block-summary")
	a.pages.AddPage("block-summary", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(modal)
}

func parsePeerIPInput(raw string) (string, bool) {
	peerIP := strings.TrimSpace(raw)
	peerIP = strings.TrimPrefix(peerIP, "[")
	peerIP = strings.TrimSuffix(peerIP, "]")
	peerIP = strings.TrimPrefix(peerIP, "::ffff:")
	parsed := net.ParseIP(peerIP)
	if parsed == nil {
		return "", false
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String(), true
	}
	return parsed.String(), true
}

func formatBlockedSpec(spec actions.PeerBlockSpec) string {
	return fmt.Sprintf("%s -> :%d", spec.PeerIP, spec.LocalPort)
}

func formatBlockedListSecondary(entry activeBlockEntry) string {
	secondary := "drop unknown | expires in n/a"
	summary := strings.TrimSpace(entry.Summary)
	if summary != "" {
		parts := strings.Split(summary, " | ")
		if len(parts) > 1 {
			secondary = strings.Join(parts[1:], " | ")
		}
	}

	expires := "n/a"
	if !entry.ExpiresAt.IsZero() {
		expires = formatRemainingDuration(time.Until(entry.ExpiresAt))
	}
	if strings.Contains(secondary, "expires in ") {
		idx := strings.LastIndex(secondary, "expires in ")
		secondary = secondary[:idx] + "expires in " + expires
	} else {
		secondary = secondary + " | expires in " + expires
	}
	return secondary
}

func blockKey(spec actions.PeerBlockSpec) string {
	return fmt.Sprintf("%s|%d", spec.PeerIP, spec.LocalPort)
}

func (a *App) addActiveBlock(entry activeBlockEntry) {
	a.blockMu.Lock()
	defer a.blockMu.Unlock()
	a.activeBlocks[blockKey(entry.Spec)] = entry
}

func (a *App) updateActiveBlockSummary(spec actions.PeerBlockSpec, summary string) {
	a.blockMu.Lock()
	defer a.blockMu.Unlock()

	key := blockKey(spec)
	entry, exists := a.activeBlocks[key]
	if !exists {
		return
	}
	entry.Summary = summary
	a.activeBlocks[key] = entry
}

func (a *App) removeActiveBlock(spec actions.PeerBlockSpec) {
	a.blockMu.Lock()
	defer a.blockMu.Unlock()
	delete(a.activeBlocks, blockKey(spec))
}

func (a *App) cleanupActiveBlocks() {
	a.blockMu.Lock()
	pending := make([]activeBlockEntry, 0, len(a.activeBlocks))
	for _, entry := range a.activeBlocks {
		pending = append(pending, entry)
	}
	a.blockMu.Unlock()

	for _, entry := range pending {
		_ = actions.UnblockPeer(entry.Spec)
		a.removeActiveBlock(entry.Spec)
	}
}

func (a *App) hasActiveBlock(spec actions.PeerBlockSpec) bool {
	a.blockMu.Lock()
	defer a.blockMu.Unlock()
	_, exists := a.activeBlocks[blockKey(spec)]
	return exists
}

func (a *App) snapshotActiveBlocks() []activeBlockEntry {
	a.blockMu.Lock()
	defer a.blockMu.Unlock()

	items := make([]activeBlockEntry, 0, len(a.activeBlocks))
	for _, entry := range a.activeBlocks {
		items = append(items, entry)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Spec.PeerIP != items[j].Spec.PeerIP {
			return items[i].Spec.PeerIP < items[j].Spec.PeerIP
		}
		return items[i].Spec.LocalPort < items[j].Spec.LocalPort
	})
	return items
}

func (a *App) snapshotDisplayActiveBlocks() []activeBlockEntry {
	items := a.snapshotActiveBlocks()
	byKey := make(map[string]activeBlockEntry, len(items))
	for _, entry := range items {
		byKey[blockKey(entry.Spec)] = entry
	}

	firewallBlocks, err := actions.ListBlockedPeers()
	if err == nil {
		for _, spec := range firewallBlocks {
			key := blockKey(spec)
			if _, exists := byKey[key]; exists {
				continue
			}
			byKey[key] = activeBlockEntry{
				Spec:    spec,
				Summary: fmt.Sprintf("Detected firewall block %s:%d", spec.PeerIP, spec.LocalPort),
			}
		}
	}

	merged := make([]activeBlockEntry, 0, len(byKey))
	for _, entry := range byKey {
		merged = append(merged, entry)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Spec.PeerIP != merged[j].Spec.PeerIP {
			return merged[i].Spec.PeerIP < merged[j].Spec.PeerIP
		}
		return merged[i].Spec.LocalPort < merged[j].Spec.LocalPort
	})
	return merged
}

func (a *App) enterKillFlowPause() {
	if a.paused {
		return
	}
	a.paused = true
	a.killFlowAutoPaused = true
	a.updateStatusBar()
}

func (a *App) exitKillFlowPause() {
	if !a.killFlowAutoPaused {
		return
	}
	a.killFlowAutoPaused = false
	a.paused = false
	a.updateStatusBar()
}
