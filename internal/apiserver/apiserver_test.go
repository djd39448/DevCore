// Tests for the apiserver package. Each test stands up a real Handler over
// fresh temp stores, so the suite exercises the full request → JSON path
// without standing up a TCP listener.
package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djd39448/DevCore/internal/apiserver"
	"github.com/djd39448/DevCore/internal/canonical"
	"github.com/djd39448/DevCore/internal/episodic"
)

// fixture is the assembled API server plus the underlying stores, so tests
// can seed data and then call the Handler.
type fixture struct {
	server    *apiserver.Server
	episodic  *episodic.Store
	canonical *canonical.Store
}

// newFixture opens temp stores and wires them through a Server.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	ep, err := episodic.Open(filepath.Join(t.TempDir(), "episodic.sqlite"))
	if err != nil {
		t.Fatalf("episodic.Open: %v", err)
	}
	t.Cleanup(func() { _ = ep.Close() })

	can, err := canonical.Open(t.TempDir())
	if err != nil {
		t.Fatalf("canonical.Open: %v", err)
	}

	srv, err := apiserver.New(ep, can)
	if err != nil {
		t.Fatalf("apiserver.New: %v", err)
	}
	return &fixture{server: srv, episodic: ep, canonical: can}
}

// vector builds a constant VectorDim-wide embedding for seeding events.
func vector(value float32) []float32 {
	v := make([]float32, episodic.VectorDim)
	for i := range v {
		v[i] = value
	}
	return v
}

// get hits the Handler with a GET and decodes the JSON body into out. It
// fails the test if the status code differs from wantStatus.
func get(t *testing.T, fx *fixture, path string, wantStatus int, out any) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	fx.server.Handler().ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("GET %s: status %d, want %d (body=%q)", path, rec.Code, wantStatus, rec.Body.String())
	}
	if out == nil {
		return
	}
	if err := json.NewDecoder(rec.Body).Decode(out); err != nil {
		t.Fatalf("GET %s: decoding response: %v", path, err)
	}
}

func TestNewRejectsNilStores(t *testing.T) {
	t.Parallel()
	if _, err := apiserver.New(nil, nil); err == nil {
		t.Fatal("New accepted nil stores, want an error")
	}
}

func TestStatsReportsCounts(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()

	if _, err := fx.episodic.LogEvent(ctx,
		episodic.Event{TS: "t", Type: "note", Summary: "an event"}, vector(0.1)); err != nil {
		t.Fatalf("LogEvent: %v", err)
	}

	var got apiserver.StatsResponse
	get(t, fx, "/api/stats", http.StatusOK, &got)
	if got.Events != 1 {
		t.Fatalf("stats events = %d, want 1", got.Events)
	}
	if got.SizeBytes <= 0 {
		t.Fatalf("stats size_bytes = %d, want a positive size", got.SizeBytes)
	}
}

func TestTasksReturnsSeededTasks(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()

	if err := fx.episodic.UpsertTask(ctx, episodic.Task{
		ID: "t1", Title: "port the data layer", Status: "active",
		CreatedAt: "1", UpdatedAt: "1",
	}); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}

	var tasks []episodic.Task
	get(t, fx, "/api/tasks", http.StatusOK, &tasks)
	if len(tasks) != 1 || tasks[0].ID != "t1" {
		t.Fatalf("/api/tasks = %+v, want one task t1", tasks)
	}
}

func TestTasksWithStatusFilter(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()

	for _, task := range []episodic.Task{
		{ID: "a", Title: "a", Status: "pending", CreatedAt: "1", UpdatedAt: "1"},
		{ID: "b", Title: "b", Status: "active", CreatedAt: "2", UpdatedAt: "2"},
	} {
		if err := fx.episodic.UpsertTask(ctx, task); err != nil {
			t.Fatalf("UpsertTask: %v", err)
		}
	}

	var tasks []episodic.Task
	get(t, fx, "/api/tasks?status=active", http.StatusOK, &tasks)
	if len(tasks) != 1 || tasks[0].ID != "b" {
		t.Fatalf("/api/tasks?status=active = %+v, want only task b", tasks)
	}
}

func TestRunsReturnsSeededRuns(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()

	if err := fx.episodic.RecordRun(ctx, episodic.Run{
		ID: "r1", Agent: "verifier", StartedAt: "1",
	}); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	var runs []episodic.Run
	get(t, fx, "/api/runs", http.StatusOK, &runs)
	if len(runs) != 1 || runs[0].ID != "r1" {
		t.Fatalf("/api/runs = %+v, want one run r1", runs)
	}
}

func TestEventsReturnsNewestFirst(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()

	for _, summary := range []string{"first", "second", "third"} {
		if _, err := fx.episodic.LogEvent(ctx,
			episodic.Event{TS: "t", Type: "note", Summary: summary}, vector(0.2)); err != nil {
			t.Fatalf("LogEvent: %v", err)
		}
	}

	var events []episodic.Event
	get(t, fx, "/api/events", http.StatusOK, &events)
	if len(events) != 3 {
		t.Fatalf("/api/events len = %d, want 3", len(events))
	}
	if events[0].Summary != "third" || events[2].Summary != "first" {
		t.Fatalf("event order = %q…%q, want third…first", events[0].Summary, events[2].Summary)
	}
}

func TestEventsHonorsLimit(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := fx.episodic.LogEvent(ctx,
			episodic.Event{TS: "t", Type: "note", Summary: "e"}, vector(0.1)); err != nil {
			t.Fatalf("LogEvent: %v", err)
		}
	}

	var events []episodic.Event
	get(t, fx, "/api/events?limit=2", http.StatusOK, &events)
	if len(events) != 2 {
		t.Fatalf("/api/events?limit=2 len = %d, want 2", len(events))
	}
}

func TestEventsRejectsBadLimit(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	for _, q := range []string{"?limit=abc", "?limit=-3", "?limit=0"} {
		get(t, fx, "/api/events"+q, http.StatusBadRequest, nil)
	}
}

func TestCanonicalListAndRead(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)

	const body = "# Contract\n\nAPI surface.\n"
	if err := fx.canonical.Write("contract/api.md", body); err != nil {
		t.Fatalf("canonical.Write: %v", err)
	}

	var list apiserver.CanonicalListResponse
	get(t, fx, "/api/canonical", http.StatusOK, &list)
	if len(list.Docs) != 1 || list.Docs[0] != "contract/api.md" {
		t.Fatalf("/api/canonical = %+v, want [contract/api.md]", list.Docs)
	}

	var read apiserver.CanonicalReadResponse
	get(t, fx, "/api/canonical?path=contract/api.md", http.StatusOK, &read)
	if read.Path != "contract/api.md" || !strings.Contains(read.Content, "API surface") {
		t.Fatalf("/api/canonical?path = %+v, want path+body for contract/api.md", read)
	}
}

func TestCanonicalRejectsTraversal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	// The canonical store rejects "..", and the API surfaces that as 404 —
	// the input is what's wrong, not the server.
	get(t, fx, "/api/canonical?path=../escape.md", http.StatusNotFound, nil)
}

func TestRejectsNonGet(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/stats", nil)
	fx.server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/stats: status %d, want 405", rec.Code)
	}
	if rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow header = %q, want GET", rec.Header().Get("Allow"))
	}
}

func TestCORSPreflight(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/api/stats", nil)
	fx.server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS /api/stats: status %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("CORS origin = %q, want *", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}
