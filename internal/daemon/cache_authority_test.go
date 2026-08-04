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
