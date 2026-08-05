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

// WorktreeRestorePreview issues separate authority to reverse retirement.
func (d *Daemon) WorktreeRestorePreview(ctx context.Context, req api.WorktreeRemovalPreviewRequest) (api.WorktreeRemovalPreviewResponse, error) {
	if req.WorktreeID == "" {
		return api.WorktreeRemovalPreviewResponse{}, errors.New("worktree restore preview requires worktree_id")
	}
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	record, err := d.store.GetWorktree(ctx, req.WorktreeID)
	if err != nil {
		return api.WorktreeRemovalPreviewResponse{}, err
	}
	validation, err := d.validateRetiredWorktree(ctx, record, false)
	if err != nil {
		return api.WorktreeRemovalPreviewResponse{}, err
	}
	return d.issueRetiredWorktreeApproval(ctx, "restore", record, validation,
		"ghostgc worktree restore --apply --approval %s --yes",
		"Apply moves the exact registered checkout back to its absent original path.")
}

// WorktreeRestoreApply consumes one restoration approval and revalidates it.
func (d *Daemon) WorktreeRestoreApply(ctx context.Context, req api.WorktreeRemovalApplyRequest) (api.WorktreeRemovalApplyResponse, error) {
	return d.applyRetiredWorktreeMove(ctx, req, "restore")
}

func (d *Daemon) applyRetiredWorktreeMove(ctx context.Context, req api.WorktreeRemovalApplyRequest,
	kind string) (api.WorktreeRemovalApplyResponse, error) {
	if req.Approval == "" {
		return api.WorktreeRemovalApplyResponse{}, errors.New("worktree restore apply requires an approval")
	}
	d.worktreeActionMu.Lock()
	defer d.worktreeActionMu.Unlock()
	now := time.Now()
	approval, refusal := d.consumeWorktreeApproval(req.Approval, now)
	if approval == nil {
		return api.WorktreeRemovalApplyResponse{}, errors.New(refusal)
	}
	if approval.kind != kind {
		refusal = "approval action does not match worktree restore"
	}
	actionID, err := newWorktreeActionID()
	if err != nil {
		return api.WorktreeRemovalApplyResponse{}, err
	}
	if refusal != "" {
		return d.rejectWorktreeAction(ctx, actionID, approval, refusal, now)
	}
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	current, err := d.store.GetWorktree(ctx, approval.record.WorktreeID)
	if err != nil || !sameRetiredAuthority(approval.record, current) {
		return d.rejectWorktreeAction(ctx, actionID, approval, "bound retirement authority changed", time.Now())
	}
	validation, err := d.validateRetiredWorktree(ctx, current, false)
	if err != nil {
		return d.rejectWorktreeAction(ctx, actionID, approval, err.Error(), time.Now())
	}
	binding, err := worktreeBinding(current, validation, d.cfg.Worktrees)
	if err != nil {
		return api.WorktreeRemovalApplyResponse{}, err
	}
	if binding != approval.bindingDigest || !sameValidation(approval.validation, validation) {
		return d.rejectWorktreeAction(ctx, actionID, approval, "a fact bound to the restore preview changed", time.Now())
	}
	return d.executeWorktreeRestore(ctx, actionID, current, validation, now)
}

func (d *Daemon) executeWorktreeRestore(ctx context.Context, actionID string, record storage.WorktreeRecord,
	validation worktreeValidation, requested time.Time) (api.WorktreeRemovalApplyResponse, error) {
	attempt := storage.WorktreeActionRecord{ActionID: actionID, WorktreeID: record.WorktreeID, Path: record.Path,
		Branch: record.Branch, RequestedNs: requested.UnixNano(), UpdatedNs: time.Now().UnixNano(),
		Result: worktreeActionAttempting, Reason: "fresh restore checks passed; reversible worktree move is about to run",
		EvidenceJSON: validationEvidence(validation), RecreateCommand: record.RecreateCommand}
	if err := d.beginWorktreeLifecycleAction(ctx, attempt, "worktree.restore.attempting"); err != nil {
		return api.WorktreeRemovalApplyResponse{}, err
	}
	original := record.OriginalPath
	if err := d.moveWorktree(ctx, validation.PrimaryPath, record.Path, original); err != nil {
		if ambiguousWorktreeMove(record.Path, original) {
			d.tripFilesystemCircuit("worktree restore failed with ambiguous path state: " + err.Error())
			return d.finishPartialWorktreeAction(ctx, attempt,
				"native worktree restore returned an error after an ambiguous path change: "+err.Error(), time.Now())
		}
		return d.finishFailedWorktreeAction(ctx, attempt, "native worktree restore failed: "+err.Error(), time.Now(), false, true)
	}
	if err := d.verifyMovedWorktree(ctx, validation.PrimaryPath, record, original); err != nil {
		d.tripFilesystemCircuit("worktree restore verification failed: " + err.Error())
		return d.finishPartialWorktreeAction(ctx, attempt, "post-restore verification failed: "+err.Error(), time.Now())
	}
	completed := time.Now()
	record.Path, record.OriginalPath = original, ""
	record.State, record.Complete = string(worktree.StateObserving), false
	record.LastSeenNs, record.LastActivityNs = completed.UnixNano(), completed.UnixNano()
	record.InactiveSinceNs, record.RetirementGraceNs = 0, 0
	record.RetiredNs, record.RemovedNs = nil, nil
	record.RecreateCommand, record.ProtectionJSON = "", `["fresh_observation_required"]`
	reason := "retired checkout was restored to its exact original path"
	if err := d.finishWorktreeLifecycleAction(ctx, attempt, record, worktreeActionRestored, reason, completed); err != nil {
		return api.WorktreeRemovalApplyResponse{}, fmt.Errorf("worktree was restored but durable action %s is unresolved: %w", actionID, err)
	}
	return worktreeActionResponse(attempt, worktreeActionRestored, reason, attempt.EvidenceJSON, completed), nil
}

func sameRetiredAuthority(a, b storage.WorktreeRecord) bool {
	return sameWorktreeAuthority(a, b) && a.OriginalPath == b.OriginalPath &&
		a.RetirementGraceNs == b.RetirementGraceNs && a.RetiredNs != nil && b.RetiredNs != nil && *a.RetiredNs == *b.RetiredNs
}
