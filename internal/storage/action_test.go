package storage

import (
	"context"
	"testing"
	"time"
)

func TestActionIsDurableBeforeCompletionAndCountedByOutcome(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	record := ActionRecord{
		ActionID: "act_1", PolicyID: "safe-helper", ProcUID: "42:1", SessionID: "s1",
		RequestedNs: ns(0), UpdatedNs: ns(0), Result: "attempting", Signal: "SIGTERM",
		Reason: "preflight passed", EvidenceJSON: `[{"rule":"preflight","detail":"passed"}]`,
	}
	if err := s.WithTx(ctx, func(tx *Tx) error { return tx.InsertAction(record) }); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListActions(ctx, ActionFilter{PolicyID: "safe-helper"})
	if err != nil || len(rows) != 1 || rows[0].Result != "attempting" {
		t.Fatalf("pre-side-effect action = %+v, %v", rows, err)
	}
	if err := s.WithTx(ctx, func(tx *Tx) error {
		return tx.UpdateAction("act_1", "signalled", "accepted", `[{"rule":"signal","detail":"accepted"}]`, ns(time.Second))
	}); err != nil {
		t.Fatal(err)
	}
	attempted, rejected, completed, err := s.ActionCounts(ctx)
	if err != nil || attempted != 1 || rejected != 0 || completed != 1 {
		t.Fatalf("counts = %d/%d/%d, %v", attempted, rejected, completed, err)
	}
}

func TestActionRetentionUsesDedicatedWindow(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.WithTx(ctx, func(tx *Tx) error {
		return tx.InsertAction(ActionRecord{
			ActionID: "act_old", PolicyID: "p", ProcUID: "42:1", SessionID: "s",
			RequestedNs: ns(0), UpdatedNs: ns(0), Result: "rejected", Signal: "SIGTERM", Reason: "test",
		})
	}); err != nil {
		t.Fatal(err)
	}
	res, err := s.Compact(ctx, RetentionPolicy{
		RawObservations: time.Hour, Scans: time.Hour, Audit: time.Hour,
		PolicyDecisions: time.Hour, Actions: time.Minute,
		ExitedProcesses: time.Hour, EndedSessions: time.Hour,
	}, time.Unix(0, ns(2*time.Minute)))
	if err != nil || res.Actions != 1 {
		t.Fatalf("retention = %+v, %v", res, err)
	}
	rows, err := s.ListActions(ctx, ActionFilter{})
	if err != nil || len(rows) != 0 {
		t.Fatalf("retained actions = %+v, %v", rows, err)
	}
}
