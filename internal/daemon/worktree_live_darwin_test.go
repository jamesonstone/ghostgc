//go:build darwin

package daemon

import (
	"context"
	"os"
	"testing"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/platform"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// This opt-in test is the controlled macOS acceptance path. It uses the real
// same-user CWD/vnode inspector and retires only a disposable worktree.
func TestLiveDarwinWorktreeRetirement(t *testing.T) {
	if os.Getenv("GHOSTGC_LIVE_WORKTREE_TEST") != "1" {
		t.Skip("set GHOSTGC_LIVE_WORKTREE_TEST=1 for controlled local acceptance")
	}
	h := newRemovalHarness(t, false)
	live, err := platform.New(platform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	h.daemon.plat = live
	records, err := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{})
	if err != nil || len(records) != 1 {
		t.Fatalf("records = %+v, %v", records, err)
	}
	preview, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{
		WorktreeID: records[0].WorktreeID,
	})
	if err != nil {
		if _, statErr := os.Stat(h.secondary); statErr != nil {
			t.Fatalf("live refusal changed the worktree: %v", statErr)
		}
		if output := removalGit(t, h.primary, "show-ref", "--verify", "refs/heads/cleanup"); output == "" {
			t.Fatal("live refusal deleted the branch")
		}
		t.Logf("real path-usage evidence refused removal closed: %v", err)
		return
	}
	result, err := h.daemon.WorktreeRemovalApply(context.Background(), api.WorktreeRemovalApplyRequest{
		Approval: preview.Approval,
	})
	if err != nil || result.Result != worktreeActionRetired {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if _, err := os.Lstat(h.secondary); !os.IsNotExist(err) {
		t.Fatalf("secondary still exists: %v", err)
	}
	retired := h.secondary + ".ghostgc-retired-" + records[0].WorktreeID[:shortIDLength]
	if _, err := os.Lstat(retired); err != nil {
		t.Fatalf("retired checkout is unavailable: %v", err)
	}
	if output := removalGit(t, h.primary, "show-ref", "--verify", "refs/heads/cleanup"); output == "" {
		t.Fatal("branch was deleted")
	}
}
