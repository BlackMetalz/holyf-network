package tui

import (
	"testing"
	"time"

	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	"github.com/gdamore/tcell/v2"
)

func TestAppendDiagnosisHistoryCoalescesSameFingerprint(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	first := time.Date(2026, 3, 17, 14, 3, 12, 0, time.UTC)
	second := first.Add(30 * time.Second)

	a.appendDiagnosisHistory(first, &tuishared.Diagnosis{
		Severity: tuishared.HealthWarn,
		Issue:    "TIME_WAIT churn",
		Scope:    "172.25.110.137 :18080",
		Signal:   "TW 3,741 | Retr LOW SAMPLE | CT 1%",
		Likely:   "short-lived conn churn, not packet loss",
		Check:    "keepalive, conn reuse, client retries",
	})
	a.appendDiagnosisHistory(second, &tuishared.Diagnosis{
		Severity: tuishared.HealthWarn,
		Issue:    "TIME_WAIT churn",
		Scope:    "172.25.110.137 :18080",
		Signal:   "TW 4,102 | Retr LOW SAMPLE | CT 1%",
		Likely:   "short-lived conn churn, not packet loss",
		Check:    "keepalive, conn reuse, client retries",
	})

	recent := a.diagnosisEngine.Recent(0)
	if len(recent) != 1 {
		t.Fatalf("expected same fingerprint to coalesce into one history entry, got=%d", len(recent))
	}
	entry := recent[0]
	if entry.FirstSeen != first || entry.LastSeen != second {
		t.Fatalf("expected time range to expand, got first=%s last=%s", entry.FirstSeen, entry.LastSeen)
	}
	if entry.Diagnosis.Signal != "TW 4,102 | Retr LOW SAMPLE | CT 1%" {
		t.Fatalf("expected latest signal to replace prior signal, got=%q", entry.Diagnosis.Signal)
	}
}

func TestAppendDiagnosisHistoryPrependsAndCaps(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	base := time.Date(2026, 3, 17, 14, 0, 0, 0, time.UTC)

	for i := 0; i < diagnosisHistoryLimit+3; i++ {
		a.appendDiagnosisHistory(base.Add(time.Duration(i)*time.Second), &tuishared.Diagnosis{
			Severity: tuishared.HealthWarn,
			Issue:    "Issue " + string(rune('A'+i)),
			Scope:    "host-wide",
			Signal:   "signal",
			Likely:   "likely",
			Check:    "check",
		})
	}

	recent := a.recentDiagnosisHistory(0)
	if len(recent) != diagnosisHistoryLimit {
		t.Fatalf("history cap mismatch: got=%d want=%d", len(recent), diagnosisHistoryLimit)
	}
	if recent[0].Diagnosis.Issue != "Issue W" {
		t.Fatalf("expected newest issue at front, got=%q", recent[0].Diagnosis.Issue)
	}
	if recent[len(recent)-1].Diagnosis.Issue != "Issue D" {
		t.Fatalf("expected oldest retained issue at tail, got=%q", recent[len(recent)-1].Diagnosis.Issue)
	}
}

func TestHandleKeyEventDOpensDiagnosisHistoryModal(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.appendDiagnosisHistory(time.Date(2026, 3, 17, 14, 3, 12, 0, time.UTC), &tuishared.Diagnosis{
		Severity: tuishared.HealthWarn,
		Issue:    "TIME_WAIT churn",
		Scope:    "172.25.110.137 :18080",
		Signal:   "TW 3,741 | Retr LOW SAMPLE | CT 1%",
		Likely:   "short-lived conn churn, not packet loss",
		Check:    "keepalive, conn reuse, client retries",
	})

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'd', 0))
	if ret != nil {
		t.Fatalf("d should be handled")
	}
	name, _ := a.pages.GetFrontPage()
	if name != "diagnosis-history" {
		t.Fatalf("expected diagnosis-history modal, got %q", name)
	}
	_, plain := a.statusHotkeysForPage(name)
	if plain != "Enter=close Esc=close" {
		t.Fatalf("expected diagnosis-history hotkeys, got=%q", plain)
	}
}
