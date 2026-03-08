package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// SnapshotWriter appends snapshot records to daily segment files.
type SnapshotWriter struct {
	mu             sync.Mutex
	cfg            WriterConfig
	file           *os.File
	currentSegment string
	appendCount    int
	lockFile       *os.File
}

func NewSnapshotWriter(cfg WriterConfig) (*SnapshotWriter, error) {
	cfg = normalizeWriterConfig(cfg)
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("data dir is required")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	lockPath := filepath.Join(cfg.DataDir, lockFileName)
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("acquire lock %s: %w", lockPath, err)
	}

	return &SnapshotWriter{
		cfg:      cfg,
		lockFile: lockFile,
	}, nil
}

func normalizeWriterConfig(cfg WriterConfig) WriterConfig {
	cfg.DataDir = ExpandPath(cfg.DataDir)
	if cfg.RetentionHours <= 0 {
		cfg.RetentionHours = defaultRetentionHours
	}
	// 0 explicitly disables periodic append-count prune.
	if cfg.PruneEverySnapshots < 0 {
		cfg.PruneEverySnapshots = defaultPruneEveryWrites
	}
	return cfg
}

func (w *SnapshotWriter) Append(record SnapshotRecord) (AppendResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if record.CapturedAt.IsZero() {
		record.CapturedAt = time.Now()
	}
	// Persist timestamps in server local time for operator-friendly replay.
	record.CapturedAt = record.CapturedAt.Local()
	if record.Groups == nil {
		record.Groups = make([]SnapshotGroup, 0)
	}

	segment := segmentFileName(record.CapturedAt)
	segmentPath := filepath.Join(w.cfg.DataDir, segment)
	result := AppendResult{SegmentPath: segmentPath}

	if w.file == nil || w.currentSegment != segment {
		if err := w.rotateSegmentLocked(segmentPath, segment); err != nil {
			return result, err
		}
		if w.cfg.PruneEverySnapshots != 0 {
			pruned, err := w.pruneLocked(record.CapturedAt)
			if err != nil {
				return result, err
			}
			result.Prune = pruned
		}
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return result, fmt.Errorf("marshal snapshot: %w", err)
	}
	if _, err := w.file.Write(append(payload, '\n')); err != nil {
		return result, fmt.Errorf("write snapshot: %w", err)
	}

	w.appendCount++
	if w.cfg.PruneEverySnapshots > 0 && w.appendCount%w.cfg.PruneEverySnapshots == 0 {
		pruned, err := w.pruneLocked(record.CapturedAt)
		if err != nil {
			return result, err
		}
		result.Prune.RemovedByAge += pruned.RemovedByAge
	}

	return result, nil
}

func (w *SnapshotWriter) rotateSegmentLocked(path, segment string) error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open segment %s: %w", path, err)
	}
	w.file = file
	w.currentSegment = segment
	return nil
}

func (w *SnapshotWriter) pruneLocked(now time.Time) (PruneResult, error) {
	return PruneDataDirByAge(w.cfg.DataDir, w.cfg.RetentionHours, now)
}

func (w *SnapshotWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var closeErr error
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			closeErr = err
		}
		w.file = nil
	}
	if w.lockFile != nil {
		_ = syscall.Flock(int(w.lockFile.Fd()), syscall.LOCK_UN)
		if err := w.lockFile.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		w.lockFile = nil
	}
	return closeErr
}
