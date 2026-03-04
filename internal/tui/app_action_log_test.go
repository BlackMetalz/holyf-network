package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddActionLogPersistsAndRotatesHistory(t *testing.T) {
	t.Parallel()

	historyPath := filepath.Join(t.TempDir(), actionHistoryDirName, actionHistoryFileName)
	a := &App{
		actionLogs:        make([]string, 0, 32),
		actionHistoryPath: historyPath,
	}

	total := actionLogRotateLimit + 5
	for i := 0; i < total; i++ {
		a.addActionLog(fmt.Sprintf("evt-%03d", i))
	}

	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	lines := splitHistoryLines(string(data))
	if len(lines) != actionLogRotateLimit {
		t.Fatalf("history line count mismatch: got=%d want=%d", len(lines), actionLogRotateLimit)
	}
	if !strings.Contains(lines[0], "evt-005") {
		t.Fatalf("oldest retained line should be evt-005, got: %q", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], "evt-504") {
		t.Fatalf("latest retained line should be evt-504, got: %q", lines[len(lines)-1])
	}

	recent := a.recentActionLogs(actionLogModalLimit)
	if len(recent) != actionLogModalLimit {
		t.Fatalf("recent log count mismatch: got=%d want=%d", len(recent), actionLogModalLimit)
	}
	if !strings.Contains(recent[0], "evt-504") {
		t.Fatalf("most recent modal entry should be evt-504, got: %q", recent[0])
	}
	if !strings.Contains(recent[len(recent)-1], "evt-485") {
		t.Fatalf("oldest modal entry should be evt-485, got: %q", recent[len(recent)-1])
	}
}

func TestAddActionLogSkipsEmptyMessage(t *testing.T) {
	t.Parallel()

	historyPath := filepath.Join(t.TempDir(), actionHistoryDirName, actionHistoryFileName)
	a := &App{
		actionLogs:        make([]string, 0, 32),
		actionHistoryPath: historyPath,
	}

	a.addActionLog("   ")

	if got := len(a.actionLogs); got != 0 {
		t.Fatalf("empty message should not be added to in-memory logs, got=%d", got)
	}
	if _, err := os.Stat(historyPath); err == nil {
		t.Fatalf("empty message should not create history file")
	}
}

func splitHistoryLines(content string) []string {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}
