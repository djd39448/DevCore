# DevCore — Build Specification

**Version:** 1.0
**Status:** Finalized — ready for Phase 0
**Date:** 2026-05-21
**Author:** Dave + Claude
**Repo:** https://github.com/djd39448/DevCore

---

## 1. Purpose & Scope

DevCore is an **iterative engine for a software development team** — an agent harness
that takes a development goal and drives it to a shipped result through a repeating
cycle of plan → build → review → verify → learn.

It is being built for a friend's dev team. Its job is to do, locally and
repeatably, the kind of multi-step engineering work a capable agent does
interactively — but structured, memory-backed, and orchestrated across multiple
specialized agents rather than one chat session.

**DevCore is the engine. It is not the product.** The applications it builds are
separate artifacts. Workload #1 is porting `sous-chef-ai` (a Replit web app) to a
native iOS app. A photo-sharing app is a likely workload #2. DevCore itself is
project-agnostic: the workload is *configured*, never hardcoded.

### What this spec covers
- The DevCore engine, agents, memory system, model layer, and repo layout.
- How DevCore is pointed at its first workload (the sous-chef port).
- A phased build plan with milestones and exit criteria.

### What this spec deliberately does NOT cover
- The sous-chef iOS app's own design (separate doc, produced *by* DevCore).
- The feedback-driven training/iteration pipeline (deferred — see §10).

### Relationship to TrustCore
DevCore is **not** a TrustCore port. It reuses two ideas from TrustCore — a
multi-agent roster and a layered memory system — and deliberately drops the rest
(Postgres cluster, eval agent, DPO training pipeline, per-agent governance).
DevCore is the leaner, portable descendant, not the same system.

---

## 2. Design Principles & Constraints

| # | Principle | What it means in practice |
|---|-----------|---------------------------|
| P1 | **Portable** | DevCore moves between machines by copying a folder. No required daemons, no container for the engine itself. Single-binary components. |
| P2 | **Model-agnostic** | The brain behind every agent is swappable via one config layer. Claude API today; local open-weight models on the Mac Studio later — same harness, no code change. |
| P3 | **Private-ready** | Nothing in the architecture *requires* a cloud call. Memory is 100% local. The model layer can run fully local once hardware allows. |
| P4 | **Lean, not robust-for-its-own-sake** | This is not TrustCore. Build the smallest thing that does the job; earn complexity. |
| P5 | **Memory is the bus** | Agents coordinate through shared memory, not direct messaging. The contract lives in memory; everyone reads it. |
| P6 | **Human-gated** | DevCore proposes; the human approves at defined milestone gates. Autonomy is earned phase by phase. |
| P7 | **Behaves like Claude Code** | The harness IS Claude Code. Agents have the Claude Code tool loop, file editing, bash, planning, subagents. Quality tracks the model behind the proxy; behavior does not. |

### Environment

- **Dev machine (now):** Apple M4 Mac Mini, 16 GB unified RAM. Serves as the DevCore
  dev environment. Astra and all other local agents on this machine are **paused**
  for the duration of the build.
- **Production machine (later):** Mac Studio (spec TBD) — runs large local models.
- **Migration path:** When the Studio arrives, DevCore is copied over, model
  profiles are repointed at local models, and the proxy is finalized. Nothing else
  changes.

---

## 3. System Overview

```
                       ┌──────────────────────────┐
                       │     Dave  (human gates)  │
                       └─────────────┬────────────┘
                                     │  approves at milestones
                       ┌─────────────▼────────────┐
                       │      DevCore Engine      │   the iterative loop
                       │   Conductor + control    │
                       └─────────────┬────────────┘
                                     │  spawns / delegates
        ┌──────────┬──────────┬──────┴───┬──────────┬──────────┐
        ▼          ▼          ▼          ▼          ▼          ▼
    Analyst    Architect   Builder    Builder    Reviewer   Verifier
                          (backend)   (ios…)               (local model)
        └──────────┴──────────┴──────────┴──────────┴──────────┘
                                     │  read / write
                       ┌─────────────▼────────────┐
                       │   devcore-memory  (MCP)  │
                       ├──────────────────────────┤
                       │ Tier 1: canonical files  │  git-versioned truth
                       │ Tier 2: episodic SQLite  │  FTS5 + vector recall
                       └──────────────────────────┘

  Reference (read-only):  pinecone MCP  →  coding standards
  Model layer:  claude CLI  ──▶  Anthropic API        (build phase)
                claude CLI  ──▶  proxy ──▶ Ollama      (Studio phase / local agent)
```

