// Package apiserver is the read-only HTTP API that DevCore's desktop shell
// queries to render real DevCore state in place of mocked data.
//
// Depends on: internal/episodic (Tier-2 read paths) and internal/canonical
// (Tier-1 read paths). No write endpoints — every handler is read-only so the
// surface is small and a misbehaving client cannot corrupt memory.
// Depended on by: cmd/devcore-api, which mounts the handler on a local TCP
// port and runs it as a subprocess of the DevCore desktop app.
// Why it exists: the desktop shell is a WKWebView hosting an HTML/JSX
// prototype served from devcore://. To show live data it needs a real HTTP
// origin to fetch from; this package is that origin.
package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/djd39448/DevCore/internal/canonical"
	"github.com/djd39448/DevCore/internal/episodic"
)

// defaultEventLimit is the page size used when an /api/events caller does not
// pass ?limit=. 200 keeps a typical render snappy without truncating most
// debugging walks of the log.
const defaultEventLimit = 200

// maxEventLimit caps how many events one request may pull. The episodic log
// can grow large; a hard cap protects the API from runaway queries.
const maxEventLimit = 2000

// Server holds the read-only stores the API serves from. It is built once at
// process start and reused across requests; the underlying stores are safe for
// concurrent reads.
type Server struct {
	episodic  *episodic.Store
	canonical *canonical.Store
}

// New returns a Server that serves from the given stores. Both must be non-nil
// — without either, half the API surface is missing.
func New(ep *episodic.Store, can *canonical.Store) (*Server, error) {
	if ep == nil || can == nil {
		return nil, errors.New("apiserver.New: episodic and canonical stores are both required")
	}
	return &Server{episodic: ep, canonical: can}, nil
}

// Handler returns an http.Handler that serves the read-only endpoints. The
// endpoints are documented in the route table below.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/tasks", s.handleTasks)
	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/canonical", s.handleCanonical)
	return withCORS(mux)
}

// withCORS allows the desktop webview, whose origin is devcore://, to fetch
// from http://127.0.0.1:<port>. The API is read-only and bound to localhost,
// so a permissive origin policy is safe here.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// StatsResponse is the JSON shape of GET /api/stats.
type StatsResponse struct {
	Events    int64 `json:"events"`
	Tasks     int64 `json:"tasks"`
	Runs      int64 `json:"runs"`
	SizeBytes int64 `json:"size_bytes"`
}

// CanonicalListResponse is the JSON shape of GET /api/canonical (no ?path).
type CanonicalListResponse struct {
	Docs []string `json:"docs"`
}

// CanonicalReadResponse is the JSON shape of GET /api/canonical?path=foo.md.
type CanonicalReadResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// handleStats answers GET /api/stats with row counts and the database file
// size. It is the cheapest endpoint and what the desktop sidebar polls.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	stats, err := s.episodic.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, StatsResponse{
		Events:    stats.Events,
		Tasks:     stats.Tasks,
		Runs:      stats.Runs,
		SizeBytes: stats.SizeBytes,
	})
}

// handleTasks answers GET /api/tasks with every task, newest first. An
// optional ?status= filter narrows the result to one status; an unknown
// status simply yields an empty list (the store does the filtering).
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	tasks, err := s.episodic.ListTasks(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if tasks == nil {
		tasks = []episodic.Task{}
	}
	writeJSON(w, tasks)
}

// handleRuns answers GET /api/runs with every run, newest first. An optional
// ?task_id= filter narrows the result to one task's runs.
func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	runs, err := s.episodic.ListRuns(r.Context(), r.URL.Query().Get("task_id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if runs == nil {
		runs = []episodic.Run{}
	}
	writeJSON(w, runs)
}

// handleEvents answers GET /api/events with the most recent events, newest
// first. ?limit= defaults to defaultEventLimit and is capped at maxEventLimit.
// A non-numeric limit is rejected with 400 — silently ignoring it would hide
// caller bugs.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	events, err := s.episodic.ListEvents(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if events == nil {
		events = []episodic.Event{}
	}
	writeJSON(w, events)
}

// handleCanonical answers GET /api/canonical. Without ?path it returns the
// list of every .md file in the store; with ?path=foo.md it returns that
// file's content. The store's resolve step rejects any path that escapes the
// memory root, so traversal is not reachable from here.
func (s *Server) handleCanonical(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		docs, err := s.canonical.List()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if docs == nil {
			docs = []string{}
		}
		writeJSON(w, CanonicalListResponse{Docs: docs})
		return
	}

	content, err := s.canonical.Read(path)
	if err != nil {
		// Read errors here are most often "no such file" or "escapes the
		// memory root"; in either case the safer answer to a caller is 404
		// than 500, because the input is what's wrong.
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, CanonicalReadResponse{Path: path, Content: content})
}

// parseLimit parses ?limit= into a bounded positive integer. An empty value
// becomes defaultEventLimit; a value above maxEventLimit is clamped down.
func parseLimit(raw string) (int, error) {
	if raw == "" {
		return defaultEventLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("limit %q is not an integer", raw)
	}
	if limit <= 0 {
		return 0, fmt.Errorf("limit must be positive, got %d", limit)
	}
	if limit > maxEventLimit {
		limit = maxEventLimit
	}
	return limit, nil
}

// methodAllowed enforces a GET-only surface. It writes the response and
// returns false when the method is not allowed.
func methodAllowed(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method == want {
		return true
	}
	w.Header().Set("Allow", want)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

// writeJSON encodes v as JSON with the standard content type. Encoding errors
// are logged via http.Error after the headers are sent — the response is
// already partially written by then, so the client sees a broken body, which
// is the honest signal.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The headers are already on the wire — best we can do is log via the
		// stream and let the client see a truncated body.
		http.Error(w, fmt.Sprintf("encoding response: %v", err), http.StatusInternalServerError)
	}
}

// writeError sends an error as JSON so the JS client can render it directly.
// The status code carries the category; the body carries the message.
func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// shutdownGrace is how long Serve waits for inflight requests to finish after
// ctx is cancelled. Short — desktop polling requests are sub-second.
const shutdownGrace = 2 * time.Second

// Serve runs an HTTP server on ln serving s.Handler() and blocks until ctx is
// cancelled. On cancellation it performs a short graceful shutdown. The
// caller owns ln, so creating it (and picking the port — including port 0)
// stays in cmd/devcore-api where lifecycle concerns belong.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler: s.Handler(),
		// Modest, fixed timeouts — a local read-only API has no reason to hold
		// idle sockets open for minutes.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		// http.ErrServerClosed is the success signal after Shutdown; surface
		// every other error.
		err := srv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			errCh <- nil
			return
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// ctx is already cancelled — that's why we're shutting down. Use a
		// fresh background context with its own deadline so Shutdown has time
		// to finish inflight requests rather than aborting them instantly.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil { //nolint:contextcheck // shutdown deadline is intentionally independent of the cancelled parent ctx
			return fmt.Errorf("shutting down api server: %w", err)
		}
		return nil
	}
}
