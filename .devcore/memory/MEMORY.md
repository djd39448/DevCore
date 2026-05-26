# DevCore — Canonical Memory Index

Tier 1 memory: the git-versioned source of truth for what DevCore knows.
This file is the map. Read it first; retrieve only what a task needs.

## Layout

| Directory | Holds |
|---|---|
| `architecture/` | DevCore engine & system design notes |
| `decisions/` | Architecture Decision Records (`NNNN-title.md`) |
| `domain/` | The current workload's domain knowledge |
| `conventions/` | The DevCore coding standard (mirror of `/CODING_STANDARDS.md`) |
| `contract/` | The shared contract agents converge on (API + data model) |
| `plan/` | Track plans the Conductor produces between `contract` and `track_plan` gates |

## Canonical documents

| Document | Summary |
|---|---|
| `conventions/devcore-coding-standards.md` | DevCore coding standard (`dc-00`–`dc-07`) — the non-negotiable bar |
| `domain/sous-chef-behaviors.md` | Behavior spec for the sous-chef iOS port (Analyst, accepted at the `behavior_spec` gate) |
| `decisions/0001-voice-features.md` | Cut voice features from the iOS port (accepted) |
| `decisions/0002-ai-provider.md` | AI provider: direct OpenAI (accepted) |
| `decisions/0003-auth.md` | Auth: Sign in with Apple + Supabase email/OTP (accepted) |
| `decisions/0004-image-storage.md` | Cookbook images: Supabase Storage + on-device LRU (accepted) |
| `decisions/0005-week-nav.md` | Meal-plan week nav: SwiftUI NavigationStack, no deep links v1 (accepted) |
| `contract/contract.md` | Sous-chef iOS shared contract — API surface + data model (Architect, accepted at the `contract` gate) |
| `decisions/0006-drop-legacy-tables.md` | Drop legacy `recipes` and `ingredient_memory` tables (accepted) |
| `decisions/0007-shopping-list-id-required.md` | Clear-checked-items requires `shoppingListId` (accepted) |
| `decisions/0008-recipe-chat-stateless.md` | Recipe-page chat stays stateless (accepted) |
| `decisions/0009-cfo-roles.md` | Keep all four CFO `usage_context.role` enum values; materialize only `inventory` and `shopping` (accepted) |
| `decisions/0010-week-timezone.md` | Client supplies Monday-of-week in its local timezone (accepted) |
| `plan/track-backend.md` | Backend track plan — Go on ECS Fargate, 41 tasks (accepted; see ADR-0011 amendment) |
| `plan/track-data.md` | Data track plan — Supabase CLI declarative schemas, RLS-aware (accepted) |
| `plan/track-ios.md` | iOS track plan — SwiftUI iOS 17, TabView shell, hybrid package + app target (accepted) |
| `plan/integration.md` | Cross-track integration synthesis — touchpoints, gate outcomes, 18 open questions (accepted) |
| `decisions/0011-jwt-aware-connection.md` | Backend connects JWT-aware; RLS is the source of truth (accepted) |
| `decisions/0012-bundle-id.md` | iOS bundle id is `com.djd39448.souschef` (accepted) |
| `decisions/0013-aws-deployment.md` | AWS: Elastic Beanstalk Docker, load-balanced ALB, us-east-1 (accepted; supersedes track-backend §3.3 Fargate choice) |
| `reviews/0001-phase4-week1.md` | Reviewer pass on Phase 4 Week 1 — all three tracks approved with findings |

## Active workload

The current workload spec lives **outside** canonical memory at
`.devcore/tasks/sous-chef-port.md` — Conductor reads it before planning. It
pins the source-repo commit, the re-platform map, the must-preserve /
must-cut lists, the open decisions, and the gates for the sous-chef iOS port
(DevCore's first workload).

The artifacts the Analyst and Architect produce **land in canonical memory**:

| Path (planned) | Owner | Produced at |
|---|---|---|
| `domain/sous-chef-behaviors.md` | Analyst | Before the `behavior_spec` gate |
| `contract/contract.md` | Architect | Before the `contract` gate |
| `decisions/NNNN-*.md` | Architect / Conductor | As open questions resolve |
| `plan/track-{backend,data,ios}.md` | Conductor | Before the `track_plan` gate |

Architecture notes, ADRs, domain knowledge, and the contract are added as the
build progresses. Episodic memory — past runs and decisions — lives in the
SQLite store (`.devcore/state/episodic.sqlite`) and is queried via the
`devcore-memory` MCP server, not here.
