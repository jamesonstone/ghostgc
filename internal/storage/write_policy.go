package storage

import "fmt"

// InsertPolicyDecision appends one audit-only policy decision.
func (t *Tx) InsertPolicyDecision(rec PolicyDecisionRecord) error {
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO policy_decisions (
		policy_id, proc_uid, session_id, ts_ns, classification_ts_ns,
		classification_state, result, reason, cooldown_until_ns, evidence
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.PolicyID, rec.ProcUID, rec.SessionID, rec.TsNs, rec.ClassificationTsNs,
		rec.ClassificationState, rec.Result, rec.Reason, rec.CooldownUntilNs, rec.EvidenceJSON)
	if err != nil {
		return fmt.Errorf("storage: inserting policy decision for %s/%s: %w", rec.PolicyID, rec.ProcUID, err)
	}
	return nil
}
