---
type: plan
title: Data track plan — sous-chef-ios
status: accepted
owner: builder.data
workload: sous-chef-ios
last_updated: 2026-05-24
contract: contract/contract.md
---

# Data track plan — sous-chef-ios

## §1 What this plan is

This is the data Builder's plan for the Sous Chef iOS port. It is read at two
points:

- The **Conductor** reads it at the `track_plan` gate (Phase 3 → 4 bridge) to
  confirm the data track is independently buildable against the contract.
- The **data Builder** reads it in Phase 4 as the local index over the work —
  the ordered task list, the file layout, the acceptance criteria each task
  must hit.

It commits to: the Supabase schema realized exactly as contract §4 specifies,
RLS that no test caller can bypass, a `cookbook-images` Storage bucket with
per-user path scoping, Sign in with Apple + email/OTP configured, and a
declarative-schema migration workflow Dave's three environments (dev /
staging / prod) all share.

It does **not** commit to the Go API (backend track) or the SwiftUI app (iOS
track). Where this track meets either, the contract is the only interface;
this plan does not look across it.

## §2 Scope

**In scope (data track delivers):**

1. Three Supabase projects: `sous-chef-dev`, `sous-chef-staging`,
   `sous-chef-prod`. Each provisioned from the same `supabase/` directory.
2. The six tables from contract §4 as declarative SQL in
   `supabase/schemas/*.sql`:
   `food_items`, `meal_plans`, `meal_plan_days`, `cookbook_recipes`,
   `shopping_lists`, `shopping_list_items`, `kitchen_conversations`,
   `kitchen_messages`. (Eight tables total when conversation/messages are
   counted as their own pair; "six" in the spec aggregates the pairs — the
   contract is authoritative.)
3. All CHECK constraints and unique indexes named in contract §4 — most
   notably the CFO `usage_context.role` enum check (admits all four values
   per ADR-0009), `category.primary` 9-value check, JS-Sunday `day_of_week`
   range, `extract(isodow from week_start_date) = 1` Monday check (per
   ADR-0010).
4. RLS policies on every table — `SELECT` / `INSERT` / `UPDATE` /
   `DELETE` policies per table, scoped `TO authenticated`, gated on
   `user_id = (select auth.uid())` directly for owner tables and through a
   `SECURITY DEFINER` helper in a private `app_private` schema for the
   ownership-flows-through-parent tables (`meal_plan_days`,
   `shopping_list_items`, `kitchen_messages`).
5. Storage bucket `cookbook-images` (private, 8 MB cap, MIME pinned to
   `image/png`) with `storage.objects` RLS policies scoping access by
   `(storage.foldername(name))[1] = (select auth.uid())::text` (per ADR-0004
   and contract §4.4).
6. A `BEFORE DELETE` trigger on `public.cookbook_recipes` that issues a
   `delete from storage.objects where bucket_id = 'cookbook-images' and
   name = user_id::text || '/' || id::text || '.png'` so the backend never
   has to call the Storage API for cookbook deletes (contract §5.6
   `DELETE /api/kitchen/cookbook/{id}`).
7. A shared `app_private.set_updated_at()` trigger function, applied as a
   `BEFORE UPDATE` trigger on every table with an `updated_at` column
   (contract §4.8). And a hardening trigger on `meal_plan_days` that nulls
   `recipe_content` and `recipe_image_prompt` when `meal_name` changes
   (defense-in-depth for the cache-clearing invariant, contract §4.3).
8. Append-only enforcement on `kitchen_messages`: no UPDATE or DELETE
   policies for `authenticated` (contract §4.6) — INSERT and SELECT only.
9. Supabase Auth provider config: SIWA (Service ID, Apple Team ID, Key ID,
   `.p8` private key as a Supabase secret) and email/OTP (six-digit code,
   no magic-link). Same configuration in all three environments, keyed off
   environment-specific Apple Service IDs where required.
10. A small seed-data script for `dev` only: one synthetic user (created via
    the Supabase Admin API in a CI-only script — **never** committed with a
    real `.p8`), a sprinkling of CFO `inventory` rows, one prior-week meal
    plan, and one cookbook recipe so the iOS and backend Builders have a
    realistic dataset.

**Out of scope (this track does not deliver):**

- The Go API (backend track) — including the JWT verification middleware,
  PostgREST vs `pgx` access pattern, and OpenAI tool dispatch.
