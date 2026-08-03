package storage

import (
	"context"
	"testing"
	"time"
)

func TestRelationshipUpsertKeepsFirstObservation(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	first := RelationshipRecord{
		SessionID: "sess-1", Kind: "reparenting", FromProcUID: "200:2",
		Detail: "parent link lost", FirstSeenNs: ns(0), LastSeenNs: ns(0),
	}
	later := first
	later.Detail = "parent link still lost"
	later.FirstSeenNs = ns(time.Hour)
	later.LastSeenNs = ns(time.Hour)

	if err := s.WithTx(ctx, func(tx *Tx) error {
		if err := tx.UpsertRelationship(first); err != nil {
			return err
		}
		return tx.UpsertRelationship(later)
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := s.SessionRelationships(ctx, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d edges, want 1", len(rows))
	}
	if rows[0].FirstSeenNs != ns(0) {
		t.Fatal("an edge must keep when it was first observed; for a reparenting event that timestamp is the whole point")
	}
	if rows[0].LastSeenNs != ns(time.Hour) {
		t.Fatal("last_seen_ns should advance")
	}

	byProc, err := s.ProcessRelationships(ctx, "200:2")
	if err != nil {
		t.Fatal(err)
	}
	if len(byProc) != 1 {
		t.Fatalf("lookup by process returned %d edges, want 1", len(byProc))
	}
}

func TestRetentionBoundsGrowth(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now()

	old := now.Add(-48 * time.Hour).UnixNano()
	recent := now.Add(-time.Minute).UnixNano()

	err := s.WithTx(ctx, func(tx *Tx) error {
		for i := 0; i < 500; i++ {
			if err := tx.InsertObservation(ObservationRecord{ProcUID: "1:1", TsNs: old}); err != nil {
				return err
			}
		}
		for i := 0; i < 10; i++ {
			if err := tx.InsertObservation(ObservationRecord{ProcUID: "1:1", TsNs: recent}); err != nil {
				return err
			}
		}
		p := proc("1:1", 1, 1, recent)
		if err := tx.UpsertProcess(p); err != nil {
			return err
		}
		return tx.AppendAudit(AuditRecord{TsNs: old, Kind: "test", Subject: "x", Summary: "old"})
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := s.Compact(ctx, RetentionPolicy{
		RawObservations:  24 * time.Hour,
		Scans:            24 * time.Hour,
		Audit:            24 * time.Hour,
		ExitedProcesses:  24 * time.Hour,
		EndedSessions:    24 * time.Hour,
		MaxDatabaseBytes: 250 << 20,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Observations != 500 {
		t.Fatalf("deleted %d observations, want the 500 outside the window", res.Observations)
	}
	if res.Audit != 1 {
		t.Fatalf("deleted %d audit entries, want 1", res.Audit)
	}

	counts, err := s.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Observations != 10 {
		t.Fatalf("%d observations survived, want the 10 inside the window", counts.Observations)
	}
}

func TestRetentionCompactsHarderWhenOverBudget(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now()

	// Everything is inside the nominal window, but the budget is one byte, so
	// the aggressive pass must halve the windows and take the older half.
	err := s.WithTx(ctx, func(tx *Tx) error {
		for i := 0; i < 50; i++ {
			if err := tx.InsertObservation(ObservationRecord{
				ProcUID: "1:1", TsNs: now.Add(-90 * time.Minute).UnixNano(),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := s.Compact(ctx, RetentionPolicy{
		RawObservations:  2 * time.Hour,
		Scans:            2 * time.Hour,
		Audit:            2 * time.Hour,
		ExitedProcesses:  2 * time.Hour,
		EndedSessions:    2 * time.Hour,
		MaxDatabaseBytes: 1,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Aggressive {
		t.Fatal("exceeding the size budget must trigger an aggressive pass")
	}
	if res.Observations == 0 {
		t.Fatal("the aggressive pass should have removed observations older than the halved window")
	}
}

func TestAuditLogIsQueryable(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	for i, kind := range []string{"session.started", "process.attributed", "session.started"} {
		if err := s.AppendAudit(ctx, AuditRecord{
			TsNs: ns(time.Duration(i) * time.Minute), Kind: kind, Subject: "s1", Summary: "entry",
			EvidenceJSON: `[{"kind":"executable","detail":"x"}]`,
		}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListAudit(ctx, AuditFilter{Kind: "session.started"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].TsNs < got[1].TsNs {
		t.Fatal("audit entries should come back newest first")
	}
	if got[0].EvidenceJSON == "" {
		t.Fatal("evidence must be preserved; a decision without evidence is not auditable")
	}
}
