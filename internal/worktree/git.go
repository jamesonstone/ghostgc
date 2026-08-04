package worktree

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	gitTimeout   = 5 * time.Second
	maxGitOutput = 4 << 20
)

// Git is one resolved, bounded, shell-free local Git adapter.
type Git struct {
	path         string
	execPath     string
	identity     GitIdentity
	execIdentity FileIdentity
	timeout      time.Duration
	maxBytes     int
	beforeExec   func()
}

// NewGit resolves Git once and binds later approvals to that executable.
func NewGit(snapshotDir string) (*Git, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("worktree: resolving git: %w", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("worktree: resolving git executable: %w", err)
	}
	return newGit(path, snapshotDir)
}

// Identity returns the exact executable identity used by every command.
func (g *Git) Identity() GitIdentity { return g.identity }

// VerifyIdentity refuses commands after the resolved executable changes.
func (g *Git) VerifyIdentity() error {
	current, err := Identify(g.path)
	if err != nil {
		return fmt.Errorf("worktree: re-identifying git executable: %w", err)
	}
	if !SameIdentity(current, g.identity.FileIdentity) {
		return errors.New("worktree: git executable identity changed")
	}
	execution, err := Identify(g.execPath)
	if err != nil || !SameIdentity(execution, g.execIdentity) {
		return errors.New("worktree: private git execution snapshot changed")
	}
	return nil
}

// Registrations returns the NUL-delimited registered inventory for a repo.
func (g *Git) Registrations(ctx context.Context, repository string) ([]Registration, error) {
	raw, err := g.run(ctx, repository, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	records, err := ParseRegistrations(raw)
	if err != nil {
		return nil, err
	}
	g.enrichAdministrativeDirs(ctx, repository, records)
	return records, nil
}

func (g *Git) enrichAdministrativeDirs(ctx context.Context, repository string, records []Registration) {
	if len(records) == 0 {
		return
	}
	common, err := g.revPath(ctx, repository, "--git-common-dir")
	if err != nil {
		return
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return
	}
	records[0].CommonGitDir, records[0].AdminGitDir = common, common
	entries, err := os.ReadDir(filepath.Join(common, "worktrees"))
	if err != nil || len(entries) > maxRegistrations {
		return
	}
	byPath := make(map[string]string, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil || entry.Type()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		admin := filepath.Join(common, "worktrees", entry.Name())
		pointer, readErr := readBoundedGitPath(filepath.Join(admin, "gitdir"))
		if readErr != nil || filepath.Base(pointer) != ".git" {
			continue
		}
		byPath[filepath.Clean(filepath.Dir(pointer))] = admin
	}
	for i := 1; i < len(records); i++ {
		if admin := byPath[filepath.Clean(records[i].Path)]; admin != "" {
			records[i].CommonGitDir, records[i].AdminGitDir = common, admin
		}
	}
}

func readBoundedGitPath(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(raw) > 4096 {
		return "", errors.New("worktree: administrative Git path exceeds bound")
	}
	value := strings.TrimSuffix(string(raw), "\n")
	if !filepath.IsAbs(value) {
		return "", errors.New("worktree: administrative Git path is not absolute")
	}
	return filepath.Clean(value), nil
}

// RegistrationID resolves the stable identity without inspecting worktree
// contents. Administrative enrichment also makes this work for missing paths.
func (g *Git) RegistrationID(record Registration) (string, error) {
	if record.CommonGitDir == "" || record.AdminGitDir == "" {
		return "", errors.New("worktree: registration lacks administrative identity")
	}
	common, err := filepath.EvalSymlinks(record.CommonGitDir)
	if err != nil {
		return "", err
	}
	admin, err := filepath.EvalSymlinks(record.AdminGitDir)
	if err != nil {
		return "", err
	}
	commonIdentity, err := Identify(common)
	if err != nil {
		return "", err
	}
	adminIdentity, err := Identify(admin)
	if err != nil {
		return "", err
	}
	return StableID(commonIdentity, adminIdentity), nil
}

func (g *Git) revPath(ctx context.Context, path, flag string) (string, error) {
	raw, err := g.run(ctx, path, "rev-parse", "--path-format=absolute", flag)
	if err != nil {
		return "", err
	}
	value := strings.TrimSuffix(string(raw), "\n")
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("worktree: git returned non-absolute %s", flag)
	}
	return filepath.Clean(value), nil
}

func (g *Git) count(ctx context.Context, path string, args ...string) (int, error) {
	raw, err := g.run(ctx, path, args...)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0, fmt.Errorf("worktree: parsing git count: %w", err)
	}
	return n, nil
}

func (g *Git) succeeds(ctx context.Context, path string, args ...string) bool {
	_, err := g.run(ctx, path, args...)
	return err == nil
}

// Remove invokes only native, non-force worktree removal.
func (g *Git) Remove(ctx context.Context, repository, canonicalPath string) error {
	if err := g.VerifyIdentity(); err != nil {
		return err
	}
	_, err := g.run(ctx, repository, "worktree", "remove", canonicalPath)
	return err
}
