package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
)

func TestFailedCacheScanRevokesCurrentAuthority(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, filesystem, _ := newCacheFixture(t, &now)
	d.CacheScanAt(ctx, now)
	now = now.Add(time.Minute)
	d.CacheScanAt(ctx, now)
	candidates, _ := d.CacheCandidates(ctx)
	if len(candidates.Artifacts) != 1 {
		t.Fatalf("candidate before failure = %#v", candidates)
	}

	filesystem.Errors["snapshot"] = errors.New("fixture metadata unavailable")
	now = now.Add(time.Minute)
	d.CacheScanAt(ctx, now)
	candidates, _ = d.CacheCandidates(ctx)
	if len(candidates.Artifacts) != 0 {
		t.Fatalf("failed scan retained current authority: %#v", candidates)
	}
	if _, err := d.CacheCleanupPreview(ctx, api.CachePreviewRequest{
		ArtifactID: "ca_any", PolicyID: "codex-snapshots",
	}); err == nil {
		t.Fatal("failed cache scan allowed a new cleanup preview")
	}
}
