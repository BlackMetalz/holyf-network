package actions

import (
	"errors"
	"testing"
	"time"
)

func TestRunKillPeerFlowsConvergeConvergesToZero(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	seq := []countSample{
		{target: 5, timeWait: 0},
		{target: 3, timeWait: 1},
		{target: 1, timeWait: 2},
		{target: 0, timeWait: 4},
	}
	hooks := fakeKillHooks(t, &now, seq)

	report := runKillPeerFlowsConverge(nil, KillConvergeOptions{
		MaxDuration:       4 * time.Second,
		MaxIterations:     12,
		SleepBetweenIters: 120 * time.Millisecond,
	}, hooks)

	if !report.Converged {
		t.Fatalf("expected converge=true, got false report=%+v", report)
	}
	if report.AfterTargetCount != 0 {
		t.Fatalf("expected after target count=0, got %d", report.AfterTargetCount)
	}
	if report.Iterations != 3 {
		t.Fatalf("expected 3 iterations, got %d", report.Iterations)
	}
	if report.TimedOut {
		t.Fatalf("expected timeout=false, got true")
	}
}

func TestRunKillPeerFlowsConvergeNoProgressTimesOutPartial(t *testing.T) {
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

	report := runKillPeerFlowsConverge(nil, KillConvergeOptions{
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
	if report.AfterTargetCount != 10 {
		t.Fatalf("expected remaining target=10, got %d", report.AfterTargetCount)
	}
	if !report.IsPartial() {
		t.Fatalf("expected partial=true for timed-out remaining flows")
	}
}

func TestRunKillPeerFlowsConvergeIntermittentProgressEventuallyConverges(t *testing.T) {
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

	report := runKillPeerFlowsConverge(nil, KillConvergeOptions{
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

func TestRunKillPeerFlowsConvergeRespectsMaxIterations(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	seq := []countSample{
		{target: 5, timeWait: 0},
		{target: 4, timeWait: 0},
		{target: 3, timeWait: 0},
		{target: 2, timeWait: 0},
	}
	hooks := fakeKillHooks(t, &now, seq)

	report := runKillPeerFlowsConverge(nil, KillConvergeOptions{
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

func TestNormalizeSSStateAndTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in         string
		want       string
		wantTarget bool
	}{
		{in: "ESTAB", want: "ESTABLISHED", wantTarget: true},
		{in: "ESTABLISHED", want: "ESTABLISHED", wantTarget: true},
		{in: "SYN-RECV", want: "SYN_RECV", wantTarget: true},
		{in: "SYN_RECV", want: "SYN_RECV", wantTarget: true},
		{in: "TIME-WAIT", want: "TIME_WAIT", wantTarget: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := normalizeSSState(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeSSState(%q)=%q want=%q", tc.in, got, tc.want)
			}
			if isKillTargetState(got) != tc.wantTarget {
				t.Fatalf("isKillTargetState(%q) mismatch", got)
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
		queryTargetTuples: func() ([]SocketTuple, error) {
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
