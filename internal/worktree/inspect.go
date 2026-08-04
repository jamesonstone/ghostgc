package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var operationPaths = []string{
	"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG",
	"rebase-apply", "rebase-merge", "sequencer", "index.lock", "locked",
}

// Inspect resolves one present registration into bounded aggregate evidence.
func (g *Git) Inspect(ctx context.Context, rec Registration, primaryPath string) Observation {
	now := time.Now()
	obs := Observation{
		Path: rec.Path, HEAD: rec.HEAD, Ref: rec.Ref, Branch: rec.Branch,
		Detached: rec.Detached, Locked: rec.Locked, Prunable: rec.Prunable,
		Present: true, Complete: false, ObservedAt: now,
	}
	info, err := os.Lstat(rec.Path)
	if err != nil {
		if os.IsNotExist(err) {
			obs.Present = false
			obs.Protection = append(obs.Protection, "worktree_missing")
			g.identifyMissingRegistration(&obs, rec)
		} else {
			obs.Protection = append(obs.Protection, "worktree_unreadable")
		}
		return obs
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		obs.Canonical = false
		obs.Protection = append(obs.Protection, "worktree_path_symlinked")
		return obs
	}
	canonical, err := filepath.EvalSymlinks(rec.Path)
	if err != nil || canonical != filepath.Clean(rec.Path) {
		obs.Protection = append(obs.Protection, "worktree_path_not_canonical")
		return obs
	}
	obs.Canonical = true
	obs.Path = canonical
	obs.PathIdentity, err = Identify(canonical)
	if err != nil {
		obs.Protection = append(obs.Protection, "worktree_unreadable")
		return obs
	}
	obs.CommonGitDir, err = g.revPath(ctx, canonical, "--git-common-dir")
	if err != nil {
		obs.Protection = append(obs.Protection, "git_common_directory_unavailable")
		return obs
	}
	obs.AdminGitDir, err = g.revPath(ctx, canonical, "--git-dir")
	if err != nil {
		obs.Protection = append(obs.Protection, "git_admin_directory_unavailable")
		return obs
	}
	common, err := filepath.EvalSymlinks(obs.CommonGitDir)
	if err != nil {
		obs.Protection = append(obs.Protection, "git_common_directory_unreadable")
		return obs
	}
	admin, err := filepath.EvalSymlinks(obs.AdminGitDir)
	if err != nil {
		obs.Protection = append(obs.Protection, "git_admin_directory_unreadable")
		return obs
	}
	obs.CommonGitDir, obs.AdminGitDir = common, admin
	obs.CommonIdentity, err = Identify(common)
	if err != nil {
		obs.Protection = append(obs.Protection, "git_common_directory_unreadable")
		return obs
	}
	obs.AdminIdentity, err = Identify(admin)
	if err != nil {
		obs.Protection = append(obs.Protection, "git_admin_directory_unreadable")
		return obs
	}
	obs.ID = StableID(obs.CommonIdentity, obs.AdminIdentity)
	primaryCanonical, _ := filepath.EvalSymlinks(primaryPath)
	obs.Primary = canonical == primaryCanonical || common == admin

	approved := func(kind byte, path string) bool {
		link, ok := approvedEnvironmentLink(canonical, primaryCanonical, path)
		if ok {
			obs.ApprovedLinks = append(obs.ApprovedLinks, link)
		}
		return ok
	}
	raw, err := g.run(ctx, canonical, "status", "--porcelain=v2", "-z", "--ignored=matching", "--untracked-files=all")
	if err != nil {
		obs.Protection = append(obs.Protection, "git_status_incomplete")
		return obs
	}
	obs.Status, err = ParseStatus(raw, approved)
	if err != nil {
		obs.Protection = append(obs.Protection, "git_status_incomplete")
		return obs
	}
	obs.Operations = gitOperations(admin)
	obs.Submodules = g.succeeds(ctx, canonical, "ls-files", "--error-unmatch", "--", ".gitmodules") ||
		pathExists(filepath.Join(common, "modules")) || pathExists(filepath.Join(admin, "modules"))
	obs.Published, obs.DetachedReachable, err = g.reachability(ctx, obs)
	if err != nil {
		obs.Protection = append(obs.Protection, "git_reachability_incomplete")
		return obs
	}
	obs.Complete = true
	obs.Protection = append(obs.Protection, baseProtections(obs)...)
	sort.Strings(obs.Protection)
	return obs
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func (g *Git) identifyMissingRegistration(obs *Observation, rec Registration) {
	if rec.CommonGitDir == "" || rec.AdminGitDir == "" {
		return
	}
	common, commonErr := filepath.EvalSymlinks(rec.CommonGitDir)
	admin, adminErr := filepath.EvalSymlinks(rec.AdminGitDir)
	if commonErr != nil || adminErr != nil {
		return
	}
	commonIdentity, commonErr := Identify(common)
	adminIdentity, adminErr := Identify(admin)
	if commonErr != nil || adminErr != nil {
		return
	}
	obs.CommonGitDir, obs.AdminGitDir = common, admin
	obs.CommonIdentity, obs.AdminIdentity = commonIdentity, adminIdentity
	obs.ID = StableID(commonIdentity, adminIdentity)
}

func approvedEnvironmentLink(root, primary, relative string) (ApprovedLink, bool) {
	if primary == "" || (relative != ".env" && relative != ".envrc") {
		return ApprovedLink{}, false
	}
	path := filepath.Join(root, relative)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return ApprovedLink{}, false
	}
	text, err := os.Readlink(path)
	if err != nil {
		return ApprovedLink{}, false
	}
	target := text
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target, err = filepath.EvalSymlinks(target)
	want := filepath.Join(primary, relative)
	if err != nil || target != want {
		return ApprovedLink{}, false
	}
	identity, err := Identify(want)
	if err != nil {
		return ApprovedLink{}, false
	}
	return ApprovedLink{Name: relative, LinkText: text, Target: identity}, true
}

