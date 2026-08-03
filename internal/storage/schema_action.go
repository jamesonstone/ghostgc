package storage

// schemaV7 records the durable state around every manually requested action.
// The attempting row is committed before the signal and completion is an
// additive update, so a crash cannot erase evidence of an unresolved effect.
const schemaV7 = `
CREATE TABLE actions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	action_id TEXT NOT NULL UNIQUE,
	policy_id TEXT NOT NULL,
	proc_uid TEXT NOT NULL,
	session_id TEXT NOT NULL,
	requested_ns INTEGER NOT NULL,
	updated_ns INTEGER NOT NULL,
	result TEXT NOT NULL,
	signal TEXT NOT NULL,
	reason TEXT NOT NULL,
	evidence TEXT NOT NULL DEFAULT '[]'
) STRICT;
CREATE INDEX actions_requested_idx ON actions(requested_ns);
CREATE INDEX actions_proc_idx ON actions(proc_uid, requested_ns);
CREATE INDEX actions_policy_idx ON actions(policy_id, requested_ns);
CREATE INDEX actions_result_idx ON actions(result, requested_ns);
`
