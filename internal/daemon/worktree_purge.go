package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/worktree"
)

const worktreePurgePlanLifetime = 2 * time.Minute

type worktreePurgeExecution struct {
	plan       api.WorktreePurgePlan
	record     storage.WorktreeRecord
	validation worktreeValidation
	action     storage.WorktreeActionRecord
	used       bool
}

// WorktreePurgePreview issues separate grace-gated finalization authority.
func (d *Daemon) WorktreePurgePreview(ctx context.Context, req api.WorktreeRemovalPreviewRequest) (api.WorktreeRemovalPreviewResponse, error) {
	if req.WorktreeID == "" {
		return api.WorktreeRemovalPreviewResponse{}, errors.New("worktree purge preview requires worktree_id")
	}
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	record, err := d.store.GetWorktree(ctx, req.WorktreeID)
	if err != nil {
		return api.WorktreeRemovalPreviewResponse{}, err
	}
	validation, err := d.validateRetiredWorktree(ctx, record, true)
	if err != nil {
		return api.WorktreeRemovalPreviewResponse{}, err
	}
	return d.issueRetiredWorktreeApproval(ctx, "purge", record, validation,
		"ghostgc worktree purge --apply --approval %s --yes --confirm "+record.WorktreeID,
		"No mutation occurred. Apply prepares a short-lived foreground native-removal plan and requires the full worktree ID.")
}

// WorktreePurgeApply commits intent but never invokes native removal.
func (d *Daemon) WorktreePurgeApply(ctx context.Context, req api.WorktreeRemovalApplyRequest) (api.WorktreePurgePrepareResponse, error) {
	if req.Approval == "" || req.Confirmation == "" {
		return api.WorktreePurgePrepareResponse{}, errors.New("worktree purge requires approval and exact confirmation")
	}
	d.worktreeActionMu.Lock()
	defer d.worktreeActionMu.Unlock()
	now := time.Now()
	approval, refusal := d.consumeWorktreeApproval(req.Approval, now)
	if approval == nil {
		return api.WorktreePurgePrepareResponse{}, errors.New(refusal)
	}
	if approval.kind != "purge" || req.Confirmation != approval.record.WorktreeID {
		refusal = "approval action or exact worktree confirmation does not match"
	}
	actionID, err := newWorktreeActionID()
	if err != nil {
		return api.WorktreePurgePrepareResponse{}, err
	}
	if refusal != "" {
		result, rejectErr := d.rejectWorktreeAction(ctx, actionID, approval, refusal, now)
		return api.WorktreePurgePrepareResponse{Action: result}, rejectErr
	}
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	current, err := d.store.GetWorktree(ctx, approval.record.WorktreeID)
	if err != nil || !sameRetiredAuthority(approval.record, current) {
		result, rejectErr := d.rejectWorktreeAction(ctx, actionID, approval, "bound retirement authority changed", time.Now())
		return api.WorktreePurgePrepareResponse{Action: result}, rejectErr
	}
	validation, err := d.validateRetiredWorktree(ctx, current, true)
	if err != nil {
		result, rejectErr := d.rejectWorktreeAction(ctx, actionID, approval, err.Error(), time.Now())
		return api.WorktreePurgePrepareResponse{Action: result}, rejectErr
	}
	binding, err := worktreeBinding(current, validation, d.cfg.Worktrees)
	if err != nil || binding != approval.bindingDigest || !sameValidation(approval.validation, validation) {
		result, rejectErr := d.rejectWorktreeAction(ctx, actionID, approval, "a fact bound to the purge preview changed", time.Now())
		return api.WorktreePurgePrepareResponse{Action: result}, rejectErr
	}
	return d.prepareWorktreePurge(ctx, actionID, current, validation, now)
}

