package storage

// schemaV5 records audit-only policy decisions and durable exact-key cooldowns.
const schemaV5 = `
CREATE TABLE policy_decisions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	policy_id TEXT NOT NULL,
	proc_uid TEXT NOT NULL,
	session_id TEXT NOT NULL,
	ts_ns INTEGER NOT NULL,
	classification_ts_ns INTEGER NOT NULL,
	classification_state TEXT NOT NULL,
	result TEXT NOT NULL,
	reason TEXT NOT NULL,
	cooldown_until_ns INTEGER NOT NULL DEFAULT 0,
	evidence TEXT NOT NULL DEFAULT '[]'
) STRICT;
CREATE INDEX policy_decisions_ts_idx ON policy_decisions(ts_ns);
CREATE INDEX policy_decisions_policy_proc_idx ON policy_decisions(policy_id, proc_uid, ts_ns);
CREATE INDEX policy_decisions_result_idx ON policy_decisions(result, ts_ns);
`

// schemaV6 gives every committed evaluation a unique identity. Timestamps can
// repeat in deterministic tests and on coarse clocks, so they cannot safely
// distinguish a new empty projection from an older non-empty one.
const schemaV6 = `
CREATE TABLE policy_evaluations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts_ns INTEGER NOT NULL
) STRICT;
CREATE INDEX policy_evaluations_ts_idx ON policy_evaluations(ts_ns);

ALTER TABLE policy_decisions ADD COLUMN evaluation_id INTEGER NOT NULL DEFAULT 0;
CREATE INDEX policy_decisions_evaluation_idx ON policy_decisions(evaluation_id, id);
`
