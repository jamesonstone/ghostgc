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
	execPath := path
	var digest string
	if executablePathMutable(path) {
		execPath, digest, err = snapshotGitExecutable(snapshotDir, source)
	} else {
		digest, err = digestOpenFile(source)
	}
	if err != nil {
		return nil, err
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

func snapshotGitExecutable(root string, source *os.File) (string, string, error) {
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
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(temporary, hash), source); err != nil {
		_ = temporary.Close()
		return "", "", errors.New("worktree: copying private git execution snapshot failed")
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
	digest := hex.EncodeToString(hash.Sum(nil))
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
	return digestOpenFile(file)
}

func digestOpenFile(file *os.File) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
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