func gitOperations(admin string) []string {
	var out []string
	for _, name := range operationPaths {
		if _, err := os.Lstat(filepath.Join(admin, name)); err == nil {
			out = append(out, name)
		}
	}
	return out
}

func baseProtections(obs Observation) []string {
	var out []string
	if obs.Primary {
		out = append(out, "primary_worktree")
	}
	if obs.Locked {
		out = append(out, "worktree_locked")
	}
	if obs.Prunable {
		out = append(out, "worktree_prunable")
	}
	if !obs.Status.Clean() {
		out = append(out, "worktree_dirty")
	}
	if len(obs.Operations) > 0 {
		out = append(out, "git_operation_in_progress")
	}
	if obs.Submodules {
		out = append(out, "submodule_metadata_present")
	}
	if !obs.Published {
		out = append(out, "local_only_commits")
	}
	if obs.Detached && !obs.DetachedReachable {
		out = append(out, "detached_head_unreachable")
	}
	return out
}

func (g *Git) reachability(ctx context.Context, obs Observation) (published, detachedReachable bool, err error) {
	if obs.Detached {
		raw, runErr := g.run(ctx, obs.Path, "for-each-ref", "--contains=HEAD", "--format=%(objectname)", "refs/heads", "refs/tags")
		if runErr != nil {
			return false, false, runErr
		}
		reachable := strings.TrimSpace(string(raw)) != ""
		return reachable, reachable, nil
	}
	if g.succeeds(ctx, obs.Path, "rev-parse", "--verify", "@{upstream}") {
		ahead, countErr := g.count(ctx, obs.Path, "rev-list", "--count", "@{upstream}..HEAD")
		return ahead == 0, false, countErr
	}
	raw, runErr := g.run(ctx, obs.Path, "branch", "-r", "--contains", "HEAD", "--format=%(objectname)")
	if runErr != nil {
		return false, false, runErr
	}
	return strings.TrimSpace(string(raw)) != "", false, nil
}

// FindRegistration resolves a stable id from a fresh registered inventory.
func (g *Git) FindRegistration(ctx context.Context, repository, id string) (Registration, string, error) {
	recs, err := g.Registrations(ctx, repository)
	if err != nil {
		return Registration{}, "", err
	}
	if len(recs) == 0 {
		return Registration{}, "", fmt.Errorf("worktree: repository has no registrations")
	}
	primary := recs[0].Path
	for _, rec := range recs {
		registrationID, identityErr := g.RegistrationID(rec)
		if identityErr == nil && registrationID == id {
			return rec, primary, nil
		}
	}
	return Registration{}, primary, fmt.Errorf("worktree: registration identity is absent")
}
