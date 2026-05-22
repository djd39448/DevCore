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

## Next · Phase 1 — the devcore-memory MCP server

Go: SQLite schema, FTS5 + sqlite-vec, Ollama embeddings, the six memory tools.
