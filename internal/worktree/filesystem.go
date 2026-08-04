package worktree

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const maxFilesystemEntries = 100000

// FilesystemEvidence is aggregate targeted removal evidence.
type FilesystemEvidence struct {
	Entries      int  `json:"entries"`
	NestedMounts int  `json:"nested_mounts"`
	Complete     bool `json:"complete"`
}

// InspectFilesystem rejects nested filesystems without retaining their paths.
func InspectFilesystem(ctx context.Context, root string) (FilesystemEvidence, error) {
	return inspectFilesystem(ctx, root, Identify)
}

func inspectFilesystem(ctx context.Context, root string, identify func(string) (FileIdentity, error)) (FilesystemEvidence, error) {
	rootID, err := identify(root)
	if err != nil {
		return FilesystemEvidence{}, errors.New("worktree: filesystem root identity was unavailable")
	}
	evidence := FilesystemEvidence{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("worktree: filesystem traversal was incomplete")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		evidence.Entries++
		if evidence.Entries > maxFilesystemEntries {
			return fmt.Errorf("worktree: filesystem inspection exceeded %d entries", maxFilesystemEntries)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !entry.IsDir() || path == root {
			return nil
		}
		identity, err := identify(path)
		if err != nil {
			return errors.New("worktree: nested filesystem identity was unavailable")
		}
		if identity.Device != rootID.Device {
			evidence.NestedMounts++
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return evidence, err
	}
	evidence.Complete = true
	return evidence, nil
}

// ValidateApprovedLinks rechecks exact root links and their primary targets.
func ValidateApprovedLinks(root string, links []ApprovedLink) error {
	seen := make(map[string]bool, len(links))
	for _, link := range links {
		if seen[link.Name] || (link.Name != ".env" && link.Name != ".envrc") {
			return fmt.Errorf("worktree: invalid approved environment link")
		}
		seen[link.Name] = true
		path := filepath.Join(root, link.Name)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("worktree: approved %s link changed", link.Name)
		}
		text, err := os.Readlink(path)
		if err != nil || text != link.LinkText {
			return fmt.Errorf("worktree: approved %s link target changed", link.Name)
		}
		current, err := Identify(link.Target.Path)
		if err != nil || !SameIdentity(current, link.Target) {
			return fmt.Errorf("worktree: approved %s primary target changed", link.Name)
		}
		resolved := text
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(root, resolved)
		}
		resolved, err = filepath.EvalSymlinks(resolved)
		if err != nil || resolved != link.Target.Path {
			return fmt.Errorf("worktree: approved %s link no longer resolves exactly", link.Name)
		}
	}
	return nil
}
