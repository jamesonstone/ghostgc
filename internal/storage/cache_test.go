package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

func TestCacheEvaluationRoundTripAndCurrentProjection(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	artifact := storedArtifact()
	evaluation := cacheartifact.Evaluation{ObservedNs: 100, ConfigurationDigest: "cfg", Complete: true, Inspected: 1, Candidates: 1}
	decision := cacheartifact.Decision{ArtifactID: artifact.ID, PolicyID: artifact.PolicyID, Result: "recommended", Reason: "exact", Evidence: []string{"one", "two"}}
	evaluationID, err := store.PersistCacheEvaluation(ctx, evaluation, []cacheartifact.Artifact{artifact}, []cacheartifact.Decision{decision})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.CacheArtifact(ctx, artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EvaluationID != evaluationID || !reflect.DeepEqual(got.Identity, artifact.Identity) || !reflect.DeepEqual(got.Evidence, artifact.Evidence) {
		t.Fatalf("cache round trip changed stable evidence: %#v", got)
	}
	current, err := store.ListCacheArtifacts(ctx, string(cacheartifact.StateRecommended), true)
	if err != nil || len(current) != 1 {
		t.Fatalf("current recommendation = %d, %v", len(current), err)
	}

	if _, err := store.PersistCacheEvaluation(ctx, cacheartifact.Evaluation{ObservedNs: 200, ConfigurationDigest: "cfg", Complete: false, Error: "scan failed"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	current, err = store.ListCacheArtifacts(ctx, "", true)
	if err != nil || len(current) != 0 {
		t.Fatalf("failed scan must fail closed instead of preserving current authority: %#v, %v", current, err)
	}
	counts, err := store.Counts(ctx)
	if err != nil || counts.CacheCandidates != 0 {
		t.Fatalf("bounded-state counts retained stale authority: %#v, %v", counts, err)
	}
}

func TestCacheLifecycleTransitionsAreDurable(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	artifact := storedArtifact()
	if _, err := store.PersistCacheEvaluation(ctx, cacheartifact.Evaluation{ObservedNs: 100, ConfigurationDigest: "cfg", Complete: true}, []cacheartifact.Artifact{artifact}, nil); err != nil {
		t.Fatal(err)
	}

	beginAction(t, store, "cleanup-1", artifact.ID, "cleanup", "attempting", 110)
	quarantine := cacheartifact.Quarantine{
		ArtifactID: artifact.ID, RootPath: artifact.RootPath, OriginalPath: artifact.RelativePath,
		QuarantinePath: ".ghostgc-quarantine/ca_test", Identity: artifact.Identity,
		ManifestDigest:   cacheartifact.ManifestDigest(".ghostgc-quarantine/ca_test", artifact.Identity),
		OriginalManifest: artifact.ManifestDigest, QuarantinedNs: 120, GraceUntilNs: 180,
		Status: "quarantined", UpdatedNs: 120, Configuration: "cfg",
	}
	if err := store.RecordCacheQuarantined(ctx, "cleanup-1", quarantine); err != nil {
		t.Fatal(err)
	}
	assertCacheState(t, store, artifact.ID, cacheartifact.StateQuarantined, "quarantined")

	beginAction(t, store, "restore-1", artifact.ID, "restore", "attempting", 130)
	if err := store.RecordCacheRestored(ctx, "restore-1", artifact.ID, artifact.Identity, artifact.ManifestDigest, 140); err != nil {
		t.Fatal(err)
	}
	assertCacheState(t, store, artifact.ID, cacheartifact.StateRestored, "restored")

	beginAction(t, store, "cleanup-2", artifact.ID, "cleanup", "attempting", 150)
	quarantine.QuarantinedNs, quarantine.UpdatedNs = 160, 160
	if err := store.RecordCacheQuarantined(ctx, "cleanup-2", quarantine); err != nil {
		t.Fatal(err)
	}
	beginAction(t, store, "purge-1", artifact.ID, "purge", "purging", 190)
	if err := store.RecordCachePurged(ctx, "purge-1", artifact.ID, 200); err != nil {
		t.Fatal(err)
	}
	assertCacheState(t, store, artifact.ID, cacheartifact.StatePurged, "purged")
}

func TestUnresolvedCacheActionSurvivesRestartAndRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ghostgc.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	artifact := storedArtifact()
	artifact.LastObservedNs = time.Now().Add(-48 * time.Hour).UnixNano()
	if _, err := store.PersistCacheEvaluation(ctx, cacheartifact.Evaluation{ObservedNs: artifact.LastObservedNs, ConfigurationDigest: "cfg", Complete: true}, []cacheartifact.Artifact{artifact}, nil); err != nil {
		t.Fatal(err)
	}
	beginAction(t, store, "interrupted", artifact.ID, "purge", "purging", artifact.LastObservedNs)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	actions, err := store.ListCacheActions(ctx, artifact.ID, "", "purging", 10)
	if err != nil || len(actions) != 1 || actions[0].ActionID != "interrupted" {
		t.Fatalf("interrupted action after restart = %#v, %v", actions, err)
	}

	policy := RetentionPolicy{
		RawObservations: time.Hour, Scans: time.Hour, Audit: time.Hour, PolicyDecisions: time.Hour,
		Actions: time.Hour, ExitedProcesses: time.Hour, EndedSessions: time.Hour,
	}
	if _, err := store.Compact(ctx, policy, time.Now()); err != nil {
		t.Fatal(err)
	}
	actions, err = store.ListCacheActions(ctx, artifact.ID, "", "purging", 10)
	if err != nil || len(actions) != 1 {
		t.Fatalf("retention removed unresolved action: %#v, %v", actions, err)
	}
}

func storedArtifact() cacheartifact.Artifact {
	identity := cacheartifact.Identity{UID: 501, Device: 1, Inode: 2, Mode: 0o100600, Nlink: 1, Size: 7, MTimeNs: 3, CTimeNs: 4, ATimeNs: 5, EntryType: "regular"}
	root := cacheartifact.Identity{UID: 501, Device: 1, Inode: 1, Mode: 0o40700, Nlink: 2, EntryType: "directory"}
	return cacheartifact.Artifact{
		ID: "ca_test", Provider: cacheartifact.ProviderCodexShellSnapshot, Agent: "codex", SessionID: "session-1",
		Kind: cacheartifact.KindShellSnapshot, RootPath: "/cache", RelativePath: "thread.1.sh",
		Identity: identity, RootIdentity: root, IdentityDigest: identity.Digest(),
		ManifestDigest: cacheartifact.ManifestDigest("thread.1.sh", identity), FirstObservedNs: 100,
		LastObservedNs: 100, StableSinceNs: 100, Lifecycle: cacheartifact.StateRecommended,
		Reason: "exact", Evidence: []string{"ownership", "stable"}, Configuration: "cfg", PolicyID: "codex-snapshots",
	}
}

func beginAction(t *testing.T, store *Store, id, artifactID, kind, result string, at int64) {
	t.Helper()
	if err := store.BeginCacheAction(context.Background(), cacheartifact.Action{
		ActionID: id, ArtifactID: artifactID, Kind: kind, PolicyID: "codex-snapshots",
		RequestedNs: at, UpdatedNs: at, Result: result, Reason: "test", Evidence: []string{"durable"},
	}); err != nil {
		t.Fatal(err)
	}
}

func assertCacheState(t *testing.T, store *Store, artifactID string, lifecycle cacheartifact.Lifecycle, actionResult string) {
	t.Helper()
	artifact, err := store.CacheArtifact(context.Background(), artifactID)
	if err != nil || artifact.Lifecycle != lifecycle {
		t.Fatalf("artifact lifecycle = %q, %v", artifact.Lifecycle, err)
	}
	actions, err := store.ListCacheActions(context.Background(), artifactID, "", actionResult, 10)
	if err != nil || len(actions) == 0 {
		t.Fatalf("missing %s action evidence: %#v, %v", actionResult, actions, err)
	}
}
