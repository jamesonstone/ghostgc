package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
	"github.com/jamesonstone/ghostgc/internal/cachefs"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform/platformtest"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

const fixtureThreadID = "019fcde3-594a-7eb1-a102-ee8c7893c2dc"

func TestCacheLifecycleFixture(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	daemon, filesystem, store := newCacheFixture(t, &now)
	root := "/tmp/codex/shell_snapshots"
	name := fixtureThreadID + ".100.sh"

	setFixtureSessionState(t, store, "active")
	daemon.CacheScanAt(ctx, now)
	assertCandidateCount(t, daemon, 0)
	setFixtureSessionState(t, store, "completed")
	daemon.CacheScanAt(ctx, now)
	assertCandidateCount(t, daemon, 0)
	now = now.Add(time.Minute)
	daemon.CacheScanAt(ctx, now)
	candidates, err := daemon.CacheCandidates(ctx)
	if err != nil || len(candidates.Artifacts) != 1 {
		t.Fatalf("stable candidates = %#v, %v", candidates, err)
	}
	artifact := candidates.Artifacts[0]

	cleanup, err := daemon.CacheCleanupPreview(ctx, api.CachePreviewRequest{ArtifactID: artifact.ID, PolicyID: "codex-snapshots"})
	if err != nil || cleanup.Approval == "" {
		t.Fatalf("cleanup preview = %#v, %v", cleanup, err)
	}
	result, err := daemon.CacheCleanupApply(ctx, api.CacheApplyRequest{Approval: cleanup.Approval})
	if err != nil || result.Result != "quarantined" {
		t.Fatalf("cleanup apply = %#v, %v", result, err)
	}
	if filesystem.Exists(root, name) || !filesystem.Exists(root, cleanup.Destination) {
		t.Fatal("cleanup must move only the exact artifact into quarantine")
	}
	if !filesystem.Exists(root, "control.txt") {
		t.Fatal("cleanup changed the protected control entry")
	}
	replayed, err := daemon.CacheCleanupApply(ctx, api.CacheApplyRequest{Approval: cleanup.Approval})
	if err != nil || replayed.Result != "rejected" {
		t.Fatalf("approval replay = %#v, %v", replayed, err)
	}

	restore, err := daemon.CacheRestorePreview(ctx, api.CachePreviewRequest{ArtifactID: artifact.ID})
	if err != nil {
		t.Fatal(err)
	}
	result, err = daemon.CacheRestoreApply(ctx, api.CacheApplyRequest{Approval: restore.Approval})
	if err != nil || result.Result != "restored" || !filesystem.Exists(root, name) {
		t.Fatalf("restore apply = %#v, %v", result, err)
	}
	if !filesystem.Exists(root, "control.txt") {
		t.Fatal("restore changed the protected control entry")
	}

	daemon.CacheScanAt(ctx, now)
	assertCandidateCount(t, daemon, 0)
	now = now.Add(time.Minute)
	daemon.CacheScanAt(ctx, now)
	candidates, _ = daemon.CacheCandidates(ctx)
	cleanup, err = daemon.CacheCleanupPreview(ctx, api.CachePreviewRequest{ArtifactID: candidates.Artifacts[0].ID, PolicyID: "codex-snapshots"})
	if err != nil {
		t.Fatal(err)
	}
	if result, err = daemon.CacheCleanupApply(ctx, api.CacheApplyRequest{Approval: cleanup.Approval}); err != nil || result.Result != "quarantined" {
		t.Fatalf("second quarantine = %#v, %v", result, err)
	}
	now = now.Add(time.Minute)
	purge, err := daemon.CachePurgePreview(ctx, api.CachePreviewRequest{ArtifactID: artifact.ID, PolicyID: "codex-snapshots"})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := daemon.CachePurgeApply(ctx, api.CacheApplyRequest{Approval: purge.Approval, Confirmation: artifact.ID})
	if err != nil || prepared.Action.Result != "purging" {
		t.Fatalf("purge prepare = %#v, %v", prepared, err)
	}
	if err := filesystem.Purge(ctx, prepared.Plan.RootPath, prepared.Plan.QuarantinePath, prepared.Plan.RootIdentity, prepared.Plan.Identity); err != nil {
		t.Fatal(err)
	}
	result, err = daemon.CachePurgeComplete(ctx, api.CachePurgeCompleteRequest{ActionID: prepared.Plan.ActionID, Completion: prepared.Plan.Completion})
	if err != nil || result.Result != "purged" || filesystem.Exists(root, cleanup.Destination) {
		t.Fatalf("purge apply = %#v, %v", result, err)
	}
	if _, err := daemon.CachePurgeComplete(ctx, api.CachePurgeCompleteRequest{
		ActionID: prepared.Plan.ActionID, Completion: prepared.Plan.Completion,
	}); err == nil {
		t.Fatal("purge completion capability replay succeeded")
	}
	if !filesystem.Exists(root, "control.txt") {
		t.Fatal("purge changed the protected control entry")
	}
	actions, err := store.ListCacheActions(ctx, artifact.ID, "", "", 20)
	if err != nil || len(actions) < 5 {
		t.Fatalf("durable action history = %#v, %v", actions, err)
	}
}

