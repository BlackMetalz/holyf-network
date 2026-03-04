package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/actions"
	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/rivo/tview"
)

// blockPeerForDuration runs in a background goroutine. snapshotTalkers is a
// copy of latestTalkers captured on the UI goroutine to avoid a data race.
func (a *App) blockPeerForDuration(target peerKillTarget, duration time.Duration, snapshotTalkers []collector.Connection) {
	spec := actions.PeerBlockSpec{
		PeerIP:    target.PeerIP,
		LocalPort: target.LocalPort,
	}
	blockedAt := time.Now()
	expiresAt := blockedAt.Add(duration)

	if err := actions.BlockPeer(spec); err != nil {
		a.addActionLog(fmt.Sprintf("block %s:%d failed: %s", spec.PeerIP, spec.LocalPort, shortStatus(err.Error(), 60)))
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

	tuples := matchingBlockTuplesFromSnapshot(snapshotTalkers, target.PeerIP, target.LocalPort)
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
	a.addActionLog(actionSummary)

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
		a.addActionLog(fmt.Sprintf("auto-unblock %s:%d failed: %s", spec.PeerIP, spec.LocalPort, shortStatus(err.Error(), 60)))
		a.app.QueueUpdateDraw(func() {
			a.setStatusNote("Auto-unblock failed: "+shortStatus(err.Error(), 64), 8*time.Second)
		})
		return
	}
	a.removeActiveBlock(spec)
	a.addActionLog(fmt.Sprintf("auto-unblocked %s:%d (expired)", spec.PeerIP, spec.LocalPort))

	a.app.QueueUpdateDraw(func() {
		a.setStatusNote(fmt.Sprintf("Unblocked %s:%d", target.PeerIP, target.LocalPort), 6*time.Second)
		a.refreshData()
	})
}

func (a *App) killPeerConnectionsOnly(target peerKillTarget, snapshotTalkers []collector.Connection) {
	spec := actions.PeerBlockSpec{
		PeerIP:    target.PeerIP,
		LocalPort: target.LocalPort,
	}

	tuples := matchingBlockTuplesFromSnapshot(snapshotTalkers, target.PeerIP, target.LocalPort)
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
	actionSummary := buildKillOnlyActionSummary(spec, beforeCount, beforeCountErr, afterCount, afterCountErr, socketErr, flowErr)
	a.addActionLog(actionSummary)

	a.app.QueueUpdateDraw(func() {
		a.setStatusNote(fmt.Sprintf("Killed connections for %s:%d%s", target.PeerIP, target.LocalPort, dropWarning), 8*time.Second)
		a.showBlockSummaryPopup(actionSummary)
		a.refreshData()
	})
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
	killPart := buildKillPart(beforeCount, beforeErr, afterCount, afterErr)

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

func buildKillOnlyActionSummary(
	spec actions.PeerBlockSpec,
	beforeCount int,
	beforeErr error,
	afterCount int,
	afterErr error,
	socketErr error,
	flowErr error,
) string {
	killPart := buildKillPart(beforeCount, beforeErr, afterCount, afterErr)

	dropPart := "drop ok"
	if socketErr != nil && flowErr != nil {
		dropPart = "drop partial"
	}

	return fmt.Sprintf(
		"Killed connections for %s:%d | %s | %s",
		spec.PeerIP,
		spec.LocalPort,
		killPart,
		dropPart,
	)
}

func buildKillPart(beforeCount int, beforeErr error, afterCount int, afterErr error) string {
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
	return killPart
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
