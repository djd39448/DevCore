# DevCore

An iterative multi-agent engine for a software development team — an agent
harness built on Claude Code that takes a development goal and drives it to a
shipped result through a repeating cycle of **plan → build → review → verify →
learn**.

DevCore is the *engine*. The applications it builds are separate artifacts.
Workload #1 is porting a web app to a native iOS app.

## Start here

- **`buildspec.md`** — the architecture and the phased build plan. Authoritative.
- **`CODING_STANDARDS.md`** — non-negotiable. The bar: hand the codebase to any
  developer, say nothing, and they understand it without guessing.

## Status

**Phase 1 — Memory layer** (in progress). See `buildspec.md` §9 for the full
phase plan and `build_log.md` for current progress.

## Setup

1. Copy `.env.example` to `.env` and fill in the values.
2. Ensure `claude`, `ollama`, and (from Phase 2) `claude-code-router` are installed.
3. Export `PINECONE_API_KEY` so the `pinecone` MCP server connects.
4. Run `claude` from the repo root.

## Layout

See `buildspec.md` §5 for the full repository layout.