- The SwiftUI app, SIWA client integration, JWT exchange (iOS track).
- The OpenAI integration and the `gpt-image-1` calls (backend track).
- CloudWatch metrics, AWS infra, anything not Supabase (backend track).
- Performance load-testing the schema (Verifier).

## §3 Architecture

### §3.1 Project layout

The data track ships a single directory in the
`sous-chef-ios` output repo:

```
supabase/
  config.toml                    -- Supabase CLI config; checked in per dc-04
  schemas/
    00_extensions.sql            -- create extension if not exists pgcrypto, uuidv7
    01_app_private.sql           -- create schema app_private; helper fns
    10_food_items.sql            -- table + CHECK constraints + indexes
    11_meal_plans.sql
    12_meal_plan_days.sql
    13_cookbook_recipes.sql
    14_shopping_lists.sql
    15_shopping_list_items.sql
    16_kitchen_conversations.sql
    17_kitchen_messages.sql
    20_triggers.sql              -- updated_at, meal-cache-clear
    30_storage_buckets.sql       -- bucket create + size/MIME constraint
    40_rls_food_items.sql        -- one file per table; one policy per op
    41_rls_meal_plans.sql
    42_rls_meal_plan_days.sql
    43_rls_cookbook_recipes.sql
    44_rls_shopping_lists.sql
    45_rls_shopping_list_items.sql
    46_rls_kitchen_conversations.sql
    47_rls_kitchen_messages.sql
    50_rls_storage.sql           -- storage.objects policies for cookbook-images
    60_cookbook_storage_delete_trigger.sql
  migrations/                    -- generated by `supabase db diff`; never hand-edited
    20260524120000_initial.sql   -- first generation; one big migration
    ...                          -- every subsequent change is a new file
  seed/
    dev_only.sql                 -- seed data; **only** runs against dev project
  README.md                      -- how to run, where the keys live
```

Each `.sql` file carries a `dc-01` header comment (what / depends on /
depended on by / why) per `CODING_STANDARDS.md` §dc-01.

### §3.2 Environment separation

**Three separate Supabase projects**, not three schemas in one project, not
table-name prefixes. Reasons:

- Auth provider keys (Apple `.p8`, SIWA Service IDs) are project-level in
  Supabase — environments must not share them.
- RLS bugs in dev cannot leak prod data if prod is a separate project.
- The publishable/anon key the iOS app embeds differs per environment, and
  is selected by build configuration.
