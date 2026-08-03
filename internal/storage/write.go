package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// Tx is a write transaction. One observation cycle is applied as a single
// transaction so that a crash mid-scan can never leave a session recorded
// without its processes, or a process attributed to a session that was not
// written.
type Tx struct {
	ctx context.Context
	tx  *sql.Tx
}

// WithTx runs fn inside a transaction, committing on success.
func (s *Store) WithTx(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: begin: %w", err)
	}
	if err := fn(&Tx{ctx: ctx, tx: tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: commit: %w", err)
	}
	return nil
}

// InsertScan records one observation cycle and returns its id.
func (t *Tx) InsertScan(rec ScanRecord) (int64, error) {
	res, err := t.tx.ExecContext(t.ctx, `
		INSERT INTO scans (started_ns, duration_us, visible_processes, inspected_processes, attributed_processes, sessions, error)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.StartedNs, rec.DurationUs, rec.VisibleProcesses, rec.InspectedProcesses,
		rec.AttributedProcesses, rec.Sessions, rec.Error)
	if err != nil {
		return 0, fmt.Errorf("storage: inserting scan: %w", err)
	}
	return res.LastInsertId()
}

// UpsertProcess writes a process row.
//
// first_seen_ns and original_ppid are written once and never updated: the
// original parent is the fact that survives reparenting, and overwriting it
// would destroy the only record of who actually created the process.
func (t *Tx) UpsertProcess(rec ProcessRecord) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT INTO processes (
			proc_uid, pid, start_time_ns, ppid, original_ppid, pgid, sid, uid,
			comm, exec_path, cmdline, cwd, tty,
			agent_id, session_id, relation, attribution_confidence, attribution_evidence,
			first_seen_ns, last_seen_ns, exited_at_ns,
			repository_path, original_parent_observed
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
		ON CONFLICT(proc_uid) DO UPDATE SET
			ppid                   = excluded.ppid,
			pgid                   = excluded.pgid,
			sid                    = excluded.sid,
			comm                   = excluded.comm,
			exec_path              = excluded.exec_path,
			cmdline                = excluded.cmdline,
			cwd                    = excluded.cwd,
			tty                    = excluded.tty,
			agent_id               = excluded.agent_id,
			session_id             = excluded.session_id,
			relation               = excluded.relation,
			attribution_confidence = excluded.attribution_confidence,
			attribution_evidence   = excluded.attribution_evidence,
			repository_path        = excluded.repository_path,
			last_seen_ns           = excluded.last_seen_ns,
			exited_at_ns           = NULL`,
		rec.ProcUID, rec.PID, rec.StartTimeNs, rec.PPID, rec.OriginalPPID, rec.PGID, rec.SID, rec.UID,
		rec.Comm, rec.ExecPath, rec.Cmdline, rec.CWD, rec.TTY,
		nullString(rec.AgentID), nullString(rec.SessionID), rec.Relation, rec.Confidence, jsonOrEmpty(rec.EvidenceJSON),
		rec.FirstSeenNs, rec.LastSeenNs, rec.RepositoryPath, rec.OriginalParentObserved)
	if err != nil {
		return fmt.Errorf("storage: upserting process %s: %w", rec.ProcUID, err)
	}
	return nil
}

// InsertObservation appends one time-series sample.
func (t *Tx) InsertObservation(rec ObservationRecord) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT INTO process_observations (proc_uid, scan_id, ts_ns, status, ppid, cpu_time_ns, rss_bytes, vsz_bytes, threads)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ProcUID, rec.ScanID, rec.TsNs, rec.Status, rec.PPID, rec.CPUTimeNs, rec.RSSBytes, rec.VSZBytes, rec.Threads)
	if err != nil {
		return fmt.Errorf("storage: inserting observation for %s: %w", rec.ProcUID, err)
	}
	return nil
}

