package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/cachefs"
)

const cachePurgePlanLifetime = 2 * time.Minute

type cachePurgeExecution struct {
	plan     api.CachePurgePlan
	approval *cacheApproval
	evidence []string
	used     bool
}

// CachePurgeApply consumes approval and prepares, but never executes, purge.
func (d *Daemon) CachePurgeApply(ctx context.Context, request api.CacheApplyRequest) (api.CachePurgePrepareResponse, error) {
	if request.Approval == "" || request.Confirmation == "" {
		return api.CachePurgePrepareResponse{}, errors.New("cache purge apply requires approval and exact artifact confirmation")
	}
	now := d.cacheClock()
	approval, refusal := d.consumeCacheApproval(request.Approval, now)
	if approval == nil {
		return api.CachePurgePrepareResponse{}, errors.New(refusal)
	}
	actionID, err := newActionID()
	if err != nil {
		return api.CachePurgePrepareResponse{}, err
	}
	if approval.kind != "purge" {
		refusal = "approval action does not match the purge endpoint"
	}
	if request.Confirmation != approval.artifact.ID {
		refusal = "foreground confirmation does not exactly match the approved artifact"
	}
	if refusal != "" {
		result, rejectErr := d.rejectCacheAction(ctx, actionID, approval, refusal, now)
		return api.CachePurgePrepareResponse{Action: result}, rejectErr
	}
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	return d.prepareCachePurgeLocked(ctx, actionID, approval, now)
}

func (d *Daemon) prepareCachePurgeLocked(ctx context.Context, actionID string, approval *cacheApproval, now time.Time) (api.CachePurgePrepareResponse, error) {
	item, evidence, err := d.currentQuarantineApproval(ctx, approval)
	if err != nil {
		result, rejectErr := d.rejectCacheAction(ctx, actionID, approval, err.Error(), now)
		return api.CachePurgePrepareResponse{Action: result}, rejectErr
	}
	if !d.cachePolicyEnabled(approval.artifact.PolicyID) || now.UnixNano() < item.GraceUntilNs {
		result, rejectErr := d.rejectCacheAction(ctx, actionID, approval,
			"purge policy changed or quarantine grace has not elapsed", now)
		return api.CachePurgePrepareResponse{Action: result}, rejectErr
	}
	completion, digest, err := newSecret(32)
	if err != nil {
		return api.CachePurgePrepareResponse{}, err
	}
	plan := api.CachePurgePlan{
		ActionID: actionID, ArtifactID: item.ArtifactID, RootPath: item.RootPath,
		QuarantinePath: item.QuarantinePath, RootIdentity: approval.artifact.RootIdentity,
		Identity: item.Identity, Configuration: d.cfg.Cache.Digest(),
		ExpiresNs: now.Add(cachePurgePlanLifetime).UnixNano(), Completion: completion,
	}
	if err := beginCacheAction(ctx, d, actionID, approval, "purging",
		"foreground-only exact quarantine purge was prepared after grace and revalidation", evidence, now); err != nil {
		return api.CachePurgePrepareResponse{}, err
	}
	d.actionMu.Lock()
	for key, execution := range d.cachePurgePlans {
		if execution.plan.ExpiresNs < now.UnixNano() {
			delete(d.cachePurgePlans, key)
		}
	}
	if len(d.cachePurgePlans) >= 1 {
		d.actionMu.Unlock()
		cause := errors.New("too many outstanding foreground purge plans")
		_ = failCacheAction(ctx, d, actionID, "failed", cause, evidence)
		return api.CachePurgePrepareResponse{}, cause
	}
	d.cachePurgePlans[digest] = &cachePurgeExecution{plan: plan, approval: approval, evidence: evidence}
	d.actionMu.Unlock()
	action := cacheActionResponse(actionID, approval, "purging",
		"daemon committed intent; only the short-lived foreground executor can unlink", evidence, now)
	return api.CachePurgePrepareResponse{Action: action, Plan: plan}, nil
}