- This is the standard Supabase pattern (`dc-04` "Develop against the local
  stack, never against production"; production is its own project).

The same `supabase/` directory drives all three projects via the Supabase
CLI's `--linked` workflow:
`supabase link --project-ref <env-ref>` + `supabase db push`.

### §3.3 Migration workflow

- The desired state lives in `supabase/schemas/*.sql` (declarative).
- New change: edit a `schemas/` file → run `supabase db diff -f
  <descriptive_name>` → a new migration file appears in
  `supabase/migrations/` → that file is committed verbatim.
- **An applied migration is never edited.** This is enforced by:
  1. `CODING_STANDARDS.md` §dc-04 ("Never edit an applied migration").
  2. A pre-commit hook that fails if any file in `supabase/migrations/`
     other than the newest is modified (script: `scripts/check-migrations.sh`,
     part of this track's deliverable).
  3. `supabase db push` refuses to apply if checksums differ from the
     database's `supabase_migrations.schema_migrations` table.

### §3.4 Local development

Local Supabase stack via `supabase start` (Docker). The iOS-track and
backend-track Builders run their integration tests against this local
stack. CI runs against an ephemeral local stack per job; deployment to a
real environment is a deliberate `supabase db push` against the linked
project.

### §3.5 CI/CD

A GitHub Actions workflow in this track's output (`.github/workflows/
supabase.yml`) does, on PR:

1. `supabase db start` → `supabase db reset` (applies all migrations to a
   clean DB).
2. Run `pgTAP` test files in `supabase/tests/` (RLS policy assertions; see
   §5 task 9).
3. On merge to `main`, run `supabase db push --linked` against staging.
4. Promotion to prod is a manual `workflow_dispatch` job — never
   auto-promoted.

## §4 Stack & toolchain

| Tool | Version | Why |
|---|---|---|
| PostgreSQL | 17.x (whatever the Supabase project ships) | Per `dc-04`. The `uuidv7()` function is provided by the `uuidv7` extension or the built-in `gen_random_uuid()` fallback; we use `uuidv7` per `dc-04`. |
| Supabase CLI | ≥ 1.200 | Declarative schemas (`supabase db diff`), local stack, linked-project push. Per `dc-04`. |
| pgTAP | 1.3+ | RLS policy unit tests run in CI. |
| Docker | engine 24+ | Required by `supabase start`. |
| `psql` | 17.x | Connection string smoke checks; not used as a migration runner. |

**Why the Supabase CLI over hand-rolled SQL files or golang-migrate:**

- `dc-04` mandates it: "*Use the Supabase CLI. Adopt declarative schemas:
  desired state lives in `supabase/schemas/*.sql`; `supabase db diff`
  generates versioned migrations in `supabase/migrations/`*."
- It generates migrations *from the diff*, so the SQL in
  `schemas/` is human-authored and the migration body is mechanically
  derived. This is the closest a Postgres workflow gets to "the schema is
  the spec".
- It owns the auth, storage, and `auth.users` extensions natively — a
  generic Postgres migration tool would need bespoke handling for each.
- The backend track will use a `pgx` connection (or PostgREST) at runtime;
  this is independent of the migration tool. The two tracks share only the
  applied schema.

## §5 Task tree

Each task lists: **dependencies → acceptance check** (contract §4 table /
ADR satisfied). Order is execution order.

1. **Provision dev Supabase project.**
   Deps: none. Accept: `supabase link --project-ref` succeeds; the
   publishable and secret keys are stored in `1Password` (per `dc-04`) and
   never committed. Tracks contract §2.

2. **Configure Supabase Auth providers in dev (SIWA + email/OTP).**
   Deps: 1. Accept: dashboard shows both providers green; a smoke-test
   `signInWithIdToken` against a SIWA sandbox token issues a JWT with
   correct `iss`. Tracks ADR-0003 + contract §2.

3. **Wire `supabase/` directory + CLI + local stack.**
   Deps: 1. Accept: `supabase start` on the data Builder's box runs the
   stack; `supabase db reset` rebuilds from `schemas/`. Tracks `dc-04`.

4. **Write `00_extensions.sql` + `01_app_private.sql`.**
   Deps: 3. Accept: `uuidv7()` is callable; `app_private` schema exists and
   is not exposed via PostgREST (per `dc-04` "non-exposed schema"); the
   `app_private.set_updated_at()` trigger function compiles. Tracks
   `dc-04`.

5. **Write `food_items` schema (`10_food_items.sql`).**
   Deps: 4. Accept: all six CHECK constraints from contract §4.2 are
   present (canonical_name lowercase + nonempty; category.primary in
   9-value enum; usage_context.role in 4-value enum per ADR-0009;
   inventory_state.status in 4-value enum; metadata.created_by in
   ai/user); `food_items_user_canonical_role_uniq` and
   `food_items_user_id_idx` are present. Tracks contract §4.2 + ADR-0006 +
   ADR-0009.

6. **Write `meal_plans` + `meal_plan_days` schemas
   (`11_*.sql`, `12_*.sql`).**
   Deps: 4. Accept: `week_start_is_monday` CHECK fires on a Tuesday date;
   `meal_plans_user_week_uniq` and `meal_plan_days_plan_day_uniq` enforced;
   FK `meal_plan_days.meal_plan_id → meal_plans(id) ON DELETE CASCADE`.
   Tracks contract §4.3 + ADR-0010.

7. **Write `cookbook_recipes` schema (`13_cookbook_recipes.sql`).**
   Deps: 4. Accept: `image_url` column present (per ADR-0004; no
   `thumbnail_url`); `cookbook_recipes_user_title_lower_idx` lower-case
   index present (contract §4.4 + §9.8). Tracks contract §4.4 + ADR-0004.

8. **Write `shopping_lists` + `shopping_list_items` schemas
   (`14_*.sql`, `15_*.sql`).**
   Deps: 4, 6. Accept: `week_start_is_monday_or_null` CHECK fires on
   Tuesday but admits NULL; `shopping_lists_user_week_uniq` partial unique
   index (`WHERE week_start_date IS NOT NULL`); `category` CHECK enum
   matches the CFO 9 values; `meal_plan_id` FK has `ON DELETE SET NULL`.
   Tracks contract §4.5 + ADR-0007.

9. **Write `kitchen_conversations` + `kitchen_messages` schemas
   (`16_*.sql`, `17_*.sql`).**
   Deps: 4. Accept: `role` CHECK admits exactly `user|assistant`;
   `kitchen_messages_conversation_created_idx` exists. Tracks contract
   §4.6 + ADR-0008.

10. **Write `updated_at` + cache-clear triggers (`20_triggers.sql`).**
    Deps: 5–9. Accept: an UPDATE on `meal_plan_days.meal_name` nulls
    `recipe_content` and `recipe_image_prompt` even when the application
    forgets; every table with `updated_at` ticks it on UPDATE. Tracks
    contract §4.3 + §4.8.

11. **Enable RLS + write per-table policies (`40_*.sql`–`47_*.sql`).**
    Deps: 5–9. Accept: every table has `ENABLE ROW LEVEL SECURITY` and
    `FORCE ROW LEVEL SECURITY`; exactly four policies per table
    (`SELECT/INSERT/UPDATE/DELETE`), all `TO authenticated`; `auth.uid()`
    is wrapped as `(select auth.uid())` per `dc-04`; ownership-via-parent
    tables use the `SECURITY DEFINER` helper in `app_private`; `EXPLAIN
    ANALYZE` shows index use (not seq scan) on the `user_id` column.
    `kitchen_messages` has only `INSERT` + `SELECT` policies (no UPDATE,
    no DELETE — append-only per contract §4.6). Tracks contract §4 RLS
    intent + `dc-04`.

12. **Create `cookbook-images` Storage bucket
    (`30_storage_buckets.sql`).**
    Deps: 3. Accept: bucket exists; is **private**; size cap 8 MB; MIME
    pinned to `image/png` via the bucket's `allowed_mime_types`. Tracks
    ADR-0004 + contract §4.4.

13. **Storage bucket RLS (`50_rls_storage.sql`).**
    Deps: 12. Accept: four policies on `storage.objects` filtered
    `bucket_id = 'cookbook-images'`, scoped by
    `(storage.foldername(name))[1] = (select auth.uid())::text`; user A
    cannot SELECT/INSERT/UPDATE/DELETE an object under user B's prefix.
    Tracks ADR-0004 + `dc-04`.

14. **Cookbook delete cascade trigger
    (`60_cookbook_storage_delete_trigger.sql`).**
    Deps: 7, 12. Accept: deleting a `cookbook_recipes` row whose
    `image_url` is set removes the corresponding object from the
    `cookbook-images` bucket within the same transaction; deleting a row
    whose `image_url` is null is a no-op. Tracks contract §5.6 `DELETE
    /api/kitchen/cookbook/{id}` + ADR-0004.

15. **Cut the initial migration.**
    Deps: 4–14. Accept: `supabase db diff -f initial` emits a single
    migration file in `supabase/migrations/`; `supabase db reset` followed
    by `supabase db diff` reports "no schema changes" (the declarative
    state and the migration are in sync). Tracks `dc-04`.

16. **Provision staging + prod projects; link.**
    Deps: 15. Accept: `supabase db push --linked` succeeds against
    staging; manual gated push to prod. Two more sets of publishable/secret
    keys land in 1Password. Tracks ADR-0003 + workload-spec §8 gates.

17. **Configure Auth providers in staging + prod.**
    Deps: 16. Accept: same as task 2 but for the new environments.

18. **pgTAP RLS test suite (`supabase/tests/`).**
    Deps: 11, 13. Accept: tests pass that assert (a) user A cannot SELECT
    user B's `food_items`; (b) user A cannot INSERT a `meal_plan_days`
    under user B's `meal_plans`; (c) `kitchen_messages` cannot be UPDATEd
    or DELETEd by `authenticated`; (d) Storage bucket access is per-user;
    (e) the cookbook delete trigger removes the storage object. CI fails
    on any miss. Tracks contract §4 + `dc-07` checklist.

19. **Dev seed script (`supabase/seed/dev_only.sql`).**
    Deps: 15. Accept: seeds run only against the dev project (guarded by a
    check on `current_database()` or by the CI workflow); one synthetic
    user + ~5 CFO inventory rows + 1 meal plan + 1 cookbook recipe land.
    Tracks workload-spec §7 execution shape (backend smoke).

20. **Migration ergonomics: pre-commit hook + README.**
    Deps: 15. Accept: `scripts/check-migrations.sh` refuses commits that
    edit a non-newest migration; `supabase/README.md` documents the
    add-a-new-migration flow, the local stack flow, and the
    promotion-to-prod flow. Tracks `dc-00` (a reader handed the repo
    understands it) + `dc-04`.

## §6 Integration points

### §6.1 This track produces → backend (Go) consumes

- **The schema.** The backend reads/writes via `pgx` (or Supabase
  PostgREST; backend-track's call). Every table, column, JSONB shape, and
  CHECK constraint in contract §4 is honored.
- **The cookbook image storage convention.** The backend's `image.Generate`
  uploads to `cookbook-images/{user_id}/{cookbook_recipe_id}.png` with
  `image/png` content-type. The publicly-readable URL the Storage API
  returns is what the backend writes to `cookbook_recipes.image_url`.
- **The cookbook delete trigger semantics.** The backend's
  `DELETE /api/kitchen/cookbook/{id}` handler **must not** call the
  Storage API to delete the object — the row delete cascades the storage
  delete via the trigger from task 14. This is documented in the
  cookbook table file's `dc-01` header.
- **The `updated_at` trigger semantics.** The backend MUST NOT write to
  `updated_at` columns; the trigger maintains them.
- **The meal-name cache-clear trigger.** The backend's `update_meal` /
  `regenerate-days` handlers MAY null `recipe_content` /
  `recipe_image_prompt` themselves; if they forget, the trigger does it.
  Defense-in-depth, not a primary mechanism.

### §6.2 This track produces → iOS consumes (indirectly)

- **Supabase Auth JWT.** The iOS app calls Supabase Auth's
  `signInWithIdToken` (SIWA path) or `signInWithOtp` (email/OTP path);
  Supabase returns a JWT signed by this track's configured project; the
  iOS app sends it as `Authorization: Bearer <jwt>` to the Go API.
- **Publishable/anon key.** Per environment, embedded by build config in
  the iOS app — the only key that touches the device.
- The iOS app does **not** speak to Postgres directly. PostgREST is
  available (RLS-protected) but v1 routes everything through the Go API.

### §6.3 Cross-track risks this track owns

- **Storage RLS + cookbook delete cascade.** The trigger from task 14 is
  the only mechanism that prevents orphan PNGs in Storage. It must fire on
  every delete path — including a future cascade from `auth.users`
  deletion (which already cascades to `cookbook_recipes` via FK; the
  trigger fires on those row-deletes too).
- **`clientWeekStartDate` plumbing — schema side.** The
  `meal_plans.week_start_date` column is a Postgres `date` (per ADR-0010);
  the iOS client computes Monday in the user's local zone and sends ISO
  `YYYY-MM-DD`; the Go backend stores it verbatim. The
  `week_start_is_monday` CHECK constraint from task 6 is the
  database-side enforcement that this convention is honored — it fails
  fast if a backend bug sends a Tuesday date.

## §7 Acceptance criteria — "track done" for the gate

1. Dev, staging, and prod Supabase projects provisioned and linked.
2. All eight tables (the contract's six logical units) match contract §4
   byte-for-byte. Verified by reading the generated migration against the
   contract.
3. Every CHECK constraint and every unique/secondary index from contract
   §4 is present and named as specified.
4. RLS is enabled and forced on every public-schema table. The pgTAP suite
   (task 18) passes — including the "user A cannot read user B" cases for
   every table.
5. `cookbook-images` bucket exists with size + MIME constraints; storage
   RLS policies pass the pgTAP cross-user tests; the cookbook delete
   trigger removes the corresponding storage object in the same
   transaction.
6. Supabase Auth providers SIWA + email/OTP are configured and emit JWTs
   in all three environments.
7. `supabase db diff` against the linked dev project reports "no
   changes" — the declarative schema and the applied state agree.
8. The dev project carries the seed data (task 19); the backend Builder
   can run a smoke read against `food_items` and get rows back.
9. `scripts/check-migrations.sh` is wired into pre-commit and CI; editing
   an already-applied migration fails the commit.

## §8 Risk register (track-specific)

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| R1 | RLS policy gap — a forgotten policy or a missed FORCE leaks rows cross-user. | Medium | Critical (data leak) | Task 18's pgTAP cross-user assertions are CI-gating. `dc-04`'s "RLS default-on" is enforced by a per-table assertion in the test suite that `pg_class.relrowsecurity` is true. |
| R2 | Migration drift — dev/staging/prod diverge. | Medium | High | `supabase db diff` is the only way to produce a migration; never hand-edit; `scripts/check-migrations.sh` blocks edits to applied files; CI runs `supabase db reset` + `diff` on every PR. |
| R3 | CFO JSONB validation gap — a malformed `usage_context` or `inventory_state` slips into a row. | Medium | Medium | Per-key CHECK constraints (`usage_context->>'role' IN (...)`) catch the enum-value cases; the shape (presence of required keys) is enforced by the Go backend at the wire boundary (per `dc-04` "JSONB only for genuinely schemaless data" — the CFO has `attributes` for that; the enumerated keys are validated). We do **not** add trigger-level JSONB shape validation; we trust contract-conformant tool output and rely on the backend's `decoder.DisallowUnknownFields()`. |
| R4 | SIWA provider config is fiddly — the Apple Service ID, Team ID, Key ID, and `.p8` private key must all align; the bundle ID must match the iOS app's. | High | High (blocks every login) | Task 2 / 17 each have a manual smoke check; the `.p8` lives only in 1Password and Supabase Vault (never in the repo). A runbook in `supabase/README.md` documents the Apple Developer console steps. |
| R5 | Storage delete trigger fires but the storage row is in a separate transaction. | Low | Medium (orphan PNGs) | `storage.objects` is a Postgres table — a trigger on `public.cookbook_recipes` that issues `DELETE FROM storage.objects WHERE …` runs in the **same** transaction. The pgTAP suite asserts atomicity (delete a recipe, assert the object is gone in the same transaction). |
| R6 | Local stack drift vs cloud — `supabase start` ships a Postgres minor that differs from the cloud project. | Low | Low | CI runs against `supabase start` *and* runs a `supabase db push --dry-run` against a sacrificial cloud project as a quarterly check. |
| R7 | `uuidv7()` extension availability. | Low | Low | If the Supabase project ships `pg_uuidv7`, use it; else fall back to `gen_random_uuid()` (a deviation from `dc-04`'s preference). Task 4's acceptance check verifies which is available; a single conditional in `00_extensions.sql`. |
| R8 | Append-only `kitchen_messages` interacts badly with cascading delete from `kitchen_conversations`. | Low | Low | The append-only constraint is enforced by absence of UPDATE/DELETE *policies*, not by an immutability trigger. `ON DELETE CASCADE` from the parent still works at the FK layer (which doesn't consult RLS policies). Tested in pgTAP. |

## §9 Open questions

1. **Supabase CLI version pin.** `dc-04` mandates the CLI but doesn't pin a
   version. The buildspec.md (DevCore-wide) does not pin it either. Ask
   the Conductor to pin a minimum CLI version so the local-stack
   Postgres-minor matches the cloud project.

2. **`uuidv7()` source.** Supabase cloud now supports `pg_uuidv7` natively
   on Postgres 17, but not every regional cluster has it enabled by
   default. Ask the Conductor whether to (a) require it, (b) accept a
   `gen_random_uuid()` fallback, or (c) write our own `uuidv7()` SQL
   function in `app_private`.

3. **Cookbook delete trigger — direct `storage.objects` DELETE, or call
   the Storage REST API?** The pure-SQL approach (this plan) is atomic
   but binds us to Supabase's `storage.objects` table layout, which is a
   private Supabase contract. The REST approach is loosely-coupled but
   non-transactional. The contract is silent on this; ask the Architect
   which to prefer.

4. **`auth.users` deletion path.** The contract says every per-user table
   FKs to `auth.users(id) ON DELETE CASCADE`. Supabase's
   `auth.users` deletion is rare in practice (typically a soft-delete via
   `banned_until`). Is a hard delete from `auth.users` a supported
   operation, or should we soft-delete-only? Ask Dave at the next gate —
   this affects whether the cookbook-image trigger needs to handle
   millions-of-rows cascade scenarios.

5. **PostgREST exposure.** `dc-04` says "default to client-side queries
   through PostgREST". The contract routes the iOS app through the Go API,
   not PostgREST. Should PostgREST be **disabled** at the project level (to
   reduce attack surface) or left enabled in case the iOS app wants a
   future direct path? Conductor's call.
