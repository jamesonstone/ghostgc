package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

func TestCacheApplyRefusesChangedSessionFacts(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*testing.T, *storage.Store, time.Time){
		"session reactivated": func(t *testing.T, store *storage.Store, now time.Time) {
			t.Helper()
			session, err := store.GetSession(ctx, "session-1")
			if err != nil {
				t.Fatal(err)
			}
			session.State = "active"
			if err := store.WithTx(ctx, func(tx *storage.Tx) error { return tx.UpsertSession(session) }); err != nil {
				t.Fatal(err)
			}
		},
		"live claimant": func(t *testing.T, store *storage.Store, now time.Time) {
			t.Helper()
			if err := store.WithTx(ctx, func(tx *storage.Tx) error {
				return tx.UpsertProcess(cacheClaimant("44:1", 44, 1, now, "session-1"))
			}); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
			d, filesystem, store := newCacheFixture(t, &now)
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
			mutate(t, store, now)
			result, err := d.CacheCleanupApply(ctx, api.CacheApplyRequest{Approval: preview.Approval})
			if err != nil || result.Result != "rejected" {
				t.Fatalf("changed session apply = %#v, %v", result, err)
			}
			if !filesystem.Exists("/tmp/codex/shell_snapshots", fixtureThreadID+".100.sh") {
				t.Fatal("session change moved the artifact before refusal")
			}
		})
	}
}

func TestPIDReuseDoesNotInheritOldCacheClaim(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, _, store := newCacheFixture(t, &now)
	old := cacheClaimant("44:1", 44, 1, now, "session-1")
	if err := store.WithTx(ctx, func(tx *storage.Tx) error { return tx.UpsertProcess(old) }); err != nil {
		t.Fatal(err)
	}
	if err := store.WithTx(ctx, func(tx *storage.Tx) error {
		if _, err := tx.MarkExitedBefore(now.Add(time.Second).UnixNano(), now.Add(time.Second).UnixNano()); err != nil {
			return err
		}
		return tx.UpsertProcess(cacheClaimant("44:2", 44, 2, now.Add(2*time.Second), ""))
	}); err != nil {
		t.Fatal(err)
	}
	facts, err := d.cacheSessionFacts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0].LiveProcesses != 0 {
		t.Fatalf("recycled PID inherited an exited session claim: %#v", facts)
	}
}

func TestRestoreRequiresTheExactCachePolicyAuthority(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, _, _ := newCacheFixture(t, &now)
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
	if result, err := d.CacheCleanupApply(ctx, api.CacheApplyRequest{Approval: preview.Approval}); err != nil || result.Result != "quarantined" {
		t.Fatalf("quarantine = %#v, %v", result, err)
	}
	d.cfg.Cache.Policies[0].Enabled = false
	if _, err := d.CacheRestorePreview(ctx, api.CachePreviewRequest{ArtifactID: candidates.Artifacts[0].ID}); err == nil {
		t.Fatal("restore preview inherited global authority after its exact policy was disabled")
	}
}

func cacheClaimant(uid string, pid int, start int64, now time.Time, sessionID string) storage.ProcessRecord {
	return storage.ProcessRecord{
		ProcUID: uid, PID: pid, StartTimeNs: start, PPID: 1, OriginalPPID: 1, PGID: pid, SID: pid, UID: 501,
		Comm: "helper", ExecPath: "/tmp/helper", Cmdline: `["helper"]`, CWD: "/tmp",
		AgentID: "codex", SessionID: sessionID, Relation: "environment", Confidence: 0.9,
		EvidenceJSON: `[]`, FirstSeenNs: now.UnixNano(), LastSeenNs: now.UnixNano(),
	}
}
