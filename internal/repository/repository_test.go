package repository

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func makeRepo(t *testing.T, head string) string {
	t.Helper()
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if head != "" {
		if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(head), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRootFindsEnclosingRepository(t *testing.T) {
	root := makeRepo(t, "ref: refs/heads/main\n")
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	f := NewFinder()
	if got := f.Root(deep); got != root {
		t.Fatalf("Root(%q) = %q, want %q", deep, got, root)
	}
	if got := f.Root(t.TempDir()); got != "" {
		t.Fatalf("Root of a plain directory = %q, want empty", got)
	}
	if got := f.Root("relative/path"); got != "" {
		t.Fatalf("Root of a relative path = %q, want empty", got)
	}
	if got := f.Root(""); got != "" {
		t.Fatalf("Root(\"\") = %q, want empty", got)
	}
}

func TestDescribeReadsBranch(t *testing.T) {
	root := makeRepo(t, "ref: refs/heads/feature/session-graph\n")
	info := NewFinder().Describe(root)

	if info.Root != root {
		t.Fatalf("Root = %q", info.Root)
	}
	if info.Branch != "feature/session-graph" {
		t.Fatalf("Branch = %q, want feature/session-graph", info.Branch)
	}
	if info.Detached {
		t.Fatal("a symbolic HEAD is not detached")
	}
	if info.Busy() {
		t.Fatal("no lock files exist, so the repository is not busy")
	}
}

func TestDescribeDetectsDetachedHead(t *testing.T) {
	root := makeRepo(t, "9c1e7b1f0d2a3b4c5d6e7f80912a3b4c5d6e7f80\n")
	info := NewFinder().Describe(root)

	if !info.Detached {
		t.Fatal("a bare object id in HEAD means a detached HEAD")
	}
	if info.Branch != "" {
		t.Fatalf("Branch = %q, want empty for a detached HEAD", info.Branch)
	}
}

// A held lock means a git operation is in flight. Section 16 forbids
// terminating a process holding one, so the daemon has to be able to see it.
func TestDescribeDetectsLocks(t *testing.T) {
	root := makeRepo(t, "ref: refs/heads/main\n")
	if err := os.WriteFile(filepath.Join(root, ".git", "index.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	info := NewFinder().Describe(root)
	if !info.Busy() {
		t.Fatal("index.lock means an operation is in flight")
	}
	if len(info.Locks) != 1 || info.Locks[0] != "index.lock" {
		t.Fatalf("Locks = %v", info.Locks)
	}
}

func TestDescribeFollowsWorktreePointer(t *testing.T) {
	realGitDir := filepath.Join(t.TempDir(), "worktrees", "wt1")
	if err := os.MkdirAll(realGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realGitDir, "HEAD"), []byte("ref: refs/heads/wt-branch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: "+realGitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	info := NewFinder().Describe(root)
	if info.Root != root {
		t.Fatalf("Root = %q, want the worktree directory", info.Root)
	}
	if info.Branch != "wt-branch" {
		t.Fatalf("Branch = %q, want the branch from the pointed-to git directory", info.Branch)
	}
}

func TestDescribeOfANonRepositoryIsZero(t *testing.T) {
	if got := NewFinder().Describe(t.TempDir()); got.Root != "" || got.Branch != "" {
		t.Fatalf("Describe of a plain directory = %+v, want the zero Info", got)
	}
}

// ghostgc records paths and metadata, never contents. Nothing this package
// returns may contain anything from the working tree.
func TestDescribeNeverReadsWorkingTreeContents(t *testing.T) {
	root := makeRepo(t, "ref: refs/heads/main\n")
	secret := "SUPER-SECRET-SOURCE-abc123"
	for _, name := range []string{"main.go", "README.md", ".env", "config.yaml"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(secret), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A file inside .git that is not plumbing ghostgc reads must also be left
	// alone.
	if err := os.WriteFile(filepath.Join(root, ".git", "COMMIT_EDITMSG"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}

	info := NewFinder().Describe(root)
	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("file contents leaked into repository metadata: %s", encoded)
	}
}

// A HEAD file that is not what it claims to be must not become a large
// allocation or a large stored string.
func TestOversizedHeadIsBounded(t *testing.T) {
	root := makeRepo(t, "")
	huge := "ref: refs/heads/" + strings.Repeat("x", 1<<20)
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte(huge), 0o644); err != nil {
		t.Fatal(err)
	}

	info := NewFinder().Describe(root)
	if len(info.Branch) > maxHeadBytes {
		t.Fatalf("branch name is %d bytes; the read must be capped at %d", len(info.Branch), maxHeadBytes)
	}
}

func TestDescribeIsCachedButExpires(t *testing.T) {
	root := makeRepo(t, "ref: refs/heads/main\n")
	f := NewFinder()
	now := time.Now()
	f.now = func() time.Time { return now }

	if got := f.Describe(root).Branch; got != "main" {
		t.Fatalf("Branch = %q", got)
	}

	// Branch changes on disk. Inside the TTL the cached answer stands.
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := f.Describe(root).Branch; got != "main" {
		t.Fatalf("Branch = %q inside the cache window, want the cached value", got)
	}

	// Past the TTL it is re-read. Branch and lock state change while ghostgc
	// is watching, so a permanent cache would report stale safety information.
	now = now.Add(infoTTL + time.Second)
	if got := f.Describe(root).Branch; got != "other" {
		t.Fatalf("Branch = %q after the cache expired, want the new value", got)
	}
}

func TestCachesAreBounded(t *testing.T) {
	f := NewFinder()
	for i := 0; i < maxCacheEntries+10; i++ {
		f.Root(filepath.Join("/nonexistent", "dir", strings.Repeat("a", i%8+1), string(rune('a'+i%26)), "x"))
	}
	if len(f.roots) > maxCacheEntries {
		t.Fatalf("root cache grew to %d entries, above the %d bound", len(f.roots), maxCacheEntries)
	}
}

func TestName(t *testing.T) {
	if got := Name("/Users/dev/src/labcore"); got != "labcore" {
		t.Fatalf("Name = %q", got)
	}
	if got := Name(""); got != "" {
		t.Fatalf("Name(\"\") = %q", got)
	}
}
