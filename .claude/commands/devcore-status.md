---
description: Show DevCore's current task, run, and memory state
---

Report DevCore's current state:

1. Call the `memory_task` tool (`op: list_tasks`) for the task tree and each
   task's status.
2. Call the `memory_task` tool (`op: list_runs`) for recent agent runs.
3. Call the `memory_stats` tool for the episodic store's size and counts.

Present a concise status: what is done, in progress, blocked, and pending.
