---
type: workload-spec
workload: sous-chef-ios
status: in-progress
last_updated: 2026-05-24
source_repo: github.com/djd39448/sous-chef-ai
source_commit: d884efae9cc150df2a58afc255b3e631d31b5d2b
output_repo: github.com/djd39448/sous-chef-ios
---

# Sous-Chef iOS Port — Workload Spec

DevCore's first real workload. Author: Dave. Read by: every agent before they act.

This file is the human-authored seed Conductor decomposes; it does **not**
describe code. It describes intent, scope, the re-platform map, what must be
preserved, what must be cut, and the gates. The Analyst and Architect produce
the deeper artifacts that follow.

## 1. Goal

Re-platform Sous Chef AI — a Replit-built React/Express/Postgres web app — to
a **native iOS app** on the DevCore target stack. The web app stays running;
the iOS app is a parallel, native rebuild that preserves the behaviour and
data model and replaces every other layer.

## 2. Source pin

- Repository: <https://github.com/djd39448/sous-chef-ai>
- Local clone: `~/sous-chef-ai`
- Commit pinned for this port: `d884efae9cc150df2a58afc255b3e631d31b5d2b`
- Surface (at pin):
  - 9 React pages (`landing`, `chat`, `meal-plan`, `calendar`, `recipe`,
    `cookbook`, `cookbook-recipe`, `shopping`, `not-found`).
  - ~27 REST endpoints under `/api/kitchen/` (server/routes.ts, 1285 LOC).
  - 11 Postgres tables; the **Canonical Food Object** (CFO) unified table
    (`food_items`) is the data-model centrepiece.
  - 4 AI tool-calls: `update_ingredients`, `create_meal_plan`,
    `create_shopping_list`, `update_meal`.
  - SSE streaming for chat; OpenAI-compatible chat + image generation.

The Analyst works from this pin. Drift since the pin (newer commits) is
out of scope unless explicitly added by a later workload-spec revision.

## 3. Re-platform map

| Layer | From | To | Reuse |
|-------|------|----|----|
| Database | Postgres + Drizzle ORM | **Supabase** (managed Postgres) | High — schema translates near-directly. |
| Backend | Express/Node + TS | **Go on AWS** | Tool-call contracts and CFO shape port; code is rewritten. |
| Frontend | React + Vite + shadcn/ui + Wouter | **SwiftUI** (native iOS) | None — full native rebuild. |
| Auth | Replit Auth via OIDC + Passport | **Supabase Auth** + Sign in with Apple | Replaced. |
| AI | OpenAI via Replit AI Integrations proxy | Direct OpenAI (or compat provider) | Replaced. |
| Streaming | SSE | SSE over `URLSession.bytes` | Preserved as wire protocol; client rewritten. |
| Images | `gpt-image-1` via Replit proxy | `gpt-image-1` direct (or equivalent) | Preserved as capability. |

## 4. Must preserve

These items are the product, not the implementation. Every must-preserve
survives the port in form and feel.

1. **Canonical Food Object (CFO) data model.** The `food_items` table layout —
   `canonical_name`, `display_name`, `quantity`, `category`, `attributes`,
   `flexibility`, `usage_context.role`, `inventory_state`, `sourcing`,
   `metadata` — is the single inventory of food in the system. All four
   roles (`inventory`, `shopping`, `planned`, `ingredient`) ride on it.
2. **AI tool-calling contract.** The four tool names, their argument shapes,
   and their semantics:
   - `update_ingredients` (CFO add/remove)
   - `create_meal_plan` (weekly plan with day-of-week)
   - `create_shopping_list` (CFO derivation)
   - `update_meal` (single day swap)
3. **Feature set documented in `replit.md` at the pinned commit.** Each
   feature is treated as a "must keep, may improve". Notably:
   - Conversational chat with streaming tool calls.
   - Weekly meal-plan generation, with edit-mode approval per day and
     selective regeneration of un-approved days.
   - Recipe detail page with auto-generated full recipe and AI-generated
     food photography.
   - Master cookbook (save / browse / edit / delete recipes, ingredient
     helper with CFO suggestions).
   - Shopping list generated from current meal plan + inventory diff.
   - Multi-conversation chat history per user.
   - Calendar month view with drill-down to weeks.
4. **Soft inventory semantics.** Ingredient status — `confirmed`, `likely`,
   `unknown`, `out` — with confidence scores survives unchanged.
