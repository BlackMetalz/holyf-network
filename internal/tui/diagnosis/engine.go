package diagnosis

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

type HistoryEntry struct {
	FirstSeen time.Time
	LastSeen  time.Time
	Diagnosis tuishared.Diagnosis
}

type stateDiagnosisCulprit struct {
	PeerIP    string
	ProcName  string
	LocalPort int
	Count     int
	Activity  int64
	Bandwidth float64
}

func BuildTopDiagnosis(
	data collector.ConnectionData,
	retrans *collector.RetransmitRates,
	conntrack *collector.ConntrackRates,
	thresholds config.HealthThresholds,
	latestTalkers []collector.Connection,
	sensitiveIP bool,
) *tuishared.Diagnosis {
	conntrackLevel := diagnosisConntrackLevel(conntrack, thresholds)
	retransLevel, sample := diagnosisRetransLevel(data, retrans, thresholds)

	if conntrack != nil && conntrack.StatsAvailable && !conntrack.FirstReading && conntrack.DropsPerSec > 0 {
		level := tuishared.ClassifyMetric(conntrack.DropsPerSec, thresholds.DropsPerSec)
		if level < tuishared.HealthWarn {
			level = tuishared.HealthWarn
		}
		return &tuishared.Diagnosis{
			Severity:   level,
			Confidence: "HIGH",
			Issue:      "Conntrack drops",
			Scope:      "host-wide",
			Signal: fmt.Sprintf(
				"CT %s | Drops %.1f/s | %s/%s",
				diagnosisConntrackText(conntrack),
				conntrack.DropsPerSec,
				tuishared.FormatNumber(conntrack.Current),
				tuishared.FormatNumber(conntrack.Max),
			),
			Likely:   "kernel cannot insert new tracked flows",
			Check:    "conntrack -S, nf_conntrack_max, NAT churn",
			Headline: "Conntrack drops active",
			Reason: fmt.Sprintf(
				"Kernel is dropping new tracked flows at %.0f/s; check conntrack pressure, NAT churn, or nf_conntrack_max.",
				conntrack.DropsPerSec,
			),
			Evidence: []string{
				fmt.Sprintf("Usage: %.0f%% (%s/%s tracked entries).", conntrack.UsagePercent, tuishared.FormatNumber(conntrack.Current), tuishared.FormatNumber(conntrack.Max)),
				fmt.Sprintf("Drops: %.1f/s from conntrack stats.", conntrack.DropsPerSec),
			},
			NextChecks: []string{
				"Run conntrack -S and confirm insert, drop, and early_drop counters.",
				"Check nf_conntrack_max and whether NAT or short-lived flows are churning.",
			},
		}
	}

	if conntrackLevel >= tuishared.HealthWarn && conntrack != nil {
		evidence := []string{
			fmt.Sprintf("Usage: %.0f%% (%s/%s tracked entries).", conntrack.UsagePercent, tuishared.FormatNumber(conntrack.Current), tuishared.FormatNumber(conntrack.Max)),
		}
		if conntrack.StatsAvailable && !conntrack.FirstReading && conntrack.DropsPerSec > 0 {
			evidence = append(evidence, fmt.Sprintf("Drops are already visible at %.1f/s.", conntrack.DropsPerSec))
		} else {
			evidence = append(evidence, "Drops are not active yet, but headroom is getting tight.")
		}
		return &tuishared.Diagnosis{
			Severity:   conntrackLevel,
			Confidence: "HIGH",
			Issue:      "Conntrack pressure",
			Scope:      "host-wide",
			Signal: fmt.Sprintf(
				"CT %s | %s/%s | Drops idle",
				diagnosisConntrackText(conntrack),
				tuishared.FormatNumber(conntrack.Current),
				tuishared.FormatNumber(conntrack.Max),
			),
			Likely:   "kernel state-table pressure is building",
			Check:    "conntrack -S, nf_conntrack_max, churn source",
			Headline: "Conntrack pressure high",
			Reason: fmt.Sprintf(
				"Table usage is %.0f%% of max; capacity is getting tight before inserts start failing.",
				conntrack.UsagePercent,
			),
			Evidence: evidence,
			NextChecks: []string{
				"Check conntrack -S for inserts vs drops and confirm recent growth rate.",
				"Review nf_conntrack_max and whether one service or NAT path is driving churn.",
			},
		}
	}

	if retransLevel >= tuishared.HealthWarn && retrans != nil {
		return &tuishared.Diagnosis{
			Severity:   retransLevel,
			Confidence: diagnosisConfidenceForRetrans(sample),
			Issue:      "TCP retrans high",
			Scope:      "host-wide",
			Signal:     fmt.Sprintf("Retr %.2f%% | Out %.1f/s | EST %d", retrans.RetransPercent, retrans.OutSegsPerSec, sample.Established),
			Likely:     "packet loss, RTT spikes, NIC errors, or congestion",
			Check:      "NIC errors/drops, ss -tin, path loss/RTT",
			Headline:   "TCP retrans is high",
			Reason: fmt.Sprintf(
				"Retrans is %.2f%% with enough traffic sample (%.1f out seg/s, %d ESTABLISHED); check packet loss, RTT, NIC errors, or path congestion.",
				retrans.RetransPercent,
				retrans.OutSegsPerSec,
				sample.Established,
			),
			Evidence: []string{
				fmt.Sprintf("Retrans: %.2f%% at %.1f retrans/s.", retrans.RetransPercent, retrans.RetransPerSec),
				fmt.Sprintf("Sample ready: %d ESTABLISHED, %.1f out seg/s.", sample.Established, retrans.OutSegsPerSec),
			},
			NextChecks: []string{
				"Check NIC errors/drops and inspect ss -tin for per-socket retrans behavior.",
				"Validate path loss, RTT spikes, or congestion between local host and peer path.",
			},
		}
	}

	retransSignal := diagnosisRetransText(retrans, sample)
	conntrackSignal := diagnosisConntrackText(conntrack)

	if diagnosis := BuildStateDiagnosis("SYN_RECV", data.States["SYN_RECV"], retransSignal, conntrackSignal, latestTalkers, sensitiveIP); diagnosis != nil {
		return diagnosis
	}
	if diagnosis := BuildStateDiagnosis("CLOSE_WAIT", data.States["CLOSE_WAIT"], retransSignal, conntrackSignal, latestTalkers, sensitiveIP); diagnosis != nil {
		return diagnosis
	}
	if conntrackLevel < tuishared.HealthWarn && retransLevel < tuishared.HealthWarn {
		if diagnosis := BuildStateDiagnosis("TIME_WAIT", data.States["TIME_WAIT"], retransSignal, conntrackSignal, latestTalkers, sensitiveIP); diagnosis != nil {
			return diagnosis
		}
	}
	if diagnosis := BuildStateDiagnosis("FIN_WAIT1", data.States["FIN_WAIT1"], retransSignal, conntrackSignal, latestTalkers, sensitiveIP); diagnosis != nil {
		return diagnosis
	}

	return &tuishared.Diagnosis{
		Severity:   tuishared.HealthOK,
		Confidence: diagnosisConfidenceForHealthy(sample),
		Issue:      "No dominant issue",
		Scope:      "host-wide",
		Signal:     fmt.Sprintf("Retr %s | CT %s | States stable", retransSignal, conntrackSignal),
		Likely:     "no warning-level signal is dominating right now",
		Check:      "watch Top/States, wait next sample",
		Headline:   "No dominant network issue",
		Reason: fmt.Sprintf(
			"Retrans is %s, conntrack is %s, and no warning-level TCP state dominates.",
			retransSignal,
			conntrackSignal,
		),
		Evidence: []string{
			fmt.Sprintf("Retrans: %s.", diagnosisRetransEvidenceText(retrans, sample)),
			fmt.Sprintf("Conntrack: %s used; no warning-level TCP state dominates.", diagnosisConntrackText(conntrack)),
		},
		NextChecks: []string{
			"Watch Top/States for a dominant peer or state shift.",
			diagnosisHealthySampleNextCheck(sample),
			"If workload shifts, run Trace Packet on the hottest peer:port.",
		},
	}
}

