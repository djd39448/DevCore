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

Phase 1 committed: `0c15199` (23 files, +2323).

## 2026-05-22 · Phase 2 — Agents & local wiring ✅

Done:
- `prompts/` — the six agent role prompts (conductor, analyst, architect,
  builder, reviewer, verifier) plus the three Builder track packs
  (`builder.backend.md`, `builder.data.md`, `builder.ios.md`).
- `.claude/agents/` — the six subagent definitions: thin Claude Code wrappers
  pointing at the canonical `prompts/` files (single source of truth).
- `.claude/commands/` — the six slash commands: `devcore-plan`, `devcore-run`,
  `devcore-recall`, `devcore-consolidate`, `devcore-standards-sync`,
  `devcore-status`.
- `.claude/settings.json` — the `SessionStart` hook surfacing the current build
  phase. The `Stop` hook was dropped — see `change_log.md`.

- `claude-code-router` v2.0.0 installed and configured for local Ollama
  (`llama3.1`); the router daemon runs on `127.0.0.1:3456`.
- `scripts/doctor.sh` — the environment health check. Both `scripts/doctor.sh`
  and `scripts/doctor.sh --test-local` pass: toolchain, Ollama + models,
  `go build`, and the full `claude → claude-code-router → Ollama` round-trip.

Phase 2 exit met: all agents are defined and callable; `scripts/doctor.sh` is
green including the local-model path. Note: the local path requires
`MAX_THINKING_TOKENS=0` — Claude Code sends thinking-enabled requests that local
models reject; `scripts/doctor.sh` and any local-profile invocation set it.

## 2026-05-22 · Phase 6 — Desktop app · shell ✅ (built ahead of Phases 3–5)

DevCore's personal desktop control surface — built now, at the user's request,
ahead of the engine phases.

- Design: a Claude Design handoff bundle — an HTML/React prototype of
  `DevCore.app` (calm-editorial; 1280×820; Chat-default; nine views — chat,
  live run, tasks, gates, recall, canonical, episodic, agents, settings).
- `desktop/Shell/main.swift` — the native macOS shell: an AppKit window + a
  `WKWebView`, with a `devcore://` URL-scheme handler serving the bundled
  prototype. Compiles clean under Swift 6.
- `desktop/web/` — the prototype, copied verbatim from the bundle.
- `desktop/build.sh` — compiles the shell and assembles `build/DevCore.app`.
- Built, launched, and confirmed rendering by the user.

Decision (see `change_log.md`): Path B — a native shell over the prototype —
chosen over a full SwiftUI port; the webview shell is for DevCore's own tool only.

Remaining for Phase 6:
- Wire the prototype's views to the live Go Engine + `devcore-memory` — it
  currently shows placeholder data.
- Vendor React / Babel / web fonts locally for offline use.
- The full SwiftUI-native pass (deferred — buildspec §10).

## 2026-05-22 · Standards sweep

A full audit of every authored source file against `CODING_STANDARDS.md`
(`cs-00`–`cs-10`, `dc-00`–`dc-07`) — automated gates plus an independent review
of what the linters cannot catch. The Swift and shell linters (`swiftlint`,
`swiftformat`, `shellcheck`) were installed and run for the first time.

Findings fixed:
- `episodic_test.go` — the header falsely described a sqlite-vec/WASM path the
  code no longer uses (stale, from before the modernc pivot). Corrected. (dc-00)
- `episodic.go` `Stats` — the `os.Stat` error was silently swallowed; now
  returned, wrapped. (cs-05)
- `desktop/Shell/main.swift` — a `try?` collapsed every read failure into
  "not found"; now a `do`/`catch` carries the real cause, and `AssetError`
  conforms to `LocalizedError`. (cs-05 / dc-03)
- `memory_task` — `upsert_task` / `list_tasks` / `record_run` now validate
  their closed-set fields (status, track, profile, run_status) rather than
  passing a typo through silently. (cs-05)
- `cmd/devcore-memory/main.go` — bare error returns now wrapped with the
  failing stage. (cs-05)
- Test coverage added: the episodic encode/decode, distance, and fusion
  helpers (new white-box `events_internal_test.go`); `RecallEvents` empty-store
  and over-limit cases; `stampLastUpdated`'s insert and unterminated-frontmatter
  branches; `memory_task` `record_run`/`list_runs`; the handlers' empty-field
  rejections. (cs-06)
- `desktop/build.sh` — added the missing `Depended on by` header line. (cs-03)
- Swift configs committed (`desktop/.swiftformat`, `.swiftlint.yml`,
  `.swift-version`); `desktop/build/` added to `.gitignore`.

All gates green afterwards: `gofumpt`, `go vet`, `golangci-lint` (0 issues),
`go test`; `swiftformat`, `swiftlint`; `shellcheck`; the Go module and
`DevCore.app` both build clean.