5. **Image-prompt-not-bytes storage.** Recipe images are stored as generation
   prompts and regenerated on demand; this stays.

## 5. Must cut

These are Replit-platform leakage and have no place in the iOS port.

1. **The entire `server/replit_integrations/` and `client/replit_integrations/`
   trees** — they exist only because of the Replit IDE.
2. **`.replit`, `replit.md` as a runtime config**, the Replit Vite plugins
   (`@replit/vite-plugin-*`), and any `import` of those plugins. `replit.md`
   itself is preserved as a historical document but is **not** a runtime
   input to the port.
3. **Replit Auth (OIDC) and the `connect-pg-simple` session table** —
   replaced wholesale by Supabase Auth + Sign in with Apple.
4. **`scripts/` and `script/` shell helpers** that drive the Replit
   build/dev loop — replaced by the iOS toolchain + a Go service.
5. **Replit audio integration** — if voice features are kept (open
   decision §10), the iOS app uses native `Speech` / `AVFoundation`.

## 6. Open decisions for the Analyst / Architect to resolve

All five are now resolved as ADRs in `.devcore/memory/decisions/`. Listed
here for traceability:

1. **Voice features** → **ADR-0001** — *Cut* (voice path was dead
   scaffolding, never wired in). Accepted at the behavior_spec gate.
2. **OpenAI provider** → **ADR-0002** — *Direct OpenAI* (tool-call shape,
   image generation, SSE all port unchanged). Accepted at the
   behavior_spec gate.
3. **Auth** → **ADR-0003** — *Sign in with Apple + Supabase email/OTP*.
   JWT-bearer to the Go API. Accepted at the behavior_spec gate.
4. **Cookbook image storage** → **ADR-0004** — *Supabase Storage +
   on-device LRU* (128 MB cap). Generate at save, not at view. Accepted
   at the behavior_spec gate.
5. **Meal-plan week nav** → **ADR-0005** — *SwiftUI NavigationStack, no
   Universal Links v1*. Accepted at the behavior_spec gate.

The Analyst's reading turned up six more open questions; they belong to
the **Architect**, are listed at the bottom of
`domain/sous-chef-behaviors.md` §9, and are resolved as ADRs `0006–...`
before the contract gate.

## 7. Execution shape

Three tracks converge on one shared contract. The Architect produces the
contract first; the human approves it at the `contract` gate; then three
Builder instances proceed in parallel:

- **data** → Supabase schema (translated from `shared/schema.ts`), migrations,
  Auth setup, RLS policies.
- **backend** → Go API on AWS implementing the contract, including the
  SSE chat path and the four tool calls.
- **ios** → SwiftUI app rendering against the contract; navigation, chat,
  plan, calendar, recipe, cookbook, shopping; Sign in with Apple.

Reviewer and Verifier run continuously; integration and a device-run on
Dave's iPhone close the port out.

## 8. Gates (Phase 3 → 4 boundary)

Per `devcore.config.yaml`:

| Gate | After / Before | Owner | Status |
|------|----------------|-------|--------|
| `behavior_spec` | after | Analyst | **Passed** — 2026-05-24 |
| `contract` | after | Architect | **Passed** — 2026-05-24 |
| `track_plan` | after | Conductor | Pending — three track plans in `plan/` are next |
| `deploy` | before | Verifier | Pending — Phase 5 |

**Phase 3 closed at the contract gate.** The next gate (`track_plan`) is the
Phase 3 → 4 bridge: three track plans (backend, data, ios) each independently
buildable against the contract.

## 9. Out of scope (this workload only)

- Porting the iOS app to **iPad** layouts — iPhone is the only target.
- A new web client — the existing web app continues to operate against
  its existing backend until the iOS app is shipped; cutover is a later
  decision.
- A general meal-planner platform — Sous Chef AI is for Dave's household,
  not a product launch.
- The voice-input path beyond a decision in §6.1.

## 10. Notes for agents

- **Read the pinned commit, not `main`.** Drift since the pin is out of
  scope.
- **Prefer behaviour over implementation.** `replit.md`,
  `server/routes.ts`, `server/openai.ts`, `shared/schema.ts`, and the
  client pages are the seed; everything else is mechanism.
- **Record every decision as an ADR** in `.devcore/memory/decisions/`.
- **Surface every open question to the human via a gate** — don't paper
  over ambiguity by picking the cheaper option.
