package tui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestAppendDiagnosisHistoryCoalescesSameFingerprint(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	first := time.Date(2026, 3, 17, 14, 3, 12, 0, time.UTC)
	second := first.Add(30 * time.Second)

	a.appendDiagnosisHistory(first, &topDiagnosis{
		Severity: healthWarn,
		Issue:    "TIME_WAIT churn",
		Scope:    "172.25.110.137 :18080",
		Signal:   "TW 3,741 | Retr LOW SAMPLE | CT 1%",
		Likely:   "short-lived conn churn, not packet loss",
		Check:    "keepalive, conn reuse, client retries",
	})
	a.appendDiagnosisHistory(second, &topDiagnosis{
		Severity: healthWarn,
		Issue:    "TIME_WAIT churn",
		Scope:    "172.25.110.137 :18080",
		Signal:   "TW 4,102 | Retr LOW SAMPLE | CT 1%",
		Likely:   "short-lived conn churn, not packet loss",
		Check:    "keepalive, conn reuse, client retries",
	})

	if len(a.diagnosisHistory) != 1 {
		t.Fatalf("expected same fingerprint to coalesce into one history entry, got=%d", len(a.diagnosisHistory))
	}
	entry := a.diagnosisHistory[0]
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
		a.appendDiagnosisHistory(base.Add(time.Duration(i)*time.Second), &topDiagnosis{
			Severity: healthWarn,
			Issue:    "Issue " + string(rune('A'+i)),
			Scope:    "host-wide",
			Signal:   "signal",
			Likely:   "likely",
			Check:    "check",
		})
	}

	if len(a.diagnosisHistory) != diagnosisHistoryLimit {
		t.Fatalf("history cap mismatch: got=%d want=%d", len(a.diagnosisHistory), diagnosisHistoryLimit)
	}
	if a.diagnosisHistory[0].Diagnosis.Issue != "Issue W" {
		t.Fatalf("expected newest issue at front, got=%q", a.diagnosisHistory[0].Diagnosis.Issue)
	}
	if a.diagnosisHistory[len(a.diagnosisHistory)-1].Diagnosis.Issue != "Issue D" {
		t.Fatalf("expected oldest retained issue at tail, got=%q", a.diagnosisHistory[len(a.diagnosisHistory)-1].Diagnosis.Issue)
	}
}

func TestHandleKeyEventDOpensDiagnosisHistoryModal(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.appendDiagnosisHistory(time.Date(2026, 3, 17, 14, 3, 12, 0, time.UTC), &topDiagnosis{
		Severity: healthWarn,
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
