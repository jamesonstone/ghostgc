package storage

// schemaV11 adds reversible worktree retirement state.
const schemaV11 = `
ALTER TABLE worktrees ADD COLUMN original_path TEXT NOT NULL DEFAULT '';
ALTER TABLE worktrees ADD COLUMN retired_ns INTEGER;
ALTER TABLE worktrees ADD COLUMN retirement_grace_until_ns INTEGER NOT NULL DEFAULT 0;
`
