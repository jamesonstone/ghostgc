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
