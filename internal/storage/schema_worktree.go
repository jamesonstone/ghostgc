package storage

// schemaV9 adds manual-only worktree inventory and pre-side-effect actions.
const schemaV9 = `
CREATE TABLE worktrees (
	worktree_id TEXT PRIMARY KEY,
	path TEXT NOT NULL,
	path_device INTEGER NOT NULL DEFAULT 0,
	path_inode INTEGER NOT NULL DEFAULT 0,
	common_git_dir TEXT NOT NULL DEFAULT '',
	admin_git_dir TEXT NOT NULL DEFAULT '',
	head TEXT NOT NULL DEFAULT '',
	ref TEXT NOT NULL DEFAULT '',
	branch TEXT NOT NULL DEFAULT '',
	sources TEXT NOT NULL DEFAULT '[]',
	state TEXT NOT NULL,
	first_seen_ns INTEGER NOT NULL,
	last_seen_ns INTEGER NOT NULL,
	last_activity_ns INTEGER NOT NULL,
	inactive_since_ns INTEGER NOT NULL DEFAULT 0,
	daemon_started_ns INTEGER NOT NULL,
	status_fingerprint TEXT NOT NULL DEFAULT '',
	protection TEXT NOT NULL DEFAULT '[]',
	evidence TEXT NOT NULL DEFAULT '{}',
	approved_links TEXT NOT NULL DEFAULT '[]',
	git_identity TEXT NOT NULL DEFAULT '{}',
	registered INTEGER NOT NULL DEFAULT 0,
	complete INTEGER NOT NULL DEFAULT 0,
	removed_ns INTEGER,
	recreate_command TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX worktrees_state_idx ON worktrees(state, last_seen_ns);
CREATE INDEX worktrees_path_idx ON worktrees(path);

CREATE TABLE worktree_actions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	action_id TEXT NOT NULL UNIQUE,
	worktree_id TEXT NOT NULL,
	path TEXT NOT NULL,
	branch TEXT NOT NULL DEFAULT '',
	requested_ns INTEGER NOT NULL,
	updated_ns INTEGER NOT NULL,
	result TEXT NOT NULL,
	reason TEXT NOT NULL,
	evidence TEXT NOT NULL DEFAULT '[]',
	recreate_command TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX worktree_actions_requested_idx ON worktree_actions(requested_ns);
CREATE INDEX worktree_actions_worktree_idx ON worktree_actions(worktree_id, requested_ns);
CREATE INDEX worktree_actions_result_idx ON worktree_actions(result, requested_ns);
`
