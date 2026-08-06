//go:build darwin || linux

package servicelog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func validateDirectory(logDir string) (string, error) {
	abs, err := filepath.Abs(logDir)
	if err != nil {
		return "", fmt.Errorf("service log: resolve directory: %w", err)
	}
	physical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("service log: resolve directory %s: %w", abs, err)
	}
	if physical != abs {
		return "", fmt.Errorf("service log: refusing symlinked directory %s", abs)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("service log: inspect directory %s: %w", abs, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || stat.Uid != uint32(os.Getuid()) || info.Mode().Perm()&0o022 != 0 {
		return "", fmt.Errorf("service log: refusing unsafe directory %s", abs)
	}
	return abs, nil
}

func inspectManagedPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("service log: inspect %s: %w", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || stat.Uid != uint32(os.Getuid()) || stat.Nlink != 1 || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("service log: refusing unsafe managed file %s", path)
	}
	return nil
}

func openManagedFile(path string, flags int) (*os.File, error) {
	fd, err := unix.Open(path, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("service log: open %s: %w", path, err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uint32(os.Getuid()) || stat.Nlink != 1 || stat.Mode&0o022 != 0 {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("service log: refusing unsafe managed file %s", path)
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("service log: protect %s: %w", path, err)
	}
	return os.NewFile(uintptr(fd), path), nil
}

func fileIdentity(file *os.File) (uint64, uint64, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return 0, 0, fmt.Errorf("service log: inspect open file: %w", err)
	}
	return uint64(stat.Dev), uint64(stat.Ino), nil
}

func validateOpenPath(file *os.File, path string, device, inode uint64) error {
	dir, err := validateDirectory(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("service log: managed directory changed: %w", err)
	}
	if dir != filepath.Dir(path) {
		return fmt.Errorf("service log: managed directory changed: %s", dir)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("service log: inspect current file %s: %w", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || stat.Uid != uint32(os.Getuid()) || stat.Nlink != 1 || info.Mode().Perm()&0o022 != 0 || uint64(stat.Dev) != device || uint64(stat.Ino) != inode {
		return fmt.Errorf("service log: refusing changed file %s", path)
	}
	openDevice, openInode, err := fileIdentity(file)
	if err != nil {
		return fmt.Errorf("service log: open file changed: %w", err)
	}
	if openDevice != device || openInode != inode {
		return fmt.Errorf("service log: open file changed")
	}
	return nil
}

func openExistingManagedFile(path string) (*os.File, error) {
	file, err := openManagedFile(path, unix.O_RDWR)
	if errors.Is(err, unix.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return file, nil
}
