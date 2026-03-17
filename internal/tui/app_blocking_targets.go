package tui

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/actions"
	"github.com/BlackMetalz/holyf-network/internal/collector"
)

// selectPeerKillTarget picks the most frequent peer->localPort tuple.
func (a *App) selectPeerKillTarget() (peerKillTarget, bool) {
	source := a.topConnectionsSource()
	if len(source) == 0 {
		return peerKillTarget{}, false
	}

	filtered := a.applyTopConnectionFilters(source)
	if a.groupView {
		filtered = applyGroupConnectionFilters(source, a.portFilter, a.textFilter)
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

func (a *App) selectedPeerKillTarget() (peerKillTarget, bool) {
	if a.groupView {
		groups := a.visiblePeerGroups()
		if len(groups) == 0 {
			return peerKillTarget{}, false
		}
		a.clampTopConnectionSelection()

		peerIP := groups[a.selectedTalkerIndex].PeerIP
		return a.selectedPeerPortTarget(peerIP)
	}

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

func (a *App) selectedPeerPortTarget(peerIP string) (peerKillTarget, bool) {
	source := a.topConnectionsSource()
	if len(source) == 0 {
		return peerKillTarget{}, false
	}

	filtered := a.applyTopConnectionFilters(source)
	if a.groupView {
		filtered = applyGroupConnectionFilters(source, a.portFilter, a.textFilter)
	}
	if len(filtered) == 0 {
		return peerKillTarget{}, false
	}

	type portAggregate struct {
		count    int
		activity int64
	}
	byPort := make(map[int]portAggregate)
	for _, conn := range filtered {
		if normalizeIP(conn.RemoteIP) != peerIP {
			continue
		}
		current := byPort[conn.LocalPort]
		current.count++
		current.activity += conn.Activity
		byPort[conn.LocalPort] = current
	}
	if len(byPort) == 0 {
		return peerKillTarget{}, false
	}

	bestPort := 0
	best := portAggregate{}
	first := true
	for port, agg := range byPort {
		if first {
			bestPort = port
			best = agg
			first = false
			continue
		}
		if agg.count > best.count ||
			(agg.count == best.count && agg.activity > best.activity) ||
			(agg.count == best.count && agg.activity == best.activity && port < bestPort) {
			bestPort = port
			best = agg
		}
	}

	return peerKillTarget{
		PeerIP:    peerIP,
		LocalPort: bestPort,
		Count:     best.count,
	}, true
}

func (a *App) countPeerMatches(peerIP string, localPort int) int {
	source := a.topConnectionsSource()
	if len(source) == 0 {
		return 0
	}

	filtered := a.applyTopConnectionFilters(source)

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

// matchingBlockTuplesFromSnapshot is like matchingBlockTuples but operates on
// a pre-captured snapshot of connections. Safe to call from any goroutine.
func matchingBlockTuplesFromSnapshot(conns []collector.Connection, peerIP string, localPort int) []actions.SocketTuple {
	if len(conns) == 0 {
		return nil
	}

	normalizedPeer := normalizeIP(peerIP)
	seen := make(map[string]struct{})
	tuples := make([]actions.SocketTuple, 0, 8)

	for _, conn := range conns {
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