func TestCacheApplyRefusesExpiredConfigAndIdentityChanges(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*Daemon, *cachefs.Fake, *time.Time){
		"expired": func(_ *Daemon, _ *cachefs.Fake, now *time.Time) { *now = (*now).Add(cacheApprovalLifetime) },
		"config":  func(d *Daemon, _ *cachefs.Fake, _ *time.Time) { d.cfg.Cache.MaxBytesPerAction-- },
		"identity": func(_ *Daemon, filesystem *cachefs.Fake, _ *time.Time) {
			changed := fixtureIdentity(2)
			changed.Size++
			filesystem.Put("/tmp/codex/shell_snapshots", fixtureThreadID+".100.sh", changed)
		},
		"inode": func(_ *Daemon, filesystem *cachefs.Fake, _ *time.Time) {
			filesystem.Put("/tmp/codex/shell_snapshots", fixtureThreadID+".100.sh", fixtureIdentity(4))
		},
		"manifest": func(_ *Daemon, filesystem *cachefs.Fake, _ *time.Time) {
			root := "/tmp/codex/shell_snapshots"
			filesystem.Delete(root, fixtureThreadID+".100.sh")
			filesystem.Put(root, fixtureThreadID+".101.sh", fixtureIdentity(2))
		},
	} {
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
			d, filesystem, _ := newCacheFixture(t, &now)
			d.CacheScanAt(ctx, now)
			now = now.Add(time.Minute)
			d.CacheScanAt(ctx, now)
			candidates, _ := d.CacheCandidates(ctx)
			preview, err := d.CacheCleanupPreview(ctx, api.CachePreviewRequest{ArtifactID: candidates.Artifacts[0].ID, PolicyID: "codex-snapshots"})
			if err != nil {
				t.Fatal(err)
			}
			mutate(d, filesystem, &now)
			result, err := d.CacheCleanupApply(ctx, api.CacheApplyRequest{Approval: preview.Approval})
			if err != nil || result.Result != "rejected" {
				t.Fatalf("unsafe apply = %#v, %v", result, err)
			}
		})
	}
}