// CachePurgeComplete verifies foreground execution before durable completion.
func (d *Daemon) CachePurgeComplete(ctx context.Context, request api.CachePurgeCompleteRequest) (api.CacheApplyResponse, error) {
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	execution, err := d.consumeCachePurgeExecution(request)
	if err != nil {
		return api.CacheApplyResponse{}, err
	}
	return d.verifyCachePurgeCompletion(ctx, execution, request.ExecutionError)
}

func (d *Daemon) consumeCachePurgeExecution(request api.CachePurgeCompleteRequest) (*cachePurgeExecution, error) {
	if request.ActionID == "" || request.Completion == "" {
		return nil, errors.New("cache purge completion requires action_id and completion capability")
	}
	digest := secretDigest(request.Completion)
	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	execution := d.cachePurgePlans[digest]
	if execution == nil || execution.plan.ActionID != request.ActionID {
		return nil, errors.New("foreground purge completion is unknown or was lost when the daemon restarted")
	}
	if execution.used {
		return nil, errors.New("foreground purge completion was already consumed")
	}
	execution.used = true
	delete(d.cachePurgePlans, digest)
	return execution, nil
}

func (d *Daemon) verifyCachePurgeCompletion(ctx context.Context, execution *cachePurgeExecution, executionError string) (api.CacheApplyResponse, error) {
	plan, approval := execution.plan, execution.approval
	expired := d.cacheClock().UnixNano() >= plan.ExpiresNs
	current, present, err := d.cacheFS.QuarantineEntry(ctx, plan.RootPath, plan.QuarantinePath, plan.RootIdentity)
	if err != nil {
		return d.ambiguousCachePurge(ctx, execution, fmt.Errorf("post-purge verification failed: %w", err))
	}
	if present && !current.Equal(plan.Identity) {
		return d.ambiguousCachePurge(ctx, execution, cachefs.ErrChangedIdentity)
	}
	completedAt := d.cacheClock()
	if present {
		cause := errors.New("foreground executor left the exact quarantine artifact present")
		if expired {
			cause = errors.New("foreground purge plan expired without a verified mutation")
		} else if executionError != "" {
			cause = fmt.Errorf("foreground executor failed without mutation: %s", executionError)
		}
		if err := failCacheAction(ctx, d, plan.ActionID, "failed", cause, execution.evidence); err != nil {
			return api.CacheApplyResponse{}, err
		}
		return cacheActionResponse(plan.ActionID, approval, "failed", cause.Error(),
			append(execution.evidence, cause.Error()), completedAt), nil
	}
	if expired {
		return d.ambiguousCachePurge(ctx, execution,
			errors.New("quarantine artifact became absent after the foreground purge plan expired"))
	}
	if err := d.store.RecordCachePurged(ctx, plan.ActionID, plan.ArtifactID, completedAt.UnixNano()); err != nil {
		return api.CacheApplyResponse{}, err
	}
	d.mu.Lock()
	d.metrics.cachePurgedBytes += plan.Identity.Size
	d.mu.Unlock()
	evidence := execution.evidence
	if executionError != "" {
		evidence = append(evidence, "executor reported an error but exact post-action absence was verified: "+executionError)
	}
	reason := "foreground executor removed one exact quarantine artifact; daemon verified absence"
	return cacheActionResponse(plan.ActionID, approval, "purged", reason, evidence, completedAt), nil
}

func (d *Daemon) ambiguousCachePurge(ctx context.Context, execution *cachePurgeExecution, cause error) (api.CacheApplyResponse, error) {
	d.tripFilesystemCircuit("cache purge completion became ambiguous: " + cause.Error())
	if err := failCacheAction(ctx, d, execution.plan.ActionID, "partial", cause, execution.evidence); err != nil {
		return api.CacheApplyResponse{}, err
	}
	return cacheActionResponse(execution.plan.ActionID, execution.approval, "partial", cause.Error(),
		append(execution.evidence, cause.Error()), d.cacheClock()), nil
}

func (d *Daemon) tripFilesystemCircuit(reason string) {
	d.mu.Lock()
	d.filesystemHealthy = false
	d.degraded = append(d.degraded, reason)
	d.mu.Unlock()
}

func (d *Daemon) filesystemMutationsHealthy() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.filesystemHealthy
}
