---
type: plan
title: Integration synthesis — sous-chef-ios cross-track choreography
status: accepted
owner: conductor
workload: sous-chef-ios
last_updated: 2026-05-24
contract: contract/contract.md
---

# Integration Synthesis — Sous-Chef iOS

This document maps the **touchpoints** where the three Builder tracks
(backend, data, iOS) meet, plus the open questions whose answers determine
how those touchpoints work. It is the Conductor's output at the `track_plan`
gate — the three track plans are independently buildable against the
contract, but they share a handful of cross-track concerns that no single
track plan can pin alone.

Read after `track-backend.md`, `track-data.md`, and `track-ios.md`. Read
alongside `contract/contract.md`. This doc does **not** restate any track's
internal plan — it only describes the seams.

---

## §1 Cross-track conflicts (resolved at the `track_plan` gate)

### 1.1 RLS-aware connection vs. service-role + app-level filtering — RESOLVED → ADR-0011 (JWT-aware)

The most consequential open question across the three plans. Backend
(track-backend §9 Q2) defaults to a **service-role connection** — the Go
service connects as a superuser-equivalent and does `WHERE user_id = $1`
in every query. Data (track-data §6 and §9) wrote RLS policies assuming
**JWT-aware connections** — `auth.uid()` flows through and Postgres
enforces per-row filtering.

The two designs are mutually exclusive at the integration boundary:

| Choice | Backend impact | Data impact | Trust model |
|---|---|---|---|
| **JWT-aware** (set `request.jwt.claim.sub` per request, use `auth.uid()` in RLS) | Connection pool must reset session GUCs per request; extra round trip per query | RLS policies are the source of truth; a Go bug cannot leak data | "Defense in depth" — Postgres enforces |
| **Service-role + app filtering** | Standard `pgxpool`; simpler connection management | RLS policies become belt-and-suspenders; Go bugs CAN leak data | "Trust the Go code" — app enforces |

Both are valid; **the choice must be the same on both sides**. The data
track has already written RLS-aware policies; if Backend stays
service-role, those policies stop being load-bearing and become
documentation of intent.

**Resolved at the gate: JWT-aware. Locked in ADR-0011** (`decisions/0011-jwt-aware-connection.md`).
The backend track plan's §3 *Architecture* is amended via a NOTE block at
the top of `plan/track-backend.md` pointing at ADR-0011; one task is added
to the Phase-4 backend task list (`internal/store.WithClaims` helper).

### 1.2 Tool-call schema location

Backend (§9 Q5 implied) and the Architect's cross-track risk #5 both
call for a single Go `const` per tool-call schema, referenced by both
the production AI-dispatch code and tests. iOS (§3) consumes only the
streamed `tool_result` events — it does **not** see the request-side
schemas.

**Resolution:** owned by Backend. The track-backend plan adopts the
single-Go-constant pattern (its task list already has the AI-client
package); no change needed beyond confirming this is the binding.
No ADR needed.

### 1.3 Recipe markdown format ownership

Behavior spec §4.4 defines the canonical recipe markdown shape. The
backend **produces** it (the AI is prompted to follow it; the backend
returns it verbatim); iOS **consumes** it (the iOS plan §3 builds a
hand-rolled `SousChefMarkdown` parser over a `[RecipeBlock]` AST).

**Resolution:** the format string lives in the **behavior spec §4.4**
only — not duplicated into the contract. iOS's parser is tested
against a corpus of real recipes pulled from the source repo (iOS plan
§3). Backend's prompt instructs the AI to follow the same format.
Drift is detected at integration time by a smoke test that round-trips
a generated recipe through the iOS parser.

If the format ever needs to evolve, it becomes a contract change and
both sides update; the behavior spec stays the source of truth.

---

## §2 Cross-track plumbing (settled by the contract; recap for the gate)

### 2.1 `clientWeekStartDate` / `week_start_date` (ADR-0010)

- **iOS** computes Monday-of-current-week in the user's local timezone
  (`Calendar.current.firstWeekday = 2`), formats as ISO date,
  attaches to chat sends and to `create_meal_plan` / `update_meal`
  tool-call arguments.
- **Backend** validates Monday-ness, propagates to tool dispatch,
  stores in `meal_plans.week_start_date`.
- **Data** declares `meal_plans.week_start_date` as `date NOT NULL`
  with a `CHECK (extract(dow from week_start_date) = 1)`.

No conflict; documented for traceability.

### 2.2 Cookbook save → image generation (ADR-0004)

This section was rewritten on 2026-05-25 to match the contract.
The original wording said the recipe row was rolled back if image
generation failed; the contract (§8.4) actually persists the row
with `image_url` left null and returns 503. Reviewer-pass 0001
flagged the divergence. **Contract wins**; this doc now mirrors it.

- **iOS** posts to `POST /api/kitchen/cookbook`, shows a spinner
  state ("generating image…"), and disables the save button until
  the response arrives (2–10 seconds). On a 503 response (Storage
  upload failure per contract §8.4), the recipe **is** saved — the
  row persists with `image_url` null. The UI surfaces a "save
  succeeded; image will retry" affordance and exposes a manual
  retry via `POST /api/kitchen/cookbook/:id/regenerate-image`. The
  user-typed content (title, recipe markdown) is never lost because
  OpenAI hiccuped.
- **Backend** writes the cookbook row first, then attempts the image
  generation + Storage upload. On image-gen / upload failure, returns
  503 with the contract §3.5 `internal_error` envelope **without
  rolling back** the row (per contract §8.4). The row's `image_url`
  stays null; a later regenerate-image call fills it in.
