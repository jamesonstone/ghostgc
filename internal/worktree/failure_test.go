package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvidenceCategoriesAreBoundedAndPathFree(t *testing.T) {
	sentinel := "private-client-filename"
	cause := &os.PathError{Op: "open", Path: sentinel, Err: errors.New("denied")}
	tests := []struct {
		err  error
		want string
	}{
		{newEvidenceError(failureDiscoveryBound, "bounded", cause), failureDiscoveryBound},
		{newEvidenceError(failureGitChanged, "changed", cause), failureGitChanged},
		{newEvidenceError(failureGitInspection, "inspection", cause), failureGitInspection},
		{newEvidenceError(failureGitUnavailable, "unavailable", cause), failureGitUnavailable},
	}
	for _, test := range tests {
		if got := EvidenceCategory(test.err); got != test.want || strings.Contains(got, sentinel) {
			t.Errorf("EvidenceCategory(%v) = %q, want %q", test.err, got, test.want)
		}
	}
	_, err := DiscoverRepositories(context.Background(), filepath.Join(t.TempDir(), sentinel))
	if got := EvidenceCategory(err); got != failureDiscoveryIncomplete {
		t.Fatalf("missing-root category = %q, want %q", got, failureDiscoveryIncomplete)
	}
}

func TestGitUnavailableAndChangedHaveDistinctEvidenceCategories(t *testing.T) {
	t.Setenv("PATH", "")
	if _, err := NewGit(filepath.Join(t.TempDir(), "snapshots")); EvidenceCategory(err) != failureGitUnavailable {
		t.Fatalf("unavailable Git category = %q", EvidenceCategory(err))
	}
	path := writeFakeGitExecutable(t)
	git, err := newGit(path, filepath.Join(filepath.Dir(path), "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := EvidenceCategory(git.VerifyIdentity()); got != failureGitChanged {
		t.Fatalf("changed Git category = %q", got)
	}
}
