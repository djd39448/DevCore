# DevCore — Change Log

This log records **decisions and changes of direction** for the DevCore build.
When the project pivots, the change is recorded here as an explicit entry. The
docs are never silently updated to make a new direction look like it was always
the plan.

**Scope:** changes to DevCore itself — its architecture, build, and process.
Decisions made *within* a workload that DevCore builds belong in that workload's
own `.devcore/memory/decisions/` ADRs, not here.

Newest first. Each entry: date, what changed, and why.

---

## 2026-05-22 — MCP registration moved to `.mcp.json`

**Changed:** buildspec §9 described Phase 0 MCP registration as living in
`.claude/settings.json`; implemented instead in `.mcp.json`.
**Why:** `.mcp.json` is the correct Claude Code location for project-shared,
committable MCP configuration. `settings.json` holds hooks and permissions.
Buildspec §4.5 and §5 were updated to match.

## 2026-05-21 — Coding standard authored in-repo, not pushed to Pinecone

**Changed:** the plan had been to author the DevCore-stack coding standard and
upsert it into Pinecone's `trustcore-systems` index. Changed to: the standard is
`CODING_STANDARDS.md` in the repo (canonical), mirrored to
`.devcore/memory/conventions/`. Pinecone stays a read-only reference.
**Why:** the standard belongs under version control alongside the code it
governs — diffable, reviewable, travelling with the repo. DevCore never writes
to Pinecone.

## 2026-05-21 — Memory layer: cognee evaluated and rejected

**Changed:** cognee was assessed as a candidate memory layer and rejected; the
custom design (one SQLite file — FTS5 + sqlite-vec — behind a Go MCP) stands.
**Why:** cognee would force a ~50-dependency Python runtime into a single-binary
Go system. It fails the lean principle (P4) and compromises portability (P1), to
provide graph relations an append-only event log does not need.

## 2026-05-21 — Proxy choice locked to claude-code-router

**Changed:** the model-swap proxy was finalized as `claude-code-router`; LiteLLM
remains the documented fallback.
**Why:** purpose-built for Claude Code, lighter-weight, fits the lean principle.

## 2026-05-21 — First workload changed to the sous-chef iOS port

**Changed:** the first workload was initially a photo-sharing app; changed to
porting `sous-chef-ai` (an existing Replit web app) to a native iOS app.
**Why:** a port is a sharper, better-defined first workload than a greenfield
build — a known input and a known output — and it still exercises the full
agent roster.

## 2026-05-20 — Harness: Claude Code + a local proxy

**Decided:** DevCore's agent harness is the Claude Code CLI plus a translation
proxy for local-model swap — chosen over the Claude Agent SDK and over building
a custom harness.
**Why:** Claude Code's headless mode, subagents, hooks, and MCP support make it
usable as an engine, not just a chat tool; the CLI-plus-proxy path gives the
cleanest swap to local models later.
