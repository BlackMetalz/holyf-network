package history

import (
	"sort"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

type AggregateDirection int

const (
	AggregateIncoming AggregateDirection = iota
	AggregateOutgoing
)

type aggregateKey struct {
	peerIP   string
	port     int
	procName string
}

// AggregateConnections groups raw connections by peer_ip + local_port + proc_name,
// sorts deterministically, and applies an optional row cap.
func AggregateConnections(conns []collector.Connection, limit int) []SnapshotGroup {
	return AggregateConnectionsByDirection(conns, AggregateIncoming, limit)
}

// AggregateConnectionsByDirection groups raw connections by peer_ip + service_port + proc_name,
// where service_port means local service port for incoming and remote service port for outgoing.
func AggregateConnectionsByDirection(conns []collector.Connection, direction AggregateDirection, limit int) []SnapshotGroup {
	if len(conns) == 0 {
		return nil
	}

	byKey := make(map[aggregateKey]*SnapshotGroup)
	for _, conn := range conns {
		peerIP := normalizePeerIP(conn.RemoteIP)
		procName := strings.TrimSpace(conn.ProcName)
		if procName == "" {
			procName = "-"
		}
		servicePort := aggregateServicePort(conn, direction)
		key := aggregateKey{
			peerIP:   peerIP,
			port:     servicePort,
			procName: procName,
		}

		group, ok := byKey[key]
		if !ok {
			group = &SnapshotGroup{
				PeerIP:    peerIP,
				Port:      servicePort,
				LocalPort: servicePort,
				ProcName:  procName,
				States:    make(map[string]int),
			}
			byKey[key] = group
		}

		group.ConnCount++
		group.TxQueue += conn.TxQueue
		group.RxQueue += conn.RxQueue
		group.TotalQueue += conn.Activity
		group.TxBytesDelta += conn.TxBytesDelta
		group.RxBytesDelta += conn.RxBytesDelta
		group.TotalBytesDelta += conn.TotalBytesDelta
		group.TxBytesPerSec += conn.TxBytesPerSec
		group.RxBytesPerSec += conn.RxBytesPerSec
		group.TotalBytesPerSec += conn.TotalBytesPerSec
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
		if rows[i].TotalBytesDelta != rows[j].TotalBytesDelta {
			return rows[i].TotalBytesDelta > rows[j].TotalBytesDelta
		}
		if rows[i].ConnCount != rows[j].ConnCount {
			return rows[i].ConnCount > rows[j].ConnCount
		}
		if rows[i].TotalQueue != rows[j].TotalQueue {
			return rows[i].TotalQueue > rows[j].TotalQueue
		}
		if rows[i].PeerIP != rows[j].PeerIP {
			return rows[i].PeerIP < rows[j].PeerIP
		}
		if rows[i].Port != rows[j].Port {
			return rows[i].Port < rows[j].Port
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

func aggregateServicePort(conn collector.Connection, direction AggregateDirection) int {
	if direction == AggregateOutgoing {
		return conn.RemotePort
	}
	return conn.LocalPort
}

func SplitConnectionsByDirection(conns []collector.Connection, listenPorts map[int]struct{}, selfPID int) ([]collector.Connection, []collector.Connection) {
	if len(conns) == 0 {
		return nil, nil
	}

	incoming := make([]collector.Connection, 0, len(conns))
	outgoing := make([]collector.Connection, 0, len(conns))
	for _, conn := range conns {
		if selfPID > 0 && conn.PID == selfPID {
			continue
		}
		if _, ok := listenPorts[conn.LocalPort]; ok {
			incoming = append(incoming, conn)
			continue
		}
		outgoing = append(outgoing, conn)
	}
	return incoming, outgoing
}
