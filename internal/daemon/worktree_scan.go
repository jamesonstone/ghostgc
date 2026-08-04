package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/sessions"
	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/worktree"
)

const (
	maxInventoryWorktrees = 500
	worktreeScanTimeout   = 30 * time.Second
)

type worktreeBatch struct {
	due     bool
	at      time.Time
	records []storage.WorktreeRecord
	audit   []storage.AuditRecord
}

type sourcedObservation struct {
	observation worktree.Observation
	sources     map[worktree.Source]bool
}

type registrationGroup struct {
	records []worktree.Registration
	sources map[worktree.Source]bool
}

func (d *Daemon) collectWorktrees(ctx context.Context, snap *process.Snapshot, result *sessions.Result) worktreeBatch {
	ctx, cancel := context.WithTimeout(ctx, worktreeScanTimeout)
	defer cancel()
	now := snap.Taken
	batch := worktreeBatch{at: now}
	if !d.cfg.Worktrees.Enabled || (!d.lastWorktreeAt.IsZero() && now.Sub(d.lastWorktreeAt) < d.cfg.Worktrees.ScanInterval.D()) {
		return batch
	}
	batch.due = true
	existing, err := d.store.ListWorktrees(ctx, storage.WorktreeFilter{Limit: maxInventoryWorktrees})
	if err != nil {
		return d.unknownWorktreeBatch(existing, now, err)
	}
	if d.worktreeGitErr != nil {
		return d.unknownWorktreeBatch(existing, now, d.worktreeGitErr)
	}
	if err := d.worktreeGit.VerifyIdentity(); err != nil {
		return d.unknownWorktreeBatch(existing, now, err)
	}
	repositories, err := d.worktreeRepositories(ctx, result)
	if err != nil {
		return d.unknownWorktreeBatch(existing, now, err)
	}
	observed := make(map[string]*sourcedObservation)
	groups := make(map[string]*registrationGroup)
	processEvidenceComplete := completeWorktreeProcessEvidence(snap)
	pathIDs := make(map[string]string, len(existing))
	for _, previous := range existing {
		pathIDs[previous.Path] = previous.WorktreeID
	}
	for repositoryPath, sources := range repositories {
		registrations, listErr := d.worktreeGit.Registrations(ctx, repositoryPath)
		if listErr != nil {
			return d.unknownWorktreeBatch(existing, now, listErr)
		}
		if len(registrations) == 0 {
			continue
		}
		key := registrations[0].CommonGitDir
		if key == "" {
			key = filepath.Clean(registrations[0].Path)
		}
		group := groups[key]
		if group == nil {
			group = &registrationGroup{records: registrations, sources: map[worktree.Source]bool{}}
			groups[key] = group
		}
		for source := range sources {
			group.sources[source] = true
		}
	}
	for _, group := range groups {
		primary := group.records[0].Path
		for _, registration := range group.records {
			obs := d.worktreeGit.Inspect(ctx, registration, primary)
			if obs.ID == "" {
				obs.ID = pathIDs[registration.Path]
			}
			if obs.ID == "" {
				continue
			}
			entry := observed[obs.ID]
			if entry == nil {
				entry = &sourcedObservation{observation: obs, sources: map[worktree.Source]bool{}}
				observed[obs.ID] = entry
			}
			for source := range group.sources {
				entry.sources[source] = true
			}
			if len(observed) > maxInventoryWorktrees {
				return d.unknownWorktreeBatch(existing, now, fmt.Errorf("worktree inventory exceeded %d entries", maxInventoryWorktrees))
			}
		}
	}
	previous := make(map[string]storage.WorktreeRecord, len(existing))
	for _, record := range existing {
		previous[record.WorktreeID] = record
	}
	for id, entry := range observed {
		batch.records = append(batch.records, d.worktreeRecord(previous[id], entry, snap, result, now, processEvidenceComplete))
		delete(previous, id)
	}
	for _, missing := range previous {
		if missing.State == string(worktree.StateRemoved) {
			continue
		}
		batch.records = append(batch.records, missingWorktreeRecord(missing, now, d.startedAt))
	}
	sort.Slice(batch.records, func(i, j int) bool { return batch.records[i].WorktreeID < batch.records[j].WorktreeID })
	return batch
}

func (d *Daemon) worktreeRepositories(ctx context.Context, current *sessions.Result) (map[string]map[worktree.Source]bool, error) {
	out := make(map[string]map[worktree.Source]bool)
	add := func(path string, source worktree.Source) {
		if path == "" {
			return
		}
		if info, err := os.Lstat(path); err != nil || !info.IsDir() {
			return
		}
		if out[path] == nil {
			out[path] = map[worktree.Source]bool{}
		}
		out[path][source] = true
	}
	stored, err := d.store.ListSessions(ctx, storage.SessionFilter{})
	if err != nil {
		return nil, err
	}
	for _, session := range stored {
		add(session.RepositoryPath, worktree.SourceSession)
	}
	for _, session := range current.Sessions {
		add(session.RepositoryPath, worktree.SourceSession)
	}
	for _, root := range d.cfg.Worktrees.Roots {
		repositories, discoverErr := worktree.DiscoverRepositories(ctx, root)
		if discoverErr != nil {
			return nil, discoverErr
		}
		for _, path := range repositories {
			add(path, worktree.SourceRoot)
		}
	}
	if len(out) > maxInventoryWorktrees {
		return nil, fmt.Errorf("worktree repository source count exceeded %d", maxInventoryWorktrees)
	}
	return out, nil
}

