package daemon

import (
	"context"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

func (d *Daemon) applyCacheRestoreLocked(ctx context.Context, actionID string, approval *cacheApproval, now time.Time) (api.CacheApplyResponse, error) {
	item, evidence, err := d.currentQuarantineApproval(ctx, approval)
	if err != nil {
		return d.rejectCacheAction(ctx, actionID, approval, err.Error(), now)
	}
	if err := beginCacheAction(ctx, d, actionID, approval, "attempting",
		"exact restoration revalidation passed; atomic rename is about to be attempted", evidence, now); err != nil {
		return api.CacheApplyResponse{}, err
	}
	restored, err := d.cacheFS.Restore(ctx, item.RootPath, item.QuarantinePath, item.OriginalPath,
		approval.artifact.RootIdentity, item.Identity)
	if err != nil {
		_ = failCacheAction(ctx, d, actionID, "failed", err, evidence)
		return cacheActionResponse(actionID, approval, "failed", err.Error(), append(evidence, err.Error()), d.cacheClock()), nil
	}
	completedAt := d.cacheClock()
	manifest := cacheartifact.ManifestDigest(item.OriginalPath, restored)
	if err := d.store.RecordCacheRestored(ctx, actionID, item.ArtifactID, restored, manifest, completedAt.UnixNano()); err != nil {
		return api.CacheApplyResponse{}, err
	}
	reason := "exact quarantined artifact was atomically restored to its absent original destination"
	return cacheActionResponse(actionID, approval, "restored", reason, evidence, completedAt), nil
}