DevCore has four moving parts:

1. **The Engine** — the orchestration loop. Plans work, dispatches agents, runs gates.
2. **The Agents** — six specialized roles, each a Claude Code instance.
3. **The Memory** — a two-tier local store, exposed to agents via an MCP server.
4. **The Model layer** — the `claude` CLI plus a translation proxy that makes the
   model behind every agent swappable.

---

## 4. Architecture

### 4.1 The Engine

The Engine is DevCore's orchestrator. It owns the control loop (§7), the task
state, and the gates.

**Phased implementation:**

- **Phase 1–3 (thin):** The Engine is a Claude Code *project* — a configured
  workspace with subagents, slash commands, and hooks. The **Conductor** runs as
  the top-level interactive Claude Code session and drives the loop; the human
  gates each milestone. Custom code at this stage = the memory MCP only.
- **Phase 4 (full):** The Engine is promoted to a **Go binary** (`devcore`) that
  spawns headless `claude -p` processes per agent, manages a task state machine,
  records runs, and supports scheduled/autonomous cycles.

**Why Go for the Engine binary:**
- Single static binary — the strongest possible portability story (P1).
- The target dev team already works in Go (the sous-chef backend track is Go) —
  DevCore dogfoods their stack and the team can maintain it.
- Shelling out to the `claude` CLI and parsing `--output-format json` is trivial
  in Go; no SDK dependency is needed (we chose Claude Code + proxy, not the Agent
  SDK).

The Engine never embeds a model. It orchestrates `claude` processes.

### 4.2 Agent Roster

