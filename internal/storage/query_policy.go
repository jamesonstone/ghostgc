package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const qualifiedPolicyDecisionColumns = `d.id, d.evaluation_id, d.policy_id, d.proc_uid, d.session_id, d.ts_ns,
	d.classification_ts_ns, d.classification_state, d.result, d.reason, d.cooldown_until_ns, d.evidence`

// LastCandidateCooldown returns the durable cooldown for an exact policy/process pair.
func (s *Store) LastCandidateCooldown(ctx context.Context, policyID, procUID string) (int64, error) {
	var until int64
	err := s.db.QueryRowContext(ctx, `SELECT cooldown_until_ns FROM policy_decisions
		WHERE policy_id = ? AND proc_uid = ? AND result = 'candidate'
		ORDER BY id DESC LIMIT 1`, policyID, procUID).Scan(&until)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("storage: reading candidate cooldown: %w", err)
	}
	return until, nil
}

// CurrentPolicyDecisions returns decisions from the unique latest committed
// evaluation for exact process rows that are still live.
func (s *Store) CurrentPolicyDecisions(ctx context.Context) ([]PolicyDecisionRecord, error) {
	var evaluationID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM policy_evaluations ORDER BY id DESC LIMIT 1`).Scan(&evaluationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage: reading latest policy evaluation: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+qualifiedPolicyDecisionColumns+`
		FROM policy_decisions d JOIN processes p ON p.proc_uid = d.proc_uid
		WHERE d.evaluation_id = ? AND p.exited_at_ns IS NULL AND d.id IN (
			SELECT MAX(id) FROM policy_decisions WHERE evaluation_id = ? GROUP BY policy_id, proc_uid
		) ORDER BY d.id`, evaluationID, evaluationID)
	if err != nil {
		return nil, fmt.Errorf("storage: listing current policy decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PolicyDecisionRecord
	for rows.Next() {
		var rec PolicyDecisionRecord
		if err := rows.Scan(&rec.ID, &rec.EvaluationID, &rec.PolicyID, &rec.ProcUID, &rec.SessionID, &rec.TsNs,
			&rec.ClassificationTsNs, &rec.ClassificationState, &rec.Result, &rec.Reason,
			&rec.CooldownUntilNs, &rec.EvidenceJSON); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
