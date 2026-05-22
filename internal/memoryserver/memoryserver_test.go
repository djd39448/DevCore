// Tests for the memoryserver package. They are white-box (package
// memoryserver) so they can call the unexported tool handlers directly,
// exercising each tool without standing up the MCP stdio transport. Each test
// uses temp stores and a fake Ollama, so the suite is isolated and offline.
package memoryserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/djd39448/DevCore/internal/canonical"
	"github.com/djd39448/DevCore/internal/embed"
	"github.com/djd39448/DevCore/internal/episodic"
)

// fakeOllama returns an httptest server whose embedding endpoint always answers
// with a valid zero vector of width embed.Dimensions.
func fakeOllama(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := map[string][][]float32{"embeddings": {make([]float32, embed.Dimensions)}}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("encoding fake embedding: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newTestServer builds a Server backed by temp stores and the fake Ollama.
func newTestServer(t *testing.T) *Server {
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
	srv, err := New(ep, can, embed.NewClient(fakeOllama(t).URL, "nomic-embed-text"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv
}

func TestNewRejectsNilSubsystems(t *testing.T) {
	t.Parallel()
	if _, err := New(nil, nil, nil); err == nil {
		t.Fatal("New accepted nil subsystems, want an error")
	}
}

func TestLogThenRecall(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	ctx := context.Background()

	_, logOut, err := s.handleLog(ctx, nil, LogInput{
		Type: "decision", Agent: "architect",
		Summary: "chose modernc.org/sqlite for the episodic store",
	})
	if err != nil {
		t.Fatalf("handleLog: %v", err)
	}
	if logOut.EventID == 0 {
		t.Fatal("handleLog returned event ID 0")
	}

	_, recallOut, err := s.handleRecall(ctx, nil, RecallInput{Query: "sqlite store"})
	if err != nil {
		t.Fatalf("handleRecall: %v", err)
	}
	if len(recallOut.Hits) == 0 {
		t.Fatal("handleRecall found nothing for a query that should match")
	}
}

func TestLogRejectsMissingFields(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	if _, _, err := s.handleLog(context.Background(), nil, LogInput{Agent: "x"}); err == nil {
		t.Fatal("handleLog accepted an event with no type or summary")
	}
}

func TestCanonicalWriteThenRead(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	ctx := context.Background()

	const body = "# Contract\n\nThe API surface.\n"
	if _, _, err := s.handleCanonicalWrite(ctx, nil, CanonicalWriteInput{
		Path: "contract/api.md", Content: body,
	}); err != nil {
		t.Fatalf("handleCanonicalWrite: %v", err)
	}

	_, readOut, err := s.handleCanonicalRead(ctx, nil, CanonicalReadInput{Path: "contract/api.md"})
	if err != nil {
		t.Fatalf("handleCanonicalRead: %v", err)
	}
	if readOut.Content != body {
		t.Fatalf("read back %q, want %q", readOut.Content, body)
	}

	_, listOut, err := s.handleCanonicalRead(ctx, nil, CanonicalReadInput{})
	if err != nil {
		t.Fatalf("handleCanonicalRead (list): %v", err)
	}
	if len(listOut.Docs) != 1 || listOut.Docs[0] != "contract/api.md" {
		t.Fatalf("list returned %v, want [contract/api.md]", listOut.Docs)
	}
}

func TestTaskLifecycle(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	ctx := context.Background()

	if _, _, err := s.handleTask(ctx, nil, TaskInput{
		Op: "upsert_task", ID: "t1", Title: "port the data layer", Status: "active",
	}); err != nil {
		t.Fatalf("handleTask upsert_task: %v", err)
	}

	_, getOut, err := s.handleTask(ctx, nil, TaskInput{Op: "get_task", ID: "t1"})
	if err != nil {
		t.Fatalf("handleTask get_task: %v", err)
	}
	if getOut.Task == nil || getOut.Task.Status != "active" {
		t.Fatalf("get_task returned %+v, want task t1 with status active", getOut.Task)
	}

	_, listOut, err := s.handleTask(ctx, nil, TaskInput{Op: "list_tasks"})
	if err != nil {
		t.Fatalf("handleTask list_tasks: %v", err)
	}
	if len(listOut.Tasks) != 1 {
		t.Fatalf("list_tasks returned %d tasks, want 1", len(listOut.Tasks))
	}
}

func TestTaskRejectsUnknownOp(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	if _, _, err := s.handleTask(context.Background(), nil, TaskInput{Op: "bogus"}); err == nil {
		t.Fatal("handleTask accepted an unknown op, want an error")
	}
}

func TestStatsCountsWrites(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	ctx := context.Background()

	if _, _, err := s.handleLog(ctx, nil, LogInput{Type: "note", Agent: "a", Summary: "an event"}); err != nil {
		t.Fatalf("handleLog: %v", err)
	}
	_, statsOut, err := s.handleStats(ctx, nil, StatsInput{})
	if err != nil {
		t.Fatalf("handleStats: %v", err)
	}
	if statsOut.Events != 1 {
		t.Fatalf("stats reported %d events, want 1", statsOut.Events)
	}
}
