package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
)

type topDiagnosis struct {
	Severity   healthLevel
	Issue      string
	Scope      string
	Signal     string
	Likely     string
	Check      string
	Headline   string
	Reason     string
	Evidence   []string
	NextChecks []string
}

type stateDiagnosisCulprit struct {
	PeerIP    string
	ProcName  string
	LocalPort int
	Count     int
	Activity  int64
	Bandwidth float64
}

func (a *App) buildTopDiagnosis(
	data collector.ConnectionData,
	retrans *collector.RetransmitRates,
	conntrack *collector.ConntrackRates,
) *topDiagnosis {
	conntrackLevel := diagnosisConntrackLevel(conntrack, a.healthThresholds)
	retransLevel, sample := diagnosisRetransLevel(data, retrans, a.healthThresholds)

	if conntrack != nil && conntrack.StatsAvailable && !conntrack.FirstReading && conntrack.DropsPerSec > 0 {
		level := classifyMetric(conntrack.DropsPerSec, a.healthThresholds.DropsPerSec)
		if level < healthWarn {
			level = healthWarn
		}
		return &topDiagnosis{
			Severity: level,
			Issue:    "Conntrack drops",
			Scope:    "host-wide",
			Signal: fmt.Sprintf(
				"CT %s | Drops %.1f/s | %s/%s",
				diagnosisConntrackText(conntrack),
				conntrack.DropsPerSec,
				formatNumber(conntrack.Current),
				formatNumber(conntrack.Max),
			),
			Likely: "kernel cannot insert new tracked flows",
			Check:  "conntrack -S, nf_conntrack_max, NAT churn",
			Headline: "Conntrack drops active",
			Reason: fmt.Sprintf(
				"Kernel is dropping new tracked flows at %.0f/s; check conntrack pressure, NAT churn, or nf_conntrack_max.",
				conntrack.DropsPerSec,
			),
			Evidence: []string{
				fmt.Sprintf("Usage: %.0f%% (%s/%s tracked entries).", conntrack.UsagePercent, formatNumber(conntrack.Current), formatNumber(conntrack.Max)),
				fmt.Sprintf("Drops: %.1f/s from conntrack stats.", conntrack.DropsPerSec),
			},
			NextChecks: []string{
				"Run conntrack -S and confirm insert, drop, and early_drop counters.",
				"Check nf_conntrack_max and whether NAT or short-lived flows are churning.",
			},
		}
	}

	if conntrackLevel >= healthWarn && conntrack != nil {
		evidence := []string{
			fmt.Sprintf("Usage: %.0f%% (%s/%s tracked entries).", conntrack.UsagePercent, formatNumber(conntrack.Current), formatNumber(conntrack.Max)),
		}
		if conntrack.StatsAvailable && !conntrack.FirstReading && conntrack.DropsPerSec > 0 {
			evidence = append(evidence, fmt.Sprintf("Drops are already visible at %.1f/s.", conntrack.DropsPerSec))
		} else {
			evidence = append(evidence, "Drops are not active yet, but headroom is getting tight.")
		}
		return &topDiagnosis{
			Severity: conntrackLevel,
			Issue:    "Conntrack pressure",
			Scope:    "host-wide",
			Signal: fmt.Sprintf(
				"CT %s | %s/%s | Drops idle",
				diagnosisConntrackText(conntrack),
				formatNumber(conntrack.Current),
				formatNumber(conntrack.Max),
			),
			Likely: "kernel state-table pressure is building",
			Check:  "conntrack -S, nf_conntrack_max, churn source",
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

	if retransLevel >= healthWarn && retrans != nil {
		return &topDiagnosis{
			Severity: retransLevel,
			Issue:    "TCP retrans high",
			Scope:    "host-wide",
			Signal:   fmt.Sprintf("Retr %.2f%% | Out %.1f/s | EST %d", retrans.RetransPercent, retrans.OutSegsPerSec, sample.Established),
			Likely:   "packet loss, RTT spikes, NIC errors, or congestion",
			Check:    "NIC errors/drops, ss -tin, path loss/RTT",
			Headline: "TCP retrans is high",
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

	if diagnosis := a.buildStateDiagnosis("SYN_RECV", data.States["SYN_RECV"], retransSignal, conntrackSignal); diagnosis != nil {
		return diagnosis
	}
	if diagnosis := a.buildStateDiagnosis("CLOSE_WAIT", data.States["CLOSE_WAIT"], retransSignal, conntrackSignal); diagnosis != nil {
		return diagnosis
	}
	if conntrackLevel < healthWarn && retransLevel < healthWarn {
		if diagnosis := a.buildStateDiagnosis("TIME_WAIT", data.States["TIME_WAIT"], retransSignal, conntrackSignal); diagnosis != nil {
			return diagnosis
		}
	}
	if diagnosis := a.buildStateDiagnosis("FIN_WAIT1", data.States["FIN_WAIT1"], retransSignal, conntrackSignal); diagnosis != nil {
		return diagnosis
	}

	return &topDiagnosis{
		Severity: healthOK,
		Issue:    "No dominant issue",
		Scope:    "host-wide",
		Signal:   fmt.Sprintf("Retr %s | CT %s | States stable", retransSignal, conntrackSignal),
		Likely:   "no warning-level signal is dominating right now",
		Check:    "watch Top/States, wait next sample",
		Headline: "No dominant network issue",
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
			"Keep watching Top Connections and Connection States for a dominant peer or state shift.",
			diagnosisHealthyNextCheck(sample),
		},
	}
}

func diagnosisConntrackLevel(conntrack *collector.ConntrackRates, thresholds config.HealthThresholds) healthLevel {
	if conntrack == nil || conntrack.Max <= 0 {
		return healthUnknown
	}
	return classifyMetric(conntrack.UsagePercent, thresholds.ConntrackPercent)
}

func diagnosisRetransLevel(
	data collector.ConnectionData,
	retrans *collector.RetransmitRates,
	thresholds config.HealthThresholds,
) (healthLevel, retransSampleStatus) {
	sample := evaluateRetransSample(data, retrans, thresholds)
	if retrans == nil || retrans.FirstReading || !sample.Ready {
		return healthUnknown, sample
	}
	return classifyMetric(retrans.RetransPercent, thresholds.RetransPercent), sample
}

func (a *App) buildStateDiagnosis(state string, totalCount int, retransSignal string, conntrackSignal string) *topDiagnosis {
	warning, ok := stateWarnings[state]
	if !ok || totalCount <= warning.threshold {
		return nil
	}

	culprit, found := findStateDiagnosisCulprit(a.latestTalkers, state)
	headline := stateDiagnosisHeadline(state, culprit, found, a.sensitiveIP)
	scope := diagnosisScope(culprit, found, a.sensitiveIP)

	evidence := []string{
		fmt.Sprintf("State count: %s %s sockets (warn > %s).", formatNumber(totalCount), state, formatNumber(warning.threshold)),
	}
	if found {
		if proc := diagnosisProcLabel(culprit.ProcName); proc != "unresolved proc" {
			evidence = append(evidence, fmt.Sprintf(
				"Culprit: %s on :%d via %s (%s sockets).",
				formatPreviewIP(culprit.PeerIP, a.sensitiveIP),
				culprit.LocalPort,
				proc,
				formatNumber(culprit.Count),
			))
		}
	}

	return &topDiagnosis{
		Severity:   stateDiagnosisSeverity(warning.color),
		Issue:      stateDiagnosisIssue(state),
		Scope:      scope,
		Signal:     fmt.Sprintf("%s %s | Retr %s | CT %s", shortStateName(state), formatNumber(totalCount), retransSignal, conntrackSignal),
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
		peerIP := normalizeIP(conn.RemoteIP)
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

func stateDiagnosisSeverity(color string) healthLevel {
	if color == "red" {
		return healthCrit
	}
	return healthWarn
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
	return fmt.Sprintf("%s on :%d from %s", base, culprit.LocalPort, formatPreviewIP(culprit.PeerIP, sensitiveIP))
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
	return fmt.Sprintf("%.0f%%", conntrack.UsagePercent)
}

func diagnosisRetransText(retrans *collector.RetransmitRates, sample retransSampleStatus) string {
	if retrans == nil || retrans.FirstReading || !sample.Ready {
		return "LOW SAMPLE"
	}
	return fmt.Sprintf("%.2f%%", retrans.RetransPercent)
}

func diagnosisRetransEvidenceText(retrans *collector.RetransmitRates, sample retransSampleStatus) string {
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

func diagnosisHealthyNextCheck(sample retransSampleStatus) string {
	if !sample.Ready {
		return "Collect more steady traffic if you need a confident retrans verdict."
	}
	return "Keep sampling if workload shifts; the current host-level signal looks stable."
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
	return fmt.Sprintf("%s :%d", formatPreviewIP(culprit.PeerIP, sensitiveIP), culprit.LocalPort)
}
