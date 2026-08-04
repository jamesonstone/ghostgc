package daemon

import (
	"context"
	"testing"
	"time"
)

func TestCacheStabilityRequiresConsecutiveCommittedPresence(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, filesystem, _ := newCacheFixture(t, &now)
	root := "/tmp/codex/shell_snapshots"
	name := fixtureThreadID + ".100.sh"

	d.CacheScanAt(ctx, now)
	now = now.Add(time.Minute)
	d.CacheScanAt(ctx, now)
	assertCandidateCount(t, d, 1)

	filesystem.Delete(root, name)
	now = now.Add(time.Minute)
	d.CacheScanAt(ctx, now)
	filesystem.Put(root, name, fixtureIdentity(2))
	now = now.Add(24 * time.Hour)
	d.CacheScanAt(ctx, now)
	assertCandidateCount(t, d, 0)

	now = now.Add(time.Minute)
	d.CacheScanAt(ctx, now)
	assertCandidateCount(t, d, 1)
}
