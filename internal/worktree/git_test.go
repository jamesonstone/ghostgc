package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) string {
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

func testRepository(t *testing.T) (primary, secondary string, git *Git) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	remote := filepath.Join(root, "remote.git")
	primary = filepath.Join(root, "primary")
	secondary = filepath.Join(root, "secondary")
	if err := os.Mkdir(primary, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--bare", remote)
	runGit(t, primary, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(primary, "tracked"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(primary, ".gitignore"), []byte(".env\n.envrc\nignored\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, primary, "add", "tracked", ".gitignore")
	runGit(t, primary, "commit", "-m", "initial")
	runGit(t, primary, "remote", "add", "origin", remote)
	runGit(t, primary, "push", "-u", "origin", "main")
	runGit(t, primary, "worktree", "add", "-b", "cleanup", secondary, "origin/main")
	git, err = NewGit()
	if err != nil {
		t.Fatal(err)
	}
	return primary, secondary, git
}

func observationFor(t *testing.T, git *Git, repository, path string) Observation {
	t.Helper()
	records, err := git.Registrations(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if filepath.Clean(record.Path) == path {
			return git.Inspect(context.Background(), record, records[0].Path)
		}
	}
	t.Fatalf("registration %s not found", path)
	return Observation{}
}

func hasProtection(obs Observation, want string) bool {
	for _, reason := range obs.Protection {
		if reason == want {
			return true
		}
	}
	return false
}

func TestGitInspectionProtectsUnsafeState(t *testing.T) {
	primary, secondary, git := testRepository(t)
	clean := observationFor(t, git, primary, secondary)
	if !clean.Complete || len(clean.Protection) != 0 {
		t.Fatalf("clean secondary = %+v", clean)
	}
	if !hasProtection(observationFor(t, git, primary, primary), "primary_worktree") {
		t.Fatal("primary was not protected")
	}
	if err := os.WriteFile(filepath.Join(secondary, "untracked"), []byte("private contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasProtection(observationFor(t, git, primary, secondary), "worktree_dirty") {
		t.Fatal("untracked content was not protected")
	}
	if err := os.Remove(filepath.Join(secondary, "untracked")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondary, "ignored"), []byte("ignored contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasProtection(observationFor(t, git, primary, secondary), "worktree_dirty") {
		t.Fatal("ignored content was not protected")
	}
	if err := os.Remove(filepath.Join(secondary, "ignored")); err != nil {
		t.Fatal(err)
	}
	runGit(t, secondary, "commit", "--allow-empty", "-m", "local")
	if !hasProtection(observationFor(t, git, primary, secondary), "local_only_commits") {
		t.Fatal("local-only commit was not protected")
	}
}

func TestOnlyMatchingEnvironmentLinksAreApproved(t *testing.T) {
	primary, secondary, git := testRepository(t)
	for _, name := range []string{".env", ".envrc"} {
		if err := os.WriteFile(filepath.Join(primary, name), []byte("never read"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(primary, name), filepath.Join(secondary, name)); err != nil {
			t.Fatal(err)
		}
	}
	obs := observationFor(t, git, primary, secondary)
	if len(obs.ApprovedLinks) != 2 || !obs.Status.Clean() || hasProtection(obs, "worktree_dirty") {
		t.Fatalf("approved links = %+v, status = %+v, protection = %v", obs.ApprovedLinks, obs.Status, obs.Protection)
	}
	if err := os.Remove(filepath.Join(secondary, ".env")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondary, ".env"), []byte("local secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasProtection(observationFor(t, git, primary, secondary), "worktree_dirty") {
		t.Fatal("regular ignored environment file was approved")
	}
}

func TestGitInspectionProtectsOperationsLocksSubmodulesAndDetachedCommits(t *testing.T) {
	t.Run("operation", func(t *testing.T) {
		primary, secondary, git := testRepository(t)
		obs := observationFor(t, git, primary, secondary)
		if err := os.Mkdir(filepath.Join(obs.AdminGitDir, "rebase-merge"), 0o700); err != nil {
			t.Fatal(err)
		}
		if !hasProtection(observationFor(t, git, primary, secondary), "git_operation_in_progress") {
			t.Fatal("Git operation was not protected")
		}
	})
	t.Run("locked", func(t *testing.T) {
		primary, secondary, git := testRepository(t)
		runGit(t, primary, "worktree", "lock", secondary)
		if !hasProtection(observationFor(t, git, primary, secondary), "worktree_locked") {
			t.Fatal("locked worktree was not protected")
		}
	})
	t.Run("submodule metadata", func(t *testing.T) {
		primary, secondary, git := testRepository(t)
		if err := os.WriteFile(filepath.Join(secondary, ".gitmodules"), []byte("[submodule \"x\"]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		runGit(t, secondary, "add", ".gitmodules")
		if !hasProtection(observationFor(t, git, primary, secondary), "submodule_metadata_present") {
			t.Fatal("submodule metadata was not protected")
		}
	})
	t.Run("unreachable detached", func(t *testing.T) {
		primary, _, git := testRepository(t)
		detached := filepath.Join(filepath.Dir(primary), "detached")
		runGit(t, primary, "worktree", "add", "--detach", detached, "HEAD")
		runGit(t, detached, "commit", "--allow-empty", "-m", "detached-only")
		if !hasProtection(observationFor(t, git, primary, detached), "detached_head_unreachable") {
			t.Fatal("unreachable detached HEAD was not protected")
		}
	})
	t.Run("missing", func(t *testing.T) {
		primary, secondary, git := testRepository(t)
		if err := os.Rename(secondary, secondary+"-missing"); err != nil {
			t.Fatal(err)
		}
		obs := observationFor(t, git, primary, secondary)
		if obs.Present || obs.ID == "" || !hasProtection(obs, "worktree_missing") {
			t.Fatalf("missing observation = %+v", obs)
		}
	})
}

func TestGitInspectionCountsStagedTrackedAndConflictedContent(t *testing.T) {
	primary, secondary, git := testRepository(t)
	if err := os.WriteFile(filepath.Join(secondary, "tracked"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	tracked := observationFor(t, git, primary, secondary)
	if tracked.Status.Tracked != 1 || !hasProtection(tracked, "worktree_dirty") {
		t.Fatalf("tracked status = %+v", tracked.Status)
	}
	runGit(t, secondary, "add", "tracked")
	staged := observationFor(t, git, primary, secondary)
	if staged.Status.Staged != 1 || !hasProtection(staged, "worktree_dirty") {
		t.Fatalf("staged status = %+v", staged.Status)
	}
	// Porcelain v2 conflict parsing is separately proven without relying on a
	// merge implementation or repository user configuration.
	conflicted, err := ParseStatus([]byte("u UU N... 100644 100644 100644 100644 a b c conflict\x00"), nil)
	if err != nil || conflicted.Conflicted != 1 {
		t.Fatalf("conflicted status = %+v, %v", conflicted, err)
	}
}

func TestRecreatedWorktreeGetsNewStableIdentity(t *testing.T) {
	primary, secondary, git := testRepository(t)
	before := observationFor(t, git, primary, secondary)
	runGit(t, primary, "worktree", "remove", secondary)
	runGit(t, primary, "worktree", "add", secondary, "cleanup")
	after := observationFor(t, git, primary, secondary)
	if before.AdminGitDir != after.AdminGitDir {
		t.Fatalf("expected Git to reuse administrative path: %q != %q", before.AdminGitDir, after.AdminGitDir)
	}
	if before.ID == after.ID {
		t.Fatal("recreated worktree reused stable identity")
	}
}

func TestMovedWorktreeRetainsStableIdentity(t *testing.T) {
	primary, secondary, git := testRepository(t)
	before := observationFor(t, git, primary, secondary)
	moved := secondary + "-moved"
	runGit(t, primary, "worktree", "move", secondary, moved)
	after := observationFor(t, git, primary, moved)
	if before.ID != after.ID {
		t.Fatalf("moved worktree identity changed: %s != %s", before.ID, after.ID)
	}
}

func TestForbiddenGitMutationPathsAreAbsent(t *testing.T) {
	raw, err := os.ReadFile("git.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{
		"--force", "worktree\", \"prune", "branch\", \"-d", "branch\", \"-D",
		"fetch\"", "pull\"", "push\"", "ls-remote\"", "remote\", \"update", "rm -rf",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("git adapter contains forbidden mutation %q", forbidden)
		}
	}
}

func TestGitEnvironmentRemovesAmbientRepositoryAuthority(t *testing.T) {
	env := gitEnvironment([]string{
		"PATH=/usr/bin", "GIT_DIR=/tmp/wrong", "GIT_WORK_TREE=/tmp/wrong",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.fsmonitor",
		"GIT_CONFIG_VALUE_0=true", "KEEP=yes",
	})
	joined := strings.Join(env, "\n")
	for _, forbidden := range []string{
		"GIT_DIR=", "GIT_WORK_TREE=", "GIT_CONFIG_COUNT=", "GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("ambient Git authority remained: %s", forbidden)
		}
	}
	if !strings.Contains(joined, "KEEP=yes") || !strings.Contains(joined, "GIT_OPTIONAL_LOCKS=0") {
		t.Fatalf("expected ordinary and safety environment, got %q", joined)
	}
}
