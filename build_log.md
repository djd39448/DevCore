# DevCore — Build Log

A chronological record of build progress, phase by phase. Oldest first; newest
at the bottom. Each entry notes the date, the phase, what was done, and status.

For *why* a direction changed, see `change_log.md`.

---

## 2026-05-20 – 2026-05-21 · Planning

Architecture worked out collaboratively: the agent harness (Claude Code + a
local-model proxy), the six-agent roster, the two-tier local memory, the model
layer, and the phased build plan. First workload chosen: porting `sous-chef-ai`
(a Replit web app) to a native iOS app.

## 2026-05-21 · Specification

- `buildspec.md` finalized at v1.0 — the authoritative architecture and phase plan.
- `CODING_STANDARDS.md` authored — the stack-specific coding standard
  (`dc-00`–`dc-07`), extending the TrustCore base.
- Initial commit `27de37b` pushed to GitHub.

## 2026-05-22 · Phase 0 — Scaffold ✅

- Repo skeleton created (buildspec §5): `.claude/`, `.devcore/memory` and
  `tasks/`, `prompts/`, `mcp/`, `scripts/`.
- `devcore.config.yaml` authored and validated — the single config file.
- `CLAUDE.md` (thin), `README.md`, `.gitignore`, `.env.example` added.
- `.mcp.json` registers the `pinecone` MCP server; its key is injected from the
  environment, never committed.
- `build_log.md` and `change_log.md` established.
- Deviation: MCP registration implemented in `.mcp.json`, not
  `.claude/settings.json` as the buildspec wording suggested — see `change_log.md`.
- Status: complete. The `devcore-memory` MCP is deferred to Phase 1 — that
  server does not exist yet.

## 2026-05-22 · Phase 1 — Memory layer ✅

Toolchain installed: Go 1.26.3, golangci-lint 2.12.2, gofumpt; `nomic-embed-text`
pulled. Go module initialised; `.golangci.yml` added with the dc-02 linter set.

Built, `gofumpt`-clean, `golangci-lint`-clean (0 issues, full dc-02 linter set),
all tests passing across five packages:
- `internal/embed` — the Ollama embeddings client.
- `internal/episodic` — the Tier-2 SQLite store: events, tasks, runs, stats, and
  fused keyword + semantic recall.
- `internal/canonical` — the Tier-1 markdown/YAML file store: path-safe access
  and `last_updated` frontmatter stamping.
- `internal/memoryserver` — the MCP server exposing the six memory tools
  (`memory_log`, `memory_recall`, `memory_canonical_read`,
  `memory_canonical_write`, `memory_task`, `memory_stats`).
- `cmd/devcore-memory` — the server entrypoint; registered in `.mcp.json`.
  Smoke-tested: opens the stores, serves over stdio, exits cleanly on EOF.

Pivot during this phase: dropped sqlite-vec; the store now runs on
`modernc.org/sqlite` (pure Go) with recall computed in Go — see `change_log.md`.

Docs reconciled with the implementation: buildspec §4.3 / §5 / §6.1 / §9 / §11
and the README brought current; the sqlite-vec pivot, the `cmd/`+`internal/`
layout choice, and the `Stop` hook moving to Phase 2 are recorded in `change_log.md`.

Phase 1 exit met: an agent can `memory_log` an event and `memory_recall` it by
keyword and by semantic similarity (verified by tests). The `Stop` logging hook
moved to Phase 2 — there are no agent runs to log until orchestration exists.
