package daemon

import (
	"time"

	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/worktree"
)

func retiredWorktreeRecord(previous storage.WorktreeRecord, obs worktree.Observation,
	now time.Time, processEvidenceComplete bool) storage.WorktreeRecord {
	previous.LastSeenNs = now.UnixNano()
	previous.Registered = true
	previous.Complete = processEvidenceComplete && validateFreshObservation(previous, obs) == nil
	if !previous.Complete {
		previous.ProtectionJSON = `["retired_identity_or_observation_changed"]`
	}
	return previous
}
