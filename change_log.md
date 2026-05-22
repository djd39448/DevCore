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

## 2026-05-22 — Desktop app added; built as a Path B native shell

**Changed:** DevCore gains a desktop app — a personal native-macOS control
surface (buildspec §4.8, Phase 6). Built as **Path B**: a native macOS shell
(an AppKit window + a `WKWebView`) hosting the design prototype, rather than a
full SwiftUI port.
**Why:** the user wants a Claude-Code-desktop-like UX, for personal use. A
complete interactive prototype arrived from Claude Design; wrapping it in a
native shell delivers a working app immediately, with effort going to wiring it
to the live engine rather than re-drawing CSS. A full SwiftUI nativization is a
later pass (§10). The webview-shell shortcut applies **only** to DevCore's own
desktop tool — every application DevCore *builds*, sous-chef included, is
genuine native SwiftUI.

## 2026-05-22 — `devcore doctor` implemented as `scripts/doctor.sh`

**Changed:** buildspec §9 Phase 2 calls for `devcore doctor --test-local`. The
`devcore` engine binary is a Phase 4 deliverable, so the Phase 2 health check is
implemented as `scripts/doctor.sh` instead.
**Why:** buildspec §5 already designates `scripts/` for "setup, doctor, backup
helpers." A shell health check needs no Go binary and can run before the engine
exists. When the engine arrives in Phase 4, a `devcore doctor` subcommand may
wrap or replace it. buildspec §9 updated.

## 2026-05-22 — `Stop` hook dropped; agents log their own runs

**Changed:** the `Stop` logging hook — moved from Phase 1 to Phase 2 earlier
today — is now dropped entirely rather than implemented. Only the `SessionStart`
hook ships in Phase 2.
**Why:** a Claude Code `Stop` hook is a shell command, and DevCore's episodic
memory sits behind an MCP server a shell hook cannot call. Run-completion
logging is already the agents' own job — every agent prompt mandates it via the
`memory_task` and `memory_log` tools. A shell `Stop` hook would be redundant and
unable to reach the store. buildspec §4.7 and §9 updated.

## 2026-05-22 — `Stop` logging hook moved from Phase 1 to Phase 2

**Changed:** buildspec §9 listed a `Stop` logging hook among Phase 1's
deliverables. Moved to Phase 2.
**Why:** the hook's job is to log agent-run completion into episodic memory, but
Phase 1 has no agent runs to log — the orchestration that produces runs arrives
later. The hook belongs with the agent and hook wiring in Phase 2. buildspec §9
updated.

## 2026-05-22 — Episodic store: dropped sqlite-vec and the FTS5 virtual table

**Changed:** buildspec §6.1 specified the episodic store with FTS5 and sqlite-vec
virtual tables. Phase 1 implements it instead as three plain SQLite tables on
`modernc.org/sqlite` (pure Go — no CGO, no WASM); embeddings are stored in a BLOB
column and keyword + semantic recall are computed in Go.
**Why:** the `sqlite-vec` Go bindings are broken against current `ncruces`. The
stable binding (v0.1.6) calls `sqlite3.Binary`, which `ncruces` v0.34 removed;
the ancient `ncruces` it pins (v0.17.1) cannot run the binding's WASM (atomics
disabled). Pinning alpha/ancient versions to chase a working pair would itself
break dc-02 (current, minimal dependencies). In-Go brute-force recall is correct
and fast at DevCore's project scale (thousands of events) — revisit with a real
vector index only if the event log ever reaches millions of rows. buildspec
§4.3, §6.1, and §11 updated to match.

## 2026-05-22 — Memory server placed under cmd/ and internal/, not mcp/

**Changed:** buildspec §5 placed the memory MCP server under `mcp/devcore-memory/`.
Implemented instead as `cmd/devcore-memory/` (the binary) plus `internal/`
packages.
**Why:** dc-02 is explicit that executables live in `cmd/<binary>/` and
non-public code in `internal/`. An MCP server is an executable. Following dc-02
keeps every DevCore binary under one convention. buildspec §5 updated.

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
