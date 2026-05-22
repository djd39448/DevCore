// Tests for the episodic package. Each test opens a fresh SQLite database in a
// temp directory, so the suite is isolated, offline, and fast. Recall is
// computed in Go over the events table — no extension, no WASM — so the tests
// exercise the real search path with nothing mocked.
package episodic_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djd39448/DevCore/internal/episodic"
)

// vector returns a constant VectorDim-wide embedding. Constant vectors give
// predictable L2 distances, which makes semantic-ranking assertions reliable.
func vector(value float32) []float32 {
	v := make([]float32, episodic.VectorDim)
	for i := range v {
		v[i] = value
	}
	return v
}

// openTestStore opens a fresh episodic store in a temp directory and registers
// its cleanup.
func openTestStore(t *testing.T) *episodic.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "episodic.sqlite")
	store, err := episodic.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return store
}

func TestOpenIsIdempotent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "episodic.sqlite")
	for attempt := 1; attempt <= 2; attempt++ {
		store, err := episodic.Open(path)
		if err != nil {
			t.Fatalf("Open attempt %d: %v", attempt, err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("Close attempt %d: %v", attempt, err)
		}
	}
}

func TestLogEventRejectsWrongDimension(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	_, err := store.LogEvent(
		context.Background(),
		episodic.Event{TS: "t", Type: "note", Summary: "x"},
		[]float32{0.1, 0.2},
	)
	if err == nil {
		t.Fatal("LogEvent accepted a wrong-dimension embedding, want an error")
	}
}

