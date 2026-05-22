// Package memoryserver wires the embedding client and the two memory stores
// into an MCP server, exposing devcore-memory's six tools over stdio.
//
// Depends on: internal/embed, internal/episodic, internal/canonical, and the
// official MCP Go SDK.
// Depended on by: cmd/devcore-memory, the server's process entrypoint.
// Why it exists: this is the boundary between DevCore's memory and the agents.
// Agents reach memory only through these six MCP tools — never the stores
// directly — so access stays uniform, validated, and observable.
package memoryserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/djd39448/DevCore/internal/canonical"
	"github.com/djd39448/DevCore/internal/embed"
	"github.com/djd39448/DevCore/internal/episodic"
)

// version is reported to MCP clients in the server handshake.
const version = "0.1.0"

// Server holds the memory subsystems the six MCP tools operate on.
type Server struct {
	episodic  *episodic.Store
	canonical *canonical.Store
	embedder  *embed.Client
}

// New builds a Server from its three subsystems. It fails if any is nil, or if
// the embedding width and the episodic store's vector width disagree — a
// mismatch that would otherwise surface later as a confusing storage error.
func New(ep *episodic.Store, can *canonical.Store, emb *embed.Client) (*Server, error) {
	if ep == nil || can == nil || emb == nil {
		return nil, fmt.Errorf("memoryserver needs a non-nil episodic store, canonical store, and embedder")
	}
	if embed.Dimensions != episodic.VectorDim {
		return nil, fmt.Errorf(
			"embedding width %d does not match the episodic store's vector width %d",
			embed.Dimensions, episodic.VectorDim,
		)
	}
	return &Server{episodic: ep, canonical: can, embedder: emb}, nil
}

// Run registers the six memory tools and serves them over stdio until ctx is
// cancelled or the MCP client disconnects.
func (s *Server) Run(ctx context.Context) error {
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "devcore-memory", Version: version}, nil)
	s.register(mcpServer)
	if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("serving devcore-memory over stdio: %w", err)
	}
	return nil
}

// register attaches all six memory tools to the MCP server.
func (s *Server) register(m *mcp.Server) {
	mcp.AddTool(m, &mcp.Tool{
		Name:        "memory_log",
		Description: "Append an event to the episodic log — a record of what an agent did.",
	}, s.handleLog)
	mcp.AddTool(m, &mcp.Tool{
		Name:        "memory_recall",
		Description: "Recall past events by a fused keyword and semantic search of the log.",
	}, s.handleRecall)
	mcp.AddTool(m, &mcp.Tool{
		Name:        "memory_canonical_read",
		Description: "Read a Tier-1 canonical doc; with no path, list every canonical doc.",
	}, s.handleCanonicalRead)
	mcp.AddTool(m, &mcp.Tool{
		Name:        "memory_canonical_write",
		Description: "Write a Tier-1 canonical doc, stamping its last_updated frontmatter.",
	}, s.handleCanonicalWrite)
	mcp.AddTool(m, &mcp.Tool{
		Name:        "memory_task",
		Description: "Create, update, or query task and run state (op selects the action).",
	}, s.handleTask)
	mcp.AddTool(m, &mcp.Tool{
		Name:        "memory_stats",
		Description: "Report row counts and the on-disk size of the episodic store.",
	}, s.handleStats)
}
