package storage

import "fmt"

// InsertPolicyEvaluation starts one unique committed current projection.
func (t *Tx) InsertPolicyEvaluation(atNs int64) (int64, error) {
	res, err := t.tx.ExecContext(t.ctx, `INSERT INTO policy_evaluations (ts_ns) VALUES (?)`, atNs)
	if err != nil {
		return 0, fmt.Errorf("storage: inserting policy evaluation: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("storage: reading policy evaluation id: %w", err)
	}
	return id, nil
}

// InsertPolicyDecision appends one audit-only policy decision.
func (t *Tx) InsertPolicyDecision(rec PolicyDecisionRecord) error {
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO policy_decisions (
		evaluation_id, policy_id, proc_uid, session_id, ts_ns, classification_ts_ns,
		classification_state, result, reason, cooldown_until_ns, evidence
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.EvaluationID, rec.PolicyID, rec.ProcUID, rec.SessionID, rec.TsNs, rec.ClassificationTsNs,
		rec.ClassificationState, rec.Result, rec.Reason, rec.CooldownUntilNs, rec.EvidenceJSON)
	if err != nil {
		return fmt.Errorf("storage: inserting policy decision for %s/%s: %w", rec.PolicyID, rec.ProcUID, err)
	}
	return nil
}