Six roles. Named by **function**, not personas (a deliberate departure from
TrustCore's named agents — this is a harness, not a cast).

| Agent | Responsibility | Default model profile |
|-------|----------------|----------------------|
| **Conductor** | Plans the goal into tasks, routes work, manages gates, owns the loop. | api |
| **Analyst** | Reads existing systems; extracts behavior specs and requirements. | api |
| **Architect** | System design; authors the **shared contract** and ADRs; schema design. | api |
| **Builder** | Implementation. Instantiated **per track** with a stack context pack (backend / data / ios). | api |
| **Reviewer** | Code review, coding-standards enforcement (Pinecone), security review. | api |
| **Verifier** | Runs builds, tests, device runs; reports pass/fail. | **local** |

**Notes:**
- **Builder is one role, multiple instances.** The same prompt is instantiated
  with a track context pack — `backend` (Go/AWS), `data` (Supabase), `ios`
  (SwiftUI). One role definition, three configured profiles. Keeps the roster lean.
- **Verifier is the local-pinned agent.** Its work is mechanical (run a build,
  read the output, report) so it degrades gracefully on a weak model — making it
  the right role to prove the local-model path. See §4.4.
- **Memory maintenance is not an agent.** It is automation: a logging hook plus a
  `consolidate` command (§4.7).

**How agents run:**
- Thin phase: agents are Claude Code **subagents** (`.claude/agents/*.md`),
  delegated to by the Conductor within one session. All share the session's model.
- Full phase: agents are separate `claude -p` processes spawned by the Go Engine,
  each with its **own model profile and environment** — this is when per-agent
  model pinning (e.g. Verifier-on-local) becomes real at runtime.

### 4.3 Memory System

Two tiers, because "hold the whole project" and "reference past behaviors" are
different problems with different storage needs. Both are **100% local**.

#### Tier 1 — Canonical memory (git-versioned files)

The source of truth for what DevCore *is* working on. Markdown with YAML
frontmatter, committed to the DevCore repo.

```
.devcore/memory/
  MEMORY.md          ← index / map of every canonical doc
  architecture/      ← system & engine design notes
  decisions/         ← ADRs, one file per decision  (NNNN-title.md)
  domain/            ← the current workload's domain knowledge
  conventions/       ← local snapshot of coding standards (synced from Pinecone)
  contract/          ← the shared contract agents converge on (API + data model)
```

**Why files:** portable (P1), diffable, reviewable (`git diff` shows what the
agents changed about their own understanding), human-editable, and read natively
by Claude Code with zero glue.

#### Tier 2 — Episodic memory (single SQLite file)

The append-only record of what agents *did* — runs, decisions, corrections,
learnings — plus the operational task state. One file: `.devcore/state/episodic.sqlite`.

- **FTS5** full-text search — built into SQLite, zero extra dependency.
- **sqlite-vec** vector search — loaded as an extension. Both wired from the start.
- Embeddings produced **locally** by `nomic-embed-text` on Ollama (768-dim).

Schema in §6.1.

#### Tier discipline
- **`CLAUDE.md` stays thin** — it loads the `MEMORY.md` index and pointers, never
  the whole corpus. Deep content is retrieved on demand via the memory MCP. This
  is the gap in Claude Code's native memory that DevCore closes.
- **Consolidation:** a periodic step (`/devcore-consolidate`) promotes durable
  learnings out of Tier 2 into Tier 1 canonical docs, so hard-won knowledge gets
  versioned. This is DevCore's lightweight stand-in for TrustCore's training loop.
- **Versioning:** Tier 1 is committed. Tier 2 (`episodic.sqlite`) is **gitignored**
  (a constantly-growing binary blob does not belong in git) and backed up as a
  single file via `devcore memory backup`.

#### Access
All memory access goes through the **`devcore-memory` MCP server** (§4.6). One
shared store; every event is **agent-attributed**. There is no per-agent memory —
the agents collaborate on one codebase.

### 4.4 Model & Proxy Layer

Every agent's brain is the `claude` CLI. What sits behind it is swappable via a
**model profile**.

| Profile | Behind the CLI | Used by |
|---------|----------------|---------|
| `api` | Anthropic API directly | All agents, build phase |
| `local` | `claude` → translation proxy → Ollama | Verifier (wiring test now; real on Studio) |

**The proxy:** `claude-code-router` — **confirmed**. Purpose-built for Claude
Code, lighter-weight than the alternatives, fits P4. LiteLLM remains the
documented fallback if tool-block fidelity issues surface. The proxy must
faithfully translate the Anthropic Messages API **including tool-use and
tool-result blocks** — the place weak proxies break.

**Build-phase local wiring test:** Before the Studio arrives, the local path is
proven end-to-end by `devcore doctor --test-local` — a standalone headless
`claude -p` call launched with `ANTHROPIC_BASE_URL` pointed at `claude-code-router`
→ a small already-installed Ollama model (`llama3.1`). This validates the plumbing
(P2) without depending on the Studio. Per-agent model pinning at runtime is a
Phase 4 capability (it needs the separate-process Engine).

**Studio cutover:** flip the Verifier — and optionally all agents — to the
`local` profile pointed at a large local model. One config file changes (§6.3).

### 4.5 Coding Standards

DevCore enforces an explicit, versioned coding standard. The bar is `dc-00`: a
developer handed any DevCore-built codebase, with no explanation, understands it
without guessing.

- **The DevCore coding standard is authored and lives in the repo.**
  `CODING_STANDARDS.md` at the root is canonical; it is mirrored byte-for-byte to
  `.devcore/memory/conventions/devcore-coding-standards.md`, the copy the
  Reviewer agent reads. It is a stack-specific layer — Go, Swift/SwiftUI,
  Supabase/Postgres, AWS — covering sections `dc-00`–`dc-07`.
- **It extends the TrustCore base** (`cs-00`–`cs-10`, general language-agnostic
  principles), which remains in Pinecone (`trustcore-systems/coding-standards`)
  as a **read-only reference**. DevCore never writes to Pinecone.
- The **Reviewer** agent enforces the standard on every change; the `dc-07`
  checklist is the pre-commit gate.

### 4.6 MCP Servers

| Server | Type | Purpose |
|--------|------|---------|
| `devcore-memory` | custom, Go, stdio | The two-tier memory. Tools below. |
| `pinecone` | existing, stdio | Coding-standards reference (read-only). |
| *(workload MCPs)* | as needed | e.g. Supabase / simulator tooling for the sous-chef workload. |

**`devcore-memory` tools:**
- `memory_recall(query, scope?, limit?)` — hybrid FTS5 + vector search across
  episodic events and (optionally) canonical docs.
- `memory_log(type, agent, summary, detail?, refs?)` — append an episodic event.
- `memory_canonical_read(path?)` — structured read of Tier 1.
- `memory_canonical_write(path, content)` — write Tier 1, auto-stamping `last_updated`.
- `memory_task(op, …)` — create/update/query task & run state.
- `memory_stats()` — counts, sizes, staleness flags.

The memory MCP is a small Go program (official `go-sdk` for MCP; `go-sqlite3` with
the `sqlite-vec` extension loaded; FTS5 is built into SQLite). Single binary — P1.

### 4.7 Hooks & Slash Commands

**Hooks** (`.claude/settings.json`) — kept minimal:
- `SessionStart` — inject the `MEMORY.md` index pointer into context.
- `Stop` — log a run-completion event to episodic memory.

**Slash commands** (`.claude/commands/`):
- `/devcore-plan <goal>` — Conductor turns a goal into a task tree.
- `/devcore-run <task-id>` — dispatch a task to its agent.
- `/devcore-recall <query>` — query memory.
- `/devcore-consolidate` — promote episodic learnings into canonical docs.
- `/devcore-standards-sync` — refresh the TrustCore base reference from Pinecone
  (the DevCore standard itself is version-controlled in-repo).
- `/devcore-status` — show task/run state and cost.

---

## 5. Repository Layout

Target layout. Go directories (`cmd/`, `internal/`) arrive in Phase 4; everything
else exists from Phase 0–2.

```
DevCore/
  README.md
  buildspec.md                  ← this document
  CODING_STANDARDS.md           ← canonical coding standard (dc-00–dc-07)
  devcore.config.yaml           ← the one config file (§6.3)
  CLAUDE.md                     ← thin: pointers + memory index only
  .gitignore                    ← ignores .devcore/state/

  .claude/
    settings.json               ← hooks, permissions, MCP registration
    agents/                     ← Claude Code subagent definitions (thin phase)
    commands/                   ← custom slash commands

  prompts/                      ← agent system prompts (source of truth)
    conductor.md
    analyst.md
    architect.md
    builder.md                  ← + track packs: builder.backend.md, .data.md, .ios.md
    reviewer.md
    verifier.md

  .devcore/
    memory/                     ← Tier 1, committed
      MEMORY.md
      architecture/  decisions/  domain/  contract/
      conventions/              ← devcore-coding-standards.md (mirror of /CODING_STANDARDS.md)
    state/                      ← Tier 2, gitignored
      episodic.sqlite
    tasks/                      ← human-authored task/workload specs

  mcp/
    devcore-memory/             ← Go: the memory MCP server

  cmd/devcore/                  ← Go: the Engine CLI            (Phase 4)
  internal/
    engine/                     ← orchestration loop            (Phase 4)
    agents/                     ← agent process runner          (Phase 4)
    memory/                     ← shared memory package         (Phase 4)
  scripts/                      ← setup, doctor, backup helpers
```

---

## 6. Data Schemas

### 6.1 Episodic DB (SQLite) — `.devcore/state/episodic.sqlite`

```sql
-- Operational task state
CREATE TABLE tasks (
  id            TEXT PRIMARY KEY,
  parent_id     TEXT REFERENCES tasks(id),
  title         TEXT NOT NULL,
  spec_ref      TEXT,                       -- path into .devcore/tasks or memory
  track         TEXT,                       -- backend | data | ios | null
  status        TEXT NOT NULL DEFAULT 'pending',
                                            -- pending|active|blocked|review|done|abandoned
  assigned_agent TEXT,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

-- One row per agent invocation
CREATE TABLE runs (
  id          TEXT PRIMARY KEY,
  task_id     TEXT REFERENCES tasks(id),
  agent       TEXT NOT NULL,
  model       TEXT,
  profile     TEXT,                         -- api | local
  started_at  TEXT NOT NULL,
  ended_at    TEXT,
  status      TEXT,                         -- ok | error | aborted
  summary     TEXT,
  tokens_in   INTEGER,
  tokens_out  INTEGER
);

-- Append-only behavioral log
CREATE TABLE events (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  ts        TEXT NOT NULL,
  agent     TEXT,
  task_id   TEXT,
  run_id    TEXT,
  type      TEXT NOT NULL,                  -- decision|action|correction|learning|error|note
  summary   TEXT NOT NULL,
  detail    TEXT,
  refs      TEXT                            -- JSON array of file/commit refs
);

-- Keyword recall (built-in)
CREATE VIRTUAL TABLE events_fts USING fts5(
  summary, detail, content='events', content_rowid='id'
);

-- Semantic recall (sqlite-vec extension; 768-dim = nomic-embed-text)
CREATE VIRTUAL TABLE events_vec USING vec0(
  event_id INTEGER, embedding FLOAT[768]
);
```

`memory_recall` runs FTS5 and vector search, merges and reranks the hits.

### 6.2 Canonical memory file schema (Tier 1)

Every file in `.devcore/memory/` carries YAML frontmatter:

```yaml
---
title: Shared API & Data Contract
type: contract            # architecture | decision | domain | convention | contract
status: active            # draft | active | superseded
last_updated: 2026-05-21
owner: architect          # the agent role responsible for this doc
supersedes: null          # optional path to a superseded doc
---
```

`MEMORY.md` is the index: a table of every canonical doc with a one-line summary —
the map agents and humans read first.

### 6.3 Configuration — `devcore.config.yaml`

The single file that defines a DevCore deployment. To repoint at a new workload,
or to swap every model from API to local, you edit only this file.

```yaml
project:
  name:          sous-chef-ios
  workload_repo: ../sous-chef-ai          # the source web app
  output_repo:   ../sous-chef-ios         # the iOS app DevCore produces
  workload_spec: .devcore/tasks/sous-chef-port.md

memory:
  canonical_dir: .devcore/memory
  episodic_db:   .devcore/state/episodic.sqlite
  embeddings:
    provider: ollama
    model:    nomic-embed-text
    endpoint: http://localhost:11434

models:
  proxy:
    type:     claude-code-router
    endpoint: http://localhost:8787
  profiles:
    api:   { base_url: null,                    model: claude-sonnet-4-6 }
    local: { base_url: http://localhost:8787,   model: llama3.1 }   # → big model on Studio

agents:
  conductor: { profile: api,   prompt: prompts/conductor.md }
  analyst:   { profile: api,   prompt: prompts/analyst.md }
  architect: { profile: api,   prompt: prompts/architect.md }
  builder:   { profile: api,   prompt: prompts/builder.md,
               tracks: [backend, data, ios] }
  reviewer:  { profile: api,   prompt: prompts/reviewer.md }
  verifier:  { profile: local, prompt: prompts/verifier.md }

gates:                                    # human approval checkpoints
  - after:  behavior_spec
  - after:  contract
  - after:  track_plan
  - before: deploy
```

---

## 7. Control Loop & Lifecycle

DevCore is an *iterative engine* — it runs a cycle, and each cycle is informed by
the memory of the last.

```
  ┌──────────────────────────────────────────────────────────────┐
  │                                                                │
  ▼                                                                │
[1] INTAKE     Conductor reads the goal + workload spec.            │
                                                                    │
[2] PLAN       Conductor decomposes into a task tree; writes        │
               tasks to state. Reads memory for prior context.      │
                                                                    │
[3] DISPATCH   Each task routed to its agent. Agent reads the       │
               contract + relevant memory before acting.            │
                                                                    │
[4] EXECUTE    Agent does the work (Claude Code loop). Every        │
               decision / action / correction logged to episodic.   │
                                                                    │
[5] REVIEW     Reviewer checks output vs. coding standards.         │
[6] VERIFY     Verifier builds / tests / runs.                      │
                                                                    │
[7] GATE       At a configured gate, the human approves or sends    │
               back. Approved work updates the canonical contract.  │
                                                                    │
[8] CONSOLIDATE Durable learnings promoted Tier 2 → Tier 1.         │
               ───────────────────────────────────────────────────┘
               loop until the goal is met
```

In the thin phase the Conductor (a live Claude Code session) walks this loop with
human gates. In the full phase the Go Engine runs it, pausing only at gates.

---

## 8. First Workload — Sous-Chef iOS Port

DevCore's first job. Defined as a workload spec at `.devcore/tasks/sous-chef-port.md`;
DevCore is *configured* for it (§6.3), not rebuilt around it.

### Goal
Port `github.com/djd39448/sous-chef-ai` — a Replit-built React/Express/Postgres web
app — to a **native iOS app** on the team's target stack.

### Method: port behavior, not code
There is near-zero line-level reuse. The Analyst extracts a **behavior spec** from
the existing app (`replit.md` and the two `attached_assets/` files are the seed);
that spec, plus the data model, become the contract the rebuild targets.

### Re-platform map

| Layer | From | To | Reuse |
|-------|------|----|----|
| Database | Postgres + Drizzle | **Supabase** | High — schema translates near-directly. |
| Backend | Express/Node + TS | **Go on AWS** | Logic only — the contracts (CFO, tool schemas) port; code is rewritten. |
| Frontend | React + shadcn/ui | **SwiftUI** | None — full native rebuild; Figma wireframes drive the UI. |
| Auth | Replit Auth (OIDC) | **Supabase Auth** + Sign in with Apple | Replaced. |
| AI | Replit AI Integrations proxy | Direct model access | Replaced. |

**Must preserve:** the Canonical Food Object data model; the AI tool-calling
contract (`update_ingredients`, `create_meal_plan`, `generate_shopping_list`); the
feature set documented in `replit.md`.

**Must cut:** the entire `server/replit_integrations/` tree, `.replit`, Replit
Vite plugins, and the Replit audio integration (→ native iOS `Speech`/`AVFoundation`
if voice is kept).

### Execution shape — three tracks, one contract
The Architect first produces the **shared contract** (REST API surface + CFO data
model) into `.devcore/memory/contract/`. Once the human approves it at the
`contract` gate, three Builder instances proceed **in parallel**:

- **data track** → Supabase schema + migrations + Auth.
- **backend track** → Go API on AWS, implementing the contract.
- **ios track** → SwiftUI app against the contract, from Figma wireframes.

Reviewer and Verifier run continuously; integration and a device run close it out.
This three-tracks-converging-on-one-contract shape is exactly what validates
DevCore's memory architecture and agent model — the port is also DevCore's
stress test.

---

## 9. Build Phases & Milestones

Each phase has concrete deliverables and an exit criterion. No code is written
before Phase 0 begins.

### Phase 0 — Scaffold
- **Deliverables:** repo skeleton (§5), `devcore.config.yaml`, thin `CLAUDE.md`,
  `.gitignore`, `.claude/settings.json` with `pinecone` + `devcore-memory`
  registered.
- **Exit:** `claude` runs in the DevCore project; both MCP servers connect.

### Phase 1 — Memory layer
- **Deliverables:** `devcore-memory` MCP (Go) — SQLite schema + migrations, FTS5 +
  sqlite-vec, Ollama embeddings, all six tools. Tier 1 directory tree + seeded
  `MEMORY.md`. The `Stop` logging hook.
- **Exit:** an agent can `memory_log` an event and `memory_recall` it by keyword
  and by semantic similarity.

### Phase 2 — Agents & local wiring
- **Deliverables:** the six agent prompt files; subagent definitions; the six slash
  commands; `/devcore-standards-sync` pulling Pinecone → `conventions/`;
  `claude-code-router` installed; `devcore doctor --test-local` proving the
  `claude → proxy → Ollama` path.
- **Exit:** all agents callable; `devcore doctor` is green including the local path.

### Phase 3 — Thin orchestration + first real work
- **Deliverables:** the Conductor loop with manual gates. **DevCore does real
  work for the first time:** the Analyst extracts the sous-chef behavior spec; the
  Architect produces the shared contract.
- **Exit:** behavior spec and contract exist in Tier 1 and pass their human gates.

### Phase 4 — The Go Engine
- **Deliverables:** the `devcore` Go binary — task state machine, headless `claude -p`
  per-agent processes, per-agent model profiles (Verifier-on-local becomes real),
  `devcore status`, scheduled/autonomous cycles.
- **Exit:** DevCore runs a full plan→verify cycle headlessly, pausing only at gates.

### Phase 5 — Studio cutover
- **Deliverables:** model profiles repointed at large local models; proxy
  finalized; full-private operation validated. Then: scope the deferred
  feedback/iteration pipeline (§10) against the actual Studio spec.
- **Exit:** DevCore runs end-to-end with no cloud model calls.

---

## 10. Open Decisions & Deferred Items

| Item | Status | Resolve when |
|------|--------|-------------|
| Feedback-driven training/iteration pipeline (the TrustCore-DPO analogue) | **Deferred** | After the Mac Studio spec is known. |
| Mac Studio model selection (which local model per agent) | Open | When Studio hardware is specced. |
| Whether the Phase 4 Go Engine is needed, or the thin orchestration suffices | Open | Decide after Phase 3 — let the first real workload tell us. |
| Voice features in the sous-chef port (keep / drop) | Open | During the behavior-spec gate. |

---

## 11. Appendix — Technology Choices & Rationale

- **Claude Code as the harness** (not the Agent SDK): the agent must "behave like
  Claude Code," and Claude Code's headless mode + subagents + hooks + MCP make it
  usable as an engine, not just a chat tool. The SDK is Anthropic-API-oriented;
  the CLI + a proxy gives a cleaner local-model story.
- **Go for the Engine + memory MCP:** single static binaries (P1); the team
  already works in Go; no SDK dependency needed since we shell out to `claude`.
- **SQLite for episodic memory** (not Postgres/Qdrant): no daemon, one portable
  file, right-sized for project-scale recall. Postgres was a TrustCore-scale
  choice; DevCore is deliberately leaner (P4).
- **Cognee evaluated and rejected** (2026-05-21): a capable GraphRAG memory
  engine, but adopting it forces a ~50-dependency Python runtime into a
  single-binary Go system — it fails P4 and compromises P1, to deliver graph
  relations an append-only event log does not need. If graph-style recall is ever
  required, the lean move is an adjacency table in the existing SQLite file, not
  a new dependency.
- **sqlite-vec + FTS5:** semantic recall for "past behaviors," keyword recall as
  the can't-fail complement — both in-process, no extra service.
- **`nomic-embed-text` on Ollama:** small, fast on the M4, fully local — memory
  never needs the network. (Not matched to Pinecone's `e5-large`: DevCore's memory
  is independent of the Pinecone standards reference.)
- **Files for canonical memory:** portable, diffable, reviewable, human-editable,
  read natively by Claude Code.
- **Coding standard authored in-repo, not in Pinecone:** the standard is
  version-controlled alongside the code it governs (diffable, reviewable, travels
  with the repo). Pinecone stays a read-only reference for the general TrustCore
  base only — DevCore never writes to it.

---

*End of specification. This document is the contract for building DevCore. It
should be updated whenever the architecture materially changes; significant
changes are recorded as ADRs under `.devcore/memory/decisions/`.*
