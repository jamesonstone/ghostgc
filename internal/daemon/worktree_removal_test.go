package daemon

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform/platformtest"
	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/worktree"
)

func removalGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test User", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

type removalHarness struct {
	daemon                   *Daemon
	store                    *storage.Store
	fake                     *platformtest.Fake
	primary, secondary, root string
}

func newRemovalHarness(t *testing.T, environmentLink bool) *removalHarness {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	remote, primary, secondary := filepath.Join(root, "remote.git"), filepath.Join(root, "primary"), filepath.Join(root, "secondary")
	if err := os.Mkdir(primary, 0o700); err != nil {
		t.Fatal(err)
	}
	removalGit(t, root, "init", "--bare", remote)
	removalGit(t, primary, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(primary, "tracked"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(primary, ".gitignore"), []byte(".env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	removalGit(t, primary, "add", "tracked", ".gitignore")
	removalGit(t, primary, "commit", "-m", "initial")
	removalGit(t, primary, "remote", "add", "origin", remote)
	removalGit(t, primary, "push", "-u", "origin", "main")
	removalGit(t, primary, "worktree", "add", "-b", "cleanup", secondary, "origin/main")
	if environmentLink {
		if err := os.WriteFile(filepath.Join(primary, ".env"), []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(primary, ".env"), filepath.Join(secondary, ".env")); err != nil {
			t.Fatal(err)
		}
	}
	store, err := storage.Open(filepath.Join(root, "ghostgc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	fake := platformtest.New(501)
	fake.PlatformName = "darwin"
	fake.PathUsage.Complete = true
	cfg := config.Default()
	cfg.Worktrees.Roots = []string{root}
	d, err := New(Options{Config: cfg, Paths: config.Paths{Database: store.Path()}, Store: store,
		Platform: fake, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	record := staleWorktreeRecord(t, d, primary, secondary)
	if err := store.WithTx(context.Background(), func(tx *storage.Tx) error { return tx.UpsertWorktree(record) }); err != nil {
		t.Fatal(err)
	}
	return &removalHarness{daemon: d, store: store, fake: fake, primary: primary, secondary: secondary, root: root}
}

func staleWorktreeRecord(t *testing.T, d *Daemon, primary, secondary string) storage.WorktreeRecord {
	t.Helper()
	records, err := d.worktreeGit.Registrations(context.Background(), primary)
	if err != nil {
		t.Fatal(err)
	}
	var obs worktree.Observation
	for _, record := range records {
		if filepath.Clean(record.Path) == secondary {
			obs = d.worktreeGit.Inspect(context.Background(), record, records[0].Path)
		}
	}
	if obs.ID == "" || len(obs.Protection) != 0 {
		t.Fatalf("secondary observation = %+v", obs)
	}
	now := time.Now()
	evidence, _ := json.Marshal(map[string]any{"status": obs.Status})
	return storage.WorktreeRecord{
		WorktreeID: obs.ID, Path: obs.Path, PathDevice: obs.PathIdentity.Device, PathInode: obs.PathIdentity.Inode,
		CommonGitDir: obs.CommonGitDir, AdminGitDir: obs.AdminGitDir, HEAD: obs.HEAD, Ref: obs.Ref,
		Branch: obs.Branch, SourcesJSON: `["configured_root"]`, State: string(worktree.StateStale),
		FirstSeenNs: now.Add(-8 * 24 * time.Hour).UnixNano(), LastSeenNs: now.UnixNano(),
		LastActivityNs:  now.Add(-7*24*time.Hour - time.Minute).UnixNano(),
		InactiveSinceNs: now.Add(-7*24*time.Hour - time.Minute).UnixNano(), DaemonStartedNs: d.startedAt.UnixNano(),
		StatusFingerprint: obs.Status.Fingerprint, ProtectionJSON: `[]`, EvidenceJSON: string(evidence),
		ApprovedLinksJSON: marshalJSON(obs.ApprovedLinks, "[]"), GitIdentityJSON: marshalJSON(d.worktreeGit.Identity(), "{}"), Complete: true,
		Registered: true,
	}
}

func TestManualRetirementPreservesCheckoutBranchAndRejectsReplay(t *testing.T) {
	h := newRemovalHarness(t, false)
	nativeMove := h.daemon.moveWorktree
	h.daemon.moveWorktree = func(ctx context.Context, repository, source, destination string) error {
		actions, err := h.store.ListWorktreeActions(ctx, storage.WorktreeActionFilter{})
		if err != nil || len(actions) != 1 || actions[0].Result != worktreeActionAttempting {
			t.Fatalf("pre-side-effect action = %+v, %v", actions, err)
		}
		audit, err := h.store.ListAudit(ctx, storage.AuditFilter{Kind: "worktree.retirement.attempting"})
		if err != nil || len(audit) != 1 {
			t.Fatalf("pre-side-effect audit = %+v, %v", audit, err)
		}
		return nativeMove(ctx, repository, source, destination)
	}
	preview, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: ""})
	if err == nil || preview.Approval != "" {
		t.Fatal("empty identity should be refused")
	}
	records, _ := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{})
	preview, err = h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: records[0].WorktreeID[:8]})
	if err != nil || preview.Approval == "" {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	result, err := h.daemon.WorktreeRemovalApply(context.Background(), api.WorktreeRemovalApplyRequest{Approval: preview.Approval})
	if err != nil || result.Result != worktreeActionRetired {
		t.Fatalf("apply = %+v, %v", result, err)
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
	if !strings.Contains(result.RecreateCommand, "worktree restore") {
		t.Fatalf("restore command = %q", result.RecreateCommand)
	}
	replay, err := h.daemon.WorktreeRemovalApply(context.Background(), api.WorktreeRemovalApplyRequest{Approval: preview.Approval})
	if err != nil || replay.Result != worktreeActionRejected {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	actions, _ := h.store.ListWorktreeActions(context.Background(), storage.WorktreeActionFilter{})
	if len(actions) != 2 || actions[1].Result != worktreeActionRetired {
		t.Fatalf("actions = %+v", actions)
	}
}

func TestPreviewFailsClosedForUsageAndRestartLoss(t *testing.T) {
	h := newRemovalHarness(t, false)
	records, _ := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{})
	h.fake.PathUsage.CWDReferences = 1
	if _, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: records[0].WorktreeID}); err == nil {
		t.Fatal("active CWD was not refused")
	}
	h.fake.PathUsage.CWDReferences = 0
	h.fake.PathUsage.OpenVnodes = 1
	if _, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: records[0].WorktreeID}); err == nil {
		t.Fatal("open vnode was not refused")
	}
	h.fake.PathUsage.OpenVnodes = 0
	h.fake.PathUsageErr = errUnavailable{}
	if _, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: records[0].WorktreeID}); err == nil {
		t.Fatal("incomplete usage inspection was not refused")
	}
	h.fake.PathUsageErr = nil
	preview, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: records[0].WorktreeID})
	if err != nil {
		t.Fatal(err)
	}
	d2, err := New(Options{Config: config.Default(), Paths: config.Paths{Database: h.store.Path()}, Store: h.store,
		Platform: h.fake, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d2.WorktreeRemovalApply(context.Background(), api.WorktreeRemovalApplyRequest{Approval: preview.Approval}); err == nil {
		t.Fatal("approval survived daemon restart")
	}
}

type errUnavailable struct{}

func (errUnavailable) Error() string { return "inspection unavailable" }

func TestApprovalExpiryAndBoundFactInvalidation(t *testing.T) {
	t.Run("expiry", func(t *testing.T) {
		h := newRemovalHarness(t, false)
		records, _ := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{})
		preview, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: records[0].WorktreeID})
		if err != nil {
			t.Fatal(err)
		}
		h.daemon.actionMu.Lock()
		h.daemon.worktreeApprovals[secretDigest(preview.Approval)].expires = time.Now().Add(-time.Second)
		h.daemon.actionMu.Unlock()
		result, err := h.daemon.WorktreeRemovalApply(context.Background(), api.WorktreeRemovalApplyRequest{Approval: preview.Approval})
		if err != nil || result.Result != worktreeActionRejected {
			t.Fatalf("expired apply = %+v, %v", result, err)
		}
	})
	t.Run("status", func(t *testing.T) {
		h := newRemovalHarness(t, false)
		records, _ := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{})
		preview, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: records[0].WorktreeID})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(h.secondary, "new-data"), []byte("valuable"), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := h.daemon.WorktreeRemovalApply(context.Background(), api.WorktreeRemovalApplyRequest{Approval: preview.Approval})
		if err != nil || result.Result != worktreeActionRejected {
			t.Fatalf("changed status apply = %+v, %v", result, err)
		}
		if _, err := os.Stat(h.secondary); err != nil {
			t.Fatal("changed worktree was removed")
		}
	})
	t.Run("configured authority", func(t *testing.T) {
		h := newRemovalHarness(t, false)
		records, _ := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{})
		preview, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: records[0].WorktreeID})
		if err != nil {
			t.Fatal(err)
		}
		h.daemon.cfg.Worktrees.Roots = nil
		result, err := h.daemon.WorktreeRemovalApply(context.Background(), api.WorktreeRemovalApplyRequest{Approval: preview.Approval})
		if err != nil || result.Result != worktreeActionRejected {
			t.Fatalf("authority change apply = %+v, %v", result, err)
		}
	})
}

func TestRetirementFailureLeavesApprovedEnvironmentLink(t *testing.T) {
	h := newRemovalHarness(t, true)
	records, _ := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{})
	preview, err := h.daemon.WorktreeRemovalPreview(context.Background(), api.WorktreeRemovalPreviewRequest{WorktreeID: records[0].WorktreeID})
	if err != nil {
		t.Fatal(err)
	}
	h.daemon.moveWorktree = func(context.Context, string, string, string) error {
		return errUnavailable{}
	}
	result, err := h.daemon.WorktreeRemovalApply(context.Background(), api.WorktreeRemovalApplyRequest{Approval: preview.Approval})
	if err != nil {
		t.Fatal(err)
	}
	if result.Result != worktreeActionFailed {
		t.Fatalf("result = %s, want failed", result.Result)
	}
	if info, err := os.Lstat(filepath.Join(h.secondary, ".env")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("approved environment link was not restored: %v", err)
	}
}