func diagnosisConntrackLevel(conntrack *collector.ConntrackRates, thresholds config.HealthThresholds) tuishared.HealthLevel {
	if conntrack == nil || conntrack.Max <= 0 {
		return tuishared.HealthUnknown
	}
	return tuishared.ClassifyMetric(conntrack.UsagePercent, thresholds.ConntrackPercent)
}

func diagnosisRetransLevel(
	data collector.ConnectionData,
	retrans *collector.RetransmitRates,
	thresholds config.HealthThresholds,
) (tuishared.HealthLevel, tuishared.RetransSampleStatus) {
	sample := tuishared.EvaluateRetransSample(data, retrans, thresholds)
	if retrans == nil || retrans.FirstReading || !sample.Ready {
		return tuishared.HealthUnknown, sample
	}
	return tuishared.ClassifyMetric(retrans.RetransPercent, thresholds.RetransPercent), sample
}

func diagnosisConfidenceForRetrans(sample tuishared.RetransSampleStatus) string {
	if sample.Ready &&
		sample.Established >= sample.MinEstablished*2 &&
		sample.OutSegsPerSec >= sample.MinOutSegsPerSec*2 {
		return "HIGH"
	}
	if sample.Ready {
		return "MEDIUM"
	}
	return "LOW"
}

func diagnosisConfidenceForState(found bool) string {
	if found {
		return "MEDIUM"
	}
	return "LOW"
}

