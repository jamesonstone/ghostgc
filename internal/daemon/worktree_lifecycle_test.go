package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/worktree"
)

func retireFixture(t *testing.T, environmentLink bool) (*removalHarness, storage.WorktreeRecord) {
	t.Helper()
	h := newRemovalHarness(t, environmentLink)
	records, err := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{})
	if err != nil || len(records) != 1 {
		t.Fatalf("worktree records = %+v, %v", records, err)
	}
	preview, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{
		WorktreeID: records[0].WorktreeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := h.daemon.WorktreeRemovalApply(context.Background(), api.WorktreeRemovalApplyRequest{Approval: preview.Approval})
	if err != nil || result.Result != worktreeActionRetired {
		t.Fatalf("retire = %+v, %v", result, err)
	}
	retired, err := h.store.GetWorktree(context.Background(), records[0].WorktreeID)
	if err != nil || retired.State != string(worktree.StateRetired) || retired.OriginalPath != h.secondary {
		t.Fatalf("retired record = %+v, %v", retired, err)
	}
	return h, retired
}

func TestRetiredWorktreeRestoresExactly(t *testing.T) {
	h, retired := retireFixture(t, true)
	preview, err := h.daemon.WorktreeRestorePreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: retired.WorktreeID})
	if err != nil || preview.Approval == "" {
		t.Fatalf("restore preview = %+v, %v", preview, err)
	}
	result, err := h.daemon.WorktreeRestoreApply(context.Background(), api.WorktreeRemovalApplyRequest{Approval: preview.Approval})
	if err != nil || result.Result != worktreeActionRestored {
		t.Fatalf("restore = %+v, %v", result, err)
	}
	if _, err := os.Lstat(h.secondary); err != nil {
		t.Fatalf("original checkout was not restored: %v", err)
	}
	if _, err := os.Lstat(retired.Path); !os.IsNotExist(err) {
		t.Fatalf("retirement path remains after restore: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(h.secondary, ".env")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("approved environment link did not survive retirement and restore: %v", err)
	}
}

func TestWorktreePurgeIsGraceGatedAndForegroundOnly(t *testing.T) {
	h, retired := retireFixture(t, false)
	canary := filepath.Join(h.root, "mission-critical-canary")
	if err := os.WriteFile(canary, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.daemon.WorktreePurgePreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: retired.WorktreeID}); err == nil {
		t.Fatal("purge preview ignored retirement grace")
	}
	retired.RetirementGraceNs = time.Now().Add(-time.Minute).UnixNano()
	if err := h.store.WithTx(context.Background(), func(tx *storage.Tx) error { return tx.UpsertWorktree(retired) }); err != nil {
		t.Fatal(err)
	}
	preview, err := h.daemon.WorktreePurgePreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: retired.WorktreeID})
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := h.daemon.WorktreePurgeApply(context.Background(), api.WorktreeRemovalApplyRequest{
		Approval: preview.Approval, Confirmation: "wrong",
	})
	if err != nil || rejected.Action.Result != worktreeActionRejected {
		t.Fatalf("wrong purge confirmation = %+v, %v", rejected, err)
	}
	preview, err = h.daemon.WorktreePurgePreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: retired.WorktreeID})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := h.daemon.WorktreePurgeApply(context.Background(), api.WorktreeRemovalApplyRequest{
		Approval: preview.Approval, Confirmation: retired.WorktreeID,
	})
	if err != nil || prepared.Action.Result != "purging" {
		t.Fatalf("purge prepare = %+v, %v", prepared, err)
	}
	if _, err := os.Lstat(retired.Path); err != nil {
		t.Fatalf("daemon executed permanent removal: %v", err)
	}
	finalizer, err := worktree.NewFinalizer(filepath.Join(h.root, "foreground-git"))
	if err != nil {
		t.Fatal(err)
	}
	if err := finalizer.Finalize(context.Background(), prepared.Plan.PrimaryPath, prepared.Plan.RetiredPath,
		prepared.Plan.GitIdentity, prepared.Plan.PathIdentity, prepared.Plan.ApprovedLinks); err != nil {
		t.Fatal(err)
	}
	result, err := h.daemon.WorktreePurgeComplete(context.Background(), api.WorktreePurgeCompleteRequest{
		ActionID: prepared.Plan.ActionID, Completion: prepared.Plan.Completion,
	})
	if err != nil || result.Result != worktreeActionRemoved {
		t.Fatalf("purge completion = %+v, %v", result, err)
	}
	if _, err := os.Lstat(canary); err != nil {
		t.Fatalf("out-of-scope canary changed: %v", err)
	}
	if _, err := h.daemon.WorktreePurgeComplete(context.Background(), api.WorktreePurgeCompleteRequest{
		ActionID: prepared.Plan.ActionID, Completion: prepared.Plan.Completion,
	}); err == nil {
		t.Fatal("purge completion capability replay succeeded")
	}
}

