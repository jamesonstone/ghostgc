//go:build darwin || linux

package worktree

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Finalizer owns the foreground-only native worktree removal capability.
type Finalizer struct {
	git          *Git
	beforeUnlink func()
}

// NewFinalizer pins a Git executable for one short-lived CLI invocation.
func NewFinalizer(snapshotDir string) (*Finalizer, error) {
	git, err := NewGit(snapshotDir)
	if err != nil {
		return nil, err
	}
	return &Finalizer{git: git}, nil
}

// Finalize removes exact approved links, then invokes native non-force removal.
func (f *Finalizer) Finalize(ctx context.Context, repository, path string,
	expectedGit GitIdentity, expectedPath FileIdentity, links []ApprovedLink) error {
	if f == nil || f.git == nil || f.git.Identity() != expectedGit {
		return errors.New("worktree: foreground Git identity differs from the purge plan")
	}
	if err := f.git.VerifyIdentity(); err != nil {
		return err
	}
	if err := ValidateApprovedLinks(path, links); err != nil {
		return err
	}
	rootFD, err := openExactDirectory(path, expectedPath)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(rootFD) }()
	if f.beforeUnlink != nil {
		f.beforeUnlink()
	}
	removed, err := unlinkApprovedLinksAt(rootFD, links)
	if err != nil {
		return err
	}
	if _, err := f.git.run(ctx, repository, "worktree", "remove", path); err != nil {
		return errors.Join(err, restoreApprovedLinksAt(rootFD, removed))
	}
	return nil
}

func openExactDirectory(path string, expected FileIdentity) (int, error) {
	if path == "" || expected.Path != path || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return -1, errors.New("worktree: finalizer path is not exact and canonical")
	}
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	fd, err := unix.Open(string(filepath.Separator), flags, 0)
	if err != nil {
		return -1, fmt.Errorf("worktree: opening filesystem root: %w", err)
	}
	for _, component := range strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator)) {
		if component == "" || component == "." || component == ".." {
			_ = unix.Close(fd)
			return -1, errors.New("worktree: unsafe finalizer path component")
		}
		next, openErr := unix.Openat(fd, component, flags, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			return -1, fmt.Errorf("worktree: opening finalizer path component: %w", openErr)
		}
		fd = next
	}
	current, err := Identify(path)
	if err != nil || !SameIdentity(current, expected) {
		_ = unix.Close(fd)
		return -1, errors.New("worktree: finalizer path identity changed")
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || uint64(stat.Dev) != expected.Device || uint64(stat.Ino) != expected.Inode {
		_ = unix.Close(fd)
		return -1, errors.New("worktree: finalizer descriptor identity changed")
	}
	return fd, nil
}

func unlinkApprovedLinksAt(rootFD int, links []ApprovedLink) ([]ApprovedLink, error) {
	removed := make([]ApprovedLink, 0, len(links))
	for _, link := range links {
		var stat unix.Stat_t
		if err := unix.Fstatat(rootFD, link.Name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFLNK {
			return removed, errors.Join(errors.New("worktree: approved link changed before unlink"), restoreApprovedLinksAt(rootFD, removed))
		}
		buffer := make([]byte, 4096)
		n, err := unix.Readlinkat(rootFD, link.Name, buffer)
		if err != nil || string(buffer[:n]) != link.LinkText {
			return removed, errors.Join(errors.New("worktree: approved link target changed before unlink"), restoreApprovedLinksAt(rootFD, removed))
		}
		if err := unix.Unlinkat(rootFD, link.Name, 0); err != nil {
			return removed, errors.Join(err, restoreApprovedLinksAt(rootFD, removed))
		}
		removed = append(removed, link)
	}
	return removed, nil
}

func restoreApprovedLinksAt(rootFD int, links []ApprovedLink) error {
	var failures []error
	for _, link := range links {
		var stat unix.Stat_t
		if err := unix.Fstatat(rootFD, link.Name, &stat, unix.AT_SYMLINK_NOFOLLOW); err == nil {
			failures = append(failures, fmt.Errorf("approved %s destination was recreated", link.Name))
			continue
		}
		if err := unix.Symlinkat(link.LinkText, rootFD, link.Name); err != nil {
			failures = append(failures, fmt.Errorf("restoring approved %s link: %w", link.Name, err))
		}
	}
	return errors.Join(failures...)
}
