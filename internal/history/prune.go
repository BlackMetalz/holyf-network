package history

import (
	"fmt"
	"os"
	"time"
)

// PruneDataDirByAge removes segment files older than retention window.
// The current local-day segment is always preserved for runtime safety.
func PruneDataDirByAge(dataDir string, retentionHours int, now time.Time) (PruneResult, error) {
	if retentionHours < 1 {
		return PruneResult{}, fmt.Errorf("retention-hours must be >= 1, got %d", retentionHours)
	}

	files, err := listSegmentFiles(dataDir)
	if err != nil {
		return PruneResult{}, err
	}

	result := PruneResult{}
	if len(files) == 0 {
		return result, nil
	}

	now = now.Local()
	cutoff := now.Add(-time.Duration(retentionHours) * time.Hour)
	currentSegment := segmentFileName(now)

	for _, file := range files {
		if file.Name == currentSegment {
			continue
		}
		segmentEnd := file.Timestamp
		if file.Span > 0 {
			segmentEnd = segmentEnd.Add(file.Span)
		}
		if segmentEnd.Before(cutoff) {
			if err := os.Remove(file.Path); err == nil {
				result.RemovedByAge++
			}
		}
	}

	return result, nil
}