func TestAmbiguousRetirementTripsMutationCircuit(t *testing.T) {
	h := newRemovalHarness(t, false)
	records, _ := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{})
	preview, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{
		WorktreeID: records[0].WorktreeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	nativeMove := h.daemon.moveWorktree
	h.daemon.moveWorktree = func(ctx context.Context, repository, source, destination string) error {
		if err := nativeMove(ctx, repository, source, destination); err != nil {
			return err
		}
		return errUnavailable{}
	}
	result, err := h.daemon.WorktreeRemovalApply(context.Background(), api.WorktreeRemovalApplyRequest{Approval: preview.Approval})
	if err != nil || result.Result != "partial" {
		t.Fatalf("ambiguous retirement = %+v, %v", result, err)
	}
	if h.daemon.filesystemMutationsHealthy() {
		t.Fatal("ambiguous retirement left the mutation circuit closed")
	}
	if _, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{
		WorktreeID: records[0].WorktreeID,
	}); err == nil {
		t.Fatal("open mutation circuit issued new authority")
	}
}

func TestExpiredWorktreePurgeCannotCompleteSuccessfully(t *testing.T) {
	h, retired := retireFixture(t, false)
	retired.RetirementGraceNs = time.Now().Add(-time.Minute).UnixNano()
	if err := h.store.WithTx(context.Background(), func(tx *storage.Tx) error { return tx.UpsertWorktree(retired) }); err != nil {
		t.Fatal(err)
	}
	preview, err := h.daemon.WorktreePurgePreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: retired.WorktreeID})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := h.daemon.WorktreePurgeApply(context.Background(), api.WorktreeRemovalApplyRequest{
		Approval: preview.Approval, Confirmation: retired.WorktreeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := worktree.NewFinalizer(filepath.Join(h.root, "expired-foreground-git"))
	if err != nil {
		t.Fatal(err)
	}
	if err := finalizer.Finalize(context.Background(), prepared.Plan.PrimaryPath, prepared.Plan.RetiredPath,
		prepared.Plan.GitIdentity, prepared.Plan.PathIdentity, prepared.Plan.ApprovedLinks); err != nil {
		t.Fatal(err)
	}
	h.daemon.actionMu.Lock()
	h.daemon.worktreePurgePlans[secretDigest(prepared.Plan.Completion)].plan.ExpiresNs = time.Now().Add(-time.Second).UnixNano()
	h.daemon.actionMu.Unlock()
	result, err := h.daemon.WorktreePurgeComplete(context.Background(), api.WorktreePurgeCompleteRequest{
		ActionID: prepared.Plan.ActionID, Completion: prepared.Plan.Completion,
	})
	if err != nil || result.Result != "partial" || h.daemon.filesystemMutationsHealthy() {
		t.Fatalf("expired completion = %+v, %v, healthy=%v", result, err, h.daemon.filesystemMutationsHealthy())
	}
}
