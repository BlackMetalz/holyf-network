package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (a *App) promptActionLog() {
	logs := a.recentActionLogs(actionLogModalLimit)

	var body strings.Builder
	if len(logs) == 0 {
		body.WriteString("  No actions yet")
	} else {
		for i, entry := range logs {
			body.WriteString("  ")
			body.WriteString(entry)
			if i < len(logs)-1 {
				body.WriteString("\n")
			}
		}
	}
	body.WriteString("\n\n")
	body.WriteString(fmt.Sprintf(
		"  [dim]Showing latest %d. Full history: %s (rolling %d events)[white]",
		actionLogModalLimit,
		actionHistoryDisplayPath,
		actionLogRotateLimit,
	))

	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetTextAlign(tview.AlignLeft).
		SetText(body.String())
	view.SetBorder(true)
	view.SetTitle(fmt.Sprintf(" Action Log (latest %d) ", actionLogModalLimit))

	closeModal := func() {
		a.pages.RemovePage("action-log")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}

	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc, tcell.KeyEnter:
			closeModal()
			return nil
		}
		return event
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(view, 100, 0, true).
			AddItem(nil, 0, 1, false),
			16, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage("action-log")
	a.pages.AddPage("action-log", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(view)
}

func (a *App) addActionLog(message string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return
	}

	line := fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), shortStatus(msg, 180))

	a.actionLogMu.Lock()
	a.actionLogs = append(a.actionLogs, line)
	if len(a.actionLogs) > inMemoryActionLogMax {
		a.actionLogs = append([]string(nil), a.actionLogs[len(a.actionLogs)-inMemoryActionLogMax:]...)
	}
	a.persistActionLogLocked(line)
	a.actionLogMu.Unlock()
}

func (a *App) recentActionLogs(limit int) []string {
	if limit <= 0 {
		limit = actionLogModalLimit
	}

	a.actionLogMu.Lock()
	defer a.actionLogMu.Unlock()

	total := len(a.actionLogs)
	if total == 0 {
		return nil
	}
	if limit > total {
		limit = total
	}

	out := make([]string, 0, limit)
	for i := total - 1; i >= total-limit; i-- {
		out = append(out, a.actionLogs[i])
	}
	return out
}

func defaultActionHistoryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return filepath.Join(home, actionHistoryDirName, actionHistoryFileName)
}

// persistActionLogLocked appends one event and keeps only the latest N lines.
// Caller must hold a.actionLogMu.
func (a *App) persistActionLogLocked(line string) {
	path := strings.TrimSpace(a.actionHistoryPath)
	if path == "" {
		path = defaultActionHistoryPath()
		a.actionHistoryPath = path
	}
	if path == "" {
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}

	lines, err := readActionHistoryLines(path)
	if err != nil {
		return
	}
	lines = append(lines, line)
	if len(lines) > actionLogRotateLimit {
		lines = append([]string(nil), lines[len(lines)-actionLogRotateLimit:]...)
	}

	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	_ = os.WriteFile(path, []byte(content), 0o644)
}

func readActionHistoryLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	content = strings.TrimRight(content, "\n")
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}
	return strings.Split(content, "\n"), nil
}
