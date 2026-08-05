package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
)

func TestCacheLifecycleRoundTripsOverUnixSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, _, _ := newCacheFixture(t, &now)
	d.CacheScanAt(ctx, now)
	now = now.Add(time.Minute)
	d.CacheScanAt(ctx, now)

	socketDir, err := os.MkdirTemp("", "gg-cache")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	server := &api.Server{Backend: d, SocketPath: filepath.Join(socketDir, "s.sock")}
	if err := server.Listen(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()

	client := api.NewClient(server.SocketPath)
	candidates, err := client.CacheCandidates(ctx)
	if err != nil || len(candidates.Artifacts) != 1 {
		t.Fatalf("cache candidates over socket = %#v, %v", candidates, err)
	}
	preview, err := client.CacheCleanupPreview(ctx, api.CachePreviewRequest{
		ArtifactID: candidates.Artifacts[0].ID, PolicyID: "codex-snapshots",
	})
	if err != nil || preview.Approval == "" {
		t.Fatalf("cache preview over socket = %#v, %v", preview, err)
	}
	result, err := client.CacheCleanupApply(ctx, api.CacheApplyRequest{Approval: preview.Approval})
	if err != nil || result.Result != "quarantined" {
		t.Fatalf("cache apply over socket = %#v, %v", result, err)
	}
	actions, err := client.CacheActions(ctx, api.CacheActionOptions{ArtifactID: result.ArtifactID, Limit: 10})
	if err != nil || len(actions.Actions) != 1 {
		t.Fatalf("cache actions over socket = %#v, %v", actions, err)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
