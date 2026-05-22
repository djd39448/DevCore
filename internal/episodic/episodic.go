// Package episodic is devcore-memory's Tier-2 store: an append-only log of what
// agents did, plus task and run state, held in a single SQLite file. It offers
// both keyword and semantic recall over the log.
//
// Depends on: modernc.org/sqlite, a pure-Go SQLite driver (no CGO, no WASM).
// Recall is computed in Go (see events.go), so the store needs no extensions.
// Depended on by: internal/memoryserver, which exposes these operations as MCP
// tools.
// Why it exists: agents need to record and recall past behaviour. Holding all
// of it in one SQLite file is the portability story — the episodic memory
// moves between machines as a single file.
package episodic

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" database/sql driver
)

// VectorDim is the embedding width every stored vector must have. It must equal
// embed.Dimensions; memoryserver verifies this at startup.
const VectorDim = 768

// Store is a handle to the episodic SQLite database.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens (creating it if absent) the episodic database at path and brings
// its schema up to date. The caller must Close the returned Store.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening episodic database %s: %w", path, err)
	}
	// SQLite is a single-file embedded database; capping the pool at one
	// connection avoids write-lock contention and keeps pragmas stable.
	db.SetMaxOpenConns(1)

	s := &Store{db: db, path: path}
	if err := s.init(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialising episodic database %s: %w", path, err)
	}
	return s, nil
}

// init enables foreign keys and applies the schema when the database is older
// than schemaVersion. The applied version is tracked in user_version.
func (s *Store) init() error {
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		return fmt.Errorf("enabling foreign keys: %w", err)
	}

	var version int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version;`).Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if version >= schemaVersion {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("applying schema v%d: %w", schemaVersion, err)
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d;", schemaVersion)); err != nil {
		return fmt.Errorf("recording schema version %d: %w", schemaVersion, err)
	}
	return nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("closing episodic database %s: %w", s.path, err)
	}
	return nil
}

// Stats summarises the contents of the episodic store.
type Stats struct {
	Events    int64 // rows in the events log
	Tasks     int64 // rows in the tasks table
	Runs      int64 // rows in the runs table
	SizeBytes int64 // on-disk size of the database file
}

// Stats returns row counts and the on-disk size of the database.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var out Stats
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM events;`).Scan(&out.Events); err != nil {
		return Stats{}, fmt.Errorf("counting events: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM tasks;`).Scan(&out.Tasks); err != nil {
		return Stats{}, fmt.Errorf("counting tasks: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM runs;`).Scan(&out.Runs); err != nil {
		return Stats{}, fmt.Errorf("counting runs: %w", err)
	}
	if info, err := os.Stat(s.path); err == nil {
		out.SizeBytes = info.Size()
	}
	return out, nil
}

// nullString maps a Go string to a SQL value, using NULL for the empty string.
// It exists for nullable foreign-key columns, where "" means "no reference".
func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
