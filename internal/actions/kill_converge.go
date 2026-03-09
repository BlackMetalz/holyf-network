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

// KillConvergeOptions controls bounded iterative flow-kill behavior.
type KillConvergeOptions struct {
	MaxDuration       time.Duration
	MaxIterations     int
	SleepBetweenIters time.Duration
}

// KillConvergeReport captures kill convergence results.
type KillConvergeReport struct {
	// Active states count all matching sockets except TIME_WAIT.
	BeforeActiveCount   int
	AfterActiveCount    int
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

// IsPartial returns true when kill sweep ended with active flows still alive.
func (r KillConvergeReport) IsPartial() bool {
	return !r.Converged && r.AfterCountErr == nil && r.AfterActiveCount > 0
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

// KillPeerFlows performs bounded iterative sweeping for one peer+port:
// 1) broad ss -K pass
// 2) exact tuple kill pass
// 3) conntrack -D pass
// 4) re-count active states
//
// Active states include all matching sockets except TIME_WAIT.
func KillPeerFlows(spec PeerBlockSpec, snapshotTuples []SocketTuple, opts KillConvergeOptions) KillConvergeReport {
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
		queryExactTuples: func() ([]SocketTuple, error) {
			snap, err := queryPeerSocketSnapshot(ip, peerIP, port)
			if err != nil {
				return nil, err
			}
			return snap.ExactTuples, nil
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
			return snap.ActiveCount, snap.TimeWaitCount, nil
		},
	}

	return runKillPeerFlows(snapshotTuples, opts, hooks)
}

type killConvergeHooks struct {
	now              func() time.Time
	sleep            func(time.Duration)
	broadKill        func()
	queryExactTuples func() ([]SocketTuple, error)
	killTuples       func([]SocketTuple) error
	dropConntrack    func() error
	countStates      func() (activeCount int, timeWaitCount int, err error)
}

func runKillPeerFlows(snapshotTuples []SocketTuple, opts KillConvergeOptions, hooks killConvergeHooks) KillConvergeReport {
	o := opts.normalized()

	report := KillConvergeReport{}
	beforeActive, beforeTW, beforeErr := hooks.countStates()
	report.BeforeActiveCount = beforeActive
	report.AfterActiveCount = beforeActive
	report.BeforeTimeWaitCount = beforeTW
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
		if hooks.queryExactTuples != nil {
			tuples, err := hooks.queryExactTuples()
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

		activeCount, timeWaitCount, err := hooks.countStates()
		if err != nil {
			report.AfterCountErr = err
		} else {
			report.AfterCountErr = nil
			report.AfterActiveCount = activeCount
			report.AfterTimeWaitCount = timeWaitCount
			if activeCount == 0 {
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
	ExactTuples   []SocketTuple
	ActiveCount   int
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

	exactSeen := make(map[string]struct{})
	activeSeen := make(map[string]struct{})
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
		if _, exists := exactSeen[key]; !exists {
			exactSeen[key] = struct{}{}
			snapshot.ExactTuples = append(snapshot.ExactTuples, tuple)
		}

		switch {
		case state == "TIME_WAIT":
			if _, exists := timeWaitSeen[key]; exists {
				continue
			}
			timeWaitSeen[key] = struct{}{}
			snapshot.TimeWaitCount++
		case isKillActiveState(state):
			if _, exists := activeSeen[key]; exists {
				continue
			}
			activeSeen[key] = struct{}{}
			snapshot.ActiveCount++
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

func isKillActiveState(state string) bool {
	state = strings.TrimSpace(state)
	if state == "" || state == "TIME_WAIT" {
		return false
	}
	return true
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
