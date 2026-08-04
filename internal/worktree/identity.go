package worktree

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Identify returns a no-follow identity for one canonical absolute path.
func Identify(path string) (FileIdentity, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return FileIdentity{}, fmt.Errorf("worktree: path %q is not canonical and absolute", path)
	}
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return FileIdentity{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return FileIdentity{}, err
	}
	return FileIdentity{
		Path: path, Device: uint64(stat.Dev), Inode: uint64(stat.Ino),
		Size: info.Size(), ModTimeNs: info.ModTime().UnixNano(),
	}, nil
}

// StableID survives registered moves but changes when Git recreates the
// administrative directory at the same pathname.
func StableID(common, admin FileIdentity) string {
	identity := []byte(common.Path + "\x00" + admin.Path + "\x00")
	for _, value := range []uint64{common.Device, common.Inode, admin.Device, admin.Inode} {
		identity = binary.BigEndian.AppendUint64(identity, value)
	}
	sum := sha256.Sum256(identity)
	return hex.EncodeToString(sum[:])
}

// SameIdentity compares every fact bound to an approval.
func SameIdentity(a, b FileIdentity) bool {
	return a == b
}
