package storage

import "fmt"

// InsertClassification appends one deterministic classification.
func (t *Tx) InsertClassification(rec ClassificationRecord) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT INTO process_classifications (
			proc_uid, session_id, ts_ns, activity_ts_ns, state, basis_state,
			detached, session_ended, stable_since_ns, evidence
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ProcUID, rec.SessionID, rec.TsNs, rec.ActivityTsNs,
		rec.State, rec.BasisState, rec.Detached, rec.SessionEnded,
		rec.StableSinceNs, rec.EvidenceJSON)
	if err != nil {
		return fmt.Errorf("storage: inserting classification for %s: %w", rec.ProcUID, err)
	}
	return nil
}