func diagnosisConfidenceForHealthy(sample tuishared.RetransSampleStatus) string {
	if sample.Ready {
		return "MEDIUM"
	}
	return "LOW"
}

func BuildStateDiagnosis(state string, totalCount int, retransSignal string, conntrackSignal string, latestTalkers []collector.Connection, sensitiveIP bool) *tuishared.Diagnosis {
	warning, ok := tuishared.StateWarnings[state]
	if !ok || totalCount <= warning.Threshold {
		return nil
	}

	culprit, found := findStateDiagnosisCulprit(latestTalkers, state)
	headline := stateDiagnosisHeadline(state, culprit, found, sensitiveIP)
	scope := diagnosisScope(culprit, found, sensitiveIP)

	evidence := []string{
		fmt.Sprintf("State count: %s %s sockets (warn > %s).", tuishared.FormatNumber(totalCount), state, tuishared.FormatNumber(warning.Threshold)),
	}
	if found {
		if proc := diagnosisProcLabel(culprit.ProcName); proc != "unresolved proc" {
			evidence = append(evidence, fmt.Sprintf(
				"Culprit: %s on :%d via %s (%s sockets).",
				tuishared.FormatPreviewIP(culprit.PeerIP, sensitiveIP),
				culprit.LocalPort,
				proc,
				tuishared.FormatNumber(culprit.Count),
			))
		}
	}

	return &tuishared.Diagnosis{
		Severity:   stateDiagnosisSeverity(warning.Color),
		Confidence: diagnosisConfidenceForState(found),
		Issue:      stateDiagnosisIssue(state),
		Scope:      scope,
		Signal:     fmt.Sprintf("%s %s | Retr %s | CT %s", tuishared.ShortStateName(state), tuishared.FormatNumber(totalCount), retransSignal, conntrackSignal),
		Likely:     stateDiagnosisLikely(state),
		Check:      stateDiagnosisCheck(state),
		Headline:   headline,
		Reason:     stateDiagnosisReason(state, totalCount),
		Evidence:   evidence,
		NextChecks: stateDiagnosisNextChecks(state),
	}
}

func findStateDiagnosisCulprit(conns []collector.Connection, state string) (stateDiagnosisCulprit, bool) {
	type aggregate struct {
		stateDiagnosisCulprit
	}

	byKey := make(map[string]aggregate)
	for _, conn := range conns {
		if !strings.EqualFold(conn.State, state) {
			continue
		}
		peerIP := tuishared.NormalizeIP(conn.RemoteIP)
		procName := strings.TrimSpace(conn.ProcName)
		if procName == "" {
			procName = "-"
		}
		key := fmt.Sprintf("%s|%s|%d", peerIP, procName, conn.LocalPort)
		current := byKey[key]
		current.PeerIP = peerIP
		current.ProcName = procName
		current.LocalPort = conn.LocalPort
		current.Count++
		current.Activity += conn.Activity
		current.Bandwidth += conn.TotalBytesPerSec
		byKey[key] = current
	}

	if len(byKey) == 0 {
		return stateDiagnosisCulprit{}, false
	}

	candidates := make([]stateDiagnosisCulprit, 0, len(byKey))
	for _, candidate := range byKey {
		candidates = append(candidates, candidate.stateDiagnosisCulprit)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Count != candidates[j].Count {
			return candidates[i].Count > candidates[j].Count
		}
		if candidates[i].Activity != candidates[j].Activity {
			return candidates[i].Activity > candidates[j].Activity
		}
		if candidates[i].Bandwidth != candidates[j].Bandwidth {
			return candidates[i].Bandwidth > candidates[j].Bandwidth
		}
		if candidates[i].LocalPort != candidates[j].LocalPort {
			return candidates[i].LocalPort < candidates[j].LocalPort
		}
		if candidates[i].PeerIP != candidates[j].PeerIP {
			return candidates[i].PeerIP < candidates[j].PeerIP
		}
		return candidates[i].ProcName < candidates[j].ProcName
	})

	return candidates[0], true
}

