package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// SessionRelationships returns the graph edges recorded for one session,
// oldest first so that the order tells the story of how the session grew.
func (s *Store) SessionRelationships(ctx context.Context, sessionID string) ([]RelationshipRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id, kind, from_proc_uid, to_proc_uid, detail, first_seen_ns, last_seen_ns
		FROM session_relationships WHERE session_id = ?
		ORDER BY first_seen_ns ASC, kind ASC, from_proc_uid ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("storage: listing session relationships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []RelationshipRecord
	for rows.Next() {
		var rec RelationshipRecord
		if err := rows.Scan(&rec.SessionID, &rec.Kind, &rec.FromProcUID, &rec.ToProcUID,
			&rec.Detail, &rec.FirstSeenNs, &rec.LastSeenNs); err != nil {
			return nil, fmt.Errorf("storage: scanning relationship: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ProcessRelationships returns the edges that name a process, from either end.
func (s *Store) ProcessRelationships(ctx context.Context, procUID string) ([]RelationshipRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id, kind, from_proc_uid, to_proc_uid, detail, first_seen_ns, last_seen_ns
		FROM session_relationships WHERE from_proc_uid = ? OR to_proc_uid = ?
		ORDER BY first_seen_ns ASC`, procUID, procUID)
	if err != nil {
		return nil, fmt.Errorf("storage: listing process relationships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []RelationshipRecord
	for rows.Next() {
		var rec RelationshipRecord
		if err := rows.Scan(&rec.SessionID, &rec.Kind, &rec.FromProcUID, &rec.ToProcUID,
			&rec.Detail, &rec.FirstSeenNs, &rec.LastSeenNs); err != nil {
			return nil, fmt.Errorf("storage: scanning relationship: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// AuditFilter narrows an audit-log query.
type AuditFilter struct {
	SinceNs int64
	Kind    string
	Subject string
	Limit   int
}

// ListAudit returns audit entries, newest first.
func (s *Store) ListAudit(ctx context.Context, f AuditFilter) ([]AuditRecord, error) {
	q := `SELECT id, ts_ns, kind, subject, summary, evidence FROM audit_log`
	var (
		where []string
		args  []any
	)
	if f.SinceNs > 0 {
		where = append(where, "ts_ns >= ?")
		args = append(args, f.SinceNs)
	}
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, f.Kind)
	}
	if f.Subject != "" {
		where = append(where, "subject = ?")
		args = append(args, f.Subject)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY ts_ns DESC, id DESC"
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	q += " LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: listing audit log: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []AuditRecord
	for rows.Next() {
		var rec AuditRecord
		if err := rows.Scan(&rec.ID, &rec.TsNs, &rec.Kind, &rec.Subject, &rec.Summary, &rec.EvidenceJSON); err != nil {
			return nil, fmt.Errorf("storage: scanning audit entry: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// LastScan returns the most recent scan summary.
func (s *Store) LastScan(ctx context.Context) (ScanRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, started_ns, duration_us, visible_processes, inspected_processes, attributed_processes, sessions, error
		FROM scans ORDER BY id DESC LIMIT 1`)
	var rec ScanRecord
	err := row.Scan(&rec.ID, &rec.StartedNs, &rec.DurationUs, &rec.VisibleProcesses,
		&rec.InspectedProcesses, &rec.AttributedProcesses, &rec.Sessions, &rec.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return ScanRecord{}, ErrNotFound
	}
	if err != nil {
		return ScanRecord{}, fmt.Errorf("storage: reading last scan: %w", err)
	}
	return rec, nil
}

// Counts summarises the contents of the database.
type Counts struct {
	Sessions       int64 `json:"sessions"`
	ActiveSessions int64 `json:"active_sessions"`
	Processes      int64 `json:"processes"`
	LiveProcesses  int64 `json:"live_processes"`
	Observations   int64 `json:"observations"`
	Scans          int64 `json:"scans"`
	AuditEntries   int64 `json:"audit_entries"`
	Relationships  int64 `json:"relationships"`
}

// Counts returns row counts for the status and doctor commands.
func (s *Store) Counts(ctx context.Context) (Counts, error) {
	var c Counts
	queries := []struct {
		dst *int64
		q   string
	}{
		{&c.Sessions, `SELECT COUNT(*) FROM sessions`},
		{&c.ActiveSessions, `SELECT COUNT(*) FROM sessions WHERE ended_ns IS NULL`},
		{&c.Processes, `SELECT COUNT(*) FROM processes`},
		{&c.LiveProcesses, `SELECT COUNT(*) FROM processes WHERE exited_at_ns IS NULL`},
		{&c.Observations, `SELECT COUNT(*) FROM process_observations`},
		{&c.Scans, `SELECT COUNT(*) FROM scans`},
		{&c.AuditEntries, `SELECT COUNT(*) FROM audit_log`},
		{&c.Relationships, `SELECT COUNT(*) FROM session_relationships`},
	}
	for _, q := range queries {
		if err := s.db.QueryRowContext(ctx, q.q).Scan(q.dst); err != nil {
			return Counts{}, fmt.Errorf("storage: counting: %w", err)
		}
	}
	return c, nil
}

// GetMeta reads a daemon key/value.
func (s *Store) GetMeta(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}
