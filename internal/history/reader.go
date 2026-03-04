package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadIndex scans segment files and returns ordered snapshot refs (oldest -> latest).
// Corrupt lines are skipped and counted in IndexStats.Corrupt.
func LoadIndex(dataDir string) ([]SnapshotRef, IndexStats, error) {
	dataDir = ExpandPath(dataDir)
	files, err := listSegmentFiles(dataDir)
	if err != nil {
		return nil, IndexStats{}, err
	}
	return loadIndexFromFiles(files)
}

// LoadIndexFromFile scans a single segment file and returns ordered snapshot refs.
// segmentPathArg can be either a basename under dataDir or an absolute file path.
func LoadIndexFromFile(dataDir, segmentPathArg string) ([]SnapshotRef, IndexStats, error) {
	dataDir = ExpandPath(dataDir)
	path := strings.TrimSpace(segmentPathArg)
	if path == "" {
		return nil, IndexStats{}, fmt.Errorf("segment file is required")
	}
	path = ExpandPath(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(dataDir, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, IndexStats{}, fmt.Errorf("segment file not found: %s", path)
	}
	if info.IsDir() {
		return nil, IndexStats{}, fmt.Errorf("segment file must be a file: %s", path)
	}

	files := []segmentFile{
		{
			Path: path,
			Name: filepath.Base(path),
		},
	}
	return loadIndexFromFiles(files)
}

func loadIndexFromFiles(files []segmentFile) ([]SnapshotRef, IndexStats, error) {

	stats := IndexStats{Files: len(files)}
	refs := make([]SnapshotRef, 0, 256)
	for _, file := range files {
		fileRefs, corrupt, err := indexSegmentFile(file.Path)
		if err != nil {
			return nil, stats, err
		}
		stats.Corrupt += corrupt
		refs = append(refs, fileRefs...)
	}

	sort.Slice(refs, func(i, j int) bool {
		if !refs[i].CapturedAt.Equal(refs[j].CapturedAt) {
			return refs[i].CapturedAt.Before(refs[j].CapturedAt)
		}
		if refs[i].FilePath != refs[j].FilePath {
			return refs[i].FilePath < refs[j].FilePath
		}
		return refs[i].Offset < refs[j].Offset
	})

	return refs, stats, nil
}

func indexSegmentFile(path string) ([]SnapshotRef, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("open segment %s: %w", path, err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	refs := make([]SnapshotRef, 0, 64)
	corrupt := 0
	offset := int64(0)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, corrupt, fmt.Errorf("read segment %s: %w", path, err)
		}

		if len(line) > 0 {
			trimmed := strings.TrimSpace(string(line))
			if trimmed != "" {
				var record SnapshotRecord
				if unmarshalErr := json.Unmarshal([]byte(trimmed), &record); unmarshalErr != nil {
					corrupt++
				} else {
					refs = append(refs, SnapshotRef{
						FilePath:   path,
						Offset:     offset,
						CapturedAt: record.CapturedAt,
						ConnCount:  len(record.Connections),
					})
				}
			}
			offset += int64(len(line))
		}

		if err == io.EOF {
			break
		}
	}

	return refs, corrupt, nil
}

func ReadSnapshot(ref SnapshotRef) (SnapshotRecord, error) {
	file, err := os.Open(ref.FilePath)
	if err != nil {
		return SnapshotRecord{}, fmt.Errorf("open segment %s: %w", ref.FilePath, err)
	}
	defer file.Close()

	if _, err := file.Seek(ref.Offset, io.SeekStart); err != nil {
		return SnapshotRecord{}, fmt.Errorf("seek segment %s: %w", ref.FilePath, err)
	}

	reader := bufio.NewReader(file)
	line, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return SnapshotRecord{}, fmt.Errorf("read snapshot line: %w", err)
	}
	if len(line) == 0 {
		return SnapshotRecord{}, fmt.Errorf("empty snapshot at offset %d", ref.Offset)
	}

	var record SnapshotRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(line))), &record); err != nil {
		return SnapshotRecord{}, fmt.Errorf("decode snapshot: %w", err)
	}
	return record, nil
}
