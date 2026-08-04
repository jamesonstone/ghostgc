package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGitInspectionProtectsSymlinkedWorktree(t *testing.T) {
	primary, secondary, git := testRepository(t)
	real := secondary + "-real"
	if err := os.Rename(secondary, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, secondary); err != nil {
		t.Fatal(err)
	}
	obs := observationFor(t, git, primary, secondary)
	if !hasProtection(obs, "worktree_path_symlinked") || obs.Complete {
		t.Fatalf("symlinked observation = %+v", obs)
	}
}

func TestGitInspectionProtectsUnreadableRegistration(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}
	primary, secondary, git := testRepository(t)
	records, err := git.Registrations(context.Background(), primary)
	if err != nil {
		t.Fatal(err)
	}
	var registration Registration
	for _, record := range records {
		if filepath.Clean(record.Path) == secondary {
			registration = record
		}
	}
	parent := filepath.Dir(secondary)
	if err := os.Chmod(parent, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	obs := git.Inspect(context.Background(), registration, primary)
	if !hasProtection(obs, "worktree_unreadable") || obs.Complete {
		t.Fatalf("unreadable observation = %+v", obs)
	}
}

func TestReachableDetachedWorktreeIsNotProtected(t *testing.T) {
	primary, _, git := testRepository(t)
	detached := filepath.Join(filepath.Dir(primary), "reachable-detached")
	runGit(t, primary, "worktree", "add", "--detach", detached, "HEAD")
	obs := observationFor(t, git, primary, detached)
	if !obs.Detached || !obs.DetachedReachable || len(obs.Protection) != 0 {
		t.Fatalf("reachable detached observation = %+v", obs)
	}
}
