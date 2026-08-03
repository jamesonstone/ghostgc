package storage

import (
	"context"
	"fmt"
	"strings"
)

// ActionFilter narrows durable action history.
type ActionFilter struct {
	ProcUID  string
	PolicyID string
	Result   string
	Limit    int
}

// InsertAction writes the pre-side-effect row.
func (t *Tx) InsertAction(rec ActionRecord) error {
	if rec.Authority == "" {
		rec.Authority = "manual"
	}
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO actions (
		action_id, policy_id, proc_uid, session_id, authority, requested_ns, updated_ns,
		result, signal, reason, evidence
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, rec.ActionID, rec.PolicyID,
		rec.ProcUID, rec.SessionID, rec.Authority, rec.RequestedNs, rec.UpdatedNs, rec.Result,
		rec.Signal, rec.Reason, jsonOrEmpty(rec.EvidenceJSON))
	if err != nil {
		return fmt.Errorf("storage: inserting action %s: %w", rec.ActionID, err)
	}
	return nil
}

// UpdateAction records the post-side-effect result.
func (t *Tx) UpdateAction(actionID, result, reason, evidence string, atNs int64) error {
	res, err := t.tx.ExecContext(t.ctx, `UPDATE actions SET result = ?, reason = ?,
		evidence = ?, updated_ns = ? WHERE action_id = ?`, result, reason,
		jsonOrEmpty(evidence), atNs, actionID)
	if err != nil {
		return fmt.Errorf("storage: updating action %s: %w", actionID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("storage: updating action %s: not found", actionID)
	}
	return nil
}

// ListActions returns action history newest first.
func (s *Store) ListActions(ctx context.Context, f ActionFilter) ([]ActionRecord, error) {
	q := `SELECT id, action_id, policy_id, proc_uid, session_id, requested_ns,
		updated_ns, result, signal, reason, evidence, authority FROM actions`
	var where []string
	var args []any
	for _, filter := range []struct{ value, column string }{
		{f.ProcUID, "proc_uid"}, {f.PolicyID, "policy_id"}, {f.Result, "result"},
	} {
		if filter.value != "" {
			where, args = append(where, filter.column+" = ?"), append(args, filter.value)
		}
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY requested_ns DESC, id DESC LIMIT ?"
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: listing actions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ActionRecord
	for rows.Next() {
		var rec ActionRecord
		if err := rows.Scan(&rec.ID, &rec.ActionID, &rec.PolicyID, &rec.ProcUID,
			&rec.SessionID, &rec.RequestedNs, &rec.UpdatedNs, &rec.Result,
			&rec.Signal, &rec.Reason, &rec.EvidenceJSON, &rec.Authority); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ActionCounts returns signal-attempt, preflight-rejection and successful
// system-call counts without treating a rejected approval as a signal attempt.
func (s *Store) ActionCounts(ctx context.Context) (attempted, rejected, completed int64, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN result <> 'rejected' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN result = 'rejected' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN result = 'signalled' THEN 1 ELSE 0 END), 0)
		FROM actions`).Scan(&attempted, &rejected, &completed)
	if err != nil {
		err = fmt.Errorf("storage: counting actions: %w", err)
	}
	return
}
