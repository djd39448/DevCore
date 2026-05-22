# DevCore

DevCore is an iterative multi-agent engine for a software dev team, built on
Claude Code. This file is intentionally thin — it points; it does not contain.

## Read first

- `buildspec.md` — the architecture and phased build plan. Authoritative.
- `CODING_STANDARDS.md` — non-negotiable. The bar is **dc-00**: any developer,
  handed this codebase with no explanation, understands it without guessing.

## Memory

- Canonical memory index: `.devcore/memory/MEMORY.md` — the map of what DevCore
  knows. Read the index, then retrieve only what a task needs.
- Episodic memory (past runs, decisions) is queried via the `devcore-memory`
  MCP server — not loaded here. (The server arrives in Phase 1.)

## Working rules

- Follow `CODING_STANDARDS.md` on every change — the `dc-07` checklist is the
  pre-commit gate.
- Every new file carries its `dc-01` header: what it does, what it depends on,
  what depends on it, why it exists.
- Record progress in `build_log.md` as each phase or work session completes.
- When direction changes, record the pivot in `change_log.md` — never silently
  edit the docs to make the new direction look like it was always the plan.
- Keep this file thin. Durable knowledge goes into `.devcore/memory/`, not here.
