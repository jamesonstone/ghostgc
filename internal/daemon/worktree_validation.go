package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"time"

	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/sessions"
	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/worktree"
)

type worktreeValidation struct {
	Observation     worktree.Observation        `json:"observation"`
	PathUsage       process.PathUsage           `json:"path_usage"`
	Filesystem      worktree.FilesystemEvidence `json:"filesystem"`
	GitIdentity     worktree.GitIdentity        `json:"git_identity"`
	PrimaryPath     string                      `json:"primary_path"`
	RecreateCommand string                      `json:"recreate_command"`
}

const worktreeValidationTimeout = 25 * time.Second

func (d *Daemon) validateWorktreeRemoval(ctx context.Context, record storage.WorktreeRecord) (worktreeValidation, error) {
	ctx, cancelValidation := context.WithTimeout(ctx, worktreeValidationTimeout)
	defer cancelValidation()
	if !d.cfg.Worktrees.Enabled {
		return worktreeValidation{}, errors.New("worktree inventory is disabled")
	}
	if d.plat.Name() != "darwin" {
		return worktreeValidation{}, errors.New("worktree removal is supported on macOS only")
	}
	if record.State != string(worktree.StateStale) || !record.Complete {
		return worktreeValidation{}, fmt.Errorf("worktree %s is %s, not stale with complete evidence", shortID(record.WorktreeID), record.State)
	}
	if record.DaemonStartedNs != d.startedAt.UnixNano() {
		return worktreeValidation{}, errors.New("daemon restart reset the inactivity window")
	}
	if record.InactiveSinceNs == 0 || time.Since(time.Unix(0, record.InactiveSinceNs)) < d.cfg.Worktrees.StaleAfter.D() {
		return worktreeValidation{}, errors.New("seven continuous days of inactivity are not established")
	}
	if d.worktreeGit == nil {
		return worktreeValidation{}, d.worktreeGitErr
	}
	if err := d.worktreeGit.VerifyIdentity(); err != nil {
		return worktreeValidation{}, err
	}
	if err := d.validateWorktreeDiscovery(ctx, record); err != nil {
		return worktreeValidation{}, err
	}
	var storedGit worktree.GitIdentity
	if err := json.Unmarshal([]byte(record.GitIdentityJSON), &storedGit); err != nil || storedGit != d.worktreeGit.Identity() {
		return worktreeValidation{}, errors.New("resolved Git executable identity differs from inventory")
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
	// Wall-clock collection time is not removal authority. The durable
	// inactivity timestamps below are bound; zeroing this scan-local timestamp
	// keeps equivalent fresh observations comparable at apply.
	observation.ObservedAt = time.Time{}
	if d.activeSessionInWorktree(ctx, record.Path) {
		return worktreeValidation{}, errors.New("an active agent session remains associated with the worktree")
	}
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	usage, err := d.plat.InspectPathUsage(inspectCtx, record.Path)
	cancel()
	if err != nil || !usage.Complete {
		if err == nil {
			err = errors.New("same-user process usage inspection was incomplete")
		}
		return worktreeValidation{}, err
	}
	if len(usage.ProcessKeys) > 0 || usage.CWDReferences > 0 || usage.OpenVnodes > 0 {
		return worktreeValidation{}, errors.New("same-user process working-directory or vnode usage remains inside the worktree")
	}
	fsCtx, fsCancel := context.WithTimeout(ctx, 10*time.Second)
	filesystem, err := worktree.InspectFilesystem(fsCtx, record.Path)
	fsCancel()
	if err != nil || !filesystem.Complete {
		if err == nil {
			err = errors.New("filesystem inspection was incomplete")
		}
		return worktreeValidation{}, err
	}
	if filesystem.NestedMounts != 0 {
		return worktreeValidation{}, errors.New("nested mounts are present inside the worktree")
	}
	var approved []worktree.ApprovedLink
	if err := json.Unmarshal([]byte(record.ApprovedLinksJSON), &approved); err != nil {
		return worktreeValidation{}, errors.New("stored environment-link evidence is invalid")
	}
	if !reflect.DeepEqual(approved, observation.ApprovedLinks) {
		return worktreeValidation{}, errors.New("approved environment symlink evidence changed")
	}
	if err := worktree.ValidateApprovedLinks(record.Path, approved); err != nil {
		return worktreeValidation{}, err
	}
	previous := worktree.Record{
		ID: record.WorktreeID, State: worktree.State(record.State), HEAD: record.HEAD, Ref: record.Ref,
		StatusFingerprint: record.StatusFingerprint, LastSeen: timeFromNs(record.LastSeenNs),
		LastActivity: timeFromNs(record.LastActivityNs), InactiveSince: timeFromNs(record.InactiveSinceNs),
		DaemonStarted: timeFromNs(record.DaemonStartedNs),
	}
	conclusion := worktree.Classify(previous, observation, time.Now(), d.startedAt,
		d.cfg.Worktrees.StaleAfter.D(), d.cfg.Worktrees.ScanInterval.D(), false, true)
	if conclusion.State != worktree.StateStale {
		return worktreeValidation{}, fmt.Errorf("fresh inactivity classification is %s", conclusion.State)
	}
	return worktreeValidation{
		Observation: observation, PathUsage: usage, Filesystem: filesystem,
		GitIdentity: d.worktreeGit.Identity(), PrimaryPath: primary,
		RecreateCommand: recreateWorktreeCommand(primary, record),
	}, nil
}

func (d *Daemon) validateWorktreeDiscovery(ctx context.Context, record storage.WorktreeRecord) error {
	repositories, err := d.worktreeRepositories(ctx, &sessions.Result{})
	if err != nil {
		return err
	}
	fresh := map[worktree.Source]bool{}
	for repositoryPath, sources := range repositories {
		registrations, listErr := d.worktreeGit.Registrations(ctx, repositoryPath)
		if listErr != nil {
			return listErr
		}
		if len(registrations) == 0 {
			continue
		}
		for _, registration := range registrations {
			if filepath.Clean(registration.Path) != record.Path {
				continue
			}
			registrationID, identityErr := d.worktreeGit.RegistrationID(registration)
			if identityErr != nil || registrationID != record.WorktreeID {
				continue
			}
			for source := range sources {
				fresh[source] = true
			}
		}
	}
	if len(fresh) == 0 {
		return errors.New("worktree is no longer reachable from configured discovery authority")
	}
	sources := make([]worktree.Source, 0, len(fresh))
	for source := range fresh {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i] < sources[j] })
	if marshalJSON(sources, "[]") != record.SourcesJSON {
		return errors.New("worktree discovery sources changed")
	}
	return nil
}

