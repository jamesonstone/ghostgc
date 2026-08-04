package daemon

import (
	"context"
	"errors"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

// CacheCleanupApply consumes one quarantine approval.
func (d *Daemon) CacheCleanupApply(ctx context.Context, request api.CacheApplyRequest) (api.CacheApplyResponse, error) {
	return d.applyCacheApproval(ctx, "cleanup", request)
}

// CacheRestoreApply consumes one restoration approval.
func (d *Daemon) CacheRestoreApply(ctx context.Context, request api.CacheApplyRequest) (api.CacheApplyResponse, error) {
	return d.applyCacheApproval(ctx, "restore", request)
}

// CachePurgeApply consumes one quarantine-only purge approval.
func (d *Daemon) CachePurgeApply(ctx context.Context, request api.CacheApplyRequest) (api.CacheApplyResponse, error) {
	return d.applyCacheApproval(ctx, "purge", request)
}

func (d *Daemon) applyCacheApproval(ctx context.Context, kind string, request api.CacheApplyRequest) (api.CacheApplyResponse, error) {
	if request.Approval == "" {
		return api.CacheApplyResponse{}, errors.New("cache apply requires an approval")
	}
	now := d.cacheClock()
	approval, refusal := d.consumeCacheApproval(request.Approval, now)
	if approval == nil {
		return api.CacheApplyResponse{}, errors.New(refusal)
	}
	actionID, err := newActionID()
	if err != nil {
		return api.CacheApplyResponse{}, err
	}
	if approval.kind != kind {
		refusal = "approval action does not match the requested endpoint"
	}
	if refusal != "" {
		return d.rejectCacheAction(ctx, actionID, approval, refusal, now)
	}

	// Cache actions hold both lanes through fresh session and filesystem checks.
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	switch kind {
	case "cleanup":
		return d.applyCacheQuarantineLocked(ctx, actionID, approval, now)
	case "restore":
		return d.applyCacheRestoreLocked(ctx, actionID, approval, now)
	case "purge":
		return d.applyCachePurgeLocked(ctx, actionID, approval, now)
	default:
		return d.rejectCacheAction(ctx, actionID, approval, "unsupported cache action", now)
	}
}

func (d *Daemon) rejectCacheAction(ctx context.Context, actionID string, approval *cacheApproval, reason string, now time.Time) (api.CacheApplyResponse, error) {
	evidence := []string{"cache action refused before filesystem mutation", reason}
	action := cacheartifact.Action{
		ActionID: actionID, ArtifactID: approval.artifact.ID, Kind: approval.kind,
		PolicyID: approval.artifact.PolicyID, RequestedNs: now.UnixNano(), UpdatedNs: now.UnixNano(),
		Result: "rejected", Reason: reason, Evidence: evidence,
	}
	if err := d.store.BeginCacheAction(ctx, action); err != nil {
		return api.CacheApplyResponse{}, err
	}
	return cacheActionResponse(actionID, approval, "rejected", reason, evidence, now), nil
}

func beginCacheAction(ctx context.Context, d *Daemon, actionID string, approval *cacheApproval, result, reason string, evidence []string, now time.Time) error {
	return d.store.BeginCacheAction(ctx, cacheartifact.Action{
		ActionID: actionID, ArtifactID: approval.artifact.ID, Kind: approval.kind,
		PolicyID: approval.artifact.PolicyID, RequestedNs: now.UnixNano(), UpdatedNs: now.UnixNano(),
		Result: result, Reason: reason, Evidence: evidence,
	})
}

func failCacheAction(ctx context.Context, d *Daemon, actionID, result string, cause error, evidence []string) error {
	evidence = append(evidence, cause.Error())
	return d.store.FinishCacheAction(ctx, actionID, result, cause.Error(), evidence, d.cacheClock().UnixNano())
}

func cacheActionResponse(actionID string, approval *cacheApproval, result, reason string, evidence []string, at time.Time) api.CacheApplyResponse {
	return api.CacheApplyResponse{
		ActionID: actionID, ArtifactID: approval.artifact.ID, Action: approval.kind,
		Result: result, Reason: reason, AtNs: at.UnixNano(), Evidence: evidence,
	}
}
