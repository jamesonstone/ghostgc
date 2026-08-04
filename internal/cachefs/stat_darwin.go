//go:build darwin

package cachefs

import (
	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
	"golang.org/x/sys/unix"
)

func identityFromStat(stat *unix.Stat_t) cacheartifact.Identity {
	return cacheartifact.Identity{
		UID:       stat.Uid,
		Device:    uint64(stat.Dev),
		Inode:     stat.Ino,
		Mode:      uint32(stat.Mode),
		Nlink:     uint64(stat.Nlink),
		Size:      stat.Size,
		ATimeNs:   stat.Atim.Nano(),
		MTimeNs:   stat.Mtim.Nano(),
		CTimeNs:   stat.Ctim.Nano(),
		BirthNs:   stat.Btim.Nano(),
		EntryType: entryType(uint32(stat.Mode)),
	}
}

func entryType(mode uint32) string {
	switch mode & unix.S_IFMT {
	case unix.S_IFREG:
		return "regular"
	case unix.S_IFDIR:
		return "directory"
	case unix.S_IFLNK:
		return "symlink"
	default:
		return "other"
	}
}
