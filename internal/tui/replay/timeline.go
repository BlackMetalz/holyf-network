package replay

import (
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
)

func FilterSnapshotRefsByRange(refs []history.SnapshotRef, begin, end *time.Time) []history.SnapshotRef {
	if (begin == nil && end == nil) || len(refs) == 0 {
		return refs
	}

	filtered := make([]history.SnapshotRef, 0, len(refs))
	for _, ref := range refs {
		if begin != nil && ref.CapturedAt.Before(*begin) {
			continue
		}
		if end != nil && ref.CapturedAt.After(*end) {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}
