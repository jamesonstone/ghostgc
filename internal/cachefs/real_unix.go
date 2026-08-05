//go:build darwin || linux

package cachefs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
	"golang.org/x/sys/unix"
)

// Real performs descriptor-anchored operations on one flat provider root.
type Real struct{}

// New constructs the production filesystem implementation.
func New() *Real { return &Real{} }

// Snapshot observes only immediate children and never follows a symlink.
func (r *Real) Snapshot(ctx context.Context, root string, limit int) (Snapshot, error) {
	if limit < 1 {
		return Snapshot{}, fmt.Errorf("%w: traversal limit must be positive", ErrUnsafePath)
	}
	fd, rootID, err := openRoot(root)
	if err != nil {
		return Snapshot{}, err
	}
	file := os.NewFile(uintptr(fd), root)
	defer func() { _ = file.Close() }()

	dirEntries, err := file.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return Snapshot{}, fmt.Errorf("cache filesystem: reading provider root: %w", err)
	}
	complete := len(dirEntries) <= limit
	if !complete {
		dirEntries = dirEntries[:limit]
	}
	entries := make([]Entry, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		name := dirEntry.Name()
		if !safeBase(name) {
			return Snapshot{}, fmt.Errorf("%w: invalid directory entry %q", ErrUnsafePath, name)
		}
		identity, err := statAt(fd, name)
		if err != nil {
			return Snapshot{}, err
		}
		entries = append(entries, Entry{Name: name, Identity: identity})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return Snapshot{Root: rootID, Entries: entries, Complete: complete}, nil
}

// Quarantine atomically moves one exact file to the private provider quarantine.
func (r *Real) Quarantine(ctx context.Context, root, relativePath, destination string, expectedRoot, expected cacheartifact.Identity) (cacheartifact.Identity, error) {
	if err := ctx.Err(); err != nil {
		return cacheartifact.Identity{}, err
	}
	rootFD, rootID, err := openRoot(root)
	if err != nil {
		return cacheartifact.Identity{}, err
	}
	defer closeFD(rootFD)
	if !rootID.SameObject(expectedRoot) {
		return cacheartifact.Identity{}, ErrChangedIdentity
	}
	if err := validateSource(rootFD, relativePath, rootID, expected); err != nil {
		return cacheartifact.Identity{}, err
	}
	qfd, err := openQuarantine(rootFD, rootID, true)
	if err != nil {
		return cacheartifact.Identity{}, err
	}
	defer closeFD(qfd)
	if !safeBase(destination) {
		return cacheartifact.Identity{}, fmt.Errorf("%w: invalid quarantine destination", ErrUnsafePath)
	}
	if existsAt(qfd, destination) {
		return cacheartifact.Identity{}, ErrDestination
	}
	if err := unix.Renameat(rootFD, relativePath, qfd, destination); err != nil {
		if errors.Is(err, unix.EXDEV) {
			return cacheartifact.Identity{}, ErrCrossDevice
		}
		return cacheartifact.Identity{}, fmt.Errorf("cache filesystem: atomic quarantine rename: %w", err)
	}
	moved, err := statAt(qfd, destination)
	if err != nil {
		return cacheartifact.Identity{}, err
	}
	if !sameAcrossRename(moved, expected) {
		return cacheartifact.Identity{}, ErrChangedIdentity
	}
	return moved, nil
}

// Restore atomically returns one exact quarantined file to its absent origin.
func (r *Real) Restore(ctx context.Context, root, quarantinePath, destination string, expectedRoot, expected cacheartifact.Identity) (cacheartifact.Identity, error) {
	if err := ctx.Err(); err != nil {
		return cacheartifact.Identity{}, err
	}
	rootFD, rootID, err := openRoot(root)
	if err != nil {
		return cacheartifact.Identity{}, err
	}
	defer closeFD(rootFD)
	if !rootID.SameObject(expectedRoot) || !safeQuarantinePath(quarantinePath) || !safeBase(destination) {
		return cacheartifact.Identity{}, ErrUnsafePath
	}
	qfd, err := openQuarantine(rootFD, rootID, false)
	if err != nil {
		return cacheartifact.Identity{}, err
	}
	defer closeFD(qfd)
	name := filepath.Base(quarantinePath)
	current, err := statAt(qfd, name)
	if err != nil {
		return cacheartifact.Identity{}, err
	}
	if !current.Equal(expected) {
		return cacheartifact.Identity{}, ErrChangedIdentity
	}
	if existsAt(rootFD, destination) {
		return cacheartifact.Identity{}, ErrDestination
	}
	if err := unix.Renameat(qfd, name, rootFD, destination); err != nil {
		if errors.Is(err, unix.EXDEV) {
			return cacheartifact.Identity{}, ErrCrossDevice
		}
		return cacheartifact.Identity{}, fmt.Errorf("cache filesystem: atomic restore rename: %w", err)
	}
	restored, err := statAt(rootFD, destination)
	if err != nil || !sameAcrossRename(restored, expected) {
		return cacheartifact.Identity{}, ErrChangedIdentity
	}
	return restored, nil
}