func stateDiagnosisSeverity(color string) tuishared.HealthLevel {
	if color == "red" {
		return tuishared.HealthCrit
	}
	return tuishared.HealthWarn
}

func stateDiagnosisIssue(state string) string {
	switch state {
	case "SYN_RECV":
		return "SYN_RECV spike"
	case "CLOSE_WAIT":
		return "CLOSE_WAIT pressure"
	case "TIME_WAIT":
		return "TIME_WAIT churn"
	case "FIN_WAIT1":
		return "FIN_WAIT1 lag"
	default:
		return state
	}
}

func stateDiagnosisHeadline(state string, culprit stateDiagnosisCulprit, found bool, sensitiveIP bool) string {
	base := map[string]string{
		"SYN_RECV":   "SYN_RECV spike",
		"CLOSE_WAIT": "CLOSE_WAIT pressure",
		"TIME_WAIT":  "TIME_WAIT churn",
		"FIN_WAIT1":  "FIN_WAIT1 cleanup lag",
	}[state]
	if base == "" {
		base = state
	}
	if !found || culprit.LocalPort == 0 || strings.TrimSpace(culprit.PeerIP) == "" {
		return base
	}
	return fmt.Sprintf("%s on :%d from %s", base, culprit.LocalPort, tuishared.FormatPreviewIP(culprit.PeerIP, sensitiveIP))
}

func stateDiagnosisReason(state string, totalCount int) string {
	switch state {
	case "SYN_RECV":
		return fmt.Sprintf("%d SYN_RECV sockets; half-open handshakes are piling up; check backlog pressure, SYN flood, or clients not completing handshakes.", totalCount)
	case "CLOSE_WAIT":
		return fmt.Sprintf("%d CLOSE_WAIT sockets; peers already closed, but the local app has not closed these sockets yet.", totalCount)
	case "TIME_WAIT":
		return fmt.Sprintf("%d TIME_WAIT sockets; short-lived connections are dominating more than a current path-quality issue.", totalCount)
	case "FIN_WAIT1":
		return fmt.Sprintf("%d FIN_WAIT1 sockets; close handshakes are stalling; check remote ACK behavior or socket cleanup lag.", totalCount)
	default:
		return fmt.Sprintf("%d %s sockets exceed the warning threshold.", totalCount, state)
	}
}

func stateDiagnosisLikely(state string) string {
	switch state {
	case "SYN_RECV":
		return "half-open handshakes are piling up"
	case "CLOSE_WAIT":
		return "peer closed; local app has not closed sockets"
	case "TIME_WAIT":
		return "short-lived conn churn, not packet loss"
	case "FIN_WAIT1":
		return "close handshake or peer ACKs are lagging"
	default:
		return "warning-level TCP state pressure"
	}
}

func stateDiagnosisNextChecks(state string) []string {
	switch state {
	case "SYN_RECV":
		return []string{
			"Check listen backlog pressure and whether clients complete the handshake.",
			"Validate SYN flood protection, accept queue saturation, or upstream load balancer behavior.",
		}
	case "CLOSE_WAIT":
		return []string{
			"Inspect the app close path and confirm handlers always close sockets after peer FIN.",
			"Correlate with process ownership in Top Connections and review recent deploy or code paths.",
		}
	case "TIME_WAIT":
		return []string{
			"Check whether one service is creating short-lived connections faster than expected.",
			"Review keepalive, connection reuse, or client retry behavior before blaming packet loss.",
		}
	case "FIN_WAIT1":
		return []string{
			"Check whether peers are ACKing close handshakes promptly.",
			"Inspect app socket cleanup timing and whether a middlebox is delaying close completion.",
		}
	default:
		return []string{
			"Inspect the dominant state directly in Connection States and Top Connections.",
			"Correlate the state spike with the owning process, peer, and local service port.",
		}
	}
}

func stateDiagnosisCheck(state string) string {
	switch state {
	case "SYN_RECV":
		return "backlog, SYN flood, handshake completion"
	case "CLOSE_WAIT":
		return "app close path, socket cleanup"
	case "TIME_WAIT":
		return "keepalive, conn reuse, client retries"
	case "FIN_WAIT1":
		return "peer ACKs, close cleanup, middlebox"
	default:
		return "dominant state, peer, local port"
	}
}

