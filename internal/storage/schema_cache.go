package storage

// schemaV9 adds the independent cache-artifact observation and action lane.
const schemaV9 = `
CREATE TABLE cache_evaluations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	observed_ns INTEGER NOT NULL,
	configuration_digest TEXT NOT NULL,
	complete INTEGER NOT NULL,
	inspected INTEGER NOT NULL,
	protected INTEGER NOT NULL,
	candidates INTEGER NOT NULL,
	error TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX cache_evaluations_observed_idx ON cache_evaluations(observed_ns);

CREATE TABLE cache_artifacts (
	artifact_id TEXT PRIMARY KEY,
	provider TEXT NOT NULL,
	agent_id TEXT NOT NULL,
	session_id TEXT NOT NULL DEFAULT '',
	artifact_kind TEXT NOT NULL,
	root_path TEXT NOT NULL,
	relative_path TEXT NOT NULL,
	identity_json TEXT NOT NULL,
	root_identity_json TEXT NOT NULL,
	identity_digest TEXT NOT NULL,
	manifest_digest TEXT NOT NULL,
	first_observed_ns INTEGER NOT NULL,
	last_observed_ns INTEGER NOT NULL,
	stable_since_ns INTEGER NOT NULL,
	lifecycle TEXT NOT NULL,
	reason TEXT NOT NULL,
	evidence TEXT NOT NULL DEFAULT '[]',
	configuration_digest TEXT NOT NULL,
	evaluation_id INTEGER NOT NULL,
	policy_id TEXT NOT NULL DEFAULT '',
	quarantine_path TEXT NOT NULL DEFAULT '',
	quarantined_at_ns INTEGER NOT NULL DEFAULT 0,
	quarantine_digest TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX cache_artifacts_state_idx ON cache_artifacts(lifecycle, last_observed_ns);
CREATE INDEX cache_artifacts_session_idx ON cache_artifacts(session_id, last_observed_ns);
CREATE INDEX cache_artifacts_evaluation_idx ON cache_artifacts(evaluation_id);

CREATE TABLE cache_observations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	artifact_id TEXT NOT NULL,
	evaluation_id INTEGER NOT NULL,
	observed_ns INTEGER NOT NULL,
	identity_digest TEXT NOT NULL,
	manifest_digest TEXT NOT NULL,
	lifecycle TEXT NOT NULL,
	complete INTEGER NOT NULL,
	evidence TEXT NOT NULL DEFAULT '[]'
) STRICT;
CREATE INDEX cache_observations_artifact_idx ON cache_observations(artifact_id, observed_ns);
CREATE INDEX cache_observations_evaluation_idx ON cache_observations(evaluation_id);

CREATE TABLE cache_decisions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	evaluation_id INTEGER NOT NULL,
	artifact_id TEXT NOT NULL,
	policy_id TEXT NOT NULL DEFAULT '',
	result TEXT NOT NULL,
	reason TEXT NOT NULL,
	evidence TEXT NOT NULL DEFAULT '[]'
) STRICT;
CREATE INDEX cache_decisions_artifact_idx ON cache_decisions(artifact_id, evaluation_id);
CREATE INDEX cache_decisions_result_idx ON cache_decisions(result, evaluation_id);

CREATE TABLE cache_quarantines (
	artifact_id TEXT PRIMARY KEY,
	root_path TEXT NOT NULL,
	original_path TEXT NOT NULL,
	quarantine_path TEXT NOT NULL,
	identity_json TEXT NOT NULL,
	manifest_digest TEXT NOT NULL,
	original_manifest_digest TEXT NOT NULL,
	quarantined_ns INTEGER NOT NULL,
	grace_until_ns INTEGER NOT NULL,
	status TEXT NOT NULL,
	updated_ns INTEGER NOT NULL,
	configuration_digest TEXT NOT NULL
) STRICT;
CREATE INDEX cache_quarantines_status_idx ON cache_quarantines(status, updated_ns);

CREATE TABLE cache_actions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	action_id TEXT NOT NULL UNIQUE,
	artifact_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	policy_id TEXT NOT NULL DEFAULT '',
	requested_ns INTEGER NOT NULL,
	updated_ns INTEGER NOT NULL,
	result TEXT NOT NULL,
	reason TEXT NOT NULL,
	evidence TEXT NOT NULL DEFAULT '[]'
) STRICT;
CREATE INDEX cache_actions_artifact_idx ON cache_actions(artifact_id, requested_ns);
CREATE INDEX cache_actions_kind_idx ON cache_actions(kind, requested_ns);
CREATE INDEX cache_actions_result_idx ON cache_actions(result, requested_ns);
`
