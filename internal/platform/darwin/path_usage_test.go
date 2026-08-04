//go:build darwin

package darwin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPathVnodeIdentitiesAreBoundedToPhysicalEntries(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("contents are not retained"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	identities, err := pathVnodeIdentities(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	var stat unix.Stat_t
	if err := unix.Lstat(file, &stat); err != nil {
		t.Fatal(err)
	}
	if !identities[vnodeIdentity{device: uint64(stat.Dev), inode: stat.Ino}] {
		t.Fatal("regular file identity was not collected")
	}
	var linkStat unix.Stat_t
	if err := unix.Lstat(filepath.Join(root, "link"), &linkStat); err != nil {
		t.Fatal(err)
	}
	if identities[vnodeIdentity{device: uint64(linkStat.Dev), inode: linkStat.Ino}] {
		t.Fatal("symlink identity should not be followed or collected")
	}
}