// UpsertSession writes a session row. The root process identity is written
// once; a session's root never changes, and a differing root means a different
// session.
func (t *Tx) UpsertSession(rec SessionRecord) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT INTO sessions (
			session_id, agent_id, root_proc_uid, root_pid, state, confidence,
			working_dir, repository_path, tty, invocation, metadata, evidence,
			started_ns, last_seen_ns, ended_ns,
			native_session_id, previous_state, state_changed_ns,
			host_proc_uid, host_pid, host_name, host_exec_path,
			branch, repository_busy, terminal_sid
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			state             = excluded.state,
			confidence        = excluded.confidence,
			working_dir       = excluded.working_dir,
			repository_path   = excluded.repository_path,
			tty               = excluded.tty,
			invocation        = excluded.invocation,
			metadata          = excluded.metadata,
			evidence          = excluded.evidence,
			last_seen_ns      = excluded.last_seen_ns,
			native_session_id = excluded.native_session_id,
			previous_state    = excluded.previous_state,
			state_changed_ns  = excluded.state_changed_ns,
			host_proc_uid     = excluded.host_proc_uid,
			host_pid          = excluded.host_pid,
			host_name         = excluded.host_name,
			host_exec_path    = excluded.host_exec_path,
			branch            = excluded.branch,
			repository_busy   = excluded.repository_busy,
			terminal_sid      = excluded.terminal_sid,
			ended_ns          = NULL`,
		rec.SessionID, rec.AgentID, rec.RootProcUID, rec.RootPID, rec.State, rec.Confidence,
		rec.WorkingDir, rec.RepositoryPath, rec.TTY, rec.Invocation,
		jsonObjectOrEmpty(rec.MetadataJSON), jsonOrEmpty(rec.EvidenceJSON),
		rec.StartedNs, rec.LastSeenNs,
		rec.NativeSessionID, rec.PreviousState, rec.StateChangedNs,
		rec.HostProcUID, rec.HostPID, rec.HostName, rec.HostExecPath,
		rec.Branch, rec.RepositoryBusy, rec.TerminalSID)
	if err != nil {
		return fmt.Errorf("storage: upserting session %s: %w", rec.SessionID, err)
	}
	return nil
}

// EndSession marks a session as no longer running.
func (t *Tx) EndSession(sessionID, previousState, state string, endedNs int64) error {
	_, err := t.tx.ExecContext(t.ctx, `
		UPDATE sessions
		SET state = ?, previous_state = ?, state_changed_ns = ?, ended_ns = COALESCE(ended_ns, ?)
		WHERE session_id = ?`, state, previousState, endedNs, endedNs, sessionID)
	if err != nil {
		return fmt.Errorf("storage: ending session %s: %w", sessionID, err)
	}
	return nil
}

// UpsertOwnership records that a process belongs to a session.
//
// Confidence only ever rises for an existing association and the relation is
// never downgraded away from "root". Ownership observed once is not discarded
// because a later snapshot could not re-derive it; that is precisely the case
// reparenting creates.
func (t *Tx) UpsertOwnership(rec OwnershipRecord) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT INTO session_processes (
			session_id, proc_uid, agent_id, relation, confidence, evidence,
			original_ppid, original_parent_observed, first_seen_ns, last_seen_ns
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, proc_uid) DO UPDATE SET
			relation     = CASE WHEN session_processes.relation = 'root' THEN 'root' ELSE excluded.relation END,
			confidence   = MAX(session_processes.confidence, excluded.confidence),
			evidence     = excluded.evidence,
			last_seen_ns = excluded.last_seen_ns`,
		rec.SessionID, rec.ProcUID, rec.AgentID, rec.Relation, rec.Confidence, jsonOrEmpty(rec.EvidenceJSON),
		rec.OriginalPPID, rec.OriginalParentObserved, rec.FirstSeenNs, rec.LastSeenNs)
	if err != nil {
		return fmt.Errorf("storage: upserting ownership %s/%s: %w", rec.SessionID, rec.ProcUID, err)
	}
	return nil
}

// MarkExitedBefore marks every process not seen in the current scan as exited.
func (t *Tx) MarkExitedBefore(scanStartNs, exitedAtNs int64) (int64, error) {
	res, err := t.tx.ExecContext(t.ctx, `
		UPDATE processes SET exited_at_ns = ?
		WHERE exited_at_ns IS NULL AND last_seen_ns < ?`, exitedAtNs, scanStartNs)
	if err != nil {
		return 0, fmt.Errorf("storage: marking exited processes: %w", err)
	}
	return res.RowsAffected()
}

// UpsertRelationship records one typed edge of a session graph.
//
// first_seen_ns is written once: an edge records when a relationship was first
// observed, which for a reparenting event is the whole point.
func (t *Tx) UpsertRelationship(rec RelationshipRecord) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT INTO session_relationships (session_id, kind, from_proc_uid, to_proc_uid, detail, first_seen_ns, last_seen_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, kind, from_proc_uid, to_proc_uid) DO UPDATE SET
			detail       = excluded.detail,
			last_seen_ns = excluded.last_seen_ns`,
		rec.SessionID, rec.Kind, rec.FromProcUID, rec.ToProcUID, rec.Detail, rec.FirstSeenNs, rec.LastSeenNs)
	if err != nil {
		return fmt.Errorf("storage: upserting relationship %s/%s: %w", rec.SessionID, rec.Kind, err)
	}
	return nil
}

// AppendAudit writes one audit entry.
func (t *Tx) AppendAudit(rec AuditRecord) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT INTO audit_log (ts_ns, kind, subject, summary, evidence) VALUES (?, ?, ?, ?, ?)`,
		rec.TsNs, rec.Kind, rec.Subject, rec.Summary, jsonOrEmpty(rec.EvidenceJSON))
	if err != nil {
		return fmt.Errorf("storage: appending audit entry: %w", err)
	}
	return nil
}

// SetMeta stores a daemon key/value.
func (t *Tx) SetMeta(key, value string) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT INTO meta(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("storage: setting meta %s: %w", key, err)
	}
	return nil
}

// AppendAudit writes one audit entry outside a scan transaction.
func (s *Store) AppendAudit(ctx context.Context, rec AuditRecord) error {
	return s.WithTx(ctx, func(t *Tx) error { return t.AppendAudit(rec) })
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func jsonOrEmpty(s string) string {
	if s == "" {
		return "[]"
	}
	return s
}

func jsonObjectOrEmpty(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}
