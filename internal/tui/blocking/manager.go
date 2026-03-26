package blocking

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/actions"
)

type ActiveBlockEntry struct {
	Spec      actions.PeerBlockSpec
	StartedAt time.Time
	ExpiresAt time.Time
	Summary   string
}

type Manager struct {
	activeBlocks       map[string]ActiveBlockEntry
	blockMu            sync.Mutex
	killFlowAutoPaused bool
}

func NewManager() *Manager {
	return &Manager{
		activeBlocks: make(map[string]ActiveBlockEntry),
	}
}

func BlockKey(spec actions.PeerBlockSpec) string {
	return fmt.Sprintf("%s|%d", spec.PeerIP, spec.LocalPort)
}

func (m *Manager) AddActiveBlock(entry ActiveBlockEntry) {
	m.blockMu.Lock()
	defer m.blockMu.Unlock()
	m.activeBlocks[BlockKey(entry.Spec)] = entry
}

func (m *Manager) UpdateActiveBlockSummary(spec actions.PeerBlockSpec, summary string) {
	m.blockMu.Lock()
	defer m.blockMu.Unlock()

	key := BlockKey(spec)
	entry, exists := m.activeBlocks[key]
	if !exists {
		return
	}
	entry.Summary = summary
	m.activeBlocks[key] = entry
}

func (m *Manager) RemoveActiveBlock(spec actions.PeerBlockSpec) {
	m.blockMu.Lock()
	defer m.blockMu.Unlock()
	delete(m.activeBlocks, BlockKey(spec))
}

func (m *Manager) CleanupActiveBlocks() {
	m.blockMu.Lock()
	pending := make([]ActiveBlockEntry, 0, len(m.activeBlocks))
	for _, entry := range m.activeBlocks {
		pending = append(pending, entry)
	}
	m.blockMu.Unlock()

	for _, entry := range pending {
		_ = actions.UnblockPeer(entry.Spec)
		m.RemoveActiveBlock(entry.Spec)
	}
}

func (m *Manager) HasActiveBlock(spec actions.PeerBlockSpec) bool {
	m.blockMu.Lock()
	defer m.blockMu.Unlock()
	_, exists := m.activeBlocks[BlockKey(spec)]
	return exists
}

func (m *Manager) SnapshotActiveBlocks() []ActiveBlockEntry {
	m.blockMu.Lock()
	defer m.blockMu.Unlock()

	items := make([]ActiveBlockEntry, 0, len(m.activeBlocks))
	for _, entry := range m.activeBlocks {
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

func (m *Manager) SnapshotDisplayActiveBlocks() []ActiveBlockEntry {
	items := m.SnapshotActiveBlocks()
	byKey := make(map[string]ActiveBlockEntry, len(items))
	for _, entry := range items {
		byKey[BlockKey(entry.Spec)] = entry
	}

	firewallBlocks, err := actions.ListBlockedPeers()
	if err == nil {
		for _, spec := range firewallBlocks {
			key := BlockKey(spec)
			if _, exists := byKey[key]; exists {
				continue
			}
			byKey[key] = ActiveBlockEntry{
				Spec:    spec,
				Summary: fmt.Sprintf("Detected firewall block %s:%d", spec.PeerIP, spec.LocalPort),
			}
		}
	}

	merged := make([]ActiveBlockEntry, 0, len(byKey))
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

func (m *Manager) EnterKillFlowPause(ctx UIContext) {
	if ctx.IsPaused() {
		return
	}
	ctx.SetPaused(true)
	m.killFlowAutoPaused = true
	ctx.UpdateStatusBar()
}

func (m *Manager) ExitKillFlowPause(ctx UIContext) {
	if !m.killFlowAutoPaused {
		return
	}
	m.killFlowAutoPaused = false
	ctx.SetPaused(false)
	ctx.UpdateStatusBar()
}
