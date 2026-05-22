// Package embed turns text into vector embeddings by calling a local Ollama
// server. It is the only component of devcore-memory that talks to a model.
//
// Depends on: the Go standard library only (net/http, encoding/json).
// Depended on by: internal/memoryserver, to embed event text on write and
// query text on recall.
// Why it exists: semantic recall needs vectors. Isolating the Ollama call here
// keeps storage and the MCP layer independent of how embeddings are produced,
// so the provider can change without touching anything else.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Dimensions is the fixed length of every embedding vector this package
// produces. nomic-embed-text emits 768-dimensional vectors; the episodic
// store holds vectors at the same fixed width, so the two must agree —
// memoryserver checks this at startup.
const Dimensions = 768

// requestTimeout caps a single embedding call. Embedding is local and fast;
// anything slower than this means Ollama is unhealthy and the caller should
// find out promptly rather than hang.
const requestTimeout = 30 * time.Second

// Client calls a local Ollama server's embedding endpoint.
type Client struct {
	endpoint string // Ollama server base URL, e.g. http://localhost:11434
	model    string // embedding model name, e.g. nomic-embed-text
	http     *http.Client
}

// NewClient builds a Client for the given Ollama endpoint and embedding model.
// endpoint is the server base URL with no trailing path; model is the Ollama
// model name. The returned Client applies a fixed per-request timeout.
func NewClient(endpoint, model string) *Client {
	return &Client{
		endpoint: endpoint,
		model:    model,
		http:     &http.Client{Timeout: requestTimeout},
	}
}

// embedRequest is the JSON body of an Ollama /api/embed call.
type embedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// embedResponse is the JSON returned by Ollama /api/embed. Ollama returns one
// embedding per input; this package always sends exactly one input.
type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed returns the embedding vector for text. The returned slice always has
// length Dimensions; a vector of any other size is reported as an error so a
// model or schema mismatch fails loudly here rather than at the database.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(embedRequest{Model: c.model, Input: text})
	if err != nil {
		return nil, fmt.Errorf("encoding embed request: %w", err)
	}

	url := c.endpoint + "/api/embed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building embed request for %s: %w", url, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"calling Ollama at %s: %w; check that `ollama serve` is running and model %q is pulled",
			url, err, c.model,
		)
	}
	// The body is fully decoded below; a Close error afterwards is not actionable.
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"embedding call returned HTTP %d for model %q; verify the model name and that Ollama is healthy",
			resp.StatusCode, c.model,
		)
	}

	var parsed embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decoding Ollama embed response: %w", err)
	}
	if len(parsed.Embeddings) != 1 {
		return nil, fmt.Errorf(
			"embedding call returned %d embeddings, want exactly 1", len(parsed.Embeddings),
		)
	}

	vec := parsed.Embeddings[0]
	if len(vec) != Dimensions {
		return nil, fmt.Errorf(
			"model %q produced a %d-dimension vector, want %d",
			c.model, len(vec), Dimensions,
		)
	}
	return vec, nil
}
