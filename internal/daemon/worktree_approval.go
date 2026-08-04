package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

type worktreeApproval struct {
	bindingDigest string
	record        storage.WorktreeRecord
	validation    worktreeValidation
	expires       time.Time
	used          bool
}

// WorktreeRemovalPreview issues one memory-only approval after fresh checks.
func (d *Daemon) WorktreeRemovalPreview(ctx context.Context, req api.WorktreeRemovalPreviewRequest) (api.WorktreeRemovalPreviewResponse, error) {
	if req.WorktreeID == "" {
		return api.WorktreeRemovalPreviewResponse{}, errors.New("worktree removal preview requires worktree_id")
	}
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	record, err := d.store.GetWorktree(ctx, req.WorktreeID)
	if err != nil {
		return api.WorktreeRemovalPreviewResponse{}, err
	}
	validation, err := d.validateWorktreeRemoval(ctx, record)
	if err != nil {
		return api.WorktreeRemovalPreviewResponse{}, err
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
	approval := &worktreeApproval{bindingDigest: binding, record: record, validation: validation, expires: now.Add(approvalLifetime)}
	d.actionMu.Lock()
	d.pruneWorktreeApprovals(now)
	if len(d.worktreeApprovals) >= maxApprovals {
		d.actionMu.Unlock()
		return api.WorktreeRemovalPreviewResponse{}, errors.New("too many outstanding worktree approvals; wait for an earlier preview to expire")
	}
	d.worktreeApprovals[digest] = approval
	d.actionMu.Unlock()
	evidence := marshalJSON([]map[string]any{{
		"rule": "worktree-removal-approval-v1", "binding_digest": binding,
		"expires_ns": approval.expires.UnixNano(), "worktree_id": record.WorktreeID,
	}}, "[]")
	if err := d.store.AppendAudit(ctx, storage.AuditRecord{
		TsNs: now.UnixNano(), Kind: "worktree.removal.previewed", Subject: record.WorktreeID,
		Summary:      fmt.Sprintf("manual worktree removal preview issued; approval expires at %s", approval.expires.Format(time.RFC3339)),
		EvidenceJSON: evidence,
	}); err != nil {
		d.actionMu.Lock()
		delete(d.worktreeApprovals, digest)
		d.actionMu.Unlock()
		return api.WorktreeRemovalPreviewResponse{}, err
	}
	return api.WorktreeRemovalPreviewResponse{
		Approval: token, ExpiresNs: approval.expires.UnixNano(), Worktree: worktreeView(record, now),
		Command: "ghostgc worktree remove --apply --approval " + token + " --yes",
		Revalidation: []string{
			"registered secondary identity, canonical paths and directory inodes unchanged",
			"HEAD, ref, branch, aggregate status and publication evidence unchanged",
			"seven-day continuous inactivity evidence and configured authority unchanged",
			"all same-user CWD and vnode usage freshly inspected with no references",
			"no nested mounts, operations, submodules, dirty content or unsafe symlinks",
			"the exact resolved Git executable is unchanged",
		},
		Note: "No filesystem change has occurred. This approval is single-use, memory-only, and expires in two minutes; the branch will remain.",
	}, nil
}

func (d *Daemon) consumeWorktreeApproval(token string, now time.Time) (*worktreeApproval, string) {
	digest := secretDigest(token)
	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	approval := d.worktreeApprovals[digest]
	if approval == nil {
		return nil, "approval is unknown or was lost when the daemon restarted"
	}
	if approval.used {
		return approval, "approval has already been consumed"
	}
	approval.used = true
	if !now.Before(approval.expires) {
		return approval, "approval has expired"
	}
	return approval, ""
}

func (d *Daemon) pruneWorktreeApprovals(now time.Time) {
	for digest, approval := range d.worktreeApprovals {
		if approval.expires.Add(approvalLifetime).Before(now) {
			delete(d.worktreeApprovals, digest)
		}
	}
}

func worktreeBinding(record storage.WorktreeRecord, validation worktreeValidation, authority any) (string, error) {
	raw, err := json.Marshal(struct {
		Identity   worktreeBindingIdentity `json:"identity"`
		Validation worktreeValidation      `json:"validation"`
		Authority  any                     `json:"authority"`
	}{worktreeBindingIdentity{
		ID: record.WorktreeID, Path: record.Path, HEAD: record.HEAD, Ref: record.Ref,
		Branch: record.Branch, SourcesJSON: record.SourcesJSON,
		LastActivityNs: record.LastActivityNs, InactiveSinceNs: record.InactiveSinceNs,
		DaemonStartedNs: record.DaemonStartedNs,
	}, validation, authority})
	if err != nil {
		return "", err
	}
	return secretDigest(string(raw)), nil
}

type worktreeBindingIdentity struct {
	ID, Path, HEAD, Ref, Branch, SourcesJSON         string
	LastActivityNs, InactiveSinceNs, DaemonStartedNs int64
}
