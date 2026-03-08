package actions

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	defaultKillConvergeMaxDuration       = 4 * time.Second
	defaultKillConvergeMaxIterations     = 12
	defaultKillConvergeSleepBetweenIters = 120 * time.Millisecond
)

var killTargetStates = map[string]struct{}{
	"ESTABLISHED": {},
	"SYN_RECV":    {},
}

// KillConvergeOptions controls bounded iterative flow-kill behavior.
type KillConvergeOptions struct {
	MaxDuration       time.Duration
	MaxIterations     int
	SleepBetweenIters time.Duration
}

// KillConvergeReport captures kill convergence results.
type KillConvergeReport struct {
	BeforeTargetCount   int
	AfterTargetCount    int
	BeforeTimeWaitCount int
	AfterTimeWaitCount  int

	BeforeCountErr error
	AfterCountErr  error
	SocketErr      error
	FlowErr        error

	Iterations int
	TimedOut   bool
	Converged  bool
}

// IsPartial returns true when kill sweep ended with target flows still alive.
func (r KillConvergeReport) IsPartial() bool {
	return !r.Converged && r.AfterCountErr == nil && r.AfterTargetCount > 0
}

// DefaultKillConvergeOptions returns production defaults for bounded kill sweep.
func DefaultKillConvergeOptions() KillConvergeOptions {
	return KillConvergeOptions{
		MaxDuration:       defaultKillConvergeMaxDuration,
		MaxIterations:     defaultKillConvergeMaxIterations,
		SleepBetweenIters: defaultKillConvergeSleepBetweenIters,
	}
}

func (o KillConvergeOptions) normalized() KillConvergeOptions {
	n := o
	if n.MaxDuration <= 0 {
		n.MaxDuration = defaultKillConvergeMaxDuration
	}
	if n.MaxIterations <= 0 {
		n.MaxIterations = defaultKillConvergeMaxIterations
	}
	if n.SleepBetweenIters < 0 {
		n.SleepBetweenIters = defaultKillConvergeSleepBetweenIters
	}
	return n
}

// KillPeerFlowsConverge performs bounded iterative sweeping for one peer+port:
// 1) broad ss -K pass
// 2) exact tuple kill pass
// 3) conntrack -D pass
// 4) re-count target states
//
// Target kill states are ESTABLISHED + SYN_RECV. TIME_WAIT is informational only.
func KillPeerFlowsConverge(spec PeerBlockSpec, snapshotTuples []SocketTuple, opts KillConvergeOptions) KillConvergeReport {
	ip, peerIP, err := validateSpec(spec)
	if err != nil {
		return KillConvergeReport{
			BeforeCountErr: err,
			AfterCountErr:  err,
		}
	}
	port := strconv.Itoa(spec.LocalPort)

	hooks := killConvergeHooks{
		now:   time.Now,
		sleep: time.Sleep,
		broadKill: func() {
			broadKillPeerSockets(peerIP, port)
		},
		queryTargetTuples: func() ([]SocketTuple, error) {
			snap, err := queryPeerSocketSnapshot(ip, peerIP, port)
			if err != nil {
				return nil, err
			}
			return snap.TargetTuples, nil
		},
		killTuples: KillSockets,
		dropConntrack: func() error {
			return deleteConntrackFlows(ip, peerIP, port)
		},
		countStates: func() (int, int, error) {
			snap, err := queryPeerSocketSnapshot(ip, peerIP, port)
			if err != nil {
				return 0, 0, err
			}
			return snap.TargetCount, snap.TimeWaitCount, nil
		},
	}

	return runKillPeerFlowsConverge(snapshotTuples, opts, hooks)
}

type killConvergeHooks struct {
	now               func() time.Time
	sleep             func(time.Duration)
	broadKill         func()
	queryTargetTuples func() ([]SocketTuple, error)
	killTuples        func([]SocketTuple) error
	dropConntrack     func() error
	countStates       func() (targetCount int, timeWaitCount int, err error)
}

