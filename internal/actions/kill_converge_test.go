package actions

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunKillPeerFlowsConvergesToZero(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	seq := []countSample{
		{target: 5, timeWait: 0},
		{target: 3, timeWait: 1},
		{target: 1, timeWait: 2},
		{target: 0, timeWait: 4},
	}
	hooks := fakeKillHooks(t, &now, seq)

	report := runKillPeerFlows(nil, KillConvergeOptions{
		MaxDuration:       4 * time.Second,
		MaxIterations:     12,
		SleepBetweenIters: 120 * time.Millisecond,
	}, hooks)

	if !report.Converged {
		t.Fatalf("expected converge=true, got false report=%+v", report)
	}
	if report.AfterActiveCount != 0 {
		t.Fatalf("expected after active count=0, got %d", report.AfterActiveCount)
	}
	if report.Iterations != 3 {
		t.Fatalf("expected 3 iterations, got %d", report.Iterations)
	}
	if report.TimedOut {
		t.Fatalf("expected timeout=false, got true")
	}
}

func TestRunKillPeerFlowsNoProgressTimesOutPartial(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	seq := []countSample{
		{target: 10, timeWait: 0},
		{target: 10, timeWait: 1},
		{target: 10, timeWait: 2},
		{target: 10, timeWait: 2},
		{target: 10, timeWait: 2},
	}
	hooks := fakeKillHooks(t, &now, seq)

	report := runKillPeerFlows(nil, KillConvergeOptions{
		MaxDuration:       250 * time.Millisecond,
		MaxIterations:     12,
		SleepBetweenIters: 120 * time.Millisecond,
	}, hooks)

	if report.Converged {
		t.Fatalf("expected converge=false for no-progress storm, got true")
	}
	if !report.TimedOut {
		t.Fatalf("expected timeout=true, got false")
	}
	if report.AfterActiveCount != 10 {
		t.Fatalf("expected remaining active=10, got %d", report.AfterActiveCount)
	}
	if !report.IsPartial() {
		t.Fatalf("expected partial=true for timed-out remaining flows")
	}
}

func TestRunKillPeerFlowsIntermittentProgressEventuallyConverges(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	seq := []countSample{
		{target: 8, timeWait: 0},
		{target: 8, timeWait: 1},
		{target: 6, timeWait: 1},
		{target: 4, timeWait: 2},
		{target: 2, timeWait: 4},
		{target: 0, timeWait: 6},
	}
	hooks := fakeKillHooks(t, &now, seq)

	report := runKillPeerFlows(nil, KillConvergeOptions{
		MaxDuration:       4 * time.Second,
		MaxIterations:     12,
		SleepBetweenIters: 120 * time.Millisecond,
	}, hooks)

	if !report.Converged {
		t.Fatalf("expected converge=true with intermittent progress, got false report=%+v", report)
	}
	if report.Iterations != 5 {
		t.Fatalf("expected converge at iteration 5, got %d", report.Iterations)
	}
	if report.AfterTimeWaitCount != 6 {
		t.Fatalf("expected final time_wait=6, got %d", report.AfterTimeWaitCount)
	}
}

func TestRunKillPeerFlowsRespectsMaxIterations(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	seq := []countSample{
		{target: 5, timeWait: 0},
		{target: 4, timeWait: 0},
		{target: 3, timeWait: 0},
		{target: 2, timeWait: 0},
	}
	hooks := fakeKillHooks(t, &now, seq)

	report := runKillPeerFlows(nil, KillConvergeOptions{
		MaxDuration:       4 * time.Second,
		MaxIterations:     2,
		SleepBetweenIters: 120 * time.Millisecond,
	}, hooks)

	if report.Iterations != 2 {
		t.Fatalf("expected 2 iterations max, got %d", report.Iterations)
	}
	if report.Converged {
		t.Fatalf("expected not converged at max-iteration boundary")
	}
	if report.TimedOut {
		t.Fatalf("did not expect timeout when stopping by max iteration")
	}
}

