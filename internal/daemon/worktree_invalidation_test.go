package daemon

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

func previewRemoval(t *testing.T, h *removalHarness) (storage.WorktreeRecord, api.WorktreeRemovalPreviewResponse) {
	t.Helper()
	records, err := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{})
	if err != nil || len(records) != 1 {
		t.Fatalf("records = %+v, %v", records, err)
	}
	preview, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{
		WorktreeID: records[0].WorktreeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return records[0], preview
}

func TestPostRemovalPersistenceFailureLeavesAttemptingRecord(t *testing.T) {
	h := newRemovalHarness(t, false)
	_, preview := previewRemoval(t, h)
	databasePath := h.store.Path()
	nativeRemove := h.daemon.removeWorktree
	h.daemon.removeWorktree = func(ctx context.Context, repository, path string) error {
		if err := nativeRemove(ctx, repository, path); err != nil {
			return err
		}
		return h.store.Close()
	}
	_, err := h.daemon.WorktreeRemovalApply(context.Background(), api.WorktreeRemovalApplyRequest{
		Approval: preview.Approval,
	})
	if err == nil || !strings.Contains(err.Error(), "remains unresolved as attempting") {
		t.Fatalf("post-removal persistence error = %v", err)
	}
	if _, statErr := os.Lstat(h.secondary); !os.IsNotExist(statErr) {
		t.Fatalf("secondary still exists: %v", statErr)
	}
	reopened, openErr := storage.Open(databasePath)
	if openErr != nil {
		t.Fatal(openErr)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	actions, listErr := reopened.ListWorktreeActions(context.Background(), storage.WorktreeActionFilter{})
	if listErr != nil || len(actions) != 1 || actions[0].Result != worktreeActionAttempting {
		t.Fatalf("unresolved action = %+v, %v", actions, listErr)
	}
	if output := removalGit(t, h.primary, "show-ref", "--verify", "refs/heads/cleanup"); output == "" {
		t.Fatal("branch was deleted")
	}
}

func assertRejectedApproval(t *testing.T, h *removalHarness, preview api.WorktreeRemovalPreviewResponse) {
	t.Helper()
	result, err := h.daemon.WorktreeRemovalApply(context.Background(), api.WorktreeRemovalApplyRequest{
		Approval: preview.Approval,
	})
	if err != nil || result.Result != worktreeActionRejected {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestApprovalInvalidatesEveryMutableAuthorityClass(t *testing.T) {
	t.Run("HEAD and ref", func(t *testing.T) {
		h := newRemovalHarness(t, false)
		_, preview := previewRemoval(t, h)
		removalGit(t, h.secondary, "commit", "--allow-empty", "-m", "changed-head")
		assertRejectedApproval(t, h, preview)
	})
	t.Run("directory inode", func(t *testing.T) {
		h := newRemovalHarness(t, false)
		record, preview := previewRemoval(t, h)
		record.PathInode++
		if err := h.store.WithTx(context.Background(), func(tx *storage.Tx) error {
			return tx.UpsertWorktree(record)
		}); err != nil {
			t.Fatal(err)
		}
		assertRejectedApproval(t, h, preview)
	})
	t.Run("registration", func(t *testing.T) {
		h := newRemovalHarness(t, false)
		_, preview := previewRemoval(t, h)
		removalGit(t, h.primary, "worktree", "lock", h.secondary)
		assertRejectedApproval(t, h, preview)
	})
	t.Run("process usage", func(t *testing.T) {
		h := newRemovalHarness(t, false)
		_, preview := previewRemoval(t, h)
		h.fake.PathUsage.OpenVnodes = 1
		assertRejectedApproval(t, h, preview)
	})
	t.Run("Git executable evidence", func(t *testing.T) {
		h := newRemovalHarness(t, false)
		record, preview := previewRemoval(t, h)
		record.GitIdentityJSON = `{"path":"different"}`
		if err := h.store.WithTx(context.Background(), func(tx *storage.Tx) error {
			return tx.UpsertWorktree(record)
		}); err != nil {
			t.Fatal(err)
		}
		assertRejectedApproval(t, h, preview)
	})
}
