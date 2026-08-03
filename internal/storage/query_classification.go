package storage

import (
	"context"
	"fmt"
	"strings"
)

// ClassificationFilter narrows classification history. Results are newest first.
type ClassificationFilter struct {
	ProcUID   string
	SessionID string
	State     string
	SinceNs   int64
	Limit     int
	Latest    bool
}

const classificationColumns = `id, proc_uid, session_id, ts_ns, activity_ts_ns,
	state, basis_state, detached, session_ended, stable_since_ns, evidence`

const qualifiedClassificationColumns = `c.id, c.proc_uid, c.session_id, c.ts_ns, c.activity_ts_ns,
	c.state, c.basis_state, c.detached, c.session_ended, c.stable_since_ns, c.evidence`

// ListClassifications returns bounded deterministic conclusions.
func (s *Store) ListClassifications(ctx context.Context, f ClassificationFilter) ([]ClassificationRecord, error) {
	q := `SELECT ` + classificationColumns + ` FROM process_classifications`
	var where []string
	var args []any
	if f.Latest {
		where = append(where, "id IN (SELECT MAX(id) FROM process_classifications GROUP BY proc_uid)")
	}
	for _, clause := range []struct {
		value string
		sql   string
	}{{f.ProcUID, "proc_uid = ?"}, {f.SessionID, "session_id = ?"}, {f.State, "state = ?"}} {
		if clause.value != "" {
			where, args = append(where, clause.sql), append(args, clause.value)
		}
	}
	if f.SinceNs > 0 {
		where, args = append(where, "ts_ns >= ?"), append(args, f.SinceNs)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY ts_ns DESC, id DESC LIMIT ?"
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: listing classifications: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ClassificationRecord
	for rows.Next() {
		var rec ClassificationRecord
		if err := rows.Scan(&rec.ID, &rec.ProcUID, &rec.SessionID, &rec.TsNs,
			&rec.ActivityTsNs, &rec.State, &rec.BasisState, &rec.Detached,
			&rec.SessionEnded, &rec.StableSinceNs, &rec.EvidenceJSON); err != nil {
			return nil, fmt.Errorf("storage: scanning classification: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// LatestClassifications returns the complete latest-per-process set. It is an
// internal join primitive and deliberately has no presentation limit.
func (s *Store) LatestClassifications(ctx context.Context, sessionID string) ([]ClassificationRecord, error) {
	q := `SELECT ` + qualifiedClassificationColumns + ` FROM process_classifications c
		JOIN (SELECT proc_uid, MAX(id) AS latest_id FROM process_classifications GROUP BY proc_uid) latest
		ON latest.latest_id = c.id`
	var args []any
	if sessionID != "" {
		q += ` WHERE c.session_id = ?`
		args = append(args, sessionID)
	}
	q += ` ORDER BY c.ts_ns DESC, c.id DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: listing complete latest classifications: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ClassificationRecord
	for rows.Next() {
		var rec ClassificationRecord
		if err := rows.Scan(&rec.ID, &rec.ProcUID, &rec.SessionID, &rec.TsNs,
			&rec.ActivityTsNs, &rec.State, &rec.BasisState, &rec.Detached,
			&rec.SessionEnded, &rec.StableSinceNs, &rec.EvidenceJSON); err != nil {
			return nil, fmt.Errorf("storage: scanning latest classification: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ClassificationCounts returns complete latest-per-process counts by state.
func (s *Store) ClassificationCounts(ctx context.Context, sessionID string) (map[string]int, error) {
	q := `SELECT c.state, COUNT(*) FROM process_classifications c
		JOIN (SELECT proc_uid, MAX(id) AS latest_id FROM process_classifications GROUP BY proc_uid) latest
		ON latest.latest_id = c.id`
	var args []any
	if sessionID != "" {
		q += ` WHERE c.session_id = ?`
		args = append(args, sessionID)
	}
	q += ` GROUP BY c.state`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: counting classifications: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]int)
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
