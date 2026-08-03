package storage

// schemaVersion is the newest migration below.
const schemaVersion = 2

// migration is one forward step. Migrations are applied in order, each in its
// own transaction, and the version is recorded as each completes.
//
// There is no down path. A migration that only adds columns and tables cannot
// destroy recorded ownership, and ownership is the one thing in this database
// that cannot be recomputed from a fresh observation.
type migration struct {
	version int
	stmts   string
}

var migrations = []migration{
	{version: 1, stmts: schemaV1},
	{version: 2, stmts: schemaV2},
}

// schemaV1 is the delivery phase 1 schema.
//
// Only agent-attributed processes are persisted. The whole process table is
// scanned every cycle, but writing all of it would both violate the
// specification's non-goal of monitoring activity outside coding-agent
// sessions and blow the storage budget: roughly a thousand user processes
// sampled four times a minute is several million rows a day. Unattributed
// processes contribute to the per-scan counters and nothing else.
const schemaV1 = `
CREATE TABLE meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
) STRICT;

-- One row per observed agent-attributed process. proc_uid is "pid:start_ns",
-- which is what makes a recycled PID a different row rather than a silent
-- identity change.
CREATE TABLE processes (
	proc_uid               TEXT PRIMARY KEY,
	pid                    INTEGER NOT NULL,
	start_time_ns          INTEGER NOT NULL,
	ppid                   INTEGER NOT NULL,
	original_ppid          INTEGER NOT NULL,
	pgid                   INTEGER NOT NULL,
	sid                    INTEGER NOT NULL,
	uid                    INTEGER NOT NULL,
	comm                   TEXT NOT NULL,
	exec_path              TEXT NOT NULL,
	cmdline                TEXT NOT NULL,
	cwd                    TEXT NOT NULL,
	tty                    TEXT NOT NULL,
	agent_id               TEXT,
	session_id             TEXT,
	relation               TEXT NOT NULL DEFAULT '',
	attribution_confidence REAL NOT NULL DEFAULT 0,
	attribution_evidence   TEXT NOT NULL DEFAULT '[]',
	first_seen_ns          INTEGER NOT NULL,
	last_seen_ns           INTEGER NOT NULL,
	exited_at_ns           INTEGER
) STRICT;
CREATE INDEX processes_pid_idx        ON processes(pid);
CREATE INDEX processes_session_idx    ON processes(session_id);
CREATE INDEX processes_last_seen_idx  ON processes(last_seen_ns);
CREATE INDEX processes_live_idx       ON processes(exited_at_ns) WHERE exited_at_ns IS NULL;

-- Time series. Deltas are computed from consecutive rows in delivery phase 3.
CREATE TABLE process_observations (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	proc_uid     TEXT NOT NULL,
	scan_id      INTEGER NOT NULL,
	ts_ns        INTEGER NOT NULL,
	status       TEXT NOT NULL,
	ppid         INTEGER NOT NULL,
	cpu_time_ns  INTEGER NOT NULL,
	rss_bytes    INTEGER NOT NULL,
	vsz_bytes    INTEGER NOT NULL,
	threads      INTEGER NOT NULL
) STRICT;
CREATE INDEX process_observations_ts_idx   ON process_observations(ts_ns);
CREATE INDEX process_observations_proc_idx ON process_observations(proc_uid, ts_ns);

CREATE TABLE scans (
	id                    INTEGER PRIMARY KEY AUTOINCREMENT,
	started_ns            INTEGER NOT NULL,
	duration_us           INTEGER NOT NULL,
	visible_processes     INTEGER NOT NULL,
	inspected_processes   INTEGER NOT NULL,
	attributed_processes  INTEGER NOT NULL,
	sessions              INTEGER NOT NULL,
	error                 TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX scans_started_idx ON scans(started_ns);

CREATE TABLE sessions (
	session_id       TEXT PRIMARY KEY,
	agent_id         TEXT NOT NULL,
	root_proc_uid    TEXT NOT NULL,
	root_pid         INTEGER NOT NULL,
	state            TEXT NOT NULL,
	confidence       REAL NOT NULL,
	working_dir      TEXT NOT NULL DEFAULT '',
	repository_path  TEXT NOT NULL DEFAULT '',
	tty              TEXT NOT NULL DEFAULT '',
	invocation       TEXT NOT NULL DEFAULT '',
	metadata         TEXT NOT NULL DEFAULT '{}',
	evidence         TEXT NOT NULL DEFAULT '[]',
	started_ns       INTEGER NOT NULL,
	last_seen_ns     INTEGER NOT NULL,
	ended_ns         INTEGER
) STRICT;
CREATE INDEX sessions_state_idx     ON sessions(state);
CREATE INDEX sessions_last_seen_idx ON sessions(last_seen_ns);

-- Ownership recorded the first time it was observed. The specification is
-- explicit that operating-system reparenting can destroy the original process
-- tree, so the association is durable: once a process has been seen to belong
-- to a session, losing the live parent link never erases that fact.
CREATE TABLE session_processes (
	session_id     TEXT NOT NULL,
	proc_uid       TEXT NOT NULL,
	agent_id       TEXT NOT NULL,
	relation       TEXT NOT NULL,
	confidence     REAL NOT NULL,
	evidence       TEXT NOT NULL DEFAULT '[]',
	original_ppid  INTEGER NOT NULL,
	first_seen_ns  INTEGER NOT NULL,
	last_seen_ns   INTEGER NOT NULL,
	PRIMARY KEY (session_id, proc_uid)
) STRICT;
CREATE INDEX session_processes_proc_idx ON session_processes(proc_uid);

-- Every state transition and every decision, with its evidence. This is the
-- audit trail the specification requires; it is append-only within a
-- retention window.
CREATE TABLE audit_log (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	ts_ns     INTEGER NOT NULL,
	kind      TEXT NOT NULL,
	subject   TEXT NOT NULL,
	summary   TEXT NOT NULL,
	evidence  TEXT NOT NULL DEFAULT '[]'
) STRICT;
CREATE INDEX audit_log_ts_idx      ON audit_log(ts_ns);
CREATE INDEX audit_log_kind_idx    ON audit_log(kind, ts_ns);
CREATE INDEX audit_log_subject_idx ON audit_log(subject, ts_ns);
`