// QuarantineEntry observes one exact quarantine child without following links.
func (r *Real) QuarantineEntry(ctx context.Context, root, quarantinePath string, expectedRoot cacheartifact.Identity) (cacheartifact.Identity, bool, error) {
	if err := ctx.Err(); err != nil {
		return cacheartifact.Identity{}, false, err
	}
	rootFD, rootID, err := openRoot(root)
	if err != nil {
		return cacheartifact.Identity{}, false, err
	}
	defer closeFD(rootFD)
	if !rootID.SameObject(expectedRoot) || !safeQuarantinePath(quarantinePath) {
		return cacheartifact.Identity{}, false, ErrUnsafePath
	}
	qfd, err := openQuarantine(rootFD, rootID, false)
	if err != nil {
		return cacheartifact.Identity{}, false, err
	}
	defer closeFD(qfd)
	name := filepath.Base(quarantinePath)
	current, err := statAt(qfd, name)
	if errors.Is(err, os.ErrNotExist) {
		return cacheartifact.Identity{}, false, nil
	}
	if err != nil {
		return cacheartifact.Identity{}, false, err
	}
	return current, true, nil
}

func openRoot(root string) (int, cacheartifact.Identity, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.ContainsRune(root, '\x00') {
		return -1, cacheartifact.Identity{}, ErrUnsafePath
	}
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	fd, err := unix.Open(string(filepath.Separator), flags, 0)
	if err != nil {
		return -1, cacheartifact.Identity{}, fmt.Errorf("cache filesystem: opening filesystem root: %w", err)
	}
	for _, component := range strings.Split(strings.TrimPrefix(root, string(filepath.Separator)), string(filepath.Separator)) {
		if !safeBase(component) {
			_ = unix.Close(fd)
			return -1, cacheartifact.Identity{}, ErrUnsafePath
		}
		next, openErr := unix.Openat(fd, component, flags, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			if errors.Is(openErr, unix.ELOOP) || errors.Is(openErr, unix.ENOTDIR) {
				return -1, cacheartifact.Identity{}, fmt.Errorf("%w: provider path component %q is not a physical directory", ErrUnsafePath, component)
			}
			return -1, cacheartifact.Identity{}, fmt.Errorf("cache filesystem: opening provider path component %q: %w", component, openErr)
		}
		fd = next
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return -1, cacheartifact.Identity{}, fmt.Errorf("cache filesystem: stating provider root: %w", err)
	}
	identity := identityFromStat(&stat)
	if identity.EntryType != "directory" {
		_ = unix.Close(fd)
		return -1, cacheartifact.Identity{}, ErrUnsafePath
	}
	return fd, identity, nil
}

func validateSource(fd int, name string, root, expected cacheartifact.Identity) error {
	if !safeBase(name) || name == cacheartifact.QuarantineDirectory {
		return ErrUnsafePath
	}
	current, err := statAt(fd, name)
	if err != nil {
		return err
	}
	if !current.Equal(expected) {
		return ErrChangedIdentity
	}
	if current.EntryType != "regular" || current.Nlink != 1 || current.UID != root.UID {
		return ErrUnsafePath
	}
	if current.Device != root.Device {
		return ErrCrossDevice
	}
	return nil
}

func statAt(fd int, name string) (cacheartifact.Identity, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return cacheartifact.Identity{}, fmt.Errorf("cache filesystem: stating %q: %w", name, err)
	}
	return identityFromStat(&stat), nil
}

func openQuarantine(rootFD int, root cacheartifact.Identity, create bool) (int, error) {
	if create {
		err := unix.Mkdirat(rootFD, cacheartifact.QuarantineDirectory, 0o700)
		if err != nil && !errors.Is(err, unix.EEXIST) {
			return -1, fmt.Errorf("cache filesystem: creating quarantine: %w", err)
		}
	}
	identity, err := statAt(rootFD, cacheartifact.QuarantineDirectory)
	if err != nil || identity.EntryType != "directory" || identity.UID != root.UID || identity.Device != root.Device || identity.Mode&0o777 != 0o700 {
		return -1, ErrUnsafePath
	}
	fd, err := unix.Openat(rootFD, cacheartifact.QuarantineDirectory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, fmt.Errorf("cache filesystem: opening quarantine: %w", err)
	}
	return fd, nil
}

func existsAt(fd int, name string) bool {
	var stat unix.Stat_t
	err := unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	return err == nil || !errors.Is(err, unix.ENOENT)
}

func closeFD(fd int) { _ = unix.Close(fd) }

func sameAcrossRename(current, expected cacheartifact.Identity) bool {
	// POSIX rename updates ctime even when every content-bearing identity
	// field is preserved. Compare everything else and persist the new ctime.
	current.CTimeNs = expected.CTimeNs
	return current.Equal(expected)
}

func safeBase(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsRune(name, '\x00')
}

func safeQuarantinePath(path string) bool {
	return filepath.Dir(path) == cacheartifact.QuarantineDirectory && safeBase(filepath.Base(path))
}
