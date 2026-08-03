package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const processColumns = `proc_uid, pid, start_time_ns, ppid, original_ppid, pgid, sid, uid,
	comm, exec_path, cmdline, cwd, tty,
	agent_id, session_id, relation, attribution_confidence, attribution_evidence,
	first_seen_ns, last_seen_ns, exited_at_ns,
	repository_path, original_parent_observed`

// ProcessFilter narrows a process listing.
type ProcessFilter struct {
	SessionID  string
	AgentID    string
	LiveOnly   bool
	PID        int
	Limit      int
	IncludeAll bool
}

// ListProcesses returns persisted processes, most recently seen first.
func (s *Store) ListProcesses(ctx context.Context, f ProcessFilter) ([]ProcessRecord, error) {
	q := `SELECT ` + processColumns + ` FROM processes`
	var (
		where []string
		args  []any
	)
	if f.SessionID != "" {
		where = append(where, "session_id = ?")
		args = append(args, f.SessionID)
	}
	if f.AgentID != "" {
		where = append(where, "agent_id = ?")
		args = append(args, f.AgentID)
	}
	if f.PID > 0 {
		where = append(where, "pid = ?")
		args = append(args, f.PID)
	}
	if f.LiveOnly {
		where = append(where, "exited_at_ns IS NULL")
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY last_seen_ns DESC, pid ASC"
	if f.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, f.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: listing processes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ProcessRecord
	for rows.Next() {
		rec, err := scanProcess(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// GetProcess returns a single persisted process by its PID-reuse-safe key.
func (s *Store) GetProcess(ctx context.Context, procUID string) (ProcessRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+processColumns+` FROM processes WHERE proc_uid = ?`, procUID)
	rec, err := scanProcess(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ProcessRecord{}, ErrNotFound
	}
	return rec, err
}

func scanProcess(r rowScanner) (ProcessRecord, error) {
	var (
		rec       ProcessRecord
		agentID   sql.NullString
		sessionID sql.NullString
		exited    sql.NullInt64
	)
	err := r.Scan(&rec.ProcUID, &rec.PID, &rec.StartTimeNs, &rec.PPID, &rec.OriginalPPID, &rec.PGID, &rec.SID, &rec.UID,
		&rec.Comm, &rec.ExecPath, &rec.Cmdline, &rec.CWD, &rec.TTY,
		&agentID, &sessionID, &rec.Relation, &rec.Confidence, &rec.EvidenceJSON,
		&rec.FirstSeenNs, &rec.LastSeenNs, &exited,
		&rec.RepositoryPath, &rec.OriginalParentObserved)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProcessRecord{}, err
		}
		return ProcessRecord{}, fmt.Errorf("storage: scanning process: %w", err)
	}
	rec.AgentID = agentID.String
	rec.SessionID = sessionID.String
	if exited.Valid {
		v := exited.Int64
		rec.ExitedAtNs = &v
	}
	return rec, nil
}

// LiveOwnership loads every recorded session/process association whose process
// has not been seen to exit.
//
// The daemon holds this in memory so that a process whose live parent link has
// been destroyed by reparenting keeps the ownership it was first observed with.
func (s *Store) LiveOwnership(ctx context.Context) (map[string]OwnershipRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sp.session_id, sp.proc_uid, sp.agent_id, sp.relation, sp.confidence, sp.evidence,
		       sp.original_ppid, sp.original_parent_observed, sp.first_seen_ns, sp.last_seen_ns
		FROM session_processes sp
		LEFT JOIN processes p ON p.proc_uid = sp.proc_uid
		WHERE p.proc_uid IS NULL OR p.exited_at_ns IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("storage: loading ownership: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]OwnershipRecord)
	for rows.Next() {
		var rec OwnershipRecord
		if err := rows.Scan(&rec.SessionID, &rec.ProcUID, &rec.AgentID, &rec.Relation, &rec.Confidence,
			&rec.EvidenceJSON, &rec.OriginalPPID, &rec.OriginalParentObserved,
			&rec.FirstSeenNs, &rec.LastSeenNs); err != nil {
			return nil, fmt.Errorf("storage: scanning ownership: %w", err)
		}
		// A process can only belong to one session; the highest-confidence
		// record wins, and ties keep the earliest observation.
		if prev, ok := out[rec.ProcUID]; ok && prev.Confidence >= rec.Confidence {
			continue
		}
		out[rec.ProcUID] = rec
	}
	return out, rows.Err()
}

// SessionProcesses returns the recorded ownership rows for one session.
func (s *Store) SessionProcesses(ctx context.Context, sessionID string) ([]OwnershipRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id, proc_uid, agent_id, relation, confidence, evidence, original_ppid,
		       original_parent_observed, first_seen_ns, last_seen_ns
		FROM session_processes WHERE session_id = ? ORDER BY first_seen_ns ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("storage: listing session processes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []OwnershipRecord
	for rows.Next() {
		var rec OwnershipRecord
		if err := rows.Scan(&rec.SessionID, &rec.ProcUID, &rec.AgentID, &rec.Relation, &rec.Confidence,
			&rec.EvidenceJSON, &rec.OriginalPPID, &rec.OriginalParentObserved,
			&rec.FirstSeenNs, &rec.LastSeenNs); err != nil {
			return nil, fmt.Errorf("storage: scanning session process: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