// schemaV2 adds the session graph: typed relationships, launch context,
// terminal and repository association, and the state machine's bookkeeping.
//
// Every statement adds; none drops or rewrites. An existing database keeps
// every ownership row it had.
const schemaV2 = `
-- The agent's own session identifier, when it exposes one. This is what lets a
-- process that merely inherited the identifier through its environment be
-- attributed to the session that actually owns it.
ALTER TABLE sessions ADD COLUMN native_session_id TEXT NOT NULL DEFAULT '';

-- State machine bookkeeping. Every transition is also written to audit_log
-- with its evidence; these columns make the current state cheap to query.
ALTER TABLE sessions ADD COLUMN previous_state   TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN state_changed_ns INTEGER NOT NULL DEFAULT 0;

-- Launch context: the nearest ancestor of the session root that is not itself
-- an agent process. An editor extension host, a terminal shell, or a CI runner
-- are all wildly different situations that look identical without this.
ALTER TABLE sessions ADD COLUMN host_proc_uid  TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN host_pid       INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN host_name      TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN host_exec_path TEXT NOT NULL DEFAULT '';

-- Repository and terminal association.
ALTER TABLE sessions ADD COLUMN branch          TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN repository_busy INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN terminal_sid    INTEGER NOT NULL DEFAULT 0;

ALTER TABLE processes ADD COLUMN repository_path TEXT NOT NULL DEFAULT '';

-- Whether the original parent was actually observed alive. When it was not,
-- original_ppid is whatever the kernel reported after reparenting -- usually 1
-- -- and presenting that as the creator would be a fabrication.
ALTER TABLE processes         ADD COLUMN original_parent_observed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE session_processes ADD COLUMN original_parent_observed INTEGER NOT NULL DEFAULT 0;

-- Typed, timestamped edges. Specification section 9 requires a graph rather
-- than a tree precisely because reparenting destroys tree edges; recording the
-- reason a process belongs survives that.
CREATE TABLE session_relationships (
	session_id     TEXT NOT NULL,
	kind           TEXT NOT NULL,
	from_proc_uid  TEXT NOT NULL,
	to_proc_uid    TEXT NOT NULL DEFAULT '',
	detail         TEXT NOT NULL DEFAULT '',
	first_seen_ns  INTEGER NOT NULL,
	last_seen_ns   INTEGER NOT NULL,
	PRIMARY KEY (session_id, kind, from_proc_uid, to_proc_uid)
) STRICT;
CREATE INDEX session_relationships_session_idx ON session_relationships(session_id);
CREATE INDEX session_relationships_from_idx    ON session_relationships(from_proc_uid);
`
