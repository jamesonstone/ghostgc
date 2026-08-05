package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/worktree"
)

func (d *Daemon) validateRetiredWorktree(ctx context.Context, record storage.WorktreeRecord,
	requireGrace bool) (worktreeValidation, error) {
	ctx, cancel := context.WithTimeout(ctx, worktreeValidationTimeout)
	defer cancel()
	if !d.filesystemMutationsHealthy() {
		return worktreeValidation{}, errors.New("filesystem mutation circuit is open; restart the daemon and re-observe")
	}
	if !d.cfg.Worktrees.Enabled || d.plat.Name() != "darwin" {
		return worktreeValidation{}, errors.New("worktree mutation is disabled or unsupported on this platform")
	}
	if record.State != string(worktree.StateRetired) || !record.Registered || !record.Complete ||
		record.OriginalPath == "" || record.RetiredNs == nil {
		return worktreeValidation{}, errors.New("worktree is not a complete registered retirement")
	}
	if requireGrace && time.Now().UnixNano() < record.RetirementGraceNs {
		return worktreeValidation{}, errors.New("worktree retirement grace has not elapsed")
	}
	if _, err := os.Lstat(record.OriginalPath); !errors.Is(err, os.ErrNotExist) {
		return worktreeValidation{}, errors.New("original worktree destination exists or is unreadable")
	}
	if d.worktreeGit == nil {
		return worktreeValidation{}, d.worktreeGitErr
	}
	if err := d.worktreeGit.VerifyIdentity(); err != nil {
		return worktreeValidation{}, err
	}
	var storedGit worktree.GitIdentity
	if err := json.Unmarshal([]byte(record.GitIdentityJSON), &storedGit); err != nil || storedGit != d.worktreeGit.Identity() {
		return worktreeValidation{}, errors.New("resolved Git executable identity differs from retirement")
	}
	registration, primary, err := d.worktreeGit.FindRegistration(ctx, record.Path, record.WorktreeID)
	if err != nil {
		return worktreeValidation{}, err
	}
	primary, err = filepath.EvalSymlinks(primary)
	if err != nil || !filepath.IsAbs(primary) {
		return worktreeValidation{}, errors.New("primary worktree path is unavailable or non-canonical")
	}
	observation := d.worktreeGit.Inspect(ctx, registration, primary)
	if err := validateFreshObservation(record, observation); err != nil {
		return worktreeValidation{}, err
	}
	observation.ObservedAt = time.Time{}
	if d.activeSessionInWorktree(ctx, record.Path) {
		return worktreeValidation{}, errors.New("an active agent session remains associated with the retired worktree")
	}
	usage, err := d.plat.InspectPathUsage(ctx, record.Path)
	if err != nil || !usage.Complete || len(usage.ProcessKeys) > 0 || usage.CWDReferences > 0 || usage.OpenVnodes > 0 {
		return worktreeValidation{}, errors.New("same-user process usage inspection is incomplete or found references")
	}
	filesystem, err := worktree.InspectFilesystem(ctx, record.Path)
	if err != nil || !filesystem.Complete || filesystem.NestedMounts != 0 {
		return worktreeValidation{}, errors.New("retired worktree filesystem inspection is incomplete or found nested mounts")
	}
	var approved []worktree.ApprovedLink
	if err := json.Unmarshal([]byte(record.ApprovedLinksJSON), &approved); err != nil ||
		!reflect.DeepEqual(approved, observation.ApprovedLinks) {
		return worktreeValidation{}, errors.New("approved environment-link evidence changed")
	}
	if err := worktree.ValidateApprovedLinks(record.Path, approved); err != nil {
		return worktreeValidation{}, err
	}
	return worktreeValidation{Observation: observation, PathUsage: process.PathUsage(usage), Filesystem: filesystem,
		GitIdentity: storedGit, PrimaryPath: primary,
		RecreateCommand: fmt.Sprintf("ghostgc worktree restore --dry-run --worktree %s", record.WorktreeID)}, nil
}