func TestRecallFindsByKeyword(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	ctx := context.Background()

	logOrFail(t, store, episodic.Event{
		TS: "2026-05-22T00:00:00Z", Agent: "analyst", Type: "decision",
		Summary: "chose claude-code-router as the proxy",
	}, vector(0.1))
	logOrFail(t, store, episodic.Event{
		TS: "2026-05-22T00:01:00Z", Agent: "architect", Type: "decision",
		Summary: "designed the Supabase schema",
	}, vector(0.2))

	hits, err := store.RecallEvents(ctx, "proxy", vector(0.5), 5)
	if err != nil {
		t.Fatalf("RecallEvents: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("RecallEvents returned no hits for a keyword that should match")
	}
	if !strings.Contains(hits[0].Event.Summary, "proxy") {
		t.Fatalf("top hit = %q, want the event mentioning the proxy", hits[0].Event.Summary)
	}
}

func TestRecallFindsBySemanticSimilarity(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	ctx := context.Background()

	logOrFail(t, store, episodic.Event{TS: "t", Type: "note", Summary: "far event"}, vector(0.1))
	logOrFail(t, store, episodic.Event{TS: "t", Type: "note", Summary: "near event"}, vector(0.9))

	// The query text matches neither summary, so only the vector can rank them.
	hits, err := store.RecallEvents(ctx, "", vector(0.9), 5)
	if err != nil {
		t.Fatalf("RecallEvents: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	if hits[0].Event.Summary != "near event" {
		t.Fatalf("top hit = %q, want %q (the vector-nearest event)", hits[0].Event.Summary, "near event")
	}
}

func TestRecallRejectsBadInput(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	ctx := context.Background()

	if _, err := store.RecallEvents(ctx, "x", []float32{0.1}, 5); err == nil {
		t.Error("RecallEvents accepted a wrong-dimension query embedding, want an error")
	}
	if _, err := store.RecallEvents(ctx, "x", vector(0.5), 0); err == nil {
		t.Error("RecallEvents accepted a non-positive limit, want an error")
	}
}

func TestTaskUpsertUpdatesExistingRow(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	ctx := context.Background()

	task := episodic.Task{
		ID: "task-1", Title: "port the data layer", Track: "data",
		Status: "pending", CreatedAt: "2026-05-22T00:00:00Z", UpdatedAt: "2026-05-22T00:00:00Z",
	}
	if err := store.UpsertTask(ctx, task); err != nil {
		t.Fatalf("UpsertTask insert: %v", err)
	}

	task.Status = "active"
	task.UpdatedAt = "2026-05-22T01:00:00Z"
	if err := store.UpsertTask(ctx, task); err != nil {
		t.Fatalf("UpsertTask update: %v", err)
	}

	got, err := store.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != "active" || got.UpdatedAt != "2026-05-22T01:00:00Z" {
		t.Fatalf("after update got status=%q updated_at=%q, want active / 01:00", got.Status, got.UpdatedAt)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	if _, err := store.GetTask(context.Background(), "missing"); err == nil {
		t.Fatal("GetTask returned no error for a missing task, want one")
	}
}

func TestListTasksFiltersByStatus(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	ctx := context.Background()

	upsertOrFail(t, store, episodic.Task{ID: "a", Title: "a", Status: "pending", CreatedAt: "1", UpdatedAt: "1"})
	upsertOrFail(t, store, episodic.Task{ID: "b", Title: "b", Status: "active", CreatedAt: "2", UpdatedAt: "2"})

	all, err := store.ListTasks(ctx, "")
	if err != nil {
		t.Fatalf("ListTasks all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListTasks(\"\") returned %d tasks, want 2", len(all))
	}

	active, err := store.ListTasks(ctx, "active")
	if err != nil {
		t.Fatalf("ListTasks active: %v", err)
	}
	if len(active) != 1 || active[0].ID != "b" {
		t.Fatalf("ListTasks(active) = %+v, want only task b", active)
	}
}

func TestRunRecordAndList(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	ctx := context.Background()

	upsertOrFail(t, store, episodic.Task{ID: "t1", Title: "t1", Status: "active", CreatedAt: "1", UpdatedAt: "1"})
	run := episodic.Run{
		ID: "run-1", TaskID: "t1", Agent: "builder", Model: "claude-sonnet-4-6",
		Profile: "api", StartedAt: "2026-05-22T00:00:00Z", Status: "ok",
		TokensIn: 1200, TokensOut: 800,
	}
	if err := store.RecordRun(ctx, run); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	runs, err := store.ListRuns(ctx, "t1")
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "run-1" || runs[0].TokensIn != 1200 {
		t.Fatalf("ListRuns(t1) = %+v, want one run-1 with 1200 tokens in", runs)
	}
}

func TestStatsCountsRows(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	ctx := context.Background()

	logOrFail(t, store, episodic.Event{TS: "t", Type: "note", Summary: "an event"}, vector(0.3))
	upsertOrFail(t, store, episodic.Task{ID: "t1", Title: "t1", Status: "pending", CreatedAt: "1", UpdatedAt: "1"})
	if err := store.RecordRun(ctx, episodic.Run{ID: "r1", TaskID: "t1", Agent: "verifier", StartedAt: "1"}); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Events != 1 || stats.Tasks != 1 || stats.Runs != 1 {
		t.Fatalf("Stats = %+v, want 1 event / 1 task / 1 run", stats)
	}
	if stats.SizeBytes <= 0 {
		t.Fatalf("Stats.SizeBytes = %d, want a positive size", stats.SizeBytes)
	}
}

func TestRecallOnEmptyStore(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)

	hits, err := store.RecallEvents(context.Background(), "anything", vector(0.5), 5)
	if err != nil {
		t.Fatalf("RecallEvents on an empty store returned an error: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("RecallEvents on an empty store returned %d hits, want 0", len(hits))
	}
}

func TestRecallLimitExceedsEventCount(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	logOrFail(t, store, episodic.Event{TS: "t", Type: "note", Summary: "only event"}, vector(0.5))

	hits, err := store.RecallEvents(context.Background(), "only", vector(0.5), 50)
	if err != nil {
		t.Fatalf("RecallEvents: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits with limit 50 over a 1-event store, want 1", len(hits))
	}
}

// logOrFail logs an event or fails the test.
func logOrFail(t *testing.T, store *episodic.Store, e episodic.Event, embedding []float32) {
	t.Helper()
	if _, err := store.LogEvent(context.Background(), e, embedding); err != nil {
		t.Fatalf("LogEvent(%q): %v", e.Summary, err)
	}
}

// upsertOrFail upserts a task or fails the test.
func upsertOrFail(t *testing.T, store *episodic.Store, task episodic.Task) {
	t.Helper()
	if err := store.UpsertTask(context.Background(), task); err != nil {
		t.Fatalf("UpsertTask(%q): %v", task.ID, err)
	}
}