func (d *Daemon) prepareWorktreePurge(ctx context.Context, actionID string, record storage.WorktreeRecord,
	validation worktreeValidation, now time.Time) (api.WorktreePurgePrepareResponse, error) {
	completion, digest, err := newSecret(32)
	if err != nil {
		return api.WorktreePurgePrepareResponse{}, err
	}
	plan := api.WorktreePurgePlan{ActionID: actionID, WorktreeID: record.WorktreeID,
		PrimaryPath: validation.PrimaryPath, RetiredPath: record.Path, GitIdentity: validation.GitIdentity,
		PathIdentity:  validation.Observation.PathIdentity,
		ApprovedLinks: validation.Observation.ApprovedLinks, ExpiresNs: now.Add(worktreePurgePlanLifetime).UnixNano(),
		Completion: completion}
	recovery := record
	recovery.Path = record.OriginalPath
	action := storage.WorktreeActionRecord{ActionID: actionID, WorktreeID: record.WorktreeID, Path: record.Path,
		Branch: record.Branch, RequestedNs: now.UnixNano(), UpdatedNs: now.UnixNano(), Result: "purging",
		Reason: "daemon committed foreground worktree finalization intent", EvidenceJSON: validationEvidence(validation),
		RecreateCommand: recreateWorktreeCommand(validation.PrimaryPath, recovery)}
	if err := d.beginWorktreeLifecycleAction(ctx, action, "worktree.purge.prepared"); err != nil {
		return api.WorktreePurgePrepareResponse{}, err
	}
	d.actionMu.Lock()
	for key, execution := range d.worktreePurgePlans {
		if execution.plan.ExpiresNs < now.UnixNano() {
			delete(d.worktreePurgePlans, key)
		}
	}
	if len(d.worktreePurgePlans) >= 1 {
		d.actionMu.Unlock()
		cause := errors.New("too many outstanding worktree purge plans")
		_ = d.store.WithTx(ctx, func(tx *storage.Tx) error {
			return tx.UpdateWorktreeAction(actionID, worktreeActionFailed, cause.Error(), action.EvidenceJSON, time.Now().UnixNano())
		})
		return api.WorktreePurgePrepareResponse{}, cause
	}
	d.worktreePurgePlans[digest] = &worktreePurgeExecution{plan: plan, record: record, validation: validation, action: action}
	d.actionMu.Unlock()
	response := worktreeActionResponse(action, "purging", action.Reason, action.EvidenceJSON, now)
	return api.WorktreePurgePrepareResponse{Action: response, Plan: plan}, nil
}

// WorktreePurgeComplete verifies native-removal results before committing them.
func (d *Daemon) WorktreePurgeComplete(ctx context.Context, req api.WorktreePurgeCompleteRequest) (api.WorktreeRemovalApplyResponse, error) {
	d.worktreeActionMu.Lock()
	defer d.worktreeActionMu.Unlock()
	execution, err := d.consumeWorktreePurgeExecution(req)
	if err != nil {
		return api.WorktreeRemovalApplyResponse{}, err
	}
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	expired := time.Now().UnixNano() >= execution.plan.ExpiresNs
	verifyErr := d.verifyRemovedWorktree(ctx, execution.plan.PrimaryPath, execution.plan.RetiredPath)
	if verifyErr != nil {
		if req.ExecutionError != "" || expired {
			current, validationErr := d.validateRetiredWorktree(ctx, execution.record, true)
			if validationErr == nil && sameValidation(execution.validation, current) {
				reason := "foreground native removal failed without verified mutation: " + req.ExecutionError
				if expired {
					reason = "foreground worktree purge plan expired without a verified mutation"
				}
				return d.finishFailedWorktreeAction(ctx, execution.action, reason, time.Now(), false, true)
			}
		}
		d.tripFilesystemCircuit("worktree purge completion became ambiguous: " + verifyErr.Error())
		return d.finishPartialWorktreeAction(ctx, execution.action, verifyErr.Error(), time.Now())
	}
	if expired {
		cause := errors.New("worktree became absent after the foreground purge plan expired")
		d.tripFilesystemCircuit("worktree purge completion became ambiguous: " + cause.Error())
		return d.finishPartialWorktreeAction(ctx, execution.action, cause.Error(), time.Now())
	}
	completed := time.Now()
	record := execution.record
	record.State, record.Registered, record.Complete = string(worktree.StateRemoved), false, false
	record.LastSeenNs, record.RemovedNs = completed.UnixNano(), pointer(completed.UnixNano())
	record.RecreateCommand = execution.action.RecreateCommand
	reason := "foreground native non-force removal completed; daemon verified path and registration absence"
	if err := d.finishWorktreeLifecycleAction(ctx, execution.action, record, worktreeActionRemoved, reason, completed); err != nil {
		return api.WorktreeRemovalApplyResponse{}, fmt.Errorf("worktree was removed but action %s is unresolved: %w", execution.plan.ActionID, err)
	}
	return worktreeActionResponse(execution.action, worktreeActionRemoved, reason, execution.action.EvidenceJSON, completed), nil
}

func (d *Daemon) consumeWorktreePurgeExecution(req api.WorktreePurgeCompleteRequest) (*worktreePurgeExecution, error) {
	if req.ActionID == "" || req.Completion == "" {
		return nil, errors.New("worktree purge completion requires action_id and completion capability")
	}
	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	execution := d.worktreePurgePlans[secretDigest(req.Completion)]
	if execution == nil || execution.plan.ActionID != req.ActionID || execution.used {
		return nil, errors.New("worktree purge completion is unknown, consumed, or lost after daemon restart")
	}
	execution.used = true
	delete(d.worktreePurgePlans, secretDigest(req.Completion))
	return execution, nil
}
