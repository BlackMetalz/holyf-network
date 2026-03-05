package collector

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type flowCounters struct {
	origBytes  int64
	replyBytes int64
}

// TupleBandwidth is traffic delta/rate from local tuple perspective.
type TupleBandwidth struct {
	TxBytesDelta     int64
	RxBytesDelta     int64
	TotalBytesDelta  int64
	TxBytesPerSec    float64
	RxBytesPerSec    float64
	TotalBytesPerSec float64
}

// BandwidthSnapshot is one calculated bandwidth sample.
type BandwidthSnapshot struct {
	ByTuple       map[FlowTuple]TupleBandwidth
	SampleSeconds float64
	Available     bool
}

// BandwidthTracker tracks conntrack counters and computes interval deltas.
type BandwidthTracker struct {
	mu     sync.Mutex
	prev   map[string]flowCounters
	prevAt time.Time
}

func NewBandwidthTracker() *BandwidthTracker {
	return &BandwidthTracker{
		prev: make(map[string]flowCounters),
	}
}

// BuildSnapshot computes tuple deltas/rates from current conntrack flows.
func (t *BandwidthTracker) BuildSnapshot(flows []ConntrackFlow, capturedAt time.Time) BandwidthSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	if capturedAt.IsZero() {
		capturedAt = time.Now()
	}

	sampleSec := 0.0
	havePrev := !t.prevAt.IsZero()
	if havePrev {
		sampleSec = capturedAt.Sub(t.prevAt).Seconds()
		if sampleSec <= 0 {
			sampleSec = 0
		}
	}

	rows := make(map[FlowTuple]TupleBandwidth, len(flows)*2)
	nextPrev := make(map[string]flowCounters, len(flows))

	for _, flow := range flows {
		orig := normalizeFlowTuple(flow.Orig)
		reply := normalizeFlowTuple(flow.Reply)
		flowID := makeFlowID(orig, reply)

		origDelta := int64(0)
		replyDelta := int64(0)
		if havePrev {
			if prev, ok := t.prev[flowID]; ok {
				origDelta = clampDelta(flow.OrigBytes - prev.origBytes)
				replyDelta = clampDelta(flow.ReplyBytes - prev.replyBytes)
			}
		}

		nextPrev[flowID] = flowCounters{
			origBytes:  flow.OrigBytes,
			replyBytes: flow.ReplyBytes,
		}

		addTupleBandwidth(rows, orig, origDelta, replyDelta, sampleSec)
		addTupleBandwidth(rows, reply, replyDelta, origDelta, sampleSec)
	}

	t.prev = nextPrev
	t.prevAt = capturedAt

	return BandwidthSnapshot{
		ByTuple:       rows,
		SampleSeconds: sampleSec,
		Available:     havePrev && sampleSec > 0,
	}
}

// EnrichConnectionsWithBandwidth attaches bandwidth deltas/rates to connections.
func EnrichConnectionsWithBandwidth(conns []Connection, snapshot BandwidthSnapshot) []Connection {
	if len(conns) == 0 {
		return conns
	}

	for i := range conns {
		conns[i].TxBytesDelta = 0
		conns[i].RxBytesDelta = 0
		conns[i].TotalBytesDelta = 0
		conns[i].TxBytesPerSec = 0
		conns[i].RxBytesPerSec = 0
		conns[i].TotalBytesPerSec = 0

		if len(snapshot.ByTuple) == 0 {
			continue
		}
		key := normalizeFlowTuple(FlowTuple{
			SrcIP:   conns[i].LocalIP,
			SrcPort: conns[i].LocalPort,
			DstIP:   conns[i].RemoteIP,
			DstPort: conns[i].RemotePort,
		})
		bw, ok := snapshot.ByTuple[key]
		if !ok {
			continue
		}
		conns[i].TxBytesDelta = bw.TxBytesDelta
		conns[i].RxBytesDelta = bw.RxBytesDelta
		conns[i].TotalBytesDelta = bw.TotalBytesDelta
		conns[i].TxBytesPerSec = bw.TxBytesPerSec
		conns[i].RxBytesPerSec = bw.RxBytesPerSec
		conns[i].TotalBytesPerSec = bw.TotalBytesPerSec
	}
	return conns
}

func addTupleBandwidth(rows map[FlowTuple]TupleBandwidth, tuple FlowTuple, txDelta, rxDelta int64, sampleSec float64) {
	if tuple.SrcPort == 0 || tuple.DstPort == 0 {
		return
	}

	current := rows[tuple]
	current.TxBytesDelta += txDelta
	current.RxBytesDelta += rxDelta
	current.TotalBytesDelta += txDelta + rxDelta

	if sampleSec > 0 {
		current.TxBytesPerSec = float64(current.TxBytesDelta) / sampleSec
		current.RxBytesPerSec = float64(current.RxBytesDelta) / sampleSec
		current.TotalBytesPerSec = float64(current.TotalBytesDelta) / sampleSec
	}

	rows[tuple] = current
}

func clampDelta(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func normalizeFlowTuple(t FlowTuple) FlowTuple {
	return FlowTuple{
		SrcIP:   normalizeFlowIP(t.SrcIP),
		SrcPort: t.SrcPort,
		DstIP:   normalizeFlowIP(t.DstIP),
		DstPort: t.DstPort,
	}
}

func normalizeFlowIP(ip string) string {
	return strings.TrimPrefix(strings.TrimSpace(ip), "::ffff:")
}

func makeFlowID(orig, reply FlowTuple) string {
	return fmt.Sprintf(
		"%s:%d>%s:%d|%s:%d>%s:%d",
		orig.SrcIP, orig.SrcPort, orig.DstIP, orig.DstPort,
		reply.SrcIP, reply.SrcPort, reply.DstIP, reply.DstPort,
	)
}
