package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
)

type topDiagnosis struct {
	Severity healthLevel
	Headline string
	Reason   string
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
	if len(a.latestTalkers) == 0 {
		return nil
	}

	conntrackLevel := diagnosisConntrackLevel(conntrack, a.healthThresholds)
	retransLevel, sample := diagnosisRetransLevel(data, retrans, a.healthThresholds)

	if conntrack != nil && conntrack.StatsAvailable && !conntrack.FirstReading && conntrack.DropsPerSec > 0 {
		level := classifyMetric(conntrack.DropsPerSec, a.healthThresholds.DropsPerSec)
		if level < healthWarn {
			level = healthWarn
		}
		return &topDiagnosis{
			Severity: level,
			Headline: "Conntrack drops active",
			Reason: fmt.Sprintf(
				"Kernel is dropping tracked flows at %.0f/s; inspect conntrack pressure, NAT churn, or nf_conntrack_max.",
				conntrack.DropsPerSec,
			),
		}
	}

	if conntrackLevel >= healthWarn && conntrack != nil {
		return &topDiagnosis{
			Severity: conntrackLevel,
			Headline: "Conntrack pressure near limit",
			Reason: fmt.Sprintf(
				"Usage is %.0f%% of max tracked entries; investigate churn and capacity before inserts start failing.",
				conntrack.UsagePercent,
			),
		}
	}

	if retransLevel >= healthWarn && retrans != nil {
		return &topDiagnosis{
			Severity: retransLevel,
			Headline: "High TCP retrans with enough sample",
			Reason: fmt.Sprintf(
				"Retrans is %.2f%% at %.1f out seg/s with %d ESTABLISHED; inspect loss, RTT, or NIC/path issues.",
				retrans.RetransPercent,
				retrans.OutSegsPerSec,
				sample.Established,
			),
		}
	}

	if diagnosis := a.buildStateDiagnosis("SYN_RECV", data.States["SYN_RECV"]); diagnosis != nil {
		return diagnosis
	}
	if diagnosis := a.buildStateDiagnosis("CLOSE_WAIT", data.States["CLOSE_WAIT"]); diagnosis != nil {
		return diagnosis
	}
	if conntrackLevel < healthWarn && retransLevel < healthWarn {
		if diagnosis := a.buildStateDiagnosis("TIME_WAIT", data.States["TIME_WAIT"]); diagnosis != nil {
			return diagnosis
		}
	}
	if diagnosis := a.buildStateDiagnosis("FIN_WAIT1", data.States["FIN_WAIT1"]); diagnosis != nil {
		return diagnosis
	}

	conntrackText := "n/a"
	if conntrack != nil && conntrack.Max > 0 {
		conntrackText = fmt.Sprintf("%.0f%%", conntrack.UsagePercent)
	}
	retransText := "LOW SAMPLE"
	if retrans != nil && !retrans.FirstReading && sample.Ready {
		retransText = fmt.Sprintf("%.2f%%", retrans.RetransPercent)
	}

	return &topDiagnosis{
		Severity: healthOK,
		Headline: "No dominant issue",
		Reason: fmt.Sprintf(
			"State mix looks normal; retrans %s, conntrack %s, and no warning-level TCP states are dominating.",
			retransText,
			conntrackText,
		),
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

func (a *App) buildStateDiagnosis(state string, totalCount int) *topDiagnosis {
	warning, ok := stateWarnings[state]
	if !ok || totalCount <= warning.threshold {
		return nil
	}

	culprit, found := findStateDiagnosisCulprit(a.latestTalkers, state)
	headline := stateDiagnosisHeadline(state, culprit, found, a.sensitiveIP)

	return &topDiagnosis{
		Severity: stateDiagnosisSeverity(warning.color),
		Headline: headline,
		Reason:   stateDiagnosisReason(state, totalCount),
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
		return fmt.Sprintf("%d SYN_RECV sockets; half-open handshakes are piling up and may indicate backlog pressure or a SYN flood.", totalCount)
	case "CLOSE_WAIT":
		return fmt.Sprintf("%d CLOSE_WAIT sockets; peers already closed and the local app likely is not closing sockets.", totalCount)
	case "TIME_WAIT":
		return fmt.Sprintf("%d TIME_WAIT sockets; short-lived connection churn is dominating more than a current path-quality issue.", totalCount)
	case "FIN_WAIT1":
		return fmt.Sprintf("%d FIN_WAIT1 sockets; close handshakes are stalling and cleanup may be lagging.", totalCount)
	default:
		return fmt.Sprintf("%d %s sockets exceed the warning threshold.", totalCount, state)
	}
}
