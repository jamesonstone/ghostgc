//go:build darwin || linux

package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFinalizerRefusesApprovedLinkReplacement(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(filepath.Dir(root), "primary-env")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, ".env")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatal(err)
	}
	targetIdentity, err := Identify(target)
	if err != nil {
		t.Fatal(err)
	}
	pathIdentity, err := Identify(root)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := NewFinalizer(filepath.Join(filepath.Dir(root), "git-snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	finalizer.beforeUnlink = func() {
		if err := os.Remove(linkPath); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(linkPath, []byte("mission critical"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err = finalizer.Finalize(context.Background(), root, root, finalizer.git.Identity(), pathIdentity,
		[]ApprovedLink{{Name: ".env", LinkText: target, Target: targetIdentity}})
	if err == nil {
		t.Fatal("regular-file replacement reached native removal")
	}
	if body, readErr := os.ReadFile(linkPath); readErr != nil || string(body) != "mission critical" {
		t.Fatalf("replacement file changed: %q, %v", body, readErr)
	}
}
