package daemon

import (
	"context"
	"errors"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/cachefs"
)

func (d *Daemon) applyCachePurgeLocked(ctx context.Context, actionID string, approval *cacheApproval, now time.Time) (api.CacheApplyResponse, error) {
	item, evidence, err := d.currentQuarantineApproval(ctx, approval)
	if err != nil {
		return d.rejectCacheAction(ctx, actionID, approval, err.Error(), now)
	}
	if !d.cachePolicyEnabled(approval.artifact.PolicyID) || now.UnixNano() < item.GraceUntilNs {
		return d.rejectCacheAction(ctx, actionID, approval, "purge policy changed or quarantine grace has not elapsed", now)
	}
	if err := beginCacheAction(ctx, d, actionID, approval, "purging",
		"exact quarantine manifest and grace passed; permanent purge is about to be attempted", evidence, now); err != nil {
		return api.CacheApplyResponse{}, err
	}
	err = d.cacheFS.Purge(ctx, item.RootPath, item.QuarantinePath, approval.artifact.RootIdentity, item.Identity)
	if err != nil {
		result := "failed"
		if errors.Is(err, cachefs.ErrPartialPurge) {
			result = "partial"
		}
		_ = failCacheAction(ctx, d, actionID, result, err, evidence)
		return cacheActionResponse(actionID, approval, result, err.Error(), append(evidence, err.Error()), d.cacheClock()), nil
	}
	completedAt := d.cacheClock()
	if err := d.store.RecordCachePurged(ctx, actionID, item.ArtifactID, completedAt.UnixNano()); err != nil {
		return api.CacheApplyResponse{}, err
	}
	d.mu.Lock()
	d.metrics.cachePurgedBytes += item.Identity.Size
	d.mu.Unlock()
	reason := "one exact quarantine artifact was permanently purged after separate approval and grace"
	return cacheActionResponse(actionID, approval, "purged", reason, evidence, completedAt), nil
}
