package daemon_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
)

func TestExplainSurfacesTheSessionGraph(t *testing.T) {
	ctx := context.Background()
	init := mk(1, 0, "/sbin/launchd", 0)
	editor := mk(500, 1, "/Applications/Editor.app/Contents/MacOS/Editor Helper", time.Second)
	root := codexRoot(100, 500, 2*time.Second)
	child := mk(200, 100, "/usr/local/bin/chrome-headless-shell", 3*time.Second)

	h := newHarness(t,
		snapshot(time.Minute, init, editor, root, child),
		snapshot(5*time.Minute, init, editor, withParent(child, 1)),
	)
	h.d.ScanNow(ctx)

	explain, err := h.d.Explain(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(explain.Relationships) == 0 {
		t.Fatal("explain must show why a process belongs, not only that it does")
	}
	if !explain.OriginalParentObserved || explain.OriginalPPID != 100 {
		t.Fatalf("creator = %d observed=%t, want pid 100 observed",
			explain.OriginalPPID, explain.OriginalParentObserved)
	}

	var attributing, context int
	for _, rel := range explain.Relationships {
		if rel.Attributing {
			attributing++
		} else {
			context++
		}
		if rel.Detail == "" {
			t.Fatalf("edge %q has no detail", rel.Kind)
		}
	}
	if attributing == 0 {
		t.Fatal("a descendant should have at least one ownership-establishing edge")
	}

	// After the root exits and the child is reparented, the reparenting is
	// recorded and the original-parent edge survives it.
	h.d.ScanNow(ctx)
	explain, err = h.d.Explain(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, rel := range explain.Relationships {
		kinds[rel.Kind] = true
	}
	if !kinds["reparenting"] {
		t.Fatalf("the reparenting must be visible: %+v", explain.Relationships)
	}
	if !kinds["original-parent"] {
		t.Fatal("the original-parent edge must survive the reparenting; it is the only record of who created the process")
	}
	if !explain.Protection.Protected {
		t.Fatal("a detached process must still be protected")
	}
}

func TestSessionDetailReportsLaunchContext(t *testing.T) {
	ctx := context.Background()
	init := mk(1, 0, "/sbin/launchd", 0)
	editor := mk(500, 1, "/Applications/Editor.app/Contents/MacOS/Editor Helper (Plugin)", time.Second)
	root := codexRoot(100, 500, 2*time.Second)

	h := newHarness(t, snapshot(time.Minute, init, editor, root))
	h.d.ScanNow(ctx)

	sessions, err := h.d.Sessions(ctx, api.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions.Sessions) != 1 {
		t.Fatalf("got %d sessions", len(sessions.Sessions))
	}
	if sessions.Sessions[0].LaunchedBy != "Editor Helper (Plugin)" {
		t.Fatalf("launched by = %q, want the editor helper", sessions.Sessions[0].LaunchedBy)
	}

	detail, err := h.d.Session(ctx, sessions.Sessions[0].SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Relationships) == 0 {
		t.Fatal("session detail must include the graph")
	}
	var sawLaunch bool
	for _, rel := range detail.Relationships {
		if rel.Kind == "launch" {
			sawLaunch = true
			if rel.ToPID != 500 {
				t.Fatalf("launch edge points at pid %d, want 500", rel.ToPID)
			}
		}
	}
	if !sawLaunch {
		t.Fatal("a launch edge must be recorded")
	}
}

func TestRepositoryAssociationIsRecorded(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "HEAD"), []byte("ref: refs/heads/session-graph\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	init := mk(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	root.CWD = repo

	h := newHarness(t, snapshot(time.Minute, init, root))
	h.d.ScanNow(ctx)

	sessions, err := h.d.Sessions(ctx, api.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := sessions.Sessions[0]
	if got.RepositoryPath != repo {
		t.Fatalf("repository = %q, want %q", got.RepositoryPath, repo)
	}
	if got.Branch != "session-graph" {
		t.Fatalf("branch = %q, want session-graph", got.Branch)
	}
	if got.RepositoryBusy {
		t.Fatal("no lock file exists, so the repository is not busy")
	}
}

func TestGitLockIsVisible(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "index.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	init := mk(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	root.CWD = repo

	h := newHarness(t, snapshot(time.Minute, init, root))
	h.d.ScanNow(ctx)

	sessions, err := h.d.Sessions(ctx, api.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !sessions.Sessions[0].RepositoryBusy {
		t.Fatal("a held git lock must be visible: section 16 forbids disturbing a process that holds one")
	}
}
