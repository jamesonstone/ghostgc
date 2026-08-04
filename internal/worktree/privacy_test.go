package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystemIdentityFailureDoesNotExposeNestedName(t *testing.T) {
	root := t.TempDir()
	sentinel := "private-client-filename"
	nested := filepath.Join(root, sentinel)
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := inspectFilesystem(context.Background(), root, func(path string) (FileIdentity, error) {
		if path == nested {
			return FileIdentity{}, &os.PathError{Op: "stat", Path: path, Err: errors.New("denied")}
		}
		return FileIdentity{Path: path, Device: 1, Inode: 1}, nil
	})
	if err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("sanitized filesystem failure = %v", err)
	}
}
