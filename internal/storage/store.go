// Package storage persists observations, sessions, ownership and the audit
// trail in SQLite.
//
// Two properties matter more than anything else here. First, a process is
// keyed by "pid:start_time_ns", so a recycled PID can never inherit a previous
// process's history. Second, ownership is durable: once a process has been
// observed to belong to a session, later reparenting cannot erase that record.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store is the SQLite-backed state store.
type Store struct {
	db   *sql.DB
	path string
}

// ErrRecovered reports that an unreadable database was moved aside and a fresh
// one created. Observation history is lost; safety is not, because nothing is
// ever concluded from absent history.
type ErrRecovered struct {
	MovedTo string
	Cause   error
}

func (e *ErrRecovered) Error() string {
	return fmt.Sprintf("storage: existing database was unusable (%v); it was moved to %s and a new one created", e.Cause, e.MovedTo)
}

// Unwrap returns the underlying cause.
func (e *ErrRecovered) Unwrap() error { return e.Cause }

// Open opens or creates the database at path.
//
// A database that cannot be opened or migrated is moved aside and recreated,
// and the caller is told via ErrRecovered. Refusing to start because of a
// corrupt observation history would be the wrong failure mode: the daemon's
// job is to keep observing, and it draws no conclusion from missing history.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("storage: creating state directory: %w", err)
	}

	store, err := open(path)
	if err == nil {
		return store, nil
	}

	if _, statErr := os.Stat(path); statErr != nil {
		return nil, err
	}
	moved := fmt.Sprintf("%s.unusable-%d", path, time.Now().UnixNano())
	if renameErr := os.Rename(path, moved); renameErr != nil {
		return nil, fmt.Errorf("storage: database unusable (%v) and could not be moved aside: %w", err, renameErr)
	}
	for _, sidecar := range []string{path + "-wal", path + "-shm"} {
		_ = os.Remove(sidecar)
	}
	store, retryErr := open(path)
	if retryErr != nil {
		return nil, fmt.Errorf("storage: recreating database after moving %s aside: %w", moved, retryErr)
	}
	return store, &ErrRecovered{MovedTo: moved, Cause: err}
}

// openRaw opens the database without migrating it. It exists so that tests can
// construct a database at an older schema version.
func openRaw(path string) (*sql.DB, error) {
	dsn := path + "?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: opening %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	return db, nil
}

func open(path string) (*Store, error) {
	dsn := path + "?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(1)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: opening %s: %w", path, err)
	}
	// SQLite permits one writer at a time. Serialising every connection keeps
	// the daemon free of "database is locked" retry logic; read volume is a
	// handful of CLI queries, so the cost is not measurable.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	s := &Store{db: db, path: path}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// migrate brings the database up to schemaVersion, applying each outstanding
// migration in its own transaction and recording the version as it completes.
//
// A partially applied sequence is therefore never possible: either a migration
// and its version bump both land, or neither does, and the next start resumes
// from the last version that committed.
func (s *Store) migrate(ctx context.Context) error {
	current, err := s.readSchemaVersion(ctx)
	if err != nil {
		return err
	}
	if current > schemaVersion {
		return fmt.Errorf(
			"storage: database is at schema version %d but this build understands version %d; a newer ghostgc wrote this database and downgrading it would silently drop the columns it added",
			current, schemaVersion)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := s.applyMigration(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) readSchemaVersion(ctx context.Context) (int, error) {
	var version int
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version)
	switch {
	case err == nil:
		return version, nil
	case isMissingTable(err), errors.Is(err, sql.ErrNoRows):
		// No meta table, or no row in it: an empty database.
		return 0, nil
	default:
		return 0, fmt.Errorf("storage: reading schema version: %w", err)
	}
}

func (s *Store) applyMigration(ctx context.Context, m migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: beginning migration to version %d: %w", m.version, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.stmts); err != nil {
		return fmt.Errorf("storage: applying schema version %d: %w", m.version, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO meta(key, value) VALUES('schema_version', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		m.version,
	); err != nil {
		return fmt.Errorf("storage: recording schema version %d: %w", m.version, err)
	}
	return tx.Commit()
}

func isMissingTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// Path returns the database file path.
func (s *Store) Path() string { return s.path }

// SizeBytes returns the on-disk size including the write-ahead log.
func (s *Store) SizeBytes() int64 {
	var total int64
	for _, p := range []string{s.path, s.path + "-wal", s.path + "-shm"} {
		if fi, err := os.Stat(p); err == nil {
			total += fi.Size()
		}
	}
	return total
}
