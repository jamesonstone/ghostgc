package worktree

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const maxGitExecutableBytes int64 = 128 << 20

func newGit(path, snapshotDir string) (*Git, error) {
	identity, err := Identify(path)
	if err != nil {
		return nil, fmt.Errorf("worktree: identifying git executable: %w", err)
	}
	source, err := os.Open(path)
	if err != nil {
		return nil, errors.New("worktree: opening git executable failed")
	}
	defer func() { _ = source.Close() }()
	opened, err := identifyOpenFile(path, source)
	if err != nil || !SameIdentity(identity, opened) {
		return nil, errors.New("worktree: git executable changed while opening")
	}
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || opened.Size < 1 || opened.Size > maxGitExecutableBytes {
		return nil, errors.New("worktree: git executable is not a bounded regular file")
	}
	execPath := path
	var digest string
	if executablePathMutable(path) {
		execPath, digest, err = snapshotGitExecutable(snapshotDir, source, opened.Size)
	} else {
		digest, err = digestExecutable(source, io.Discard, opened.Size)
	}
	if err != nil {
		return nil, err
	}
	openedAfter, err := identifyOpenFile(path, source)
	if err != nil || !SameIdentity(opened, openedAfter) {
		return nil, errors.New("worktree: git executable changed while reading")
	}
	current, err := Identify(path)
	if err != nil || !SameIdentity(identity, current) {
		return nil, errors.New("worktree: git executable changed while snapshotting")
	}
	execIdentity, err := Identify(execPath)
	if err != nil {
		return nil, errors.New("worktree: private git execution snapshot is unavailable")
	}
	return &Git{
		path: path, execPath: execPath, identity: GitIdentity{FileIdentity: identity, Digest: digest},
		execIdentity: execIdentity, timeout: gitTimeout, maxBytes: maxGitOutput,
	}, nil
}

func executablePathMutable(path string) bool {
	if unix.Access(path, unix.W_OK) == nil {
		return true
	}
	for directory := filepath.Dir(path); ; directory = filepath.Dir(directory) {
		if unix.Access(directory, unix.W_OK) == nil {
			return true
		}
		if directory == filepath.Dir(directory) {
			return false
		}
	}
}

func snapshotGitExecutable(root string, source *os.File, expectedBytes int64) (string, string, error) {
	root, err := prepareSnapshotDirectory(root)
	if err != nil {
		return "", "", err
	}
	temporary, err := os.CreateTemp(root, ".building-")
	if err != nil {
		return "", "", errors.New("worktree: creating private git execution snapshot failed")
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	digest, err := digestExecutable(source, temporary, expectedBytes)
	if err != nil {
		_ = temporary.Close()
		return "", "", err
	}
	if err := temporary.Chmod(0o500); err != nil {
		_ = temporary.Close()
		return "", "", errors.New("worktree: securing private git execution snapshot failed")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", "", errors.New("worktree: syncing private git execution snapshot failed")
	}
	if err := temporary.Close(); err != nil {
		return "", "", errors.New("worktree: closing private git execution snapshot failed")
	}
	target := filepath.Join(root, digest)
	if err := installSnapshot(temporaryPath, target, digest); err != nil {
		return "", "", err
	}
	if err := pruneOtherSnapshots(root, digest); err != nil {
		return "", "", err
	}
	return target, digest, nil
}

func prepareSnapshotDirectory(root string) (string, error) {
	if root == "" || !filepath.IsAbs(root) {
		return "", errors.New("worktree: private git snapshot directory must be absolute")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", errors.New("worktree: creating private git snapshot directory failed")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errors.New("worktree: resolving private git snapshot directory failed")
	}
	info, err := os.Lstat(canonical)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("worktree: private git snapshot directory is unsafe")
	}
	if err := os.Chmod(canonical, 0o700); err != nil {
		return "", errors.New("worktree: securing private git snapshot directory failed")
	}
	return canonical, nil
}

func installSnapshot(temporaryPath, target, digest string) error {
	if err := os.Link(temporaryPath, target); err != nil && !os.IsExist(err) {
		return errors.New("worktree: installing private git execution snapshot failed")
	}
	actual, err := digestFile(target)
	if err != nil || actual != digest {
		return errors.New("worktree: private git execution snapshot content changed")
	}
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o222 != 0 {
		return errors.New("worktree: private git execution snapshot is unsafe")
	}
	return nil
}

func digestFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("worktree: private git snapshot is not a regular file")
	}
	return digestExecutable(file, io.Discard, info.Size())
}

func digestExecutable(source io.Reader, destination io.Writer, expectedBytes int64) (string, error) {
	if expectedBytes < 1 || expectedBytes > maxGitExecutableBytes {
		return "", errors.New("worktree: git executable exceeds its size bound")
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(source, expectedBytes+1))
	if err != nil {
		return "", errors.New("worktree: reading git executable failed")
	}
	if written != expectedBytes {
		return "", errors.New("worktree: git executable size changed while reading")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func pruneOtherSnapshots(root, keep string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return errors.New("worktree: listing private git snapshots failed")
	}
	for _, entry := range entries {
		if entry.Name() == keep {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return errors.New("worktree: private git snapshot directory contains an unsafe entry")
		}
		if err := os.Remove(filepath.Join(root, entry.Name())); err != nil {
			return errors.New("worktree: pruning old private git snapshot failed")
		}
	}
	return nil
}
