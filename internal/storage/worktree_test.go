package storage

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testWorktree(id, state string, at int64) WorktreeRecord {
	return WorktreeRecord{
		WorktreeID: id, Path: "/tmp/" + id, SourcesJSON: `["configured_root"]`,
		State: state, FirstSeenNs: at, LastSeenNs: at, LastActivityNs: at,
		DaemonStartedNs: at, ProtectionJSON: `[]`, EvidenceJSON: `{}`,
		ApprovedLinksJSON: `[]`, GitIdentityJSON: `{}`,
	}
}

func TestWorktreePrefixResolutionAndActionRetention(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	removed := testWorktree("abcdef11", "removed", ns(0))
	removedAt := ns(0)
	removed.RemovedNs = &removedAt
	live := testWorktree("abcdef22", "observing", ns(0))
	if err := s.WithTx(ctx, func(tx *Tx) error {
		if err := tx.UpsertWorktree(removed); err != nil {
			return err
		}
		if err := tx.UpsertWorktree(live); err != nil {
			return err
		}
		for _, action := range []WorktreeActionRecord{
			{ActionID: "wta_attempting", WorktreeID: live.WorktreeID, Path: live.Path,
				RequestedNs: ns(0), UpdatedNs: ns(0), Result: "attempting", Reason: "before side effect"},
			{ActionID: "wta_rejected", WorktreeID: live.WorktreeID, Path: live.Path,
				RequestedNs: ns(0), UpdatedNs: ns(0), Result: "rejected", Reason: "refused"},
		} {
			if err := tx.InsertWorktreeAction(action); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetWorktree(ctx, "abcdef"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("ambiguous prefix = %v", err)
	}
	if got, err := s.GetWorktree(ctx, "abcdef2"); err != nil || got.WorktreeID != live.WorktreeID {
		t.Fatalf("resolved = %+v, %v", got, err)
	}
	result, err := s.Compact(ctx, RetentionPolicy{
		RawObservations: time.Hour, Scans: time.Hour, Audit: time.Hour,
		PolicyDecisions: time.Hour, Actions: time.Minute,
		ExitedProcesses: time.Hour, EndedSessions: time.Hour,
	}, time.Unix(0, ns(2*time.Minute)))
	if err != nil || result.WorktreeActions != 1 || result.WorktreeTombstones != 1 {
		t.Fatalf("retention = %+v, %v", result, err)
	}
	actions, err := s.ListWorktreeActions(ctx, WorktreeActionFilter{})
	if err != nil || len(actions) != 1 || actions[0].Result != "attempting" {
		t.Fatalf("retained actions = %+v, %v", actions, err)
	}
	if got, err := s.GetWorktree(ctx, live.WorktreeID); err != nil || got.State != "observing" {
		t.Fatalf("live inventory = %+v, %v", got, err)
	}
}
