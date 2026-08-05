package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
)

func TestDisabledCachePerformsNoObservationOrMutation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, filesystem, _ := newCacheFixture(t, &now)
	d.cfg.Cache.Enabled = false
	d.CacheScanAt(ctx, now)
	artifacts, err := d.CacheArtifacts(ctx, api.CacheArtifactOptions{Current: true})
	if err != nil || len(artifacts.Artifacts) != 0 {
		t.Fatalf("disabled cache observation = %#v, %v", artifacts, err)
	}
	if !filesystem.Exists("/tmp/codex/shell_snapshots", fixtureThreadID+".100.sh") {
		t.Fatal("disabled cache changed the fixture")
	}
}

func TestAuditCacheCannotIssueMutationAuthority(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, filesystem, _ := newCacheFixture(t, &now)
	d.cfg.Cache.GlobalMode = config.ModeAudit
	d.CacheScanAt(ctx, now)
	now = now.Add(time.Minute)
	d.CacheScanAt(ctx, now)
	candidates, err := d.CacheCandidates(ctx)
	if err != nil || len(candidates.Artifacts) != 1 {
		t.Fatalf("audit candidate evidence = %#v, %v", candidates, err)
	}
	if _, err := d.CacheCleanupPreview(ctx, api.CachePreviewRequest{
		ArtifactID: candidates.Artifacts[0].ID, PolicyID: "codex-snapshots",
	}); err == nil {
		t.Fatal("audit cache issued mutation authority")
	}
	if !filesystem.Exists("/tmp/codex/shell_snapshots", fixtureThreadID+".100.sh") {
		t.Fatal("audit cache changed the fixture")
	}
}

func TestCachePurgeRequiresExactArtifactConfirmation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, filesystem, _ := newCacheFixture(t, &now)
	d.CacheScanAt(ctx, now)
	now = now.Add(time.Minute)
	d.CacheScanAt(ctx, now)
	candidates, _ := d.CacheCandidates(ctx)
	artifact := candidates.Artifacts[0]
	cleanup, err := d.CacheCleanupPreview(ctx, api.CachePreviewRequest{ArtifactID: artifact.ID, PolicyID: "codex-snapshots"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.CacheCleanupApply(ctx, api.CacheApplyRequest{Approval: cleanup.Approval}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	purge, err := d.CachePurgePreview(ctx, api.CachePreviewRequest{ArtifactID: artifact.ID, PolicyID: "codex-snapshots"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := d.CachePurgeApply(ctx, api.CacheApplyRequest{Approval: purge.Approval, Confirmation: "ca_wrong"})
	if err != nil || result.Action.Result != "rejected" {
		t.Fatalf("wrong confirmation = %+v, %v", result, err)
	}
	if !filesystem.Exists(artifact.RootPath, cleanup.Destination) {
		t.Fatal("wrong confirmation changed the quarantine artifact")
	}
}

func TestExpiredCachePurgeCannotCompleteSuccessfully(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, filesystem, _ := newCacheFixture(t, &now)
	d.CacheScanAt(ctx, now)
	now = now.Add(time.Minute)
	d.CacheScanAt(ctx, now)
	candidates, _ := d.CacheCandidates(ctx)
	artifact := candidates.Artifacts[0]
	cleanup, err := d.CacheCleanupPreview(ctx, api.CachePreviewRequest{ArtifactID: artifact.ID, PolicyID: "codex-snapshots"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.CacheCleanupApply(ctx, api.CacheApplyRequest{Approval: cleanup.Approval}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	preview, err := d.CachePurgePreview(ctx, api.CachePreviewRequest{ArtifactID: artifact.ID, PolicyID: "codex-snapshots"})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := d.CachePurgeApply(ctx, api.CacheApplyRequest{Approval: preview.Approval, Confirmation: artifact.ID})
	if err != nil {
		t.Fatal(err)
	}
	filesystem.Delete(prepared.Plan.RootPath, prepared.Plan.QuarantinePath)
	now = now.Add(cachePurgePlanLifetime)
	result, err := d.CachePurgeComplete(ctx, api.CachePurgeCompleteRequest{
		ActionID: prepared.Plan.ActionID, Completion: prepared.Plan.Completion,
	})
	if err != nil || result.Result != "partial" || d.filesystemMutationsHealthy() {
		t.Fatalf("expired completion = %+v, %v, healthy=%v", result, err, d.filesystemMutationsHealthy())
	}
}
