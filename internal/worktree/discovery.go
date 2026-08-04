package worktree

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

const (
	maxDiscoveryDepth   = 4
	maxDiscoveryEntries = 50000
)

// DiscoverRepositories finds repository markers beneath one validated root.
// WalkDir never follows directory symlinks.
func DiscoverRepositories(ctx context.Context, root string) ([]string, error) {
	var repositories []string
	entries := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entries++
		if entries > maxDiscoveryEntries {
			return fmt.Errorf("worktree: root traversal exceeded %d entries", maxDiscoveryEntries)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		depth := 0
		if rel != "." {
			depth = len(strings.Split(rel, string(filepath.Separator)))
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == ".git" && depth > 0 && depth-1 <= maxDiscoveryDepth {
			repositories = append(repositories, filepath.Dir(path))
			if entry.IsDir() {
				return filepath.SkipDir
			}
		}
		if depth > maxDiscoveryDepth {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return repositories, nil
}
