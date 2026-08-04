package daemon

import (
	"context"
	"path/filepath"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

func (d *Daemon) applyCacheQuarantineLocked(ctx context.Context, actionID string, approval *cacheApproval, now time.Time) (api.CacheApplyResponse, error) {
	artifact, evidence, err := d.freshCleanupArtifact(ctx, approval)
	if err != nil {
		return d.rejectCacheAction(ctx, actionID, approval, err.Error(), now)
	}
	if err := beginCacheAction(ctx, d, actionID, approval, "attempting",
		"fresh revalidation passed; atomic same-filesystem quarantine is about to be attempted", evidence, now); err != nil {
		return api.CacheApplyResponse{}, err
	}
	destination := filepath.Base(approval.destination)
	moved, err := d.cacheFS.Quarantine(ctx, artifact.RootPath, artifact.RelativePath, destination, artifact.RootIdentity, artifact.Identity)
	if err != nil {
		_ = failCacheAction(ctx, d, actionID, "failed", err, evidence)
		return cacheActionResponse(actionID, approval, "failed", err.Error(), append(evidence, err.Error()), d.cacheClock()), nil
	}
	completedAt := d.cacheClock()
	quarantinePath := filepath.Join(cacheartifact.QuarantineDirectory, destination)
	item := cacheartifact.Quarantine{
		ArtifactID: artifact.ID, RootPath: artifact.RootPath, OriginalPath: artifact.RelativePath,
		QuarantinePath: quarantinePath, Identity: moved,
		ManifestDigest:   cacheartifact.ManifestDigest(quarantinePath, moved),
		OriginalManifest: artifact.ManifestDigest, QuarantinedNs: completedAt.UnixNano(),
		GraceUntilNs: completedAt.Add(d.cfg.Cache.QuarantineGrace.D()).UnixNano(),
		Status:       "quarantined", UpdatedNs: completedAt.UnixNano(), Configuration: d.cfg.Cache.Digest(),
	}
	if err := d.store.RecordCacheQuarantined(ctx, actionID, item); err != nil {
		return api.CacheApplyResponse{}, err
	}
	d.mu.Lock()
	d.metrics.cacheQuarantinedBytes += moved.Size
	d.mu.Unlock()
	reason := "one exact artifact was atomically renamed into provider-local quarantine; no disk space was reclaimed"
	return cacheActionResponse(actionID, approval, "quarantined", reason, evidence, completedAt), nil
}
