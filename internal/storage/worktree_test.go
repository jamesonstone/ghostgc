package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func testWorktree(id, state string, at int64) WorktreeRecord {
	return WorktreeRecord{
		WorktreeID: id, Path: "/tmp/" + id, SourcesJSON: `["configured_root"]`,
		State: state, FirstSeenNs: at, LastSeenNs: at, LastActivityNs: at,
		DaemonStartedNs: at, ProtectionJSON: `[]`, EvidenceJSON: `{}`,
		ApprovedLinksJSON: `[]`, GitIdentityJSON: `{}`, Registered: true,
	}
}

func TestAbsentWorktreeInventoryIsHardBoundedAndRetainedByAge(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	attemptingID := fmt.Sprintf("%064x", 510)
	if err := s.WithTx(ctx, func(tx *Tx) error {
		for i := 0; i < 510; i++ {
			record := testWorktree(fmt.Sprintf("%064x", i+1), "missing", ns(0))
			record.Registered = false
			if err := tx.UpsertWorktree(record); err != nil {
				return err
			}
		}
		for i := 0; i < 2; i++ {
			record := testWorktree(fmt.Sprintf("%064x", 1000+i), "missing", ns(0))
			if err := tx.UpsertWorktree(record); err != nil {
				return err
			}
		}
		if err := tx.InsertWorktreeAction(WorktreeActionRecord{
			ActionID: "wta_unresolved", WorktreeID: attemptingID, Path: "/tmp/unresolved",
			RequestedNs: ns(0), UpdatedNs: ns(0), Result: "attempting", Reason: "side effect unresolved",
		}); err != nil {
			return err
		}
		removed, err := tx.PruneAbsentWorktrees(500)
		if err != nil {
			return err
		}
		if removed != 12 {
			return fmt.Errorf("hard-bound removal = %d, want 12", removed)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	current, err := s.ListCurrentWorktrees(ctx, 500)
	if err != nil || len(current) != 500 {
		t.Fatalf("bounded inventory = %d, %v", len(current), err)
	}
	registered, attemptingPresent := 0, false
	for _, record := range current {
		if record.Registered {
			registered++
		}
		attemptingPresent = attemptingPresent || record.WorktreeID == attemptingID
	}
	if registered != 2 || !attemptingPresent {
		t.Fatalf("protected inventory: registered=%d attempting=%t", registered, attemptingPresent)
	}
	result, err := s.Compact(ctx, RetentionPolicy{Actions: time.Minute}, time.Unix(0, ns(2*time.Minute)))
	if err != nil || result.WorktreeMissing != 497 {
		t.Fatalf("missing retention = %+v, %v", result, err)
	}
	current, err = s.ListCurrentWorktrees(ctx, 500)
	if err != nil || len(current) != 3 {
		t.Fatalf("registered inventory after retention = %+v, %v", current, err)
	}
	if _, err := s.GetWorktree(ctx, attemptingID); err != nil {
		t.Fatalf("unresolved action subject was pruned: %v", err)
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
	if got, err := s.GetWorktree(ctx, live.WorktreeID); err != nil || got.WorktreeID != live.WorktreeID {
		t.Fatalf("exact id = %+v, %v", got, err)
	}
	for _, invalid := range []string{"", "%", "_", "abc/def", "ABCDEF", strings.Repeat("a", 65)} {
		if _, err := s.GetWorktree(ctx, invalid); err == nil {
			t.Errorf("invalid selector %q was accepted", invalid)
		}
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
