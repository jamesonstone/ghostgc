// Package codex implements the exact Codex shell-snapshot cache provider.
package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
	"github.com/jamesonstone/ghostgc/internal/cachefs"
	"github.com/jamesonstone/ghostgc/internal/cacheprovider"
)

var threadIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

const minimumExclusiveConfidence = 0.95

// Provider observes only CODEX_HOME/shell_snapshots.
type Provider struct {
	uid   uint32
	roots map[string]bool
}

// New constructs the provider for the daemon user.
func New(uid uint32, roots ...string) *Provider {
	allowed := make(map[string]bool, len(roots))
	for _, root := range roots {
		allowed[root] = true
	}
	return &Provider{uid: uid, roots: allowed}
}

// ID returns the pinned provider contract identifier.
func (p *Provider) ID() string { return cacheartifact.ProviderCodexShellSnapshot }

// Observe maps each entry to an exact known Codex session or protects it.
func (p *Provider) Observe(ctx context.Context, sessions []cacheprovider.Session, filesystem cachefs.Filesystem, maxEntries int) (cacheprovider.Result, error) {
	if maxEntries < 1 {
		return cacheprovider.Result{Complete: false}, errors.New("codex cache provider: traversal bound must be positive")
	}
	complete := true
	sessions = append([]cacheprovider.Session(nil), sessions...)
	sort.Slice(sessions, func(i, j int) bool {
		left := sessions[i].ID + "\x00" + sessions[i].NativeID + "\x00" + sessions[i].CodexHome
		right := sessions[j].ID + "\x00" + sessions[j].NativeID + "\x00" + sessions[j].CodexHome
		return left < right
	})
	if len(sessions) > maxEntries {
		sessions = sessions[:maxEntries]
		complete = false
	}
	byNative := make(map[string][]cacheprovider.Session)
	roots := make(map[string]bool)
	for _, session := range sessions {
		if session.Agent != cacheartifact.AgentCodex || session.CodexHome == "" {
			continue
		}
		if session.NativeID != "" {
			byNative[session.NativeID] = append(byNative[session.NativeID], session)
		}
		root := filepath.Join(session.CodexHome, "shell_snapshots")
		if p.roots[root] {
			roots[root] = true
		}
	}
	rootPaths := make([]string, 0, len(roots))
	for root := range roots {
		rootPaths = append(rootPaths, root)
	}
	sort.Strings(rootPaths)
	if len(rootPaths) > maxEntries {
		rootPaths = rootPaths[:maxEntries]
		complete = false
	}

	result := cacheprovider.Result{Complete: complete}
	remaining := maxEntries
	for _, root := range rootPaths {
		if remaining < 1 {
			result.Complete = false
			break
		}
		snapshot, err := filesystem.Snapshot(ctx, root, remaining)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return cacheprovider.Result{}, fmt.Errorf("codex cache provider: %w", err)
		}
		remaining -= len(snapshot.Entries)
		result.Inspected += len(snapshot.Entries)
		if !snapshot.Complete {
			result.Complete = false
		}
		for _, entry := range snapshot.Entries {
			if entry.Name == cacheartifact.QuarantineDirectory {
				continue
			}
			artifact := p.artifact(root, snapshot.Root, entry, byNative)
			if !snapshot.Complete {
				protect(&artifact, "provider traversal limit was exhausted; the scan is incomplete")
			}
			result.Artifacts = append(result.Artifacts, artifact)
		}
	}
	if !result.Complete {
		for i := range result.Artifacts {
			protect(&result.Artifacts[i], "bounded provider scan was incomplete")
		}
	}
	return result, nil
}

func (p *Provider) artifact(root string, rootID cacheartifact.Identity, entry cachefs.Entry, sessions map[string][]cacheprovider.Session) cacheartifact.Artifact {
	identity := entry.Identity
	artifact := cacheartifact.Artifact{
		ID:             cacheartifact.ArtifactID(p.ID(), rootID, identity),
		Provider:       p.ID(),
		Agent:          cacheartifact.AgentCodex,
		Kind:           cacheartifact.KindShellSnapshot,
		RootPath:       root,
		RelativePath:   entry.Name,
		Identity:       identity,
		RootIdentity:   rootID,
		IdentityDigest: identity.Digest(),
		ManifestDigest: cacheartifact.ManifestDigest(entry.Name, identity),
		Lifecycle:      cacheartifact.StateObserved,
		Evidence:       []string{"OpenAI Codex names shell snapshots from one ThreadId beneath CODEX_HOME/shell_snapshots"},
	}
	if rootID.UID != p.uid || identity.UID != p.uid {
		protect(&artifact, "provider root or entry belongs to another user")
	}
	if identity.Device != rootID.Device {
		protect(&artifact, "entry crosses the provider root filesystem")
	}
	if identity.EntryType != "regular" {
		protect(&artifact, "entry is not a regular file")
	}
	if identity.Nlink != 1 {
		protect(&artifact, "regular file has an unexpected hard-link count")
	}
	nativeID, ok := parseSnapshotName(entry.Name)
	if !ok {
		protect(&artifact, "filename does not match the pinned Codex shell-snapshot contract")
		return artifact
	}
	matches := sessions[nativeID]
	if len(matches) != 1 {
		protect(&artifact, fmt.Sprintf("thread id maps to %d known Codex sessions; exact ownership requires one", len(matches)))
		return artifact
	}
	session := matches[0]
	artifact.SessionID = session.ID
	artifact.Evidence = append(artifact.Evidence, "filename thread id exactly matches the known Codex native session id "+nativeID)
	if session.Confidence < minimumExclusiveConfidence {
		protect(&artifact, fmt.Sprintf("owning session confidence %.2f is below the exclusive-ownership threshold %.2f", session.Confidence, minimumExclusiveConfidence))
	}
	if session.State != "completed" {
		protect(&artifact, "owning session lifecycle is "+session.State+", not completed")
	}
	if session.LiveProcesses != 0 {
		protect(&artifact, fmt.Sprintf("%d live process(es) still claim the owning session", session.LiveProcesses))
	}
	return artifact
}

func parseSnapshotName(name string) (string, bool) {
	extension := filepath.Ext(name)
	if extension != ".sh" && extension != ".ps1" {
		return "", false
	}
	stem := strings.TrimSuffix(name, extension)
	nativeID, generation, ok := strings.Cut(stem, ".")
	if !ok || strings.Contains(generation, ".") || !threadIDPattern.MatchString(nativeID) {
		return "", false
	}
	if _, err := strconv.ParseUint(generation, 10, 64); err != nil {
		return "", false
	}
	return nativeID, true
}

func protect(artifact *cacheartifact.Artifact, reason string) {
	artifact.Lifecycle = cacheartifact.StateProtected
	if artifact.Reason == "" {
		artifact.Reason = reason
	}
	for _, existing := range artifact.Evidence {
		if existing == reason {
			return
		}
	}
	artifact.Evidence = append(artifact.Evidence, reason)
}
