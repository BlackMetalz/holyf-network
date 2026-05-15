package shared

import "time"

// RingSample is one timestamped data point.
type RingSample struct {
	Timestamp time.Time
	Value     float64
}

// RingBuffer is a fixed-size circular buffer for time-series samples.
type RingBuffer struct {
	samples []RingSample
	size    int
	head    int // next write position
	count   int
}

// NewRingBuffer creates a ring buffer that holds up to size samples.
func NewRingBuffer(size int) *RingBuffer {
	if size < 1 {
		size = 60
	}
	return &RingBuffer{
		samples: make([]RingSample, size),
		size:    size,
	}
}

// Push adds a new sample, overwriting the oldest if full.
func (r *RingBuffer) Push(ts time.Time, value float64) {
	r.samples[r.head] = RingSample{Timestamp: ts, Value: value}
	r.head = (r.head + 1) % r.size
	if r.count < r.size {
		r.count++
	}
}

// Samples returns all stored samples in oldest-to-newest order.
func (r *RingBuffer) Samples() []RingSample {
	if r.count == 0 {
		return nil
	}
	out := make([]RingSample, r.count)
	start := (r.head - r.count + r.size) % r.size
	for i := 0; i < r.count; i++ {
		out[i] = r.samples[(start+i)%r.size]
	}
	return out
}

// Values returns just the float64 values in oldest-to-newest order.
func (r *RingBuffer) Values() []float64 {
	if r.count == 0 {
		return nil
	}
	out := make([]float64, r.count)
	start := (r.head - r.count + r.size) % r.size
	for i := 0; i < r.count; i++ {
		out[i] = r.samples[(start+i)%r.size].Value
	}
	return out
}

// Last returns the most recent sample value and true, or 0 and false if empty.
func (r *RingBuffer) Last() (float64, bool) {
	if r.count == 0 {
		return 0, false
	}
	idx := (r.head - 1 + r.size) % r.size
	return r.samples[idx].Value, true
}

// Count returns the number of stored samples.
func (r *RingBuffer) Count() int {
	return r.count
}
