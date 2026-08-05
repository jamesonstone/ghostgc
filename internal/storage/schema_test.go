package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestOpenCreatesSchemaAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ghostgc.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := s.Counts(context.Background()); err != nil {
		t.Fatalf("Counts on a fresh database: %v", err)
	}
	_ = s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopening an existing database: %v", err)
	}
	defer func() { _ = s2.Close() }()
	want := strconv.Itoa(schemaVersion)
	if v, err := s2.GetMeta(context.Background(), "schema_version"); err != nil || v != want {
		t.Fatalf("schema_version = %q (want %q), err %v", v, want, err)
	}
}

// An existing database must be migrated forward without losing anything.
// Ownership is the one thing here that cannot be recomputed from a fresh
// observation: if a migration dropped it, every process belonging to a session
// that has already finished would silently become unattributed.
func TestMigrationPreservesRecordedOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ghostgc.db")
	ctx := context.Background()

	// Build a database at schema version 1 only.
	v1 := &Store{}
	db, err := openRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	v1.db, v1.path = db, path
	if err := v1.applyMigration(ctx, migrations[0]); err != nil {
		t.Fatal(err)
	}
	if err := v1.WithTx(ctx, func(tx *Tx) error {
		if _, err := tx.tx.ExecContext(ctx, `
			INSERT INTO session_processes (session_id, proc_uid, agent_id, relation, confidence, evidence, original_ppid, first_seen_ns, last_seen_ns)
			VALUES ('sess-1', '100:1', 'codex', 'root', 0.97, '[]', 42, 1, 2)`); err != nil {
			return err
		}
		_, err := tx.tx.ExecContext(ctx, `
			INSERT INTO sessions (session_id, agent_id, root_proc_uid, root_pid, state, confidence, started_ns, last_seen_ns)
			VALUES ('sess-1', 'codex', '100:1', 100, 'active', 0.97, 1, 2)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = v1.Close()

	// Reopening runs the outstanding migrations.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("migrating an existing database: %v", err)
	}
	defer func() { _ = s.Close() }()

	if v, _ := s.GetMeta(ctx, "schema_version"); v != strconv.Itoa(schemaVersion) {
		t.Fatalf("schema_version after migration = %q", v)
	}
	rows, err := s.SessionProcesses(ctx, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d ownership rows after migration, want the one that was there", len(rows))
	}
	if rows[0].Relation != "root" || rows[0].Confidence != 0.97 || rows[0].OriginalPPID != 42 {
		t.Fatalf("ownership was altered by the migration: %+v", rows[0])
	}
	// A column added by a migration must take its default on existing rows,
	// not be presented as though it had been observed.
	if rows[0].OriginalParentObserved {
		t.Fatal("a row written before the column existed must not claim its original parent was observed")
	}
	sess, err := s.GetSession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("the session did not survive the migration: %v", err)
	}
	if sess.NativeSessionID != "" {
		t.Fatalf("native_session_id = %q, want the empty default", sess.NativeSessionID)
	}
}

func TestCacheSchemaMigratesFromShippedWorktreeV9(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ghostgc.db")
	ctx := context.Background()
	v9 := &Store{}
	db, err := openRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	v9.db, v9.path = db, path
	for _, migration := range migrations {
		if migration.version > 9 {
			break
		}
		if err := v9.applyMigration(ctx, migration); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := v9.db.ExecContext(ctx, `INSERT INTO worktrees
		(worktree_id, path, state, first_seen_ns, last_seen_ns, last_activity_ns, daemon_started_ns)
		VALUES ('wt-existing', '/tmp/existing', 'protected', 1, 2, 2, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := v9.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(path)
	if err != nil {
		t.Fatalf("upgrading schema v9: %v", err)
	}
	defer func() { _ = upgraded.Close() }()
	var state string
	if err := upgraded.db.QueryRowContext(ctx,
		`SELECT state FROM worktrees WHERE worktree_id = 'wt-existing'`).Scan(&state); err != nil || state != "protected" {
		t.Fatalf("worktree state after v10 migration = %q, err %v", state, err)
	}
	var cacheRows int
	if err := upgraded.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cache_evaluations`).Scan(&cacheRows); err != nil {
		t.Fatalf("cache schema after v10 migration: %v", err)
	}
	var originalPath string
	var retirementGrace int64
	if err := upgraded.db.QueryRowContext(ctx,
		`SELECT original_path, retirement_grace_until_ns FROM worktrees WHERE worktree_id = 'wt-existing'`).
		Scan(&originalPath, &retirementGrace); err != nil || originalPath != "" || retirementGrace != 0 {
		t.Fatalf("worktree retirement defaults after v11 migration = %q, %d, %v", originalPath, retirementGrace, err)
	}
}

func TestDatabaseFromANewerBuildIsRefused(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.WithTx(ctx, func(tx *Tx) error {
		return tx.SetMeta("schema_version", strconv.Itoa(schemaVersion+1))
	}); err != nil {
		t.Fatal(err)
	}
	err := s.migrate(ctx)
	if err == nil {
		t.Fatal("a database written by a newer build must be refused, not silently downgraded")
	}
	if !strings.Contains(err.Error(), "newer ghostgc") {
		t.Fatalf("the refusal should explain itself, got: %v", err)
	}
}

func TestUnusableDatabaseIsMovedAsideAndRecreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ghostgc.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite database at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	var recovered *ErrRecovered
	if !errors.As(err, &recovered) {
		t.Fatalf("Open returned %v, want ErrRecovered", err)
	}
	defer func() { _ = s.Close() }()

	if _, statErr := os.Stat(recovered.MovedTo); statErr != nil {
		t.Fatalf("the unusable database should have been preserved at %s", recovered.MovedTo)
	}
	if _, err := s.Counts(context.Background()); err != nil {
		t.Fatalf("the recreated database must be usable: %v", err)
	}
}
