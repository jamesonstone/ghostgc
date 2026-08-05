package worktree

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitExecutionUsesPinnedSnapshotAcrossSourcePathSwap(t *testing.T) {
	path := writeFakeGitExecutable(t)
	git, err := newGit(path, filepath.Join(filepath.Dir(path), "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	if git.execPath == git.path {
		t.Fatal("writable executable did not receive a private snapshot")
	}
	marker := filepath.Join(filepath.Dir(path), "replacement-ran")
	git.beforeExec = func() {
		if err := os.Rename(path, path+"-original"); err != nil {
			t.Fatal(err)
		}
		replacement := "#!/bin/sh\ntouch '" + marker + "'\nexit 99\n"
		if err := os.WriteFile(path, []byte(replacement), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	out, err := git.run(context.Background(), "", "--version")
	if err != nil || !strings.Contains(string(out), "git version fake") {
		t.Fatalf("pinned snapshot execution = %q, %v", out, err)
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("replacement executable ran: %v", err)
	}
	if err := git.VerifyIdentity(); err == nil {
		t.Fatal("replacement path retained approval identity")
	}
}

func TestMoveRefusesExecutableChangedBeforeInvocation(t *testing.T) {
	path := writeFakeGitExecutable(t)
	git, err := newGit(path, filepath.Join(filepath.Dir(path), "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := git.Move(context.Background(), t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "destination")); err == nil {
		t.Fatal("changed executable was accepted for move")
	}
}

func TestGitSnapshotRejectsOversizedAndGrowingExecutables(t *testing.T) {
	oversized := filepath.Join(t.TempDir(), "git")
	file, err := os.OpenFile(oversized, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxGitExecutableBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := newGit(oversized, filepath.Join(filepath.Dir(oversized), "snapshots")); err == nil {
		t.Fatal("oversized executable was accepted")
	}
	if _, err := digestExecutable(bytes.NewReader([]byte("growing")), io.Discard, 4); err == nil {
		t.Fatal("executable growth beyond opened size was accepted")
	}
}

func writeFakeGitExecutable(t *testing.T) string {
	t.Helper()
	targetPath := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(targetPath, []byte("#!/bin/sh\necho 'git version fake'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return targetPath
}
