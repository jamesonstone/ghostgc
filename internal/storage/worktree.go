package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const worktreeColumns = `worktree_id, path, path_device, path_inode,
	common_git_dir, admin_git_dir, head, ref, branch, sources, state,
	first_seen_ns, last_seen_ns, last_activity_ns, inactive_since_ns,
	daemon_started_ns, status_fingerprint, protection, evidence, approved_links,
	git_identity, complete, removed_ns, recreate_command`

// UpsertWorktree stores one current conclusion without changing first seen.
func (t *Tx) UpsertWorktree(rec WorktreeRecord) error {
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO worktrees (`+worktreeColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(worktree_id) DO UPDATE SET
		path=excluded.path, path_device=excluded.path_device, path_inode=excluded.path_inode,
		common_git_dir=excluded.common_git_dir, admin_git_dir=excluded.admin_git_dir,
		head=excluded.head, ref=excluded.ref, branch=excluded.branch, sources=excluded.sources,
		state=excluded.state, last_seen_ns=excluded.last_seen_ns,
		last_activity_ns=excluded.last_activity_ns, inactive_since_ns=excluded.inactive_since_ns,
		daemon_started_ns=excluded.daemon_started_ns, status_fingerprint=excluded.status_fingerprint,
		protection=excluded.protection, evidence=excluded.evidence,
		approved_links=excluded.approved_links, git_identity=excluded.git_identity,
		complete=excluded.complete, removed_ns=excluded.removed_ns,
		recreate_command=excluded.recreate_command`, rec.WorktreeID, rec.Path,
		rec.PathDevice, rec.PathInode, rec.CommonGitDir, rec.AdminGitDir, rec.HEAD,
		rec.Ref, rec.Branch, jsonOrEmpty(rec.SourcesJSON), rec.State, rec.FirstSeenNs,
		rec.LastSeenNs, rec.LastActivityNs, rec.InactiveSinceNs, rec.DaemonStartedNs,
		rec.StatusFingerprint, jsonOrEmpty(rec.ProtectionJSON), jsonOrEmpty(rec.EvidenceJSON),
		jsonOrEmpty(rec.ApprovedLinksJSON), jsonOrEmpty(rec.GitIdentityJSON), rec.Complete,
		rec.RemovedNs, rec.RecreateCommand)
	if err != nil {
		return fmt.Errorf("storage: upserting worktree %s: %w", rec.WorktreeID, err)
	}
	return nil
}

// ResetWorktreeInactivity fails every live inventory row closed after a daemon
// observation gap. Removed tombstones remain historical facts.
func (t *Tx) ResetWorktreeInactivity(atNs, daemonStartedNs int64) error {
	_, err := t.tx.ExecContext(t.ctx, `UPDATE worktrees SET state = 'unknown',
		last_seen_ns = ?, last_activity_ns = ?, inactive_since_ns = 0,
		daemon_started_ns = ?, complete = 0,
		protection = '["daemon_scan_incomplete"]' WHERE state <> 'removed'`,
		atNs, atNs, daemonStartedNs)
	if err != nil {
		return fmt.Errorf("storage: resetting worktree inactivity: %w", err)
	}
	return nil
}

// ListWorktrees returns newest inventory first.
func (s *Store) ListWorktrees(ctx context.Context, f WorktreeFilter) ([]WorktreeRecord, error) {
	q := `SELECT ` + worktreeColumns + ` FROM worktrees`
	var where []string
	var args []any
	if f.State != "" {
		where, args = append(where, "state = ?"), append(args, f.State)
	}
	if f.Source != "" {
		where, args = append(where, "sources LIKE ?"), append(args, `%"`+f.Source+`"%`)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY last_seen_ns DESC, worktree_id ASC LIMIT ?"
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: listing worktrees: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []WorktreeRecord
	for rows.Next() {
		rec, err := scanWorktree(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// GetWorktree accepts an exact id or an unambiguous prefix.
func (s *Store) GetWorktree(ctx context.Context, value string) (WorktreeRecord, error) {
	if value == "" {
		return WorktreeRecord{}, ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+worktreeColumns+`
		FROM worktrees WHERE worktree_id = ? OR worktree_id LIKE ?
		ORDER BY last_seen_ns DESC LIMIT 2`, value, value+"%")
	if err != nil {
		return WorktreeRecord{}, err
	}
	defer func() { _ = rows.Close() }()
	var found []WorktreeRecord
	for rows.Next() {
		rec, scanErr := scanWorktree(rows)
		if scanErr != nil {
			return WorktreeRecord{}, scanErr
		}
		if rec.WorktreeID == value {
			return rec, nil
		}
		found = append(found, rec)
	}
	if err := rows.Err(); err != nil {
		return WorktreeRecord{}, err
	}
	if len(found) == 0 {
		return WorktreeRecord{}, ErrNotFound
	}
	if len(found) > 1 {
		return WorktreeRecord{}, fmt.Errorf("%w: %q", ErrAmbiguous, value)
	}
	return found[0], nil
}

func scanWorktree(row rowScanner) (WorktreeRecord, error) {
	var rec WorktreeRecord
	var removed sql.NullInt64
	err := row.Scan(&rec.WorktreeID, &rec.Path, &rec.PathDevice, &rec.PathInode,
		&rec.CommonGitDir, &rec.AdminGitDir, &rec.HEAD, &rec.Ref, &rec.Branch,
		&rec.SourcesJSON, &rec.State, &rec.FirstSeenNs, &rec.LastSeenNs,
		&rec.LastActivityNs, &rec.InactiveSinceNs, &rec.DaemonStartedNs,
		&rec.StatusFingerprint, &rec.ProtectionJSON, &rec.EvidenceJSON,
		&rec.ApprovedLinksJSON, &rec.GitIdentityJSON, &rec.Complete, &removed,
		&rec.RecreateCommand)
	if err != nil {
		return rec, fmt.Errorf("storage: scanning worktree: %w", err)
	}
	if removed.Valid {
		value := removed.Int64
		rec.RemovedNs = &value
	}
	return rec, nil
}

// WorktreeStateCounts returns current inventory counts by conclusion.
func (s *Store) WorktreeStateCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT state, COUNT(*) FROM worktrees GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, err
		}
		out[state] = count
	}
	return out, rows.Err()
}
