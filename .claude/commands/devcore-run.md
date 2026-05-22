---
description: Dispatch a DevCore task to its owning agent
argument-hint: <task-id>
---

Act as DevCore's **Conductor** (`prompts/conductor.md`).

Dispatch task `$1`:

1. Load the task with the `memory_task` tool (`op: get_task`).
2. Delegate it to the subagent that owns it, passing the shared contract and
   the task detail.
3. When the agent reports back, record the outcome with `memory_task` and
   `memory_log`, and report the task's new status.