func (d *Daemon) worktreeRecord(previous storage.WorktreeRecord, entry *sourcedObservation,
	snap *process.Snapshot, result *sessions.Result, now time.Time, processEvidenceComplete bool) storage.WorktreeRecord {
	obs := entry.observation
	active := worktreeActive(obs.Path, snap, result)
	sources := make([]worktree.Source, 0, len(entry.sources))
	for source := range entry.sources {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i] < sources[j] })
	sourcesJSON := marshalJSON(sources, "[]")
	prior := worktree.Record{
		ID: previous.WorktreeID, State: worktree.State(previous.State), HEAD: previous.HEAD,
		Ref: previous.Ref, StatusFingerprint: previous.StatusFingerprint,
		LastSeen: timeFromNs(previous.LastSeenNs), LastActivity: timeFromNs(previous.LastActivityNs),
		InactiveSince: timeFromNs(previous.InactiveSinceNs), DaemonStarted: timeFromNs(previous.DaemonStartedNs),
	}
	if previous.WorktreeID == "" {
		prior = worktree.Record{}
	}
	if previous.WorktreeID != "" && (previous.Path != obs.Path ||
		previous.PathDevice != obs.PathIdentity.Device || previous.PathInode != obs.PathIdentity.Inode) {
		prior.State = worktree.StateUnknown
	}
	if previous.WorktreeID != "" && previous.SourcesJSON != sourcesJSON {
		prior.State = worktree.StateUnknown
	}
	conclusion := worktree.Classify(prior, obs, now, d.startedAt, d.cfg.Worktrees.StaleAfter.D(),
		d.cfg.Worktrees.ScanInterval.D(), active, processEvidenceComplete)
	if active {
		conclusion.Protection = append(conclusion.Protection, "same_user_process_or_active_session")
	}
	if !processEvidenceComplete {
		conclusion.Protection = append(conclusion.Protection, "process_scan_incomplete")
	}
	firstSeen := previous.FirstSeenNs
	if firstSeen == 0 {
		firstSeen = now.UnixNano()
	}
	evidence, _ := json.Marshal(struct {
		Status                                                      worktree.StatusEvidence `json:"status"`
		Primary, Detached, Published, DetachedReachable, Submodules bool
		Operations                                                  []string `json:"operations,omitempty"`
	}{obs.Status, obs.Primary, obs.Detached, obs.Published, obs.DetachedReachable, obs.Submodules, obs.Operations})
	return storage.WorktreeRecord{
		WorktreeID: obs.ID, Path: obs.Path, PathDevice: obs.PathIdentity.Device, PathInode: obs.PathIdentity.Inode,
		CommonGitDir: obs.CommonGitDir, AdminGitDir: obs.AdminGitDir, HEAD: obs.HEAD, Ref: obs.Ref, Branch: obs.Branch,
		SourcesJSON: sourcesJSON, State: string(conclusion.State), FirstSeenNs: firstSeen,
		LastSeenNs: now.UnixNano(), LastActivityNs: conclusion.LastActivity.UnixNano(),
		InactiveSinceNs: timeOrZero(conclusion.InactiveSince), DaemonStartedNs: d.startedAt.UnixNano(),
		StatusFingerprint: obs.Status.Fingerprint, ProtectionJSON: marshalJSON(conclusion.Protection, "[]"),
		EvidenceJSON: string(evidence), ApprovedLinksJSON: marshalJSON(obs.ApprovedLinks, "[]"),
		GitIdentityJSON: marshalJSON(d.worktreeGit.Identity(), "{}"), Complete: obs.Complete && processEvidenceComplete,
	}
}

func completeWorktreeProcessEvidence(snap *process.Snapshot) bool {
	for _, observed := range snap.Processes {
		if !observed.Detailed || (observed.Status != process.StatusZombie && observed.CWD == "") {
			return false
		}
	}
	return true
}

func worktreeActive(root string, snap *process.Snapshot, result *sessions.Result) bool {
	for _, observed := range snap.Processes {
		if observed.Detailed && withinWorktree(root, observed.CWD) {
			return true
		}
	}
	for _, session := range result.Sessions {
		if sessions.State(session.State).Live() && session.RepositoryPath == root {
			return true
		}
	}
	return false
}

func withinWorktree(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func timeOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixNano()
}

func timeFromNs(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value)
}
func marshalJSON(value any, fallback string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(raw)
}
