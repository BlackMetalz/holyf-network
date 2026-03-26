// Package livetrace owns the state and data types for the live tcpdump
// trace packet engine. The actual UI and orchestration methods remain on
// the App struct in the root tui package and access state through Engine.
package livetrace

import (
	"context"
	"path/filepath"
	"sync"

	tuitrace "github.com/BlackMetalz/holyf-network/internal/tui/trace"
)

// Engine holds all mutable state that was previously embedded directly in App
// for the trace-packet subsystem.
type Engine struct {
	// Capture lifecycle — accessed only from the main goroutine (no lock needed).
	CaptureRunning bool
	CaptureCancel  context.CancelFunc

	// Pause state while the trace form / progress modal is open.
	FlowAutoPaused bool

	// Trace history (persisted as NDJSON).
	// All fields below are protected by mu.
	mu             sync.Mutex
	history        []tuitrace.Entry
	historyLoaded  bool
	historyDataDir string

	// Selected entry index used by the history modal.
	SelectedIndex int
}

// NewEngine creates an Engine ready for use.
func NewEngine(historyDataDir string) *Engine {
	return &Engine{
		history:        make([]tuitrace.Entry, 0, 32),
		historyDataDir: historyDataDir,
	}
}

// NewEngineLoaded creates an Engine with historyLoaded=true so that tests
// skip the disk-read in ensureTraceHistoryLoadedLocked.
func NewEngineLoaded() *Engine {
	return &Engine{
		history:       make([]tuitrace.Entry, 0, 8),
		historyLoaded: true,
	}
}

// --- Lock helpers (App callers hold the lock and call the Locked variants) ---

// Lock acquires the history mutex.
func (e *Engine) Lock() { e.mu.Lock() }

// Unlock releases the history mutex.
func (e *Engine) Unlock() { e.mu.Unlock() }

// --- Methods that require the caller to hold the lock (suffix: Locked) ---
// These do NOT acquire the lock themselves to avoid deadlocks.

// HistoryLocked returns the raw slice. Caller must hold Lock().
func (e *Engine) HistoryLocked() []tuitrace.Entry { return e.history }

// SetHistoryLocked replaces the full history slice. Caller must hold Lock().
func (e *Engine) SetHistoryLocked(entries []tuitrace.Entry) { e.history = entries }

// AppendEntryLocked appends an entry. Caller must hold Lock().
func (e *Engine) AppendEntryLocked(entry tuitrace.Entry) {
	e.history = append(e.history, entry)
}

// IsHistoryLoadedLocked reports whether the history was loaded from disk. Caller must hold Lock().
func (e *Engine) IsHistoryLoadedLocked() bool { return e.historyLoaded }

// MarkHistoryLoadedLocked marks the history as loaded. Caller must hold Lock().
func (e *Engine) MarkHistoryLoadedLocked() { e.historyLoaded = true }

// HistoryDataDirLocked returns the data directory. Caller must hold Lock().
func (e *Engine) HistoryDataDirLocked() string { return e.historyDataDir }

// SetHistoryDataDirLocked sets the data directory. Caller must hold Lock().
func (e *Engine) SetHistoryDataDirLocked(d string) { e.historyDataDir = d }

// --- Safe read-only accessor that acquires own lock ---

// HistoryDataDir returns the data dir (acquires its own short lock).
func (e *Engine) HistoryDataDir() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.historyDataDir
}

// HistorySegmentDir returns the path to the history segment directory.
func (e *Engine) HistorySegmentDir() string {
	e.mu.Lock()
	dir := e.historyDataDir
	e.mu.Unlock()
	return filepath.Join(dir, "segments")
}

// MarkHistoryLoaded marks the history as loaded (safe to call without holding Lock).
// Useful in tests that call this before any App methods run.
func (e *Engine) MarkHistoryLoaded() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.historyLoaded = true
}
