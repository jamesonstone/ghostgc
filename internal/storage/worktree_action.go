package storage

import (
	"context"
	"fmt"
	"strings"
)

// InsertWorktreeAction commits intent before the filesystem side effect.
func (t *Tx) InsertWorktreeAction(rec WorktreeActionRecord) error {
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO worktree_actions (
		action_id, worktree_id, path, branch, requested_ns, updated_ns,
		result, reason, evidence, recreate_command
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, rec.ActionID, rec.WorktreeID,
		rec.Path, rec.Branch, rec.RequestedNs, rec.UpdatedNs, rec.Result, rec.Reason,
		jsonOrEmpty(rec.EvidenceJSON), rec.RecreateCommand)
	if err != nil {
		return fmt.Errorf("storage: inserting worktree action %s: %w", rec.ActionID, err)
	}
	return nil
}

// UpdateWorktreeAction records the post-side-effect result.
func (t *Tx) UpdateWorktreeAction(id, result, reason, evidence string, atNs int64) error {
	res, err := t.tx.ExecContext(t.ctx, `UPDATE worktree_actions SET result = ?,
		reason = ?, evidence = ?, updated_ns = ? WHERE action_id = ?`, result,
		reason, jsonOrEmpty(evidence), atNs, id)
	if err != nil {
		return fmt.Errorf("storage: updating worktree action %s: %w", id, err)
	}
	if count, _ := res.RowsAffected(); count != 1 {
		return fmt.Errorf("storage: updating worktree action %s: not found", id)
	}
	return nil
}

// ListWorktreeActions returns removal history newest first.
func (s *Store) ListWorktreeActions(ctx context.Context, f WorktreeActionFilter) ([]WorktreeActionRecord, error) {
	q := `SELECT id, action_id, worktree_id, path, branch, requested_ns,
		updated_ns, result, reason, evidence, recreate_command FROM worktree_actions`
	var where []string
	var args []any
	if f.WorktreeID != "" {
		where, args = append(where, "worktree_id = ?"), append(args, f.WorktreeID)
	}
	if f.Result != "" {
		where, args = append(where, "result = ?"), append(args, f.Result)
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
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []WorktreeActionRecord
	for rows.Next() {
		var rec WorktreeActionRecord
		if err := rows.Scan(&rec.ID, &rec.ActionID, &rec.WorktreeID, &rec.Path,
			&rec.Branch, &rec.RequestedNs, &rec.UpdatedNs, &rec.Result, &rec.Reason,
			&rec.EvidenceJSON, &rec.RecreateCommand); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// WorktreeActionCounts returns attempted, rejected, removed and failed totals.
func (s *Store) WorktreeActionCounts(ctx context.Context) (attempted, rejected, removed, failed int64, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN result <> 'rejected' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN result = 'rejected' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN result = 'removed' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN result = 'failed' THEN 1 ELSE 0 END), 0)
		FROM worktree_actions`).Scan(&attempted, &rejected, &removed, &failed)
	return
}
