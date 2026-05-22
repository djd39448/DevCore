// schema.go defines the episodic database schema and its version. See
// episodic.go for the package overview.

package episodic

// schemaVersion is the current schema version. It is stored in SQLite's
// user_version pragma; init() applies schemaSQL when an opened database is
// older than this.
const schemaVersion = 1

// schemaSQL is the complete schema at schemaVersion. Every statement is
// idempotent, so applying it to a fresh database is safe.
//
// The store is three plain tables — no virtual tables. Keyword and semantic
// recall are computed in Go over the events table (see events.go); this keeps
// the store dependency-light and simple, which is the right trade at DevCore's
// project scale. events.embedding holds VectorDim float32 values packed
// little-endian (VectorDim*4 bytes).
const schemaSQL = `
CREATE TABLE IF NOT EXISTS tasks (
    id             TEXT PRIMARY KEY,
    parent_id      TEXT REFERENCES tasks(id),
    title          TEXT NOT NULL,
    spec_ref       TEXT NOT NULL DEFAULT '',
    track          TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'pending',
    assigned_agent TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
    id         TEXT PRIMARY KEY,
    task_id    TEXT REFERENCES tasks(id),
    agent      TEXT NOT NULL,
    model      TEXT NOT NULL DEFAULT '',
    profile    TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    ended_at   TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT '',
    summary    TEXT NOT NULL DEFAULT '',
    tokens_in  INTEGER NOT NULL DEFAULT 0,
    tokens_out INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS events (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    ts        TEXT NOT NULL,
    agent     TEXT NOT NULL DEFAULT '',
    task_id   TEXT NOT NULL DEFAULT '',
    run_id    TEXT NOT NULL DEFAULT '',
    type      TEXT NOT NULL,
    summary   TEXT NOT NULL,
    detail    TEXT NOT NULL DEFAULT '',
    refs      TEXT NOT NULL DEFAULT '',
    embedding BLOB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_task ON events(task_id);
CREATE INDEX IF NOT EXISTS idx_runs_task ON runs(task_id);
`
