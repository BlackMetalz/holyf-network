package tui

import (
	"fmt"
	"sort"

	"github.com/BlackMetalz/holyf-network/internal/actions"
)

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
	if a.paused.Load() {
		return
	}
	a.paused.Store(true)
	a.killFlowAutoPaused = true
	a.updateStatusBar()
}

func (a *App) exitKillFlowPause() {
	if !a.killFlowAutoPaused {
		return
	}
	a.killFlowAutoPaused = false
	a.paused.Store(false)
	a.updateStatusBar()
}
