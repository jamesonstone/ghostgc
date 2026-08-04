package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/worktree"
)

const (
	worktreeActionAttempting = "attempting"
	worktreeActionRemoved    = "removed"
	worktreeActionRejected   = "rejected"
	worktreeActionFailed     = "failed"
)

// WorktreeRemovalApply consumes one approval and repeats every safety check.
func (d *Daemon) WorktreeRemovalApply(ctx context.Context, req api.WorktreeRemovalApplyRequest) (api.WorktreeRemovalApplyResponse, error) {
	if req.Approval == "" {
		return api.WorktreeRemovalApplyResponse{}, errors.New("worktree removal apply requires an approval")
	}
	d.worktreeActionMu.Lock()
	defer d.worktreeActionMu.Unlock()
	now := time.Now()
	approval, refusal := d.consumeWorktreeApproval(req.Approval, now)
	if approval == nil {
		return api.WorktreeRemovalApplyResponse{}, errors.New(refusal)
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
	if err != nil {
		return d.rejectWorktreeAction(ctx, actionID, approval, err.Error(), time.Now())
	}
	if !sameWorktreeAuthority(approval.record, current) {
		return d.rejectWorktreeAction(ctx, actionID, approval, "bound inactivity or inventory authority changed", time.Now())
	}
	validation, err := d.validateWorktreeRemoval(ctx, current)
	if err != nil {
		return d.rejectWorktreeAction(ctx, actionID, approval, err.Error(), time.Now())
	}
	binding, err := worktreeBinding(current, validation, d.cfg.Worktrees)
	if err != nil {
		return api.WorktreeRemovalApplyResponse{}, err
	}
	if binding != approval.bindingDigest || !sameValidation(approval.validation, validation) {
		return d.rejectWorktreeAction(ctx, actionID, approval, "a fact bound to the preview changed", time.Now())
	}
	return d.executeWorktreeRemoval(ctx, actionID, current, validation, now)
}

func (d *Daemon) executeWorktreeRemoval(ctx context.Context, actionID string,
	record storage.WorktreeRecord, validation worktreeValidation, requested time.Time) (api.WorktreeRemovalApplyResponse, error) {
	evidence := validationEvidence(validation)
	attempt := storage.WorktreeActionRecord{
		ActionID: actionID, WorktreeID: record.WorktreeID, Path: record.Path, Branch: record.Branch,
		RequestedNs: requested.UnixNano(), UpdatedNs: time.Now().UnixNano(), Result: worktreeActionAttempting,
		Reason:       "full fresh revalidation passed; native non-force worktree removal is about to be attempted",
		EvidenceJSON: evidence, RecreateCommand: validation.RecreateCommand,
	}
	if err := d.store.WithTx(ctx, func(tx *storage.Tx) error {
		if err := tx.InsertWorktreeAction(attempt); err != nil {
			return err
		}
		return tx.AppendAudit(storage.AuditRecord{
			TsNs: attempt.UpdatedNs, Kind: "worktree.removal.attempting", Subject: record.WorktreeID,
			Summary:      fmt.Sprintf("worktree action %s committed before native non-force removal", actionID),
			EvidenceJSON: evidence,
		})
	}); err != nil {
		return api.WorktreeRemovalApplyResponse{}, err
	}
	links := validation.Observation.ApprovedLinks
	removedLinks, sideEffectErr := unlinkApprovedLinks(record.Path, links)
	if sideEffectErr == nil {
		sideEffectErr = d.removeWorktree(ctx, validation.PrimaryPath, record.Path)
	}
	if sideEffectErr != nil {
		restoreErr := restoreApprovedLinks(record.Path, removedLinks)
		reason := "native non-force worktree removal failed: " + sideEffectErr.Error()
		if restoreErr != nil {
			reason += "; restoring approved environment links also failed: " + restoreErr.Error()
		}
		return d.finishFailedWorktreeAction(ctx, attempt, reason, time.Now())
	}
	if err := d.verifyWorktreeAbsent(ctx, validation.PrimaryPath, record.Path); err != nil {
		return d.finishFailedWorktreeAction(ctx, attempt, "post-removal verification failed: "+err.Error(), time.Now())
	}
	completed := time.Now()
	reason := "registered worktree and directory were removed; the branch remains available"
	record.State = string(worktree.StateRemoved)
	record.LastSeenNs = completed.UnixNano()
	record.Complete = false
	record.RemovedNs = pointer(completed.UnixNano())
	record.RecreateCommand = validation.RecreateCommand
	record.ProtectionJSON = `[]`
	record.EvidenceJSON = evidence
	if err := d.store.WithTx(ctx, func(tx *storage.Tx) error {
		if err := tx.UpdateWorktreeAction(actionID, worktreeActionRemoved, reason, evidence, completed.UnixNano()); err != nil {
			return err
		}
		if err := tx.UpsertWorktree(record); err != nil {
			return err
		}
		return tx.AppendAudit(storage.AuditRecord{
			TsNs: completed.UnixNano(), Kind: "worktree.removal.removed", Subject: record.WorktreeID,
			Summary:      fmt.Sprintf("worktree action %s removed the checkout; branch %s remains", actionID, record.Branch),
			EvidenceJSON: evidence,
		})
	}); err != nil {
		return api.WorktreeRemovalApplyResponse{}, fmt.Errorf("worktree was removed but durable action %s remains unresolved as attempting: %w; the branch remains; recreate with: %s", actionID, err, validation.RecreateCommand)
	}
	return worktreeActionResponse(attempt, worktreeActionRemoved, reason, evidence, completed), nil
}

func (d *Daemon) finishFailedWorktreeAction(ctx context.Context, attempt storage.WorktreeActionRecord,
	reason string, at time.Time) (api.WorktreeRemovalApplyResponse, error) {
	evidence := attempt.EvidenceJSON
	if err := d.store.WithTx(ctx, func(tx *storage.Tx) error {
		if err := tx.UpdateWorktreeAction(attempt.ActionID, worktreeActionFailed, reason, evidence, at.UnixNano()); err != nil {
			return err
		}
		return tx.AppendAudit(storage.AuditRecord{
			TsNs: at.UnixNano(), Kind: "worktree.removal.failed", Subject: attempt.WorktreeID,
			Summary: fmt.Sprintf("worktree action %s failed: %s", attempt.ActionID, reason), EvidenceJSON: evidence,
		})
	}); err != nil {
		return api.WorktreeRemovalApplyResponse{}, fmt.Errorf("worktree action %s remains unresolved as attempting: %w", attempt.ActionID, err)
	}
	return worktreeActionResponse(attempt, worktreeActionFailed, reason, evidence, at), nil
}

func (d *Daemon) rejectWorktreeAction(ctx context.Context, actionID string, approval *worktreeApproval,
	reason string, at time.Time) (api.WorktreeRemovalApplyResponse, error) {
	evidence := marshalJSON([]map[string]string{{"rule": "worktree-removal-refusal-v1", "detail": reason}}, "[]")
	record := storage.WorktreeActionRecord{
		ActionID: actionID, WorktreeID: approval.record.WorktreeID, Path: approval.record.Path,
		Branch: approval.record.Branch, RequestedNs: at.UnixNano(), UpdatedNs: at.UnixNano(),
		Result: worktreeActionRejected, Reason: reason, EvidenceJSON: evidence,
		RecreateCommand: approval.validation.RecreateCommand,
	}
	if err := d.store.WithTx(ctx, func(tx *storage.Tx) error {
		if err := tx.InsertWorktreeAction(record); err != nil {
			return err
		}
		return tx.AppendAudit(storage.AuditRecord{
			TsNs: at.UnixNano(), Kind: "worktree.removal.rejected", Subject: record.WorktreeID,
			Summary: fmt.Sprintf("worktree action %s rejected before removal: %s", actionID, reason), EvidenceJSON: evidence,
		})
	}); err != nil {
		return api.WorktreeRemovalApplyResponse{}, err
	}
	return worktreeActionResponse(record, worktreeActionRejected, reason, evidence, at), nil
}

func newWorktreeActionID() (string, error) {
	token, _, err := newSecret(12)
	if err != nil {
		return "", err
	}
	return "wta_" + token, nil
}

func sameWorktreeAuthority(a, b storage.WorktreeRecord) bool {
	return a.WorktreeID == b.WorktreeID && a.Path == b.Path && a.HEAD == b.HEAD && a.Ref == b.Ref &&
		a.Branch == b.Branch && a.SourcesJSON == b.SourcesJSON && a.LastActivityNs == b.LastActivityNs &&
		a.InactiveSinceNs == b.InactiveSinceNs && a.DaemonStartedNs == b.DaemonStartedNs
}

func sameValidation(a, b worktreeValidation) bool { return reflect.DeepEqual(a, b) }

func validationEvidence(validation worktreeValidation) string {
	raw, err := json.Marshal(validation)
	if err != nil {
		return `{}`
	}
	return string(raw)
}

func worktreeActionResponse(record storage.WorktreeActionRecord, result, reason, evidence string, at time.Time) api.WorktreeRemovalApplyResponse {
	return api.WorktreeRemovalApplyResponse{
		ActionID: record.ActionID, WorktreeID: record.WorktreeID, Path: record.Path,
		Branch: record.Branch, Result: result, AtNs: at.UnixNano(), Reason: reason,
		Evidence: json.RawMessage(evidence), RecreateCommand: record.RecreateCommand,
	}
}

func recreateWorktreeCommand(primary string, record storage.WorktreeRecord) string {
	if record.Branch != "" {
		return "git -C " + shellQuote(primary) + " worktree add " + shellQuote(record.Path) + " " + shellQuote(record.Branch)
	}
	return "git -C " + shellQuote(primary) + " worktree add --detach " + shellQuote(record.Path) + " " + shellQuote(record.HEAD)
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func unlinkApprovedLinks(root string, links []worktree.ApprovedLink) ([]worktree.ApprovedLink, error) {
	if err := worktree.ValidateApprovedLinks(root, links); err != nil {
		return nil, err
	}
	var removed []worktree.ApprovedLink
	for _, link := range links {
		if err := os.Remove(filepath.Join(root, link.Name)); err != nil {
			return removed, err
		}
		removed = append(removed, link)
	}
	return removed, nil
}

func restoreApprovedLinks(root string, links []worktree.ApprovedLink) error {
	var failures []error
	for _, link := range links {
		path := filepath.Join(root, link.Name)
		if _, err := os.Lstat(path); err == nil {
			failures = append(failures, fmt.Errorf("approved %s destination was recreated", link.Name))
			continue
		}
		if err := os.Symlink(link.LinkText, path); err != nil {
			failures = append(failures, fmt.Errorf("restoring approved %s link: %w", link.Name, err))
		}
	}
	return errors.Join(failures...)
}

func (d *Daemon) verifyWorktreeAbsent(ctx context.Context, repository, path string) error {
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return errors.New("worktree directory is still present or unreadable")
	}
	records, err := d.worktreeGit.Registrations(ctx, repository)
	if err != nil {
		return err
	}
	for _, record := range records {
		if filepath.Clean(record.Path) == path {
			return errors.New("worktree remains registered")
		}
	}
	return nil
}

func pointer[T any](value T) *T { return &value }
