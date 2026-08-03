package storage

import (
	"context"
	"testing"
)

func TestPolicyDecisionCooldownAndCurrentLiveProjection(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	live := proc("42:1", 42, 1, 100)
	exited := proc("43:1", 43, 1, 100)
	if err := s.WithTx(ctx, func(tx *Tx) error {
		evaluationID, err := tx.InsertPolicyEvaluation(200)
		if err != nil {
			return err
		}
		if err := tx.UpsertProcess(live); err != nil {
			return err
		}
		if err := tx.UpsertProcess(exited); err != nil {
			return err
		}
		for _, rec := range []PolicyDecisionRecord{
			{PolicyID: "p1", ProcUID: live.ProcUID, SessionID: "s1", TsNs: 200,
				ClassificationTsNs: 190, ClassificationState: "orphaned", Result: "candidate",
				Reason: "matched", CooldownUntilNs: 500, EvidenceJSON: "[]"},
			{PolicyID: "p1", ProcUID: exited.ProcUID, SessionID: "s1", TsNs: 200,
				ClassificationTsNs: 190, ClassificationState: "orphaned", Result: "candidate",
				Reason: "matched", CooldownUntilNs: 500, EvidenceJSON: "[]"},
			{PolicyID: "p1", ProcUID: live.ProcUID, SessionID: "s1", TsNs: 200,
				ClassificationTsNs: 195, ClassificationState: "orphaned", Result: "cooldown",
				Reason: "cooling down", CooldownUntilNs: 500, EvidenceJSON: "[]"},
		} {
			rec.EvaluationID = evaluationID
			if err := tx.InsertPolicyDecision(rec); err != nil {
				return err
			}
		}
		if _, err := tx.tx.ExecContext(ctx, `UPDATE processes SET exited_at_ns = 150 WHERE proc_uid = ?`, exited.ProcUID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	until, err := s.LastCandidateCooldown(ctx, "p1", live.ProcUID)
	if err != nil || until != 500 {
		t.Fatalf("cooldown = %d, %v", until, err)
	}
	current, err := s.CurrentPolicyDecisions(ctx)
	if err != nil || len(current) != 1 || current[0].ProcUID != live.ProcUID || current[0].Result != "cooldown" {
		t.Fatalf("current decisions = %+v, %v", current, err)
	}
	if err := s.WithTx(ctx, func(tx *Tx) error {
		_, err := tx.InsertPolicyEvaluation(200)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	current, err = s.CurrentPolicyDecisions(ctx)
	if err != nil || len(current) != 0 {
		t.Fatalf("same-timestamp empty evaluation left stale decisions: %+v, %v", current, err)
	}
	counts, err := s.Counts(ctx)
	if err != nil || counts.PolicyDecisions != 3 {
		t.Fatalf("counts = %+v, %v", counts, err)
	}
}
