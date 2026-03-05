package collector

import (
	"sync"
	"time"
)

type socketCounterPair struct {
	acked int64
	recv  int64
}

// SocketBandwidthTracker computes interval deltas/rates from ss TCP counters.
type SocketBandwidthTracker struct {
	mu     sync.Mutex
	prev   map[FlowTuple]socketCounterPair
	prevAt time.Time
}

func NewSocketBandwidthTracker() *SocketBandwidthTracker {
	return &SocketBandwidthTracker{
		prev: make(map[FlowTuple]socketCounterPair),
	}
}

func (t *SocketBandwidthTracker) BuildSnapshot(counters []SocketCounter, capturedAt time.Time) BandwidthSnapshot {
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

	nextPrev := make(map[FlowTuple]socketCounterPair, len(counters))
	rows := make(map[FlowTuple]TupleBandwidth, len(counters))

	for _, c := range counters {
		tuple := normalizeFlowTuple(c.Tuple)
		txDelta := int64(0)
		rxDelta := int64(0)
		if havePrev {
			if prev, ok := t.prev[tuple]; ok {
				txDelta = clampDelta(c.BytesAcked - prev.acked)
				rxDelta = clampDelta(c.BytesReceived - prev.recv)
			} else {
				// First-seen tuple after baseline.
				txDelta = clampDelta(c.BytesAcked)
				rxDelta = clampDelta(c.BytesReceived)
			}
		}
		nextPrev[tuple] = socketCounterPair{
			acked: c.BytesAcked,
			recv:  c.BytesReceived,
		}
		addTupleBandwidth(rows, tuple, txDelta, rxDelta, sampleSec)
	}

	t.prev = nextPrev
	t.prevAt = capturedAt

	return BandwidthSnapshot{
		ByTuple:       rows,
		SampleSeconds: sampleSec,
		Available:     havePrev && sampleSec > 0,
	}
}
