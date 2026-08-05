package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

func (d *Daemon) issueRetiredWorktreeApproval(ctx context.Context, kind string,
	record storage.WorktreeRecord, validation worktreeValidation, command, note string) (api.WorktreeRemovalPreviewResponse, error) {
	if !d.filesystemMutationsHealthy() {
		return api.WorktreeRemovalPreviewResponse{}, errors.New("filesystem mutation circuit is open; restart the daemon and re-observe")
	}
	binding, err := worktreeBinding(record, validation, d.cfg.Worktrees)
	if err != nil {
		return api.WorktreeRemovalPreviewResponse{}, err
	}
	token, digest, err := newSecret(32)
	if err != nil {
		return api.WorktreeRemovalPreviewResponse{}, err
	}
	now := time.Now()
	approval := &worktreeApproval{kind: kind, bindingDigest: binding, record: record,
		validation: validation, expires: now.Add(approvalLifetime)}
	d.actionMu.Lock()
	d.pruneWorktreeApprovals(now)
	if len(d.worktreeApprovals) >= maxApprovals {
		d.actionMu.Unlock()
		return api.WorktreeRemovalPreviewResponse{}, errors.New("too many outstanding worktree approvals")
	}
	d.worktreeApprovals[digest] = approval
	d.actionMu.Unlock()
	if err := d.store.AppendAudit(ctx, storage.AuditRecord{TsNs: now.UnixNano(),
		Kind: "worktree." + kind + ".previewed", Subject: record.WorktreeID,
		Summary:      fmt.Sprintf("manual worktree %s preview issued; expires at %s", kind, approval.expires.Format(time.RFC3339)),
		EvidenceJSON: marshalJSON([]string{"binding " + binding}, "[]")}); err != nil {
		d.actionMu.Lock()
		delete(d.worktreeApprovals, digest)
		d.actionMu.Unlock()
		return api.WorktreeRemovalPreviewResponse{}, err
	}
	return api.WorktreeRemovalPreviewResponse{Approval: token, ExpiresNs: approval.expires.UnixNano(),
		Worktree: worktreeView(record, now), Command: fmt.Sprintf(command, token),
		Revalidation: []string{"exact registration and directory identity unchanged", "Git executable identity unchanged",
			"path usage and filesystem inspections remain complete", "destination remains absent"},
		Note: note}, nil
}

func (d *Daemon) beginWorktreeLifecycleAction(ctx context.Context, action storage.WorktreeActionRecord, auditKind string) error {
	return d.store.WithTx(ctx, func(tx *storage.Tx) error {
		if err := tx.InsertWorktreeAction(action); err != nil {
			return err
		}
		return tx.AppendAudit(storage.AuditRecord{TsNs: action.UpdatedNs, Kind: auditKind,
			Subject: action.WorktreeID, Summary: fmt.Sprintf("worktree action %s committed before mutation", action.ActionID),
			EvidenceJSON: action.EvidenceJSON})
	})
}

func (d *Daemon) finishWorktreeLifecycleAction(ctx context.Context, action storage.WorktreeActionRecord,
	record storage.WorktreeRecord, result, reason string, at time.Time) error {
	return d.store.WithTx(ctx, func(tx *storage.Tx) error {
		if err := tx.UpdateWorktreeAction(action.ActionID, result, reason, action.EvidenceJSON, at.UnixNano()); err != nil {
			return err
		}
		if err := tx.UpsertWorktree(record); err != nil {
			return err
		}
		return tx.AppendAudit(storage.AuditRecord{TsNs: at.UnixNano(), Kind: "worktree." + result,
			Subject: record.WorktreeID, Summary: fmt.Sprintf("worktree action %s completed as %s", action.ActionID, result),
			EvidenceJSON: action.EvidenceJSON})
	})
}

func (d *Daemon) finishPartialWorktreeAction(ctx context.Context, action storage.WorktreeActionRecord,
	reason string, at time.Time) (api.WorktreeRemovalApplyResponse, error) {
	if err := d.store.WithTx(ctx, func(tx *storage.Tx) error {
		if err := tx.UpdateWorktreeAction(action.ActionID, "partial", reason, action.EvidenceJSON, at.UnixNano()); err != nil {
			return err
		}
		return tx.AppendAudit(storage.AuditRecord{TsNs: at.UnixNano(), Kind: "worktree.lifecycle.partial",
			Subject: action.WorktreeID, Summary: reason, EvidenceJSON: action.EvidenceJSON})
	}); err != nil {
		return api.WorktreeRemovalApplyResponse{}, err
	}
	return worktreeActionResponse(action, "partial", reason, action.EvidenceJSON, at), nil
}
