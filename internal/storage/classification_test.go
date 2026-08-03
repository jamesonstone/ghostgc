package storage

import (
	"context"
	"testing"
)

func TestClassificationHistoryAndLatest(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.WithTx(ctx, func(tx *Tx) error {
		for _, rec := range []ClassificationRecord{
			{ProcUID: "42:100", SessionID: "s1", TsNs: 1, ActivityTsNs: 1, State: "idle", BasisState: "idle", StableSinceNs: 1, EvidenceJSON: "[]"},
			{ProcUID: "42:100", SessionID: "s1", TsNs: 2, ActivityTsNs: 2, State: "active", BasisState: "active", StableSinceNs: 2, EvidenceJSON: "[]"},
		} {
			if err := tx.InsertClassification(rec); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	history, err := s.ListClassifications(ctx, ClassificationFilter{ProcUID: "42:100", Limit: 10})
	if err != nil || len(history) != 2 || history[0].State != "active" {
		t.Fatalf("history = %+v, %v", history, err)
	}
	latest, err := s.ListClassifications(ctx, ClassificationFilter{Latest: true})
	if err != nil || len(latest) != 1 || latest[0].TsNs != 2 {
		t.Fatalf("latest = %+v, %v", latest, err)
	}
	counts, err := s.Counts(ctx)
	if err != nil || counts.Classifications != 2 {
		t.Fatalf("counts = %+v, %v", counts, err)
	}
}
