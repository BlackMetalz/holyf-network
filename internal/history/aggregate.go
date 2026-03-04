package history

import (
	"sort"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

type aggregateKey struct {
	peerIP    string
	localPort int
	procName  string
}

// AggregateConnections groups raw connections by peer_ip + local_port + proc_name,
// sorts deterministically, and applies an optional row cap.
func AggregateConnections(conns []collector.Connection, limit int) []SnapshotGroup {
	if len(conns) == 0 {
		return nil
	}

	byKey := make(map[aggregateKey]*SnapshotGroup)
	for _, conn := range conns {
		peerIP := normalizePeerIP(conn.RemoteIP)
		procName := strings.TrimSpace(conn.ProcName)
		if procName == "" {
			procName = "unknown"
		}
		key := aggregateKey{
			peerIP:    peerIP,
			localPort: conn.LocalPort,
			procName:  procName,
		}

		group, ok := byKey[key]
		if !ok {
			group = &SnapshotGroup{
				PeerIP:    peerIP,
				LocalPort: conn.LocalPort,
				ProcName:  procName,
				States:    make(map[string]int),
			}
			byKey[key] = group
		}

		group.ConnCount++
		group.TxQueue += conn.TxQueue
		group.RxQueue += conn.RxQueue
		group.TotalQueue += conn.Activity
		state := strings.TrimSpace(conn.State)
		if state == "" {
			state = "UNKNOWN"
		}
		group.States[state]++
	}

	rows := make([]SnapshotGroup, 0, len(byKey))
	for _, group := range byKey {
		rows = append(rows, *group)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ConnCount != rows[j].ConnCount {
			return rows[i].ConnCount > rows[j].ConnCount
		}
		if rows[i].TotalQueue != rows[j].TotalQueue {
			return rows[i].TotalQueue > rows[j].TotalQueue
		}
		if rows[i].PeerIP != rows[j].PeerIP {
			return rows[i].PeerIP < rows[j].PeerIP
		}
		if rows[i].LocalPort != rows[j].LocalPort {
			return rows[i].LocalPort < rows[j].LocalPort
		}
		return rows[i].ProcName < rows[j].ProcName
	})

	if limit > 0 && len(rows) > limit {
		return rows[:limit]
	}
	return rows
}

func normalizePeerIP(ip string) string {
	return strings.TrimPrefix(strings.TrimSpace(ip), "::ffff:")
}
