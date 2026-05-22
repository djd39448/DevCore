// Tests for the embed package. Every test runs offline against an httptest
// server standing in for Ollama, so the suite is fast and deterministic.
package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mustJSON marshals v to a JSON string or fails the test. It exists so test
// data setup never silently drops a marshal error.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling test data: %v", err)
	}
	return string(b)
}

// fakeOllama returns an httptest server that answers /api/embed with the given
// HTTP status and raw JSON body, and fails the test on any other path.
func fakeOllama(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("request path = %q, want /api/embed", r.URL.Path)
		}
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// okBody builds a valid single-embedding response of length Dimensions.
func okBody(t *testing.T) string {
	t.Helper()
	vec := make([]float32, Dimensions)
	vec[0] = 0.5
	return mustJSON(t, embedResponse{Embeddings: [][]float32{vec}})
}

func TestNewClientSetsFields(t *testing.T) {
	t.Parallel()
	c := NewClient("http://localhost:11434", "nomic-embed-text")
	if c.endpoint != "http://localhost:11434" || c.model != "nomic-embed-text" {
		t.Fatalf("NewClient stored endpoint=%q model=%q", c.endpoint, c.model)
	}
	if c.http == nil || c.http.Timeout != requestTimeout {
		t.Fatalf("NewClient did not apply the request timeout")
	}
}

func TestEmbedSuccess(t *testing.T) {
	t.Parallel()
	srv := fakeOllama(t, http.StatusOK, okBody(t))
	c := NewClient(srv.URL, "nomic-embed-text")

	vec, err := c.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	if len(vec) != Dimensions {
		t.Fatalf("embedding length = %d, want %d", len(vec), Dimensions)
	}
}

func TestEmbedRejectsBadResponses(t *testing.T) {
	t.Parallel()
	shortVec := mustJSON(t, embedResponse{Embeddings: [][]float32{{0.1, 0.2}}})
	twoVecs := mustJSON(t, embedResponse{Embeddings: [][]float32{
		make([]float32, Dimensions), make([]float32, Dimensions),
	}})

	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"http error status", http.StatusInternalServerError, `{}`},
		{"vector of wrong dimension", http.StatusOK, shortVec},
		{"not exactly one embedding", http.StatusOK, twoVecs},
		{"malformed json", http.StatusOK, `{not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := fakeOllama(t, tc.status, tc.body)
			c := NewClient(srv.URL, "nomic-embed-text")

			if _, err := c.Embed(context.Background(), "hello"); err == nil {
				t.Fatalf("Embed accepted a bad response (%s), want an error", tc.name)
			}
		})
	}
}
