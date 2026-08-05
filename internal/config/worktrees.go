package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const maxWorktreeRoots = 32

func (c Config) validateWorktrees() error {
	if c.Worktrees.ScanInterval.D() < time.Minute {
		return fmt.Errorf("worktrees.scanInterval is %s, which is below the one-minute minimum", c.Worktrees.ScanInterval.D())
	}
	if c.Worktrees.StaleAfter.D() < 7*24*time.Hour {
		return fmt.Errorf("worktrees.staleAfter is %s, which is below the minimum of 168h", c.Worktrees.StaleAfter.D())
	}
	if c.Worktrees.RetirementGrace.D() < 24*time.Hour {
		return fmt.Errorf("worktrees.retirementGrace is %s, which is below the minimum of 24h", c.Worktrees.RetirementGrace.D())
	}
	if len(c.Worktrees.Roots) > maxWorktreeRoots {
		return fmt.Errorf("worktrees.roots has %d entries, which exceeds the maximum of %d", len(c.Worktrees.Roots), maxWorktreeRoots)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("worktrees.roots: resolving home directory: %w", err)
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		return fmt.Errorf("worktrees.roots: canonicalizing home directory: %w", err)
	}
	seen := make(map[string]bool, len(c.Worktrees.Roots))
	for _, root := range c.Worktrees.Roots {
		if err := validateWorktreeRoot(root, home); err != nil {
			return err
		}
		if seen[root] {
			return fmt.Errorf("worktrees.roots contains duplicate canonical root %q", root)
		}
		seen[root] = true
	}
	return nil
}

func validateWorktreeRoot(root, home string) error {
	if root == "" || !filepath.IsAbs(root) {
		return fmt.Errorf("worktrees.roots entry %q must be absolute", root)
	}
	if filepath.Clean(root) != root {
		return fmt.Errorf("worktrees.roots entry %q must already be canonical", root)
	}
	volumeRoot := filepath.VolumeName(root) + string(filepath.Separator)
	if root == volumeRoot || root == home {
		return fmt.Errorf("worktrees.roots entry %q cannot grant filesystem-root or whole-home authority", root)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("worktrees.roots entry %q is unavailable: %w", root, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("worktrees.roots entry %q must be a real directory, not a symlink", root)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved != root {
		return fmt.Errorf("worktrees.roots entry %q must be its canonical physical path", root)
	}
	return nil
}
