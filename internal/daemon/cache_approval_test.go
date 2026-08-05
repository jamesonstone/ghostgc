package daemon

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
)

func TestCacheApprovalIsSingleUseUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, _, _ := newCacheFixture(t, &now)
	d.CacheScanAt(ctx, now)
	now = now.Add(time.Minute)
	d.CacheScanAt(ctx, now)
	candidates, err := d.CacheCandidates(ctx)
	if err != nil || len(candidates.Artifacts) != 1 {
		t.Fatalf("candidate = %#v, %v", candidates, err)
	}
	preview, err := d.CacheCleanupPreview(ctx, api.CachePreviewRequest{
		ArtifactID: candidates.Artifacts[0].ID, PolicyID: "codex-snapshots",
	})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan api.CacheApplyResponse, 2)
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := d.CacheCleanupApply(ctx, api.CacheApplyRequest{Approval: preview.Approval})
			results <- result
			errors <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errors)

	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent apply returned transport error: %v", err)
		}
	}
	counts := map[string]int{}
	for result := range results {
		counts[result.Result]++
	}
	if counts["quarantined"] != 1 || counts["rejected"] != 1 {
		t.Fatalf("single-use outcomes = %#v", counts)
	}
}

func TestCacheScanAndApplySerializeOnExactState(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, filesystem, _ := newCacheFixture(t, &now)
	d.CacheScanAt(ctx, now)
	now = now.Add(time.Minute)
	d.CacheScanAt(ctx, now)
	candidates, _ := d.CacheCandidates(ctx)
	preview, err := d.CacheCleanupPreview(ctx, api.CachePreviewRequest{
		ArtifactID: candidates.Artifacts[0].ID, PolicyID: "codex-snapshots",
	})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	resultCh := make(chan api.CacheApplyResponse, 1)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		d.CacheScanAt(ctx, now)
	}()
	go func() {
		defer wg.Done()
		<-start
		result, err := d.CacheCleanupApply(ctx, api.CacheApplyRequest{Approval: preview.Approval})
		resultCh <- result
		errCh <- err
	}()
	close(start)
	wg.Wait()
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	result := <-resultCh
	originExists := filesystem.Exists("/tmp/codex/shell_snapshots", fixtureThreadID+".100.sh")
	if result.Result == "quarantined" && originExists {
		t.Fatal("successful serialized apply left the approved origin in place")
	}
	if result.Result == "rejected" && !originExists {
		t.Fatal("rejected serialized apply mutated the approved origin")
	}
	if result.Result != "quarantined" && result.Result != "rejected" {
		t.Fatalf("serialized apply = %#v", result)
	}
}
