// tasktool.go holds the memory_task tool: a single tool whose `op` field
// selects one of five task/run operations. See server.go for the package
// overview.

package memoryserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/djd39448/DevCore/internal/episodic"
)

// TaskInput is the argument schema for the memory_task tool. Which fields are
// required depends on Op; each handler validates its own.
type TaskInput struct {
	Op string `json:"op" jsonschema:"the operation: upsert_task, get_task, list_tasks, record_run, or list_runs"`

	// Task fields. ID also identifies get_task; Status also filters list_tasks.
	ID            string `json:"id,omitempty"`
	ParentID      string `json:"parent_id,omitempty"`
	Title         string `json:"title,omitempty"`
	SpecRef       string `json:"spec_ref,omitempty"`
	Track         string `json:"track,omitempty"`
	Status        string `json:"status,omitempty"`
	AssignedAgent string `json:"assigned_agent,omitempty"`

	// Run fields. TaskID also filters list_runs.
	RunID     string `json:"run_id,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	Agent     string `json:"agent,omitempty"`
	Model     string `json:"model,omitempty"`
	Profile   string `json:"profile,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
	RunStatus string `json:"run_status,omitempty"`
	Summary   string `json:"summary,omitempty"`
	TokensIn  int64  `json:"tokens_in,omitempty"`
	TokensOut int64  `json:"tokens_out,omitempty"`
}

// TaskOutput is the result of the memory_task tool. Which field is populated
// depends on the operation.
type TaskOutput struct {
	OK    bool            `json:"ok"`
	Task  *episodic.Task  `json:"task,omitempty"`
	Tasks []episodic.Task `json:"tasks,omitempty"`
	Runs  []episodic.Run  `json:"runs,omitempty"`
}

// handleTask dispatches to the operation named by in.Op.
func (s *Server) handleTask(
	ctx context.Context, _ *mcp.CallToolRequest, in TaskInput,
) (*mcp.CallToolResult, TaskOutput, error) {
	switch in.Op {
	case "upsert_task":
		return s.upsertTask(ctx, in)
	case "get_task":
		return s.getTask(ctx, in)
	case "list_tasks":
		return s.listTasks(ctx, in)
	case "record_run":
		return s.recordRun(ctx, in)
	case "list_runs":
		return s.listRuns(ctx, in)
	default:
		return nil, TaskOutput{}, fmt.Errorf(
			"memory_task: unknown op %q; expected upsert_task, get_task, list_tasks, record_run, or list_runs",
			in.Op,
		)
	}
}

// upsertTask creates or updates a task. created_at is supplied on every call
// but the store ignores it on update, so an existing task keeps its original.
func (s *Server) upsertTask(ctx context.Context, in TaskInput) (*mcp.CallToolResult, TaskOutput, error) {
	if in.ID == "" || in.Title == "" {
		return nil, TaskOutput{}, fmt.Errorf("memory_task upsert_task requires id and title")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	task := episodic.Task{
		ID:            in.ID,
		ParentID:      in.ParentID,
		Title:         in.Title,
		SpecRef:       in.SpecRef,
		Track:         in.Track,
		Status:        in.Status,
		AssignedAgent: in.AssignedAgent,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if task.Status == "" {
		task.Status = "pending"
	}
	if err := s.episodic.UpsertTask(ctx, task); err != nil {
		return nil, TaskOutput{}, fmt.Errorf("upserting task: %w", err)
	}
	return nil, TaskOutput{OK: true, Task: &task}, nil
}

// getTask returns a single task by ID.
func (s *Server) getTask(ctx context.Context, in TaskInput) (*mcp.CallToolResult, TaskOutput, error) {
	if in.ID == "" {
		return nil, TaskOutput{}, fmt.Errorf("memory_task get_task requires id")
	}
	task, err := s.episodic.GetTask(ctx, in.ID)
	if err != nil {
		return nil, TaskOutput{}, fmt.Errorf("getting task: %w", err)
	}
	return nil, TaskOutput{OK: true, Task: &task}, nil
}

// listTasks returns all tasks, optionally filtered by Status.
func (s *Server) listTasks(ctx context.Context, in TaskInput) (*mcp.CallToolResult, TaskOutput, error) {
	tasks, err := s.episodic.ListTasks(ctx, in.Status)
	if err != nil {
		return nil, TaskOutput{}, fmt.Errorf("listing tasks: %w", err)
	}
	return nil, TaskOutput{OK: true, Tasks: tasks}, nil
}

// recordRun inserts a run. started_at defaults to now when not supplied.
func (s *Server) recordRun(ctx context.Context, in TaskInput) (*mcp.CallToolResult, TaskOutput, error) {
	if in.RunID == "" || in.Agent == "" {
		return nil, TaskOutput{}, fmt.Errorf("memory_task record_run requires run_id and agent")
	}
	started := in.StartedAt
	if started == "" {
		started = time.Now().UTC().Format(time.RFC3339)
	}
	err := s.episodic.RecordRun(ctx, episodic.Run{
		ID:        in.RunID,
		TaskID:    in.TaskID,
		Agent:     in.Agent,
		Model:     in.Model,
		Profile:   in.Profile,
		StartedAt: started,
		EndedAt:   in.EndedAt,
		Status:    in.RunStatus,
		Summary:   in.Summary,
		TokensIn:  in.TokensIn,
		TokensOut: in.TokensOut,
	})
	if err != nil {
		return nil, TaskOutput{}, fmt.Errorf("recording run: %w", err)
	}
	return nil, TaskOutput{OK: true}, nil
}

// listRuns returns runs, optionally filtered by TaskID.
func (s *Server) listRuns(ctx context.Context, in TaskInput) (*mcp.CallToolResult, TaskOutput, error) {
	runs, err := s.episodic.ListRuns(ctx, in.TaskID)
	if err != nil {
		return nil, TaskOutput{}, fmt.Errorf("listing runs: %w", err)
	}
	return nil, TaskOutput{OK: true, Runs: runs}, nil
}
