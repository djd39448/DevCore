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

## 2026-05-23 · Phase 6 — Desktop app · read-wiring (layer 1)

The desktop shell now shows **real DevCore state** in place of the prototype's
mock fixtures, via a small local HTTP API:

- `cmd/devcore-api` — a new entrypoint binary. Opens the same two stores
  (`internal/episodic`, `internal/canonical`) read-only, binds `127.0.0.1` on
  a kernel-chosen ephemeral port, prints `LISTENING:<port>\n` on stdout, and
  serves the read paths under `/api/`.
- `internal/apiserver` — a new package holding the HTTP handler. Five
  read-only endpoints: `/api/stats`, `/api/tasks`, `/api/runs`, `/api/events`,
  `/api/canonical` (list + read by `?path=`). All GET-only, JSON-only,
  `Access-Control-Allow-Origin: *` (the page origin is `devcore://`).
  Limits are clamped server-side; bad limits return 400; canonical path
  rejections surface as 404. Server has its own modest timeouts and a
  two-second graceful shutdown.
- `internal/episodic` — added `ListEvents(ctx, limit)` (newest-first, the
  simple read path) and JSON struct tags on `Event` so the API can encode
  rows directly.
- `desktop/Shell/main.swift` — the AppKit shell now owns an `APIProcess`. On
  launch it walks up from the bundle looking for `devcore.config.yaml` to
  locate the repo root, then spawns `devcore-api` with explicit
  `--episodic-db` / `--canonical-dir` flags, waits up to five seconds for
  the `LISTENING:<port>` handshake, and loads the page with
  `?api=http://127.0.0.1:<port>`. The subprocess is terminated on app exit.
  If the API fails to start, the page loads without `?api=` and falls back
  to the prototype mocks rather than blanking out.
- `desktop/web/api.jsx` — new file. Exposes hooks
  (`useStats`, `useTasks`, `useEvents`, `useCanonical`, `useCanonicalDoc`)
  on `window.DevCoreAPI`. Each hook polls on its own cadence (5s for stats
  and events; 10s for tasks; 30s for canonical). The API base URL is read
  from `?api=` once, validated as a localhost-only URL, and any value
  pointing elsewhere is rejected.
- `desktop/web/app.jsx` / `views.jsx` — `Sidebar`, the statusbar,
  `EventsView`, `CanonicalView`, and `TasksView` now consume the hooks and
  overlay real numbers on top of the cosmetic `useLiveRun()` animation.
  Each view labels itself "live" or "placeholder" so it's clear which mode
  is rendering. The prototype's static rows remain as the offline fallback.
- `desktop/build.sh` — now builds `devcore-api` alongside the Swift shell
  and bundles it under `Contents/MacOS/`.

Tests added: `apiserver` package gets 11 tests covering every endpoint,
filters, limit handling, traversal rejection, method-not-allowed, and the
CORS preflight. `episodic` package gets 3 tests for `ListEvents` (ordering,
limit, rejection of non-positive limit).

All gates green: `gofumpt`, `go vet`, `golangci-lint` (0 issues), `go test`
across all packages; `swiftformat`, `swiftlint`; `shellcheck`. `./build.sh`
produces a working `DevCore.app` (Swift shell ~150 KB + Go API binary
~14 MB). End-to-end smoke test: launching the binary directly prints
`LISTENING:<port>` and `curl http://127.0.0.1:<port>/api/stats` returns
real counts from an empty `.devcore/state/episodic.sqlite`.

The page chrome (toolbar, sidebar, live-run animation) is intentionally left
on its cosmetic ticker — those layers wire to real run state in Phase 4 when
the Go Engine produces actual agent runs to log against.

## 2026-05-24 · Phase 3 — Thin orchestration + first real work ✅

DevCore did real work for the first time. The Conductor walked the human-
gated loop manually (this Claude Code session); the Analyst and Architect
were dispatched as subagents over the cloned source repo. Two gates passed.

**Inputs**
- Source pin: `d884efae9cc150df2a58afc255b3e631d31b5d2b` of
  `github.com/djd39448/sous-chef-ai`, cloned to `~/sous-chef-ai`.
- Workload spec authored at `.devcore/tasks/sous-chef-port.md` — the
  human-authored seed. Re-platform map, must-preserve / must-cut, five
  open decisions, four gates.

**Behavior_spec gate ✅**

Analyst run (general-purpose sub-agent under the Analyst role prompt,
~7 min, 167k tokens, 43 tool uses) produced:
- `domain/sous-chef-behaviors.md` — 1098 lines. Product summary, 11
  entities (CFO with its four roles + four inventory states + uniqueness
  rule; meal_plans, days, recipes, cookbook, shopping, conversations,
  ingredient_memory, users, sessions), all 9 React pages with state
  transitions, AI behaviors (system prompt + four tool calls + SSE
  protocol + image generation + meal-plan random-cuisine generation +
  recipe-page chat), the full REST surface, five state machines, the
  auth flow split into "survives / does not", must-preserve / must-cut,
  and 10 open questions for the Architect.
- `decisions/0001-voice-features.md` — Voice cut. Reasoning: the voice
  path is **complete scaffolding that is never wired in** at the pin,
  so we're "deciding not to add" rather than "removing".

At the gate Dave pre-decided the four remaining workload-spec open
decisions, captured by the Conductor as ADRs 0002–0005:
- **0002** — Direct OpenAI (tool-call shape, gpt-image-1, SSE all port).
- **0003** — SIWA + Supabase email/OTP; Supabase JWT bearer to Go API.
- **0004** — Cookbook images: Supabase Storage + on-device LRU (128 MB);
  generate at save, regenerate only on explicit request.
