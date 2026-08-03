package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound is returned when a lookup matches nothing.
var ErrNotFound = errors.New("storage: not found")

// ErrAmbiguous is returned when a session id prefix matches more than one
// session.
var ErrAmbiguous = errors.New("storage: identifier prefix matches more than one record")

const sessionColumns = `session_id, agent_id, root_proc_uid, root_pid, state, confidence,
	working_dir, repository_path, tty, invocation, metadata, evidence,
	started_ns, last_seen_ns, ended_ns,
	native_session_id, previous_state, state_changed_ns,
	host_proc_uid, host_pid, host_name, host_exec_path,
	branch, repository_busy, terminal_sid`

// SessionFilter narrows a session listing.
type SessionFilter struct {
	States  []string
	AgentID string
	Limit   int
}

// ListSessions returns sessions most recently seen first.
func (s *Store) ListSessions(ctx context.Context, f SessionFilter) ([]SessionRecord, error) {
	q := `SELECT ` + sessionColumns + ` FROM sessions`
	var (
		where []string
		args  []any
	)
	if len(f.States) > 0 {
		where = append(where, "state IN ("+placeholders(len(f.States))+")")
		for _, st := range f.States {
			args = append(args, st)
		}
	}
	if f.AgentID != "" {
		where = append(where, "agent_id = ?")
		args = append(args, f.AgentID)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY last_seen_ns DESC"
	if f.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, f.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: listing sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SessionRecord
	for rows.Next() {
		rec, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// GetSession resolves a full session id or an unambiguous prefix.
func (s *Store) GetSession(ctx context.Context, idOrPrefix string) (SessionRecord, error) {
	if idOrPrefix == "" {
		return SessionRecord{}, ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE session_id = ? OR session_id LIKE ? ORDER BY last_seen_ns DESC LIMIT 2`,
		idOrPrefix, idOrPrefix+"%")
	if err != nil {
		return SessionRecord{}, fmt.Errorf("storage: getting session: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var found []SessionRecord
	for rows.Next() {
		rec, err := scanSession(rows)
		if err != nil {
			return SessionRecord{}, err
		}
		if rec.SessionID == idOrPrefix {
			return rec, nil
		}
		found = append(found, rec)
	}
	if err := rows.Err(); err != nil {
		return SessionRecord{}, err
	}
	switch len(found) {
	case 0:
		return SessionRecord{}, ErrNotFound
	case 1:
		return found[0], nil
	default:
		return SessionRecord{}, fmt.Errorf("%w: %q", ErrAmbiguous, idOrPrefix)
	}
}

type rowScanner interface{ Scan(dest ...any) error }

func scanSession(r rowScanner) (SessionRecord, error) {
	var (
		rec   SessionRecord
		ended sql.NullInt64
	)
	err := r.Scan(&rec.SessionID, &rec.AgentID, &rec.RootProcUID, &rec.RootPID, &rec.State, &rec.Confidence,
		&rec.WorkingDir, &rec.RepositoryPath, &rec.TTY, &rec.Invocation, &rec.MetadataJSON, &rec.EvidenceJSON,
		&rec.StartedNs, &rec.LastSeenNs, &ended,
		&rec.NativeSessionID, &rec.PreviousState, &rec.StateChangedNs,
		&rec.HostProcUID, &rec.HostPID, &rec.HostName, &rec.HostExecPath,
		&rec.Branch, &rec.RepositoryBusy, &rec.TerminalSID)
	if err != nil {
		return SessionRecord{}, fmt.Errorf("storage: scanning session: %w", err)
	}
	if ended.Valid {
		v := ended.Int64
		rec.EndedNs = &v
	}
	return rec, nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
