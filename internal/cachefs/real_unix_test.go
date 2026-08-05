//go:build darwin || linux

package cachefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

func TestRealFilesystemQuarantineRestoreAndPurge(t *testing.T) {
	root := filepath.Join(canonicalTemp(t), "shell_snapshots")
	mustMkdir(t, root)
	name := "019fcde3-594a-7eb1-a102-ee8c7893c2dc.1.sh"
	mustWrite(t, filepath.Join(root, name))
	real := New()
	snapshot := mustSnapshot(t, real, root)

	moved, err := real.Quarantine(context.Background(), root, name, "ca_test", snapshot.Root, snapshot.Entries[0].Identity)
	if err != nil {
		t.Fatal(err)
	}
	qpath := filepath.Join(cacheartifact.QuarantineDirectory, "ca_test")
	if _, err := os.Lstat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("origin still exists after quarantine: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, cacheartifact.QuarantineDirectory))
	if err != nil {
		t.Fatalf("quarantine must exist: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("quarantine mode = %v, want 0700", info.Mode().Perm())
	}

	restored, err := real.Restore(context.Background(), root, qpath, name, snapshot.Root, moved)
	if err != nil || !restored.SameObject(moved) {
		t.Fatalf("restore = %#v, %v", restored, err)
	}
	moved, err = real.Quarantine(context.Background(), root, name, "ca_test", snapshot.Root, restored)
	if err != nil {
		t.Fatal(err)
	}
	if err := real.Purge(context.Background(), root, qpath, snapshot.Root, moved); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, qpath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("purged artifact still exists: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("provider root must never be deleted: %v", err)
	}
}

func TestRealFilesystemRejectsIdentityAndDestinationChanges(t *testing.T) {
	root := filepath.Join(canonicalTemp(t), "shell_snapshots")
	mustMkdir(t, root)
	name := "019fcde3-594a-7eb1-a102-ee8c7893c2dc.1.sh"
	mustWrite(t, filepath.Join(root, name))
	real := New()
	snapshot := mustSnapshot(t, real, root)
	expected := snapshot.Entries[0].Identity
	if err := os.Chmod(filepath.Join(root, name), 0o400); err != nil {
		t.Fatal(err)
	}
	if _, err := real.Quarantine(context.Background(), root, name, "ca_test", snapshot.Root, expected); !errors.Is(err, ErrChangedIdentity) {
		t.Fatalf("changed metadata must be refused, got %v", err)
	}

	snapshot = mustSnapshot(t, real, root)
	moved, err := real.Quarantine(context.Background(), root, name, "ca_test", snapshot.Root, snapshot.Entries[0].Identity)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, name))
	if _, err := real.Restore(context.Background(), root, filepath.Join(cacheartifact.QuarantineDirectory, "ca_test"), name, snapshot.Root, moved); !errors.Is(err, ErrDestination) {
		t.Fatalf("occupied restore destination must be refused, got %v", err)
	}
}

func TestRealFilesystemRejectsLinksAndDoesNotCreateOnReadActions(t *testing.T) {
	parent := canonicalTemp(t)
	root := filepath.Join(parent, "shell_snapshots")
	mustMkdir(t, root)
	name := "019fcde3-594a-7eb1-a102-ee8c7893c2dc.1.sh"
	mustWrite(t, filepath.Join(root, name))
	real := New()
	snapshot := mustSnapshot(t, real, root)
	if err := os.Link(filepath.Join(root, name), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := real.Quarantine(context.Background(), root, name, "ca_test", snapshot.Root, snapshot.Entries[0].Identity); !errors.Is(err, ErrChangedIdentity) {
		t.Fatalf("hard link must change exact identity and be refused, got %v", err)
	}
	if err := os.Remove(filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, name)); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	mustWrite(t, outside)
	if err := os.Symlink(outside, filepath.Join(root, name)); err != nil {
		t.Fatal(err)
	}
	if _, err := real.Quarantine(context.Background(), root, name, "ca_test", snapshot.Root, snapshot.Entries[0].Identity); !errors.Is(err, ErrChangedIdentity) {
		t.Fatalf("symlink replacement must be refused, got %v", err)
	}

	linkedRoot := filepath.Join(parent, "linked-root")
	if err := os.Symlink(root, linkedRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := real.Snapshot(context.Background(), linkedRoot, 10); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink root must be refused, got %v", err)
	}

	emptyRoot := filepath.Join(parent, "empty")
	mustMkdir(t, emptyRoot)
	empty := mustSnapshot(t, real, emptyRoot)
	missing := cacheartifact.Identity{UID: empty.Root.UID, Device: empty.Root.Device, Inode: 9, Nlink: 1, EntryType: "regular"}
	_ = real.Purge(context.Background(), emptyRoot, filepath.Join(cacheartifact.QuarantineDirectory, "absent"), empty.Root, missing)
	if _, err := os.Lstat(filepath.Join(emptyRoot, cacheartifact.QuarantineDirectory)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("purge validation must not create quarantine, got %v", err)
	}
}

func mustSnapshot(t *testing.T, filesystem *Real, root string) Snapshot {
	t.Helper()
	snapshot, err := filesystem.Snapshot(context.Background(), root, 100)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func canonicalTemp(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
