// tools.go holds five of the six memory tools: memory_log, memory_recall,
// memory_canonical_read, memory_canonical_write, and memory_stats. The sixth,
// memory_task, lives in tasktool.go. See server.go for the package overview.

package memoryserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/djd39448/DevCore/internal/episodic"
)

// defaultRecallLimit is the number of results memory_recall returns when the
// caller does not specify a limit.
const defaultRecallLimit = 10

// LogInput is the argument schema for the memory_log tool.
type LogInput struct {
	Type    string `json:"type" jsonschema:"event type: decision, action, correction, learning, error, or note"`
	Agent   string `json:"agent" jsonschema:"the agent recording the event"`
	Summary string `json:"summary" jsonschema:"a one-line description of what happened"`
	Detail  string `json:"detail,omitempty" jsonschema:"an optional longer description"`
	TaskID  string `json:"task_id,omitempty" jsonschema:"the task this event belongs to, if any"`
	RunID   string `json:"run_id,omitempty" jsonschema:"the run this event belongs to, if any"`
	Refs    string `json:"refs,omitempty" jsonschema:"an optional JSON array of file or commit references"`
}

// LogOutput is the result of the memory_log tool.
type LogOutput struct {
	EventID int64 `json:"event_id" jsonschema:"the ID assigned to the new event"`
}

// handleLog embeds the event text and appends the event to the episodic log.
func (s *Server) handleLog(
	ctx context.Context, _ *mcp.CallToolRequest, in LogInput,
) (*mcp.CallToolResult, LogOutput, error) {
	if in.Type == "" || in.Summary == "" {
		return nil, LogOutput{}, fmt.Errorf("memory_log requires a non-empty type and summary")
	}
	text := in.Summary
	if in.Detail != "" {
		text += "\n" + in.Detail
	}
	vec, err := s.embedder.Embed(ctx, text)
	if err != nil {
		return nil, LogOutput{}, fmt.Errorf("embedding event text: %w", err)
	}
	id, err := s.episodic.LogEvent(ctx, episodic.Event{
		TS:      time.Now().UTC().Format(time.RFC3339),
		Agent:   in.Agent,
		TaskID:  in.TaskID,
		RunID:   in.RunID,
		Type:    in.Type,
		Summary: in.Summary,
		Detail:  in.Detail,
		Refs:    in.Refs,
	}, vec)
	if err != nil {
		return nil, LogOutput{}, fmt.Errorf("logging event: %w", err)
	}
	return nil, LogOutput{EventID: id}, nil
}

// RecallInput is the argument schema for the memory_recall tool.
type RecallInput struct {
	Query string `json:"query" jsonschema:"the text to search the event log for"`
	Limit int    `json:"limit,omitempty" jsonschema:"the maximum number of results (default 10)"`
}

// RecallHit is one event returned by memory_recall.
type RecallHit struct {
	EventID int64   `json:"event_id"`
	TS      string  `json:"ts"`
	Agent   string  `json:"agent"`
	Type    string  `json:"type"`
	Summary string  `json:"summary"`
	Score   float64 `json:"score"`
	Source  string  `json:"source"` // keyword | semantic | both
}

// RecallOutput is the result of the memory_recall tool.
type RecallOutput struct {
	Hits []RecallHit `json:"hits"`
}

// handleRecall embeds the query and returns the fused keyword + semantic hits.
func (s *Server) handleRecall(
	ctx context.Context, _ *mcp.CallToolRequest, in RecallInput,
) (*mcp.CallToolResult, RecallOutput, error) {
	if in.Query == "" {
		return nil, RecallOutput{}, fmt.Errorf("memory_recall requires a non-empty query")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultRecallLimit
	}
	vec, err := s.embedder.Embed(ctx, in.Query)
	if err != nil {
		return nil, RecallOutput{}, fmt.Errorf("embedding the query: %w", err)
	}
	hits, err := s.episodic.RecallEvents(ctx, in.Query, vec, limit)
	if err != nil {
		return nil, RecallOutput{}, fmt.Errorf("recalling events: %w", err)
	}
	out := RecallOutput{Hits: make([]RecallHit, 0, len(hits))}
	for _, h := range hits {
		out.Hits = append(out.Hits, RecallHit{
			EventID: h.Event.ID,
			TS:      h.Event.TS,
			Agent:   h.Event.Agent,
			Type:    h.Event.Type,
			Summary: h.Event.Summary,
			Score:   h.Score,
			Source:  h.Source,
		})
	}
	return nil, out, nil
}

