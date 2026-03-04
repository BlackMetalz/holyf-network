package tui

import (
	"fmt"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

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