- **Data** owns the `cookbook-images` Storage bucket and the
  `BEFORE DELETE` trigger on `cookbook_recipes` that evicts the
  storage object in the same transaction.

No remaining conflict — contract, integration doc, and all three
track plans now agree.

### 2.3 SSE wire format (contract §6)

- **Backend** writes `data: <json>\n\n` framed events with a 20s
  heartbeat (track-backend §3).
- **iOS** consumes via `URLSession.bytes` (track-ios §3), maintaining
  a buffer for partial-event handling.
- **Data** is uninvolved.

Heartbeat interval (20s) must be **under** the ALB idle timeout
(track-backend §9 Q3 — confirm with Dave that staging ALB is set to
600s or higher). Until confirmed, treat the SSE path as gated on Q3.

### 2.4 Supabase Auth provider config (track-ios §9 open Q1)

- **Dave-actions:**
  - Apple Developer Program enrollment (already done? — confirm).
  - Create the Apple Services ID and a `.p8` key.
  - Paste both into Supabase Dashboard → Authentication → Providers →
    Apple.
- **iOS task A3 (SIWA)** is blocked on this.
- **iOS task A4 (email/OTP)** and all non-auth tasks can proceed
  in parallel without waiting.

This is a Conductor item to surface, not a Builder item.

---

## §3 Open questions consolidated (18 total)

Grouped by who can answer.

### 3.1 Conductor decisions — resolved at the `track_plan` gate

| # | From | Question | Decision |
|---|---|---|---|
| 1 | backend Q2 + data §6 | RLS-aware vs service-role connection | **JWT-aware** — ADR-0011 |
| 2 | iOS Q4 | TabView shell vs. navigation drawer | **TabView** — five tabs (Chat, Plan, Calendar, Cookbook, Shopping); matches behavior spec §3 |
| 3 | data Q5 | Disable PostgREST entirely | **Disabled** — iOS goes through the Go API only; shrinks attack surface |

### 3.2 Dave-actions (block specific tasks; not blocking the gate)

| # | From | Action |
|---|---|---|
| 4 | iOS Q1 | Apple Developer enrollment + Services ID + .p8 key + Supabase Apple provider config — blocks iOS task A3 |
| 5 | backend Q3 | Confirm AWS staging ALB idle-timeout settable to 600s — gates SSE in staging |
| 6 | iOS Q2 | Decide dev/staging/prod `APIBaseURL` values — needed at iOS Phase B2 |

### 3.3 Phase-4 Builder decisions (cheap to defer; surface so Dave knows)

| # | From | Question | Default |
|---|---|---|---|
| 7 | data Q1 | Pin Supabase CLI version | Latest at Phase 4 start, pinned then |
| 8 | data Q2 | UUIDv7 source — `pg_uuidv7` extension vs `gen_random_uuid()` fallback | `pg_uuidv7` if available in Supabase, else fallback |
| 9 | data Q3 | Cookbook delete trigger: direct `storage.objects` SQL vs Storage REST | Direct SQL (atomic same-transaction) |
| 10 | data Q4 | `auth.users` delete semantics — hard vs soft | Hard (we have no compliance reason to retain) |
| 11 | backend Q1 | Pin dated OpenAI model in prod vs float | Pin in prod, float in staging |
| 12 | backend Q4 | `/regenerate-image` ownership param | Require `cookbookRecipeId` — symmetric with the save endpoint |
| 13 | backend Q5 | Title auto-generation is deterministic truncation, not LLM | Confirmed: deterministic — matches behavior spec §4.9 |
| 14 | backend Q6 | (rest folded into ADRs already; check track-backend §9) | — |
| 15 | backend Q7 | (rest folded; check track-backend §9) | — |
| 16 | backend Q8 | (rest folded; check track-backend §9) | — |
| 17 | iOS Q3 | Inactivity window for fresh-chat-on-session-start (contract §9.7) | 15 minutes |
| 18 | iOS Q5 | Disk-cache location | `~/Library/Caches/SousChef/Images/` (purgeable) |

---

## §4 Phase 4 starting order (recommended)

When Dave approves the `track_plan` gate and Phase 4 opens, the
following order minimizes blocking:

1. **Week 1 — parallel foundations.** Data: Supabase project + Auth +
   the six tables + RLS (no app code consuming yet). Backend: Go
   service shell, JWT middleware, `/healthz`. iOS: Xcode project,
   `SousChefKit` package skeleton, email/OTP login (defer SIWA until
   Dave-action #4 lands).
2. **Week 2 — first end-to-end slice.** Data: storage bucket + delete
   trigger. Backend: read endpoints (conversations, meal-plans).
   iOS: meal-plan tab read-only against backend.
3. **Week 3 — AI surface.** Backend: AI client + chat SSE + the four
   tools. iOS: chat tab. The contract's most complex section comes
   online together.
4. **Week 4 — close the loop.** Cookbook + image gen end-to-end,
   shopping list, polish, TestFlight.

This order is a suggestion; the actual cadence is each Builder's
call. The shape — foundation → read path → write/AI path → polish —
is what matters.

---

## §5 `track_plan` gate — outcome

Passed 2026-05-24. Dave's decisions captured above:
1. Three track plans accepted as written.
2. **ADR-0011** — JWT-aware connection.
3. **TabView** iOS shell.
4. **PostgREST disabled.**
5. The three Dave-actions in §3.2 will land before the tasks they block.

The eleven Phase-4 Builder decisions in §3.3 are documented with
defaults; each Builder uses its default unless Dave objects in
Phase 4.

**Phase 3 closed.** Phase 4 opens whenever Dave chooses to begin
implementation.
