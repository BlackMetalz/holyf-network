// Package actionlog manages the session action history and persistent logs.
package actionlog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// In-memory buffer size for fast UI access.
	inMemoryMax = 500
	// Hard limit for the persistent log file.
	rotateLimit = 500

	// Default relative path for history if not provided.
	defaultHistoryDir  = ".holyf-network"
	defaultHistoryFile = "history.log"
)

// Logger holds the in-memory action logs and manages their persistence to disk.
type Logger struct {
	mu   sync.Mutex
	logs []string
	path string
}

// NewLogger creates a new Logger with the specified persistent path.
// If path is empty, it will be resolved on the first write using defaultHistoryPath.
func NewLogger(historyPath string) *Logger {
	return &Logger{
		logs: make([]string, 0, 32),
		path: historyPath,
	}
}

// Add appends a new action message to the log.
func (l *Logger) Add(message string) {
	msg := sanitize(message)
	if msg == "" {
		return
	}

	line := fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), truncate(msg, 180))

	l.mu.Lock()
	defer l.mu.Unlock()

	l.logs = append(l.logs, line)
	if len(l.logs) > inMemoryMax {
		l.logs = append([]string(nil), l.logs[len(l.logs)-inMemoryMax:]...)
	}

	l.persistLocked(line)
}

// Recent returns the latest N logs (newest first).
func (l *Logger) Recent(limit int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	total := len(l.logs)
	if total == 0 {
		return nil
	}
	if limit <= 0 || limit > total {
		limit = total
	}

	out := make([]string, 0, limit)
	for i := total - 1; i >= total-limit; i-- {
		out = append(out, l.logs[i])
	}
	return out
}

// HistoryPath returns the current path used for persistent logging.
func (l *Logger) HistoryPath() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

func (l *Logger) persistLocked(line string) {
	if l.path == "" {
		l.path = defaultHistoryPath()
	}
	if l.path == "" {
		return
	}

	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return
	}

	lines, err := readHistoryLines(l.path)
	if err != nil {
		return
	}
	lines = append(lines, line)
	if len(lines) > rotateLimit {
		lines = append([]string(nil), lines[len(lines)-rotateLimit:]...)
	}

	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	_ = os.WriteFile(l.path, []byte(content), 0o644)
}

func defaultHistoryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return filepath.Join(home, defaultHistoryDir, defaultHistoryFile)
}

func readHistoryLines(path string) ([]string, error) {
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

func sanitize(message string) string {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return ""
	}
	if !strings.HasPrefix(msg, "Blocked ") {
		return msg
	}

	parts := strings.Split(msg, " | ")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "expires in ") || p == "killed 0/0 flows" {
			continue
		}
		filtered = append(filtered, p)
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, " | ")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