func diagnosisConntrackText(conntrack *collector.ConntrackRates) string {
	if conntrack == nil || conntrack.Max <= 0 {
		return "n/a"
	}
	return tuishared.FormatConntrackPercentShort(conntrack.UsagePercent)
}

func diagnosisRetransText(retrans *collector.RetransmitRates, sample tuishared.RetransSampleStatus) string {
	if retrans == nil || retrans.FirstReading || !sample.Ready {
		return "LOW SAMPLE"
	}
	return fmt.Sprintf("%.2f%%", retrans.RetransPercent)
}

func diagnosisRetransEvidenceText(retrans *collector.RetransmitRates, sample tuishared.RetransSampleStatus) string {
	if retrans == nil {
		return "n/a"
	}
	if retrans.FirstReading {
		return "waiting for the next refresh"
	}
	if !sample.Ready {
		return fmt.Sprintf("LOW SAMPLE (est %d/%d, out %.1f/%.1f seg/s)", sample.Established, sample.MinEstablished, sample.OutSegsPerSec, sample.MinOutSegsPerSec)
	}
	return fmt.Sprintf("%.2f%% at %.1f retrans/s with %.1f out seg/s", retrans.RetransPercent, retrans.RetransPerSec, retrans.OutSegsPerSec)
}

func diagnosisHealthySampleNextCheck(sample tuishared.RetransSampleStatus) string {
	if !sample.Ready {
		return "LOW SAMPLE: keep steady traffic to raise retrans confidence."
	}
	return "Sample ready: retrans is stable now, keep monitoring trend."
}

func diagnosisProcLabel(proc string) string {
	proc = strings.TrimSpace(proc)
	if proc == "" || proc == "-" {
		return "unresolved proc"
	}
	return proc
}

func diagnosisScope(culprit stateDiagnosisCulprit, found bool, sensitiveIP bool) string {
	if !found || culprit.LocalPort == 0 || strings.TrimSpace(culprit.PeerIP) == "" {
		return "host-wide"
	}
	return fmt.Sprintf("%s :%d", tuishared.FormatPreviewIP(culprit.PeerIP, sensitiveIP), culprit.LocalPort)
}

const historyLimit = 20

// Engine manages the diagnosis history and state.
type Engine struct {
	history []HistoryEntry
}

// NewEngine creates a new diagnosis engine.
func NewEngine() *Engine {
	return &Engine{
		history: make([]HistoryEntry, 0, historyLimit),
	}
}

// Append adds a new diagnosis to the history, collapsing identical consecutive ones.
func (e *Engine) Append(now time.Time, diagnosis *tuishared.Diagnosis) {
	if diagnosis == nil {
		return
	}

	entry := HistoryEntry{
		FirstSeen: now,
		LastSeen:  now,
		Diagnosis: cloneDiagnosis(diagnosis),
	}

	if len(e.history) > 0 {
		last := &e.history[0]
		if fingerprint(&last.Diagnosis) == fingerprint(diagnosis) {
			last.LastSeen = now
			last.Diagnosis = entry.Diagnosis
			return
		}
	}

	e.history = append([]HistoryEntry{entry}, e.history...)
	if len(e.history) > historyLimit {
		e.history = append([]HistoryEntry(nil), e.history[:historyLimit]...)
	}
}

// Recent returns the latest N history entries (newest first).
func (e *Engine) Recent(limit int) []HistoryEntry {
	if limit <= 0 || limit > historyLimit {
		limit = historyLimit
	}
	if len(e.history) == 0 {
		return nil
	}
	if limit > len(e.history) {
		limit = len(e.history)
	}
	out := make([]HistoryEntry, limit)
	copy(out, e.history[:limit])
	return out
}

func fingerprint(d *tuishared.Diagnosis) string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("%d|%s|%s|%s|%s|%s",
		d.Severity,
		strings.TrimSpace(d.Issue),
		strings.TrimSpace(d.Scope),
		strings.TrimSpace(d.Likely),
		strings.TrimSpace(d.Check),
		strings.TrimSpace(d.Confidence),
	)
}

func cloneDiagnosis(diagnosis *tuishared.Diagnosis) tuishared.Diagnosis {
	if diagnosis == nil {
		return tuishared.Diagnosis{}
	}
	clone := *diagnosis
	if len(diagnosis.Evidence) > 0 {
		clone.Evidence = append([]string(nil), diagnosis.Evidence...)
	}
	if len(diagnosis.NextChecks) > 0 {
		clone.NextChecks = append([]string(nil), diagnosis.NextChecks...)
	}
	return clone
}