- **0005** — SwiftUI NavigationStack, no Universal Links v1.

**Contract gate ✅**

Architect run (general-purpose sub-agent under the Architect role prompt,
~8 min, 109k tokens, 18 tool uses) produced:
- `contract/contract.md` — 1487 lines, nine sections: framing, auth
  (Supabase JWT + JWKS rejection table), wire conventions, Supabase
  schema (six tables in DDL, JSONB shapes spelled out, RLS intent),
  REST surface grouped by resource (full request/response shapes),
  SSE wire format, AI tool-calling (four tools + recipe-page
  `update_meal` variant), image generation (Supabase Storage vs
  transient data URL), ten pinned behavior rules.
- Five Architect ADRs (0006–0010), all approved at the gate:
  - **0006** — Drop legacy `recipes` and `ingredient_memory` tables.
  - **0007** — Clear-checked-items requires `shoppingListId`; source
    bug not preserved.
  - **0008** — Recipe-page chat stays stateless.
  - **0009** — Keep all four CFO role enum values; materialize only
    `inventory` and `shopping` in v1.
  - **0010** — Client computes Monday-of-week in local timezone; sends
    ISO date; server validates Monday-ness, treats date as opaque.

The Architect flagged five cross-track risks for the upcoming
`track_plan` gate — `clientWeekStartDate` plumbing, cookbook-save UX
during image gen, storage RLS + delete cascade, recipe markdown format
indirection, and byte-identical tool-call schemas via a single Go
constant.

**Phase 3 closed.** All artifacts under `.devcore/memory/` (12 files
added: 1 behavior spec, 1 contract, 10 ADRs). MEMORY.md updated. The
workload spec marks `behavior_spec` and `contract` gates Passed.

**Track_plan gate ✅ (same session)**

Three Builder track-pack subagents ran in parallel (backend, data, iOS)
each producing one plan against the contract. Aggregate ~17 minutes
wall-clock; the parallelism was real — none consulted the others.

- `plan/track-backend.md` — 940 lines, 41 tasks across 12 phases. **AWS
  target: ECS Fargate** (Lambda+API Gateway breaks SSE — idle timeout,
  buffered writes, 30s ceiling). **SSE: stdlib only** with `http.Flusher`
  + a 20s heartbeat goroutine; no external library. Single Go constant
  per tool-call schema (the Architect's recommended pattern).
- `plan/track-data.md` — 465 lines. **Supabase CLI declarative schemas**
  per dc-04 — desired state in `supabase/schemas/*.sql`, diff-generated
  migrations in `supabase/migrations/`, applied migrations never edited
  (pre-commit + CI checksum gate). RLS-aware policies using
  `(select auth.uid())`, owner tables direct, ownership-through-parent
  tables via `STABLE SECURITY DEFINER` helpers in an `app_private`
  schema. Cookbook image cascade as a same-transaction
  `BEFORE DELETE` trigger.
- `plan/track-ios.md` — 797 lines. **iOS 17** (for `@Observable`).
  **Hybrid layout: one Xcode app target + an in-repo `SousChefKit`
  Swift Package** split into five library targets (`Domain`, `API`,
  `Auth`, `Markdown`, `ImageCache`); package targets cannot
  `import SwiftUI`, making dc-03's "models never import UI" a
  compile-time guarantee. **Hand-rolled `SousChefMarkdown` parser**
  over the canonical recipe format (SwiftUI's `AttributedString` is
  inline-only; third-party renderers couple to SwiftUI).
- `plan/integration.md` — Conductor synthesis. Maps the touchpoints
  where the tracks meet (`clientWeekStartDate` plumbing; cookbook
  save UX during image gen; SSE wire format; Supabase Auth provider
  config). Consolidates 18 open questions across the three plans:
  3 Conductor-decide, 3 Dave-actions, 11 Phase-4 Builder defaults
  with documented choices, 1 conflict.

**The conflict** the gate surfaced — and resolved:

- Backend track defaulted to a **service-role** Postgres connection
  with app-level `WHERE user_id = $1` filtering.
- Data track wrote **RLS-aware policies** using `auth.uid()`,
  assuming the JWT flows through.
- Mutually exclusive. Decided at the gate as **ADR-0011 — JWT-aware
  connection.** The Go service connects as the `authenticated`
  Postgres role; `SET LOCAL request.jwt.claim.sub` per request;
  Postgres RLS is the source of truth. Defense-in-depth — a Go bug
  cannot leak rows. Track-backend gets a one-task amendment via a
  NOTE block at the top of its plan; data and iOS plans need no
  changes.

Other gate decisions (in `plan/integration.md` §3.1):
- **TabView** iOS shell (five tabs: Chat, Plan, Calendar, Cookbook,
  Shopping).
- **PostgREST disabled** — iOS goes through the Go API only.

Three Dave-actions called out for Phase 4 (not gate-blocking):
Apple Developer enrollment + Service ID + SIWA provider config;
confirm AWS staging ALB idle timeout is 600s+; settle the
`APIBaseURL` per environment.

Eleven Phase-4 Builder decisions left with documented defaults
(UUIDv7 source, OpenAI model pin policy, `/regenerate-image`
ownership param, inactivity window for fresh-chat, disk-cache
location, etc.) — each Builder uses its default unless Dave objects.

**Phase 3 fully closed.** 17 canonical artifacts total: 1 behavior
spec, 1 contract, 11 ADRs, 3 track plans, 1 integration synthesis.
The workload spec marks all three gates passed.

Suggested Phase 4 starting order (`plan/integration.md` §4):
foundations in parallel (week 1) → first read-only slice end-to-end
(week 2) → AI surface (week 3) → polish + TestFlight (week 4).
