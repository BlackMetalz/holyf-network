package replay

import (
	"fmt"

	"github.com/BlackMetalz/holyf-network/internal/history"
)

func RawPositionLabel(refs []history.SnapshotRef, index int) string {
	if len(refs) == 0 || index < 0 || index >= len(refs) {
		return "raw 0/0"
	}
	return fmt.Sprintf("raw %d/%d", index+1, len(refs))
}

func EmptyCountAfter(refs []history.SnapshotRef, index int, count func(history.SnapshotRef) int) int {
	if len(refs) == 0 || count == nil {
		return 0
	}
	if index < -1 {
		index = -1
	}
	if index >= len(refs)-1 {
		return 0
	}
	total := 0
	for i := index + 1; i < len(refs); i++ {
		if count(refs[i]) <= 0 {
			total++
		}
	}
	return total
}

func EmptyCountBefore(refs []history.SnapshotRef, index int, count func(history.SnapshotRef) int) int {
	if len(refs) == 0 || index <= 0 || count == nil {
		return 0
	}
	if index >= len(refs) {
		index = len(refs) - 1
	}
	total := 0
	for i := 0; i < index; i++ {
		if count(refs[i]) <= 0 {
			total++
		}
	}
	return total
}

func FindPrevNonEmptyIndex(refs []history.SnapshotRef, from int, count func(history.SnapshotRef) int) (int, int, bool) {
	if len(refs) == 0 || count == nil {
		return -1, 0, false
	}
	if from >= len(refs) {
		from = len(refs) - 1
	}
	skipped := 0
	for i := from; i >= 0; i-- {
		if count(refs[i]) > 0 {
			return i, skipped, true
		}
		skipped++
	}
	return -1, skipped, false
}

func FindNextNonEmptyIndex(refs []history.SnapshotRef, from int, count func(history.SnapshotRef) int) (int, int, bool) {
	if len(refs) == 0 || count == nil {
		return -1, 0, false
	}
	if from < 0 {
		from = 0
	}
	skipped := 0
	for i := from; i < len(refs); i++ {
		if count(refs[i]) > 0 {
			return i, skipped, true
		}
		skipped++
	}
	return -1, skipped, false
}

func FindNearestNonEmptyIndex(refs []history.SnapshotRef, target int, count func(history.SnapshotRef) int) (int, bool) {
	if len(refs) == 0 || target < 0 || target >= len(refs) || count == nil {
		return -1, false
	}
	if count(refs[target]) > 0 {
		return target, true
	}

	left, right := target-1, target+1
	for left >= 0 || right < len(refs) {
		if left >= 0 && count(refs[left]) > 0 {
			return left, true
		}
		if right < len(refs) && count(refs[right]) > 0 {
			return right, true
		}
		left--
		right++
	}
	return -1, false
}
