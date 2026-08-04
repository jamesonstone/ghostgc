package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRootTraversalIsBoundedAndDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "one", "two", "repo")
	if err := os.MkdirAll(filepath.Join(want, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	boundary := filepath.Join(root, "one", "two", "three", "repo")
	if err := os.MkdirAll(filepath.Join(boundary, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	tooDeep := filepath.Join(root, "a", "b", "c", "d", "repo")
	if err := os.MkdirAll(filepath.Join(tooDeep, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	if err := os.MkdirAll(filepath.Join(external, "repo", ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	repositories, err := DiscoverRepositories(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repositories) != 2 || repositories[0] != want || repositories[1] != boundary {
		t.Fatalf("repositories = %v, want [%s %s]", repositories, want, boundary)
	}
}

func TestFilesystemInspectionDetectsNestedDevices(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "mounted")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	evidence, err := inspectFilesystem(context.Background(), root, func(path string) (FileIdentity, error) {
		device := uint64(1)
		if path == child {
			device = 2
		}
		return FileIdentity{Path: path, Device: device, Inode: 1}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.Complete || evidence.NestedMounts != 1 {
		t.Fatalf("evidence = %+v", evidence)
	}
}