func TestRunKillPeerFlowsRequeriesExactTuplesEachIteration(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	seq := []countSample{
		{target: 2, timeWait: 0},
		{target: 1, timeWait: 0},
		{target: 0, timeWait: 1},
	}

	queryCalls := 0
	killedKeysByIter := make([][]string, 0, 2)
	hooks := killConvergeHooks{
		now: func() time.Time { return now },
		sleep: func(d time.Duration) {
			now = now.Add(d)
		},
		broadKill: func() {},
		queryExactTuples: func() ([]SocketTuple, error) {
			queryCalls++
			if queryCalls == 1 {
				return []SocketTuple{
					{LocalIP: "10.0.0.1", LocalPort: 22, RemoteIP: "10.0.0.2", RemotePort: 40001},
				}, nil
			}
			return []SocketTuple{
				{LocalIP: "10.0.0.1", LocalPort: 22, RemoteIP: "10.0.0.2", RemotePort: 40002},
			}, nil
		},
		killTuples: func(tuples []SocketTuple) error {
			keys := make([]string, 0, len(tuples))
			for _, tuple := range tuples {
				keys = append(keys, socketTupleKey(tuple))
			}
			killedKeysByIter = append(killedKeysByIter, keys)
			return nil
		},
		dropConntrack: func() error { return nil },
		countStates: func() (int, int, error) {
			if len(seq) == 0 {
				return 0, 0, nil
			}
			sample := seq[0]
			seq = seq[1:]
			return sample.target, sample.timeWait, sample.err
		},
	}

	report := runKillPeerFlows(nil, KillConvergeOptions{
		MaxDuration:       4 * time.Second,
		MaxIterations:     12,
		SleepBetweenIters: 120 * time.Millisecond,
	}, hooks)
	if !report.Converged {
		t.Fatalf("expected converged report, got %+v", report)
	}
	if queryCalls < 2 {
		t.Fatalf("expected queryExactTuples called each iteration, got %d calls", queryCalls)
	}
	if len(killedKeysByIter) < 2 {
		t.Fatalf("expected killTuples called at least twice, got %d", len(killedKeysByIter))
	}
	secondIter := strings.Join(killedKeysByIter[1], ",")
	if !strings.Contains(secondIter, "40002") {
		t.Fatalf("expected second-iteration kill set to include newly discovered tuple, got %q", secondIter)
	}
}

func TestNormalizeSSStateAndTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: "ESTAB", want: "ESTABLISHED"},
		{in: "ESTABLISHED", want: "ESTABLISHED"},
		{in: "SYN-RECV", want: "SYN_RECV"},
		{in: "SYN_RECV", want: "SYN_RECV"},
		{in: "TIME-WAIT", want: "TIME_WAIT"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := normalizeSSState(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeSSState(%q)=%q want=%q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsKillActiveState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state string
		want  bool
	}{
		{state: "ESTABLISHED", want: true},
		{state: "SYN_RECV", want: true},
		{state: "LAST_ACK", want: true},
		{state: "TIME_WAIT", want: false},
		{state: "", want: false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.state, func(t *testing.T) {
			t.Parallel()
			if got := isKillActiveState(tc.state); got != tc.want {
				t.Fatalf("isKillActiveState(%q)=%v want=%v", tc.state, got, tc.want)
			}
		})
	}
}

type countSample struct {
	target   int
	timeWait int
	err      error
}

func fakeKillHooks(t *testing.T, now *time.Time, sequence []countSample) killConvergeHooks {
	t.Helper()

	countIdx := 0
	return killConvergeHooks{
		now: func() time.Time { return *now },
		sleep: func(d time.Duration) {
			*now = (*now).Add(d)
		},
		broadKill: func() {},
		queryExactTuples: func() ([]SocketTuple, error) {
			return []SocketTuple{
				{LocalIP: "10.0.0.1", LocalPort: 22, RemoteIP: "10.0.0.2", RemotePort: 12345},
			}, nil
		},
		killTuples:    func([]SocketTuple) error { return nil },
		dropConntrack: func() error { return nil },
		countStates: func() (int, int, error) {
			if len(sequence) == 0 {
				return 0, 0, errors.New("empty count sequence")
			}
			if countIdx >= len(sequence) {
				last := sequence[len(sequence)-1]
				return last.target, last.timeWait, last.err
			}
			sample := sequence[countIdx]
			countIdx++
			return sample.target, sample.timeWait, sample.err
		},
	}
}