// CanonicalReadInput is the argument schema for the memory_canonical_read tool.
type CanonicalReadInput struct {
	Path string `json:"path,omitempty" jsonschema:"the doc to read, relative to the memory root; omit to list all docs"`
}

// CanonicalReadOutput is the result of the memory_canonical_read tool. Exactly
// one of Content (with Path) or Docs is populated.
type CanonicalReadOutput struct {
	Path    string   `json:"path,omitempty"`
	Content string   `json:"content,omitempty"`
	Docs    []string `json:"docs,omitempty"`
}

// handleCanonicalRead reads one canonical doc, or lists them all when no path
// is given.
func (s *Server) handleCanonicalRead(
	_ context.Context, _ *mcp.CallToolRequest, in CanonicalReadInput,
) (*mcp.CallToolResult, CanonicalReadOutput, error) {
	if in.Path == "" {
		docs, err := s.canonical.List()
		if err != nil {
			return nil, CanonicalReadOutput{}, fmt.Errorf("listing canonical docs: %w", err)
		}
		return nil, CanonicalReadOutput{Docs: docs}, nil
	}
	content, err := s.canonical.Read(in.Path)
	if err != nil {
		return nil, CanonicalReadOutput{}, fmt.Errorf("reading canonical doc: %w", err)
	}
	return nil, CanonicalReadOutput{Path: in.Path, Content: content}, nil
}

// CanonicalWriteInput is the argument schema for the memory_canonical_write tool.
type CanonicalWriteInput struct {
	Path    string `json:"path" jsonschema:"the doc to write, relative to the memory root"`
	Content string `json:"content" jsonschema:"the full new content of the doc"`
}

// CanonicalWriteOutput is the result of the memory_canonical_write tool.
type CanonicalWriteOutput struct {
	Path string `json:"path"`
}

// handleCanonicalWrite writes a canonical doc, stamping its last_updated field.
func (s *Server) handleCanonicalWrite(
	_ context.Context, _ *mcp.CallToolRequest, in CanonicalWriteInput,
) (*mcp.CallToolResult, CanonicalWriteOutput, error) {
	if in.Path == "" {
		return nil, CanonicalWriteOutput{}, fmt.Errorf("memory_canonical_write requires a path")
	}
	if err := s.canonical.Write(in.Path, in.Content); err != nil {
		return nil, CanonicalWriteOutput{}, fmt.Errorf("writing canonical doc: %w", err)
	}
	return nil, CanonicalWriteOutput{Path: in.Path}, nil
}

// StatsInput is the (empty) argument schema for the memory_stats tool.
type StatsInput struct{}

// StatsOutput is the result of the memory_stats tool.
type StatsOutput struct {
	Events    int64 `json:"events"`
	Tasks     int64 `json:"tasks"`
	Runs      int64 `json:"runs"`
	SizeBytes int64 `json:"size_bytes"`
}

// handleStats reports the size and contents of the episodic store.
func (s *Server) handleStats(
	ctx context.Context, _ *mcp.CallToolRequest, _ StatsInput,
) (*mcp.CallToolResult, StatsOutput, error) {
	st, err := s.episodic.Stats(ctx)
	if err != nil {
		return nil, StatsOutput{}, fmt.Errorf("reading episodic stats: %w", err)
	}
	return nil, StatsOutput{
		Events:    st.Events,
		Tasks:     st.Tasks,
		Runs:      st.Runs,
		SizeBytes: st.SizeBytes,
	}, nil
}