func runKillPeerFlowsConverge(snapshotTuples []SocketTuple, opts KillConvergeOptions, hooks killConvergeHooks) KillConvergeReport {
	o := opts.normalized()

	report := KillConvergeReport{}
	beforeTarget, beforeTW, beforeErr := hooks.countStates()
	report.BeforeTargetCount = beforeTarget
	report.BeforeTimeWaitCount = beforeTW
	report.AfterTargetCount = beforeTarget
	report.AfterTimeWaitCount = beforeTW
	report.BeforeCountErr = beforeErr
	report.AfterCountErr = beforeErr

	startedAt := hooks.now()
	cachedSnapshotTuples := dedupeSocketTuples(snapshotTuples)

	for iter := 1; iter <= o.MaxIterations; iter++ {
		report.Iterations = iter

		if hooks.broadKill != nil {
			hooks.broadKill()
		}

		exactTuples := []SocketTuple(nil)
		if hooks.queryTargetTuples != nil {
			tuples, err := hooks.queryTargetTuples()
			if err != nil {
				report.SocketErr = err
			} else {
				exactTuples = tuples
			}
		}

		tuplesToKill := dedupeSocketTuples(append(exactTuples, cachedSnapshotTuples...))
		if len(tuplesToKill) > 0 && hooks.killTuples != nil {
			if err := hooks.killTuples(tuplesToKill); err != nil {
				report.SocketErr = err
			} else {
				report.SocketErr = nil
			}
		}

		if hooks.dropConntrack != nil {
			if err := hooks.dropConntrack(); err != nil {
				report.FlowErr = err
			} else {
				report.FlowErr = nil
			}
		}

		targetCount, timeWaitCount, err := hooks.countStates()
		if err != nil {
			report.AfterCountErr = err
		} else {
			report.AfterCountErr = nil
			report.AfterTargetCount = targetCount
			report.AfterTimeWaitCount = timeWaitCount
			if targetCount == 0 {
				report.Converged = true
				break
			}
		}

		if hooks.now().Sub(startedAt) >= o.MaxDuration {
			report.TimedOut = true
			break
		}
		if iter < o.MaxIterations && o.SleepBetweenIters > 0 && hooks.sleep != nil {
			hooks.sleep(o.SleepBetweenIters)
		}
	}

	return report
}

type peerSocketSnapshot struct {
	TargetTuples  []SocketTuple
	TargetCount   int
	TimeWaitCount int
}

func queryPeerSocketSnapshot(_ net.IP, peerIP, localPort string) (peerSocketSnapshot, error) {
	if _, err := exec.LookPath("ss"); err != nil {
		return peerSocketSnapshot{}, fmt.Errorf("ss: command not found")
	}

	out, err := exec.Command("ss", "-tnp").CombinedOutput()
	if err != nil {
		return peerSocketSnapshot{}, fmt.Errorf("ss query: %w", err)
	}

	normalizedPeer := strings.TrimPrefix(peerIP, "::ffff:")
	lines := strings.Split(string(out), "\n")

	targetSeen := make(map[string]struct{})
	timeWaitSeen := make(map[string]struct{})

	var snapshot peerSocketSnapshot
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "State") {
			continue
		}

		state, localAddr, remoteAddr, ok := parseSSStateLine(line)
		if !ok {
			continue
		}

		localIP, localP, ok := splitHostPort(localAddr)
		if !ok || localP != localPort {
			continue
		}
		remoteIP, remoteP, ok := splitHostPort(remoteAddr)
		if !ok {
			continue
		}
		if strings.TrimPrefix(remoteIP, "::ffff:") != normalizedPeer {
			continue
		}

		localPortInt, err := strconv.Atoi(localP)
		if err != nil {
			continue
		}
		remotePortInt, err := strconv.Atoi(remoteP)
		if err != nil {
			continue
		}

		tuple := SocketTuple{
			LocalIP:    localIP,
			LocalPort:  localPortInt,
			RemoteIP:   remoteIP,
			RemotePort: remotePortInt,
		}
		key := socketTupleKey(tuple)

		switch {
		case isKillTargetState(state):
			if _, exists := targetSeen[key]; exists {
				continue
			}
			targetSeen[key] = struct{}{}
			snapshot.TargetCount++
			snapshot.TargetTuples = append(snapshot.TargetTuples, tuple)
		case state == "TIME_WAIT":
			if _, exists := timeWaitSeen[key]; exists {
				continue
			}
			timeWaitSeen[key] = struct{}{}
			snapshot.TimeWaitCount++
		}
	}

	return snapshot, nil
}

func parseSSStateLine(line string) (state, localAddr, remoteAddr string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return "", "", "", false
	}
	return normalizeSSState(fields[0]), fields[3], fields[4], true
}

func normalizeSSState(raw string) string {
	state := strings.ToUpper(strings.TrimSpace(raw))
	state = strings.ReplaceAll(state, "-", "_")
	switch state {
	case "ESTAB":
		return "ESTABLISHED"
	case "SYNRECV":
		return "SYN_RECV"
	case "TIMEWAIT":
		return "TIME_WAIT"
	default:
		return state
	}
}

func isKillTargetState(state string) bool {
	_, ok := killTargetStates[state]
	return ok
}

func dedupeSocketTuples(tuples []SocketTuple) []SocketTuple {
	if len(tuples) < 2 {
		return tuples
	}

	seen := make(map[string]struct{}, len(tuples))
	out := make([]SocketTuple, 0, len(tuples))
	for _, tuple := range tuples {
		key := socketTupleKey(tuple)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tuple)
	}
	return out
}

func socketTupleKey(tuple SocketTuple) string {
	return fmt.Sprintf("%s|%d|%s|%d", tuple.LocalIP, tuple.LocalPort, tuple.RemoteIP, tuple.RemotePort)
}