func validateFreshObservation(record storage.WorktreeRecord, obs worktree.Observation) error {
	if !obs.Complete || !obs.Present || !obs.Canonical {
		return errors.New("fresh Git observation is incomplete")
	}
	if obs.ID != record.WorktreeID || obs.Path != record.Path || obs.CommonGitDir != record.CommonGitDir || obs.AdminGitDir != record.AdminGitDir {
		return errors.New("registered worktree identity or path changed")
	}
	if obs.PathIdentity.Device != record.PathDevice || obs.PathIdentity.Inode != record.PathInode {
		return errors.New("worktree directory inode changed")
	}
	if obs.HEAD != record.HEAD || obs.Ref != record.Ref || obs.Branch != record.Branch || obs.Status.Fingerprint != record.StatusFingerprint {
		return errors.New("worktree HEAD, ref, branch or aggregate status changed")
	}
	if len(obs.Protection) > 0 {
		return fmt.Errorf("worktree is protected: %v", obs.Protection)
	}
	return nil
}

func (d *Daemon) activeSessionInWorktree(ctx context.Context, path string) bool {
	records, err := d.store.ListSessions(ctx, storage.SessionFilter{})
	if err != nil {
		return true
	}
	for _, record := range records {
		if record.RepositoryPath == path && sessions.State(record.State).Live() {
			return true
		}
	}
	return false
}