func TestPartialPurgeRemainsVisible(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, filesystem, _ := newCacheFixture(t, &now)
	d.CacheScanAt(ctx, now)
	now = now.Add(time.Minute)
	d.CacheScanAt(ctx, now)
	candidates, _ := d.CacheCandidates(ctx)
	preview, _ := d.CacheCleanupPreview(ctx, api.CachePreviewRequest{ArtifactID: candidates.Artifacts[0].ID, PolicyID: "codex-snapshots"})
	_, _ = d.CacheCleanupApply(ctx, api.CacheApplyRequest{Approval: preview.Approval})
	now = now.Add(time.Minute)
	preview, _ = d.CachePurgePreview(ctx, api.CachePreviewRequest{ArtifactID: candidates.Artifacts[0].ID, PolicyID: "codex-snapshots"})
	prepared, err := d.CachePurgeApply(ctx, api.CacheApplyRequest{Approval: preview.Approval, Confirmation: candidates.Artifacts[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	filesystem.Delete(prepared.Plan.RootPath, prepared.Plan.QuarantinePath)
	filesystem.Put(prepared.Plan.RootPath, prepared.Plan.QuarantinePath, fixtureIdentity(9))
	result, err := d.CachePurgeComplete(ctx, api.CachePurgeCompleteRequest{ActionID: prepared.Plan.ActionID, Completion: prepared.Plan.Completion})
	if err != nil || result.Result != "partial" {
		t.Fatalf("partial purge = %#v, %v", result, err)
	}
	actions, _ := d.CacheActions(ctx, api.CacheActionOptions{ArtifactID: candidates.Artifacts[0].ID, Result: "partial"})
	if len(actions.Actions) != 1 {
		t.Fatalf("partial purge evidence = %#v", actions)
	}
	artifact, err := d.CacheArtifact(ctx, candidates.Artifacts[0].ID)
	if err != nil || artifact.Artifact.Lifecycle != cacheartifact.StatePartial {
		t.Fatalf("partial purge projection = %#v, %v", artifact, err)
	}
	replayed, err := d.CachePurgeApply(ctx, api.CacheApplyRequest{Approval: preview.Approval, Confirmation: candidates.Artifacts[0].ID})
	if err != nil || replayed.Action.Result != "rejected" {
		t.Fatalf("partial purge reused its approval: %#v, %v", replayed, err)
	}
	retry, err := d.CachePurgePreview(ctx, api.CachePreviewRequest{
		ArtifactID: candidates.Artifacts[0].ID, PolicyID: "codex-snapshots",
	})
	if err == nil || retry.Approval != "" {
		t.Fatalf("ambiguous purge did not trip the mutation circuit: %#v, %v", retry, err)
	}
}

func newCacheFixture(t *testing.T, now *time.Time) (*Daemon, *cachefs.Fake, *storage.Store) {
	t.Helper()
	store, err := storage.Open(t.TempDir() + "/ghostgc.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.WithTx(ctx, func(tx *storage.Tx) error {
		return tx.UpsertSession(storage.SessionRecord{
			SessionID: "session-1", NativeSessionID: fixtureThreadID, AgentID: "codex", State: "completed",
			RootProcUID: "1:1", RootPID: 1, Confidence: 1, MetadataJSON: `{"extra":{"CODEX_HOME":"/tmp/codex"}}`,
			EvidenceJSON: `[]`, StartedNs: (*now).UnixNano(), LastSeenNs: (*now).UnixNano(),
		})
	}); err != nil {
		t.Fatal(err)
	}
	filesystem := cachefs.NewFake()
	root := "/tmp/codex/shell_snapshots"
	filesystem.SetRoot(root, fixtureIdentity(1))
	filesystem.Put(root, fixtureThreadID+".100.sh", fixtureIdentity(2))
	filesystem.Put(root, "control.txt", fixtureIdentity(3))
	cfg := config.Default()
	cfg.Cache.Enabled = true
	cfg.Cache.GlobalMode = config.ModeRecommend
	cfg.Cache.Roots = []string{root}
	cfg.Cache.MinStable = config.Duration(time.Minute)
	cfg.Cache.QuarantineGrace = config.Duration(time.Minute)
	cfg.Cache.Policies = []config.CachePolicy{{
		ID: "codex-snapshots", Description: "fixture", Enabled: true, Mode: config.ModeRecommend,
		Provider: cacheartifact.ProviderCodexShellSnapshot, Agent: "codex",
		ArtifactKind: cacheartifact.KindShellSnapshot, SessionState: "completed",
	}}
	d, err := New(Options{Config: cfg, Store: store, Platform: platformtest.New(501), CacheFilesystem: filesystem, CacheClock: func() time.Time { return *now }})
	if err != nil {
		t.Fatal(err)
	}
	return d, filesystem, store
}

func fixtureIdentity(inode uint64) cacheartifact.Identity {
	kind, mode, links := "regular", uint32(0o100600), uint64(1)
	if inode == 1 {
		kind, mode, links = "directory", 0o40700, 2
	}
	return cacheartifact.Identity{UID: 501, Device: 7, Inode: inode, Mode: mode, Nlink: links, Size: 7, MTimeNs: 1, CTimeNs: 2, ATimeNs: 3, EntryType: kind}
}

func assertCandidateCount(t *testing.T, d *Daemon, want int) {
	t.Helper()
	candidates, err := d.CacheCandidates(context.Background())
	if err != nil || len(candidates.Artifacts) != want {
		t.Fatalf("candidate count = %d, want %d (err %v)", len(candidates.Artifacts), want, err)
	}
}

func setFixtureSessionState(t *testing.T, store *storage.Store, state string) {
	t.Helper()
	session, err := store.GetSession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	session.State = state
	if err := store.WithTx(context.Background(), func(tx *storage.Tx) error { return tx.UpsertSession(session) }); err != nil {
		t.Fatal(err)
	}
}
