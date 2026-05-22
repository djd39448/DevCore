// tasks.go holds task and run state operations for the episodic store. See
// episodic.go for the package overview.

package episodic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Task is a unit of work DevCore tracks. Status is one of pending, active,
// blocked, review, done, or abandoned. ParentID and Track may be empty.
type Task struct {
	ID            string `json:"id"`
	ParentID      string `json:"parent_id"` // parent task, or "" for a root task
	Title         string `json:"title"`
	SpecRef       string `json:"spec_ref"` // path to the task's spec, or ""
	Track         string `json:"track"`    // backend | data | ios, or ""
	Status        string `json:"status"`
	AssignedAgent string `json:"assigned_agent"`
	CreatedAt     string `json:"created_at"` // RFC3339
	UpdatedAt     string `json:"updated_at"` // RFC3339
}

// Run is a single agent invocation against a task.
type Run struct {
	ID        string `json:"id"`
	TaskID    string `json:"task_id"` // the task this run served, or ""
	Agent     string `json:"agent"`
	Model     string `json:"model"`
	Profile   string `json:"profile"`    // api | local
	StartedAt string `json:"started_at"` // RFC3339
	EndedAt   string `json:"ended_at"`   // RFC3339, or "" while in progress
	Status    string `json:"status"`     // ok | error | aborted, or "" while in progress
	Summary   string `json:"summary"`
	TokensIn  int64  `json:"tokens_in"`
	TokensOut int64  `json:"tokens_out"`
}

// taskColumns is the column list shared by every task SELECT, in struct order.
const taskColumns = `id, parent_id, title, spec_ref, track, status, assigned_agent, created_at, updated_at`

// runColumns is the column list shared by every run SELECT, in struct order.
const runColumns = `id, task_id, agent, model, profile, started_at, ended_at, status, summary, tokens_in, tokens_out`

// UpsertTask inserts t, or updates the existing row when a task with the same
// ID already exists. The caller supplies CreatedAt and UpdatedAt.
func (s *Store) UpsertTask(ctx context.Context, t Task) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks(`+taskColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		     parent_id = excluded.parent_id, title = excluded.title,
		     spec_ref = excluded.spec_ref, track = excluded.track,
		     status = excluded.status, assigned_agent = excluded.assigned_agent,
		     updated_at = excluded.updated_at`,
		t.ID, nullString(t.ParentID), t.Title, t.SpecRef, t.Track, t.Status,
		t.AssignedAgent, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upserting task %s: %w", t.ID, err)
	}
	return nil
}

// GetTask loads a single task by ID.
func (s *Store) GetTask(ctx context.Context, id string) (Task, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
	task, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, fmt.Errorf("task %s not found", id)
	}
	if err != nil {
		return Task{}, fmt.Errorf("looking up task %s: %w", id, err)
	}
	return task, nil
}

// ListTasks returns all tasks, newest first. A non-empty status returns only
// tasks in that status.
func (s *Store) ListTasks(ctx context.Context, status string) ([]Task, error) {
	rows, err := s.queryTasks(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning task row: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading tasks: %w", err)
	}
	return tasks, nil
}

// queryTasks runs the task list query, with an optional status filter.
func (s *Store) queryTasks(ctx context.Context, status string) (*sql.Rows, error) {
	if status == "" {
		return s.db.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks ORDER BY created_at DESC`)
	}
	return s.db.QueryContext(ctx,
		`SELECT `+taskColumns+` FROM tasks WHERE status = ? ORDER BY created_at DESC`, status)
}

// scanner is the read side common to *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanTask reads one task row. The column order must match taskColumns.
func scanTask(row scanner) (Task, error) {
	var t Task
	var parent sql.NullString
	err := row.Scan(&t.ID, &parent, &t.Title, &t.SpecRef, &t.Track,
		&t.Status, &t.AssignedAgent, &t.CreatedAt, &t.UpdatedAt)
	t.ParentID = parent.String
	return t, err
}

// RecordRun inserts a run row.
func (s *Store) RecordRun(ctx context.Context, r Run) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runs(`+runColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, nullString(r.TaskID), r.Agent, r.Model, r.Profile, r.StartedAt,
		r.EndedAt, r.Status, r.Summary, r.TokensIn, r.TokensOut)
	if err != nil {
		return fmt.Errorf("recording run %s: %w", r.ID, err)
	}
	return nil
}

// ListRuns returns runs, newest first. A non-empty taskID returns only runs for
// that task.
func (s *Store) ListRuns(ctx context.Context, taskID string) ([]Run, error) {
	rows, err := s.queryRuns(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("listing runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var runs []Run
	for rows.Next() {
		var r Run
		var task sql.NullString
		if err := rows.Scan(&r.ID, &task, &r.Agent, &r.Model, &r.Profile,
			&r.StartedAt, &r.EndedAt, &r.Status, &r.Summary, &r.TokensIn, &r.TokensOut); err != nil {
			return nil, fmt.Errorf("scanning run row: %w", err)
		}
		r.TaskID = task.String
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading runs: %w", err)
	}
	return runs, nil
}

// queryRuns runs the run list query, with an optional task filter.
func (s *Store) queryRuns(ctx context.Context, taskID string) (*sql.Rows, error) {
	if taskID == "" {
		return s.db.QueryContext(ctx, `SELECT `+runColumns+` FROM runs ORDER BY started_at DESC`)
	}
	return s.db.QueryContext(ctx,
		`SELECT `+runColumns+` FROM runs WHERE task_id = ? ORDER BY started_at DESC`, taskID)
}
