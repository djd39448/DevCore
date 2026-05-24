---
type: contract
title: Sous Chef iOS — Shared Contract (API + Data Model)
status: accepted
owner: architect
workload: sous-chef-ios
last_updated: 2026-05-24
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# Sous Chef iOS — Shared Contract

The single document the **backend**, **data**, and **iOS** Builders read.
It binds all three. Every wire shape, every column, every endpoint, every
streaming event a Builder needs to implement against — without consulting
either other Builder — is here.

If a Builder needs something not specified here, the answer is **stop and
push back to the Architect**; do not invent.

---

## §1 What this is

This is the wire contract for the Sous Chef AI iOS port. It covers:

- The Supabase Postgres schema (§4).
- The REST API surface the Go backend exposes and the iOS client consumes
  (§5).
- The SSE streaming protocol for chat (§6).
- The OpenAI-style tool-calling contract the Go backend implements
  server-side (§7).
- The image generation flow (§8).
- The remaining behavior-spec ambiguities the Architect pinned down (§9).

It does **not** cover:

- The Go backend's internal architecture (`AIClient` interface,
  goroutine layout, AWS topology) — that's the backend track plan.
- The iOS app's screen hierarchy, view models, or navigation values —
  that's the iOS track plan.
- The Supabase migration script ordering or RLS policy SQL — that's
  the data track plan. The contract states **intent** per table; the
  data track writes the SQL.

**Readers:** the three Builder agents (`backend`, `data`, `ios`), the
Reviewer, the Verifier, the Conductor at the `track_plan` gate, and Dave
at the `contract` gate.

**Source pin:** `d884efae9cc150df2a58afc255b3e631d31b5d2b` of
`~/sous-chef-ai`. The behavior the contract preserves is the behavior
extracted from this commit and documented in
`domain/sous-chef-behaviors.md`.

**Coding-standard relationship:** every artifact derived from this
contract (Go file, Swift file, SQL migration) carries its `dc-01` header
and is held to `dc-00` — a reader with no other context understands the
system from the code.

---

## §2 Identity & auth

### §2.1 The user identity

A `User` is identified by a Supabase Auth UUID. The Go backend trusts the
following fields about the authenticated user:

| Field | Source | Notes |
|---|---|---|
| `id` (UUID) | JWT `sub` claim | The canonical user id. Every per-user row joins on this. |
| `email` (text) | JWT `email` claim | Contact only. Not a primary key. May be a SIWA private-relay address. |
| `created_at` | `auth.users.created_at` (read-only) | Supabase-managed. |

The Go backend **never** trusts a `user_id` from a request body, path,
query string, or header other than `Authorization`. Identity always
derives from the verified JWT.

### §2.2 Authentication

Every endpoint under `/api/kitchen/*` requires:

```
Authorization: Bearer <Supabase JWT>
```

Per ADR-0003, the JWT is issued by Supabase Auth after either SIWA
(`signInWithIdToken`) or email/OTP. The Go API verifies signatures
against Supabase's JWKS endpoint (cached, refreshed on `kid` miss).

### §2.3 Rejection behavior

| Condition | HTTP | Response body |
|---|---|---|
| Missing `Authorization` header | `401 Unauthorized` | `{ "error": "missing_authorization" }` |
| Bearer token absent / malformed | `401 Unauthorized` | `{ "error": "malformed_token" }` |
| Signature invalid | `401 Unauthorized` | `{ "error": "invalid_token" }` |
| `exp` in the past | `401 Unauthorized` | `{ "error": "expired_token" }` |
| `iss` not the configured Supabase project | `401 Unauthorized` | `{ "error": "wrong_issuer" }` |
| Token valid, user attempts to access another user's row | `403 Forbidden` | `{ "error": "not_owner" }` |

The Go API does **not** distinguish "no row exists for that id" from
"row exists but belongs to another user" — both return `404 Not Found`
with `{ "error": "not_found" }`. This prevents enumeration.

### §2.4 No logout endpoint

Per ADR-0003: token expiry plus Supabase's `signOut` SDK on the client
handles logout. The contract specifies no `POST /logout`.

---

## §3 Wire conventions

### §3.1 Transport

- Content-Type for JSON bodies: `application/json; charset=utf-8`.
- Content-Type for SSE responses: `text/event-stream; charset=utf-8`.
- Unknown JSON fields on request bodies are **rejected** with `400 Bad
  Request` and `{ "error": "unknown_field", "details": { "field":
  "<name>" } }`. (Go: `decoder.DisallowUnknownFields()`.)
- Response bodies use camelCase for field names. Database columns are
  snake_case (per `dc-04`); the Go handlers translate at the wire
  boundary. **Exception:** JSONB sub-document fields (CFO
  `usage_context`, `inventory_state`, etc.) are returned verbatim with
  the underlying snake_case keys — they are the same shape the AI tool
  calls produce and consume.

### §3.2 Timestamps and dates

- **Timestamps** (any moment with a clock): RFC 3339 in UTC with a `Z`
  suffix. Example: `2026-05-24T18:42:11.142Z`.
- **Dates** (calendar dates with no time): ISO 8601 `YYYY-MM-DD`.
  Example: `2026-05-25`. No timezone is implied; the date is a label
  in the user's calendar (ADR-0010).
- **Week-start dates** are always the Monday of the week being
  referenced (ADR-0010). The Go backend validates `time.Weekday() ==
  time.Monday`; non-Monday dates are rejected with `400 Bad Request`
  and `{ "error": "week_start_not_monday" }`.

### §3.3 Day-of-week convention

The wire convention for `day_of_week` is **JS-Sunday-first integers
0–6**:

```
0 = Sunday, 1 = Monday, 2 = Tuesday, 3 = Wednesday,
4 = Thursday, 5 = Friday, 6 = Saturday
```

This matches the source pin's storage (behavior spec §2.2) and the
existing `create_meal_plan` AI tool argument shape. The iOS UI orders
days Monday-first; that ordering is a **client concern** and does not
affect the wire.

The contract uses `day_of_week` (snake_case) in JSONB and tool-call
arguments, and `dayOfWeek` (camelCase) in JSON response bodies.

### §3.4 Pagination & rate limiting

- **Pagination:** none in v1. List endpoints return all rows. The
  product is single-user with bounded data (≤ a few hundred recipes
  per user; ≤ ~50 weeks of meal plans accumulated). The Go backend
  enforces a hard cap of 1000 rows per list endpoint as defensive
  programming; exceeding the cap returns the first 1000 with a
  `"truncated": true` field. If a real user ever hits the cap, the
  contract is amended.
- **Rate limiting:** none enforced at the API edge in v1. OpenAI's
  upstream limits are the de facto cap. CloudWatch metrics track
  per-user request rate; an alert at >100 req/min/user triggers a
  human review.

### §3.5 Error response shape

Every non-2xx JSON response uses:

```json
{ "error": "<machine_readable_code>", "details": <optional any> }
```

The `error` value is a snake_case identifier the client can branch on.
The `details` object is optional and carries structured context
(invalid field name, validation reason, etc.). The Go backend **never**
leaks internal error text, stack traces, or wrapped error chains to the
wire (`dc-02`).

Common error codes are normative:

| Code | When |
|---|---|
| `missing_authorization`, `malformed_token`, `invalid_token`, `expired_token`, `wrong_issuer` | Auth (§2.3) |
| `not_owner`, `not_found` | Resource access (§2.3) |
| `unknown_field`, `missing_field`, `invalid_field` | Request validation |
| `week_start_not_monday` | Date validation (§3.2) |
| `empty_title`, `empty_content` | Cookbook validation (§5.6) |
| `ai_provider_error` | OpenAI returned an error (§6) |
| `internal_error` | Anything else (always logged, always opaque) |

### §3.6 Health endpoint

`GET /healthz` → `200 { "status": "ok" }`. Unauthenticated. Used by the
load balancer (per `dc-05`).

---

## §4 Data model — the Supabase schema

The schema is owned by the **data** track and lives in
`supabase/schemas/*.sql`. The contract specifies every table, every
column, the JSONB sub-document shapes, the constraints, the indexes
required for RLS, and the RLS policy intent. The data Builder produces
the migration SQL from this spec.

All tables live in the default `public` schema.

### §4.1 `users` (read-only view of `auth.users`)

The `auth.users` table is Supabase-managed. The Go backend **reads**
the following fields when it needs to display user info; it never
writes.

| Field | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key. JWT `sub` claim. |
| `email` | `text` | JWT `email` claim. |
| `created_at` | `timestamptz` | Auto. |

Per-user tables FK to `auth.users(id) ON DELETE CASCADE`. If a Supabase
user is deleted, every per-user row goes with them.

### §4.2 `food_items` — the Canonical Food Object

The single shape for inventory, shopping items, planned ingredients,
and recipe ingredients (per behavior spec §2.1; preserved per workload
spec §4.1).

```
food_items (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id         uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  canonical_name  text NOT NULL,                      -- lowercase, singular, generic
  display_name    text NOT NULL,                      -- human-readable
  quantity        jsonb,                              -- nullable; { amount, unit }
  category        jsonb NOT NULL,                     -- { primary, secondary? }
  attributes      jsonb NOT NULL DEFAULT '{}'::jsonb, -- open map
  flexibility     jsonb NOT NULL DEFAULT
                    '{"substitution_allowed": true,
                      "acceptable_variants": [],
                      "strict": false}'::jsonb,
  usage_context   jsonb NOT NULL,                     -- { role, ... } — see below
  inventory_state jsonb NOT NULL DEFAULT
                    '{"status": "unknown",
                      "on_hand_amount": null,
                      "last_confirmed": null}'::jsonb,
  sourcing        jsonb NOT NULL DEFAULT
                    '{"store_affinity": null,
                      "bulk_allowed": true,
                      "generic_ok": true}'::jsonb,
  metadata        jsonb NOT NULL,                     -- { created_by, confidence }
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT canonical_name_lowercase
    CHECK (canonical_name = lower(canonical_name)),
  CONSTRAINT canonical_name_nonempty
    CHECK (length(canonical_name) > 0),
  CONSTRAINT category_primary_known
    CHECK (category->>'primary' IN
      ('produce','dairy','meat','seafood','pantry','frozen',
       'bakery','beverages','other')),
  CONSTRAINT usage_context_role_known
    CHECK (usage_context->>'role' IN
      ('inventory','shopping','planned','ingredient')),
  CONSTRAINT inventory_state_status_known
    CHECK (inventory_state->>'status' IN
      ('confirmed','likely','unknown','out')),
  CONSTRAINT metadata_created_by_known
    CHECK (metadata->>'created_by' IN ('ai','user'))
);

-- Uniqueness invariant (behavior spec §2.1):
-- (user_id, canonical_name, usage_context.role) is unique.
CREATE UNIQUE INDEX food_items_user_canonical_role_uniq
  ON food_items (user_id, canonical_name, (usage_context->>'role'));

-- RLS index targets
CREATE INDEX food_items_user_id_idx ON food_items (user_id);
```

**JSONB sub-document shapes** (these are wire shapes — see §7 for the
write paths):

```
quantity:        { "amount": <number>, "unit": <string> } | null
category:        { "primary": <enum>, "secondary"?: <string> }
flexibility:     { "substitution_allowed": <bool>,
                   "acceptable_variants": <string[]>,
                   "strict": <bool> }
usage_context:   { "role": <enum>,
                   "required"?: <bool>,
                   "recipe_ids"?: <uuid[]>,
                   "meal_plan_id"?: <uuid>,
                   "shopping_list_id"?: <uuid> }
inventory_state: { "status": <enum>,
                   "on_hand_amount": <number> | null,
                   "last_confirmed": <RFC3339> | null }
sourcing:        { "store_affinity": <string> | null,
                   "bulk_allowed": <bool>,
                   "generic_ok": <bool> }
metadata:        { "created_by": "ai" | "user",
                   "confidence": <number 0..1> }
```

**Enums** (mirror behavior spec §2.1):

- `category.primary` ∈ `{ produce, dairy, meat, seafood, pantry, frozen,
  bakery, beverages, other }`.
- `usage_context.role` ∈ `{ inventory, shopping, planned, ingredient }`.
  Per ADR-0009, only `inventory` and `shopping` are written by v1 code;
  the other two are reserved.
- `inventory_state.status` ∈ `{ confirmed, likely, unknown, out }`.

**RLS intent:** only the owner can SELECT/INSERT/UPDATE/DELETE its own
rows. Policies scope by `user_id = (select auth.uid())`. Per `dc-04`,
one policy per operation, `TO authenticated`, with the indexed
`user_id` column.

**Recipe-image storage notes:** `food_items` does not store any image
data. Image storage is exclusively on `cookbook_recipes.image_url` (§4.4)
and on `meal_plan_days.recipe_image_prompt` (the prompt; bytes are
generated on demand, §8).

### §4.3 `meal_plans` and `meal_plan_days`

A weekly plan and its day rows. Behavior spec §2.2.

```
meal_plans (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id         uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  week_start_date date NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT week_start_is_monday
    CHECK (extract(isodow from week_start_date) = 1)
);

-- Invariant: one plan per (user, week).
CREATE UNIQUE INDEX meal_plans_user_week_uniq
  ON meal_plans (user_id, week_start_date);

CREATE INDEX meal_plans_user_id_idx ON meal_plans (user_id);
```

```
meal_plan_days (
  id                   uuid PRIMARY KEY DEFAULT uuidv7(),
  meal_plan_id         uuid NOT NULL REFERENCES meal_plans(id) ON DELETE CASCADE,
  day_of_week          smallint NOT NULL,        -- JS-Sunday-first 0..6
  meal_name            text NOT NULL,
  notes                text,
  recipe_content       text,                     -- markdown cache; nullable
  recipe_image_prompt  text,                     -- prompt cache; nullable
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT day_of_week_range CHECK (day_of_week BETWEEN 0 AND 6),
  CONSTRAINT meal_name_nonempty CHECK (length(meal_name) > 0)
);

-- A meal plan day is unique within its plan.
CREATE UNIQUE INDEX meal_plan_days_plan_day_uniq
  ON meal_plan_days (meal_plan_id, day_of_week);

CREATE INDEX meal_plan_days_meal_plan_id_idx
  ON meal_plan_days (meal_plan_id);
```

**Cache-clearing invariant (behavior spec §2.2):** any UPDATE that
changes `meal_name` must also NULL `recipe_content` and
`recipe_image_prompt`. The Go backend enforces this in its update
helpers; the data Builder MAY add a trigger as defense-in-depth.

**RLS intent:** ownership of `meal_plan_days` flows through
`meal_plans.user_id`. Policies use a `STABLE` helper or a subquery
matching `meal_plan_id IN (select id from meal_plans where user_id =
(select auth.uid()))`. Per `dc-04`, this is wrapped in a `SECURITY
DEFINER` function in a non-exposed schema for performance.

### §4.4 `cookbook_recipes`

User-saved recipes. Behavior spec §2.4. Per ADR-0004, the
`thumbnail_url` data-URL approach is dropped in favor of
`image_url` pointing at Supabase Storage.

```
cookbook_recipes (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id      uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  title        text NOT NULL,
  content      text NOT NULL,            -- markdown in canonical recipe format
  image_prompt text,                     -- the prompt; the regeneration seed
  image_url    text,                     -- Supabase Storage URL; null until generated
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT title_nonempty   CHECK (length(title) > 0),
  CONSTRAINT content_nonempty CHECK (length(content) > 0)
);

CREATE INDEX cookbook_recipes_user_id_idx ON cookbook_recipes (user_id);

-- Cookbook-first recipe lookup is case-insensitive on title.
CREATE INDEX cookbook_recipes_user_title_lower_idx
  ON cookbook_recipes (user_id, lower(title));
```

**Storage bucket** (per ADR-0004):

- Bucket name: `cookbook-images`.
- Object key: `{user_id}/{cookbook_recipe_id}.png`.
- MIME: `image/png`.
- Size cap (bucket-level): 8 MB.
- RLS on `storage.objects`: only the path-prefix owner (`user_id =
  (select auth.uid())::text`) can read/write objects.

**Lifecycle:**

- Image bytes generated at cookbook save (§5.6 `POST
  /api/kitchen/cookbook` and the auto-save path inside §5.4
  `POST /api/kitchen/meal-plan-days/{dayId}/generate-recipe`).
- `image_url` is set only after the bytes are stored.
- Explicit regeneration via `POST /api/kitchen/cookbook/{id}/
  regenerate-image` (§5.6) overwrites the Storage object and appends a
  cache-busting `?v=<unix>` query string to the returned URL.

**RLS intent:** owner-scoped, same pattern as §4.2.

### §4.5 `shopping_lists` and `shopping_list_items`

Behavior spec §2.5.

```
shopping_lists (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id         uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  name            text NOT NULL DEFAULT 'Shopping List',
  week_start_date date,                                       -- nullable
  meal_plan_id    uuid REFERENCES meal_plans(id) ON DELETE SET NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT week_start_is_monday_or_null
    CHECK (week_start_date IS NULL
        OR extract(isodow from week_start_date) = 1)
);

-- Invariant: one week-tied list per (user, week).
CREATE UNIQUE INDEX shopping_lists_user_week_uniq
  ON shopping_lists (user_id, week_start_date)
  WHERE week_start_date IS NOT NULL;

CREATE INDEX shopping_lists_user_id_idx ON shopping_lists (user_id);
```

```
shopping_list_items (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  shopping_list_id uuid NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
  name             text NOT NULL,
  quantity         text,                       -- free-form "{amount} {unit}"
  category         text NOT NULL,              -- same 9-value enum as CFO
  checked          boolean NOT NULL DEFAULT false,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT category_known
    CHECK (category IN
      ('produce','dairy','meat','seafood','pantry','frozen',
       'bakery','beverages','other')),
  CONSTRAINT name_nonempty CHECK (length(name) > 0)
);

CREATE INDEX shopping_list_items_list_id_idx
  ON shopping_list_items (shopping_list_id);
```

**Note on dual writes:** the source pin writes parallel `food_items`
(`role = "shopping"`) rows alongside `shopping_list_items` (behavior
spec §2.5). The Go backend preserves this dual write — the CFO rows
exist for the AI tools to reason about; the `shopping_list_items` rows
back the check-off UI. Toggling `checked` mutates only
`shopping_list_items`; the CFO row is unaffected. This is a deliberate
behavior-spec preservation (§2.5), not a bug.

**`checked` is a `boolean`** in the new schema (the source used
`integer 0|1` per behavior spec §2.5). The wire returns `true|false`.

**RLS intent:** `shopping_lists` is owner-scoped. `shopping_list_items`
ownership flows through `shopping_list_id`.

### §4.6 `kitchen_conversations` and `kitchen_messages`

Behavior spec §2.6.

```
kitchen_conversations (
  id         uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id    uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  title      text NOT NULL DEFAULT 'New Chat',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX kitchen_conversations_user_updated_idx
  ON kitchen_conversations (user_id, updated_at DESC);
```

```
kitchen_messages (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  conversation_id uuid NOT NULL REFERENCES kitchen_conversations(id) ON DELETE CASCADE,
  role            text NOT NULL,             -- 'user' | 'assistant'
  content         text NOT NULL,
  metadata        jsonb,                     -- nullable; future use
  created_at      timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT role_known CHECK (role IN ('user', 'assistant')),
  CONSTRAINT content_nonempty CHECK (length(content) > 0)
);

CREATE INDEX kitchen_messages_conversation_created_idx
  ON kitchen_messages (conversation_id, created_at);
```

**Append-only.** No UPDATE or DELETE policy on `kitchen_messages` for
authenticated users — only INSERT and SELECT.

**Title auto-generation** (behavior spec §2.6, §6.4): on the first user
message into a conversation whose title is `'New Chat'` or
`'Kitchen Chat'`, the Go backend SETs the title to the first ~6 words
(max 40 chars, ellipsized) of that message.

**Per ADR-0008, the recipe-page chat does not persist into
`kitchen_messages`.** No `recipe_day_id` discriminator column is added.

**RLS intent:** `kitchen_conversations` owner-scoped. `kitchen_messages`
ownership flows through `conversation_id`.

### §4.7 Dropped tables

Per ADR-0006, the following source-pin tables **do not appear** in the
new schema:

- `recipes` — legacy structured-recipe table, never written by active
  code at the pin. Recipe content lives on `meal_plan_days.recipe_content`
  and `cookbook_recipes.content`.
- `ingredient_memory` — legacy pre-CFO soft inventory, dual-written for
  back-compat. Subsumed by `food_items` with `usage_context.role =
  "inventory"`.

The `sessions` table (Replit Auth artifact) is also dropped — Supabase
Auth replaces the cookie-session mechanism.

### §4.8 `updated_at` triggers

Every table with an `updated_at` column has a `BEFORE UPDATE` trigger
that sets `updated_at = now()`. The data Builder writes this as a
shared helper function in a private schema, applied per table (per
`dc-04`: maintained by a trigger, not by application code).

---

## §5 REST API surface

Every endpoint is rooted at `/api/kitchen` and requires `Authorization:
Bearer <jwt>` (§2.2). `user_id` is always derived from the JWT.

Endpoints are grouped by resource. For each: method, path, params,
request, response, status codes, side effects.

### §5.1 Conversations

#### `GET /api/kitchen/conversations`

List all conversations for the authenticated user, newest-updated first.

- **Response 200** — `{ "conversations": [{ "id": "<uuid>", "title":
  "<string>", "createdAt": "<rfc3339>", "updatedAt": "<rfc3339>" }] }`.

#### `POST /api/kitchen/conversations`

Create a new empty conversation.

- **Request** — `{ "title"?: "<string>" }` (defaults to `"New Chat"`).
- **Response 201** — the new conversation object.
- **Replaces** source pin's `GET /api/kitchen/conversation` (create-on-
  GET) and `POST /api/kitchen/conversation/new`. The iOS client calls
  this explicitly when the user wants a new chat **and** on first
  app-launch session start.

#### `GET /api/kitchen/conversations/{id}`

Conversation + all its messages, oldest-first.

- **Path** — `id` is a `uuid`.
- **Response 200** — `{ "conversation": <conversation>, "messages":
  [{ "id": "<uuid>", "role": "user"|"assistant", "content": "<string>",
  "createdAt": "<rfc3339>" }] }`.
- **404** if the conversation is not owned by the caller.

#### `POST /api/kitchen/conversations/{id}/messages`

Send a chat message. **Streams an SSE response** (see §6 for the wire
shape).

- **Path** — conversation `id`.
- **Request** — `{ "content": "<string>", "clientWeekStartDate":
  "<YYYY-MM-DD>" }` (the second field per ADR-0010 — the user's local
  Monday-of-this-week, used when the AI fires `create_meal_plan`).
- **Response** — `200 text/event-stream`. See §6.
- **Side effects** — server persists the user message on receipt;
  persists the assistant message when the stream completes; may write
  via tool calls (§7).

### §5.2 Meal plans

#### `GET /api/kitchen/weeks/{weekStartDate}`

Get the meal plan and shopping list (if either exists) for a specific
week.

- **Path** — `weekStartDate` is an ISO date that must be a Monday (§3.2).
- **Response 200** — `{ "mealPlan": <meal_plan_with_days> | null,
  "shoppingList": <shopping_list> | null }`.
- **400** with `week_start_not_monday` if the date is not a Monday.

```
meal_plan_with_days: {
  "id": "<uuid>",
  "weekStartDate": "<YYYY-MM-DD>",
  "createdAt": "<rfc3339>",
  "updatedAt": "<rfc3339>",
  "days": [
    {
      "id": "<uuid>",
      "dayOfWeek": 0..6,
      "mealName": "<string>",
      "notes": "<string>" | null,
      "hasRecipeContent": <bool>,
      "hasRecipeImagePrompt": <bool>
    }
  ]
}
```

The day object omits `recipeContent` and `recipeImagePrompt` from list
responses — the cache markdown is bulky and only needed on the recipe
detail screen.

#### `GET /api/kitchen/calendar`

Summary of every week the user has touched, for calendar drilldown.

- **Response 200** — `{ "mealPlans": [{ "id": "<uuid>", "weekStartDate":
  "<YYYY-MM-DD>", "dayCount": <int> }], "shoppingLists": [{ "id":
  "<uuid>", "weekStartDate": "<YYYY-MM-DD>" | null, "name": "<string>",
  "itemCount": <int> }] }`.

#### `POST /api/kitchen/meal-plans`

Generate a fresh meal plan for a week.

- **Request** — `{ "weekStartDate": "<YYYY-MM-DD>" }`.
- **Response 201** — `<meal_plan_with_days>`.
- **Side effects** — Server calls OpenAI (per behavior spec §4.6), with
  randomized cuisine/seasonal context; persists the plan (replacing any
  existing plan for the same `(user_id, week_start_date)` — cascade
  deletes the prior plan and its days first). Hand-coded variety
  fallback if the AI returns unparseable output (behavior spec §4.6).

#### `POST /api/kitchen/meal-plans/{weekStartDate}/regenerate-days`

Regenerate a subset of days within an existing week's plan.

- **Path** — `weekStartDate` must be a Monday.
- **Request** — `{ "daysToRegenerate": [0..6, ...] }`.
- **Response 200** — `<meal_plan_with_days>`.
- **Side effects** — OpenAI call (temperature 0.95) with "avoid these
  meals" context from the kept days; updates only the listed
  `day_of_week` rows; per the cache-clearing invariant (§4.3),
  `recipe_content` and `recipe_image_prompt` are nulled for updated
  days.
- **404** if no meal plan exists for that week.

#### `GET /api/kitchen/meal-plan-days/{dayId}`

Single day with its cached recipe content.

- **Path** — `dayId` is a `uuid`.
- **Response 200** — full day object, including `recipeContent` and
  `recipeImagePrompt` (which may be null).

#### `POST /api/kitchen/meal-plan-days/{dayId}/generate-recipe`

Generate (or cookbook-replay) a full recipe for a meal plan day.
**Streams an SSE response** (see §6).

- **Path** — `dayId`.
- **Request** — empty body.
- **Response** — `text/event-stream`. See §6.
- **Side effects** — Server first checks the cookbook by case-
  insensitive `title = meal_name` (behavior spec §6.2). If hit: streams
  the cookbook's content back and writes it to the meal plan day. If
  miss: streams a fresh OpenAI completion; persists `recipe_content`
  and a templated `recipe_image_prompt`; auto-saves to cookbook if no
  case-insensitive title match exists. The auto-save **generates the
  image at save time** per ADR-0004 (this is a deliberate change from
  the source's regenerate-on-view pattern).

### §5.3 Recipe-page chat (per ADR-0008, stateless)

#### `POST /api/kitchen/recipe-message`

Stateless follow-up chat scoped to one meal-plan day. **Streams SSE**.

- **Request** — `{ "content": "<string>", "dayId": "<uuid>", "mealName":
  "<string>", "dayName": "<string>", "currentRecipe": "<string>"? }`.
- **Response** — `text/event-stream`. See §6.
- **Side effects** — Server may call the recipe-page `update_meal` tool
  (§7); on swap, the day's `meal_name` is updated and
  `recipe_content`/`recipe_image_prompt` are nulled. **No message
  persistence** (ADR-0008).

### §5.4 Shopping lists

#### `GET /api/kitchen/shopping-lists`

Index of all the user's lists, newest-first.

- **Response 200** — `{ "shoppingLists": [{ "id": "<uuid>", "name":
  "<string>", "weekStartDate": "<YYYY-MM-DD>" | null, "createdAt":
  "<rfc3339>", "itemCount": <int> }] }`.

#### `GET /api/kitchen/shopping-lists/{identifier}`

Get a list by id or `weekStartDate`. The path param is interpreted as a
UUID if it parses as one, else as an ISO date.

- **Response 200** — `<shopping_list_with_items>`.

```
shopping_list_with_items: {
  "id": "<uuid>",
  "name": "<string>",
  "weekStartDate": "<YYYY-MM-DD>" | null,
  "mealPlanId": "<uuid>" | null,
  "createdAt": "<rfc3339>",
  "items": [
    {
      "id": "<uuid>",
      "name": "<string>",
      "quantity": "<string>" | null,
      "category": "<enum>",
      "checked": <bool>
    }
  ]
}
```

#### `POST /api/kitchen/shopping-lists`

Generate a shopping list from the current week's meal plan + CFO
inventory diff.

- **Request** — `{ "weekStartDate": "<YYYY-MM-DD>"? }` (defaults to
  the Monday associated with the most-recent meal plan).
- **Response 201** — `<shopping_list_with_items>`.
- **Side effects** — Server queries the meal plan + CFO inventory for
  the week, calls OpenAI to derive the list, persists `food_items`
  (`role = "shopping"`) rows alongside `shopping_list_items` rows
  (behavior spec §2.5 dual-write). Replaces any existing list for the
  same `(user_id, week_start_date)`.

#### `PATCH /api/kitchen/shopping-items/{id}`

Toggle a single item's checked state.

- **Request** — `{ "checked": <bool> }`.
- **Response 200** — the updated item.

#### `DELETE /api/kitchen/shopping-lists/{shoppingListId}/checked-items`

Delete all checked items from the **specified** list (per ADR-0007 —
the source pin's bug is fixed; the path is no longer flat).

- **Path** — `shoppingListId` is a `uuid`; ownership verified.
- **Response 200** — `{ "deletedCount": <int> }`.

### §5.5 Ingredients (CFO read paths)

#### `GET /api/kitchen/ingredients`

The user's inventory CFO rows (per ADR-0009: filtered by
`usage_context.role = "inventory"` and `inventory_state.status != "out"`).

- **Response 200** — `{ "ingredients": [<food_item>] }`. Each item is
  the full CFO shape (§4.2).

#### `GET /api/kitchen/ingredient-suggestions`

Up to 20 deduped CFO inventory names (behavior spec §3.6 — used by the
cookbook ingredient-helper panel).

- **Response 200** — `{ "suggestions": [{ "canonicalName": "<string>",
  "displayName": "<string>", "category": "<enum>" }] }`. Per ADR-0006,
  this is sourced from `food_items` only — no `ingredient_memory`
  fallback (that table is dropped).

### §5.6 Cookbook

#### `GET /api/kitchen/cookbook`

All saved recipes, newest-first.

- **Response 200** — `{ "recipes": [{ "id": "<uuid>", "title":
  "<string>", "imageUrl": "<string>" | null, "createdAt": "<rfc3339>",
  "updatedAt": "<rfc3339>" }] }`. Content is omitted from the list
  view; fetch the single recipe for content.

#### `GET /api/kitchen/cookbook/{id}`

One recipe.

- **Response 200** — `{ "id": "<uuid>", "title": "<string>", "content":
  "<string>", "imagePrompt": "<string>" | null, "imageUrl": "<string>"
  | null, "createdAt": "<rfc3339>", "updatedAt": "<rfc3339>" }`.

#### `POST /api/kitchen/cookbook`

Save a new recipe.

- **Request** — `{ "title": "<string>", "content": "<string>",
  "imagePrompt": "<string>"? }`.
- **Response 201** — the new recipe object **including a populated
  `imageUrl`** (per ADR-0004, the image is generated inline before
  returning).
- **Validation** — `title` and `content` must be non-empty (`empty_title`
  / `empty_content` errors).
- **Side effects** — calls the image generator (§8) using the supplied
  `imagePrompt` (or a default templated from `title` if absent),
  stores PNG bytes in Supabase Storage at
  `{user_id}/{recipe_id}.png`, persists `image_url`.

#### `PUT /api/kitchen/cookbook/{id}`

Update a saved recipe.

- **Request** — `{ "title"?: "<string>", "content"?: "<string>",
  "imagePrompt"?: "<string>" }`.
- **Response 200** — the updated recipe.
- **Validation** — if present, `title` and `content` must be non-empty.
- **Note:** updating `title` or `content` does **not** auto-regenerate
  the image. Image regeneration is explicit (`regenerate-image`).

#### `DELETE /api/kitchen/cookbook/{id}`

Delete a saved recipe.

- **Response 204** — no body.
- **Side effects** — cascades nothing (cookbook is leaf data); Supabase
  Storage object at `{user_id}/{id}.png` is also deleted (a storage
  trigger on row delete, owned by the data track).

#### `POST /api/kitchen/cookbook/{id}/regenerate-image`

Issue a fresh image generation and overwrite the Storage object.

- **Request** — empty body. (The prompt is taken from
  `cookbook_recipes.image_prompt`.)
- **Response 200** — `{ "imageUrl": "<string>" }` (URL includes a
  `?v=<unix>` cache-buster).
- **Side effects** — calls the image generator (§8); overwrites the
  Storage object at the same key; updates `image_url` on the row.
- **New in the port** per ADR-0004. The source pin has no equivalent;
  the source regenerated the image on every recipe view.

### §5.7 New / changed endpoints summary

| Endpoint | Status in v1 port |
|---|---|
| `POST /api/kitchen/conversations` | **New** (replaces `GET /api/kitchen/conversation` + `POST /api/kitchen/conversation/new`). |
| `DELETE /api/kitchen/shopping-lists/{id}/checked-items` | **Changed shape** — requires `shoppingListId`, was flat (ADR-0007). |
| `POST /api/kitchen/cookbook/{id}/regenerate-image` | **New** (ADR-0004). |
| `POST /api/kitchen/cookbook` | **Changed semantics** — generates image inline at save (ADR-0004). |
| `POST /api/kitchen/conversations/{id}/messages` | **Changed shape** — gains `clientWeekStartDate` (ADR-0010). |
| `POST /api/kitchen/meal-plan-days/{dayId}/generate-recipe` | **Changed semantics** — auto-save now also generates the image (ADR-0004). |
| All week-bearing endpoints | **Changed validation** — `week_start_not_monday` rejection (ADR-0010). |
| `GET /api/login`, `/api/callback`, `/api/logout`, `GET /api/auth/user` | **Dropped** — Supabase Auth replaces (ADR-0003). |

All other endpoints port their behavior with only the path-shape
adjustments (snake to camelCase, plural resource paths).

---

## §6 Streaming

The Go backend exposes two SSE endpoints — both `POST` so the request
carries a body, both `text/event-stream` response:

1. `POST /api/kitchen/conversations/{id}/messages` (the main chat).
2. `POST /api/kitchen/meal-plan-days/{dayId}/generate-recipe` (recipe
   generation stream).
3. `POST /api/kitchen/recipe-message` (the stateless recipe-page chat).

The wire format is identical across all three: standard SSE (`data:
<json>\n\n` per event), terminated by a `done` event. iOS consumes via
`URLSession.bytes(for:)` and a line-delimited JSON parser.

### §6.1 Event shape

Every event is a JSON object with a `type` discriminator:

```
{ "type": "text",        "content": "<string>" }
{ "type": "tool_call",   "tool_name": "<string>", "arguments": <object> }
{ "type": "tool_result", "tool_name": "<string>", "result": <object> }
{ "type": "done",        ...endpoint-specific fields... }
{ "type": "error",       "error": "<code>", "details": <any>? }
```

### §6.2 Per-endpoint terminal events

**Chat (`/conversations/{id}/messages`):**
```
{ "type": "done", "assistant_message_id": "<uuid>" }
```
The `id` is the persisted assistant message's primary key. The iOS
client updates its in-memory message row from optimistic to confirmed.

**Recipe generation (`/meal-plan-days/{dayId}/generate-recipe`):**
```
{ "type": "done", "image_prompt": "<string>" }
```
The `image_prompt` is the templated prompt the server persisted to
`meal_plan_days.recipe_image_prompt`. The iOS client stores it; image
bytes are generated only when the user taps the placeholder (§8).

**Recipe-page chat (`/recipe-message`):**
```
{ "type": "done", "updated_meal": { "meal_name": "<string>",
                                    "notes": "<string>" }? }
```
The `updated_meal` is present only if the AI fired the recipe-page
`update_meal` tool variant during the stream.

### §6.3 Ordering rules

- `text` events arrive in token-emission order. The client appends to
  the rendered message in order.
- `tool_call` events arrive at most once per tool invocation, *after*
  the model has finished accumulating that tool's arguments
  server-side (so the client receives the full args, not deltas).
- `tool_result` events follow their `tool_call`; same ordering.
- `done` is the last event. No events follow `done`.
- `error` is terminal — the client treats it like `done` for stream
  state. Server-side, the assistant message is still persisted if any
  `text` content was emitted before the error.

### §6.4 Client buffering rule

The iOS SSE parser **must** accumulate incomplete chunks across network
reads. Split on `\n`; treat the trailing non-terminated line as a
pending buffer for the next chunk. This is the same rule the web client
fixed (behavior spec §4.5); the iOS implementation MUST repeat it.

### §6.5 Cancellation

- The client cancels by canceling the underlying `URLSessionDataTask`
  (`Task.cancel()` on the SwiftUI `.task { }`).
- The server detects cancellation via the request's `context.Done()`
  channel; aborts the OpenAI stream; persists whatever assistant
  content was emitted to that point (so the user sees the partial
  reply on reload).

### §6.6 Errors mid-stream

- OpenAI / network errors: emit `{ "type": "error", "error":
  "ai_provider_error" }`; close the stream.
- Tool-execution errors (e.g. DB write fails during a tool call):
  emit `{ "type": "tool_result", "tool_name": "<name>", "result":
  { "success": false, "error": "<code>" } }` and let the model
  continue if it wants to recover, or close with `error` if not.
- Auth failures cannot occur mid-stream (auth is verified before
  upgrade).

---

## §7 AI tool-calling contract

The Go backend uses OpenAI's chat-completions API with `tools` and
`tool_choice: "auto"` (per ADR-0002, the provider is OpenAI; the
contract specifies wire shapes that match OpenAI's tool-call format).

The server is the only thing that talks to OpenAI. The iOS client never
sees an OpenAI request directly; it consumes the SSE stream the server
emits (§6).

Five tools total: the four main-chat tools plus the recipe-page
`update_meal` variant.

### §7.1 `update_ingredients`

Update the user's CFO inventory.

**JSON Schema (OpenAI tool format):**

```json
{
  "name": "update_ingredients",
  "description": "Add ingredients to or remove them from the user's pantry/fridge inventory.",
  "parameters": {
    "type": "object",
    "properties": {
      "ingredients": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "canonical_name": { "type": "string" },
            "display_name":   { "type": "string" },
            "quantity": {
              "type": "object",
              "properties": {
                "amount": { "type": "number" },
                "unit":   { "type": "string" }
              },
              "required": ["amount", "unit"]
            },
            "category": {
              "type": "string",
              "enum": ["produce","dairy","meat","seafood","pantry",
                       "frozen","bakery","beverages","other"]
            },
            "status": {
              "type": "string",
              "enum": ["confirmed","likely","out"]
            },
            "action": {
              "type": "string",
              "enum": ["add","remove"]
            }
          },
          "required": ["canonical_name", "category", "action"]
        }
      }
    },
    "required": ["ingredients"]
  }
}
```

**Server-side handler:**

- For each item with `action: "add"`: upsert into `food_items` by
  `(user_id, lower(canonical_name), 'inventory')` setting
  `usage_context.role = "inventory"`,
  `inventory_state.status = status` (default `"confirmed"`),
  `inventory_state.last_confirmed = now()`,
  `metadata.created_by = "ai"`, `metadata.confidence = 1.0` if status
  is `"confirmed"` else `0.8`.
- For each item with `action: "remove"`: look up the inventory-role row
  by canonical_name; set `inventory_state.status = "out"` and
  `inventory_state.on_hand_amount = 0`. **Do not delete the row**
  (behavior spec §2.1).
- Per ADR-0006, no parallel write to `ingredient_memory` (table is
  dropped).

**Tool result emitted to the model:**
```json
{ "success": true, "applied": <int>, "removed": <int> }
```

**User-facing side effect:** the user's inventory state updates; the
next chat turn sees the new state in the system prompt's
ingredient-context block.

### §7.2 `create_meal_plan`

Create the upcoming week's plan (or replace it).

**JSON Schema:**

```json
{
  "name": "create_meal_plan",
  "description": "Create or replace the meal plan for the week containing week_start_date.",
  "parameters": {
    "type": "object",
    "properties": {
      "week_start_date": {
        "type": "string",
        "description": "Monday-of-week ISO date YYYY-MM-DD. Comes from the user's local calendar."
      },
      "meals": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "day_of_week": { "type": "integer", "minimum": 0, "maximum": 6 },
            "meal_name":   { "type": "string" },
            "notes":       { "type": "string" }
          },
          "required": ["day_of_week", "meal_name"]
        }
      }
    },
    "required": ["week_start_date", "meals"]
  }
}
```

**Note on `week_start_date` (ADR-0010):** the server-side dispatcher
**overrides** any `week_start_date` the model produces with the
`clientWeekStartDate` from the chat-send request (§5.1). The schema
exposes the field so the AI's tool-call output mirrors the wire's
mental model, but the **authoritative** value is the client's. This
prevents the model from getting the week wrong.

**Server-side handler:**

- Validate the (overridden) `week_start_date` is a Monday.
- Begin transaction.
- DELETE any existing `meal_plans` row for `(user_id,
  week_start_date)` (CASCADE removes its days).
- INSERT a fresh `meal_plans` row.
- INSERT one `meal_plan_days` row per meal in the call.
- Commit.

**Tool result:**
```json
{ "success": true, "meal_plan_id": "<uuid>", "day_count": <int> }
```

### §7.3 `create_shopping_list`

Create a shopping list from a derived set of items.

**JSON Schema:**

```json
{
  "name": "create_shopping_list",
  "description": "Create a shopping list, replacing any existing list for the same week.",
  "parameters": {
    "type": "object",
    "properties": {
      "week_start_date": {
        "type": "string",
        "description": "Monday-of-week ISO date. Optional; null = general list."
      },
      "items": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "canonical_name": { "type": "string" },
            "display_name":   { "type": "string" },
            "quantity": {
              "type": "object",
              "properties": {
                "amount": { "type": "number" },
                "unit":   { "type": "string" }
              }
            },
            "category": {
              "type": "string",
              "enum": ["produce","dairy","meat","seafood","pantry",
                       "frozen","bakery","beverages","other"]
            },
            "substitution_allowed": { "type": "boolean" },
            "generic_ok":           { "type": "boolean" }
          },
          "required": ["canonical_name", "category"]
        }
      }
    },
    "required": ["items"]
  }
}
```

**Server-side handler (behavior spec §4.3 `create_shopping_list`):**

- If `week_start_date` is supplied, validate Monday; the
  server-side dispatcher injects `clientWeekStartDate` if missing.
- Begin transaction.
- DELETE any existing `shopping_lists` row for `(user_id,
  week_start_date)` when the date is non-null (CASCADE removes items).
- INSERT a fresh `shopping_lists` row; link `meal_plan_id` if a meal
  plan exists for that week.
- For each item:
  - UPSERT a `food_items` row with `usage_context.role = "shopping"`,
    `flexibility.substitution_allowed = <arg | true>`,
    `sourcing.generic_ok = <arg | true>`, `metadata.created_by = "ai"`,
    `metadata.confidence = 0.9`. Set
    `usage_context.shopping_list_id` to the new list id.
  - INSERT a `shopping_list_items` row: `name = display_name ??
    canonical_name`, `quantity = "{amount} {unit}"` if present,
    `category = items[].category`, `checked = false`.
- Commit.

**Tool result:**
```json
{ "success": true, "shopping_list_id": "<uuid>", "item_count": <int> }
```

### §7.4 `update_meal` (main chat — with `day` name)

Swap one day in the current week's plan.

**JSON Schema:**

```json
{
  "name": "update_meal",
  "description": "Update a single day's meal in the current week's plan.",
  "parameters": {
    "type": "object",
    "properties": {
      "week_start_date": {
        "type": "string",
        "description": "Monday-of-week ISO date. Server-overridden by client."
      },
      "day": {
        "type": "string",
        "enum": ["sunday","monday","tuesday","wednesday",
                 "thursday","friday","saturday"]
      },
      "meal_name": { "type": "string" },
      "notes":     { "type": "string" }
    },
    "required": ["day", "meal_name"]
  }
}
```

**Day-name → integer map** (matching JS `Date.getDay()`):
`sunday=0, monday=1, tuesday=2, wednesday=3, thursday=4, friday=5,
saturday=6`.

**Server-side handler:**

- Server injects `clientWeekStartDate` as `week_start_date`.
- Look up `meal_plans` for `(user_id, week_start_date)`.
  - If none: INSERT a meal plan + this single day row.
  - If present:
    - If a `meal_plan_days` row exists at this `day_of_week`: UPDATE
      `meal_name`, `notes`, and **null `recipe_content` and
      `recipe_image_prompt`** (cache-clearing invariant, §4.3).
    - If absent: INSERT a new day row.

**Tool result:**
```json
{ "success": true, "day_id": "<uuid>" }
```

### §7.5 Recipe-page `update_meal` variant (no `day` field)

Used only in `POST /api/kitchen/recipe-message`. The day is implicit
from the `dayId` carried on the request.

**JSON Schema:**

```json
{
  "name": "update_meal",
  "description": "Swap the current day's meal for a new one.",
  "parameters": {
    "type": "object",
    "properties": {
      "meal_name": { "type": "string" },
      "notes":     { "type": "string" }
    },
    "required": ["meal_name"]
  }
}
```

The two tool definitions are kept distinct by the route registering
them: the main-chat endpoint registers §7.4; the recipe-message
endpoint registers this one. The tool name `update_meal` is shared; the
schema differs by endpoint. This is allowed because the iOS client
never sees the OpenAI tool registration — only the SSE stream.

**Server-side handler:**

- Look up the `meal_plan_days` row by `dayId`; verify ownership.
- UPDATE `meal_name`, `notes`; NULL `recipe_content` and
  `recipe_image_prompt`.

**Stream terminal event** (per §6.2):
```json
{ "type": "done",
  "updated_meal": { "meal_name": "<string>", "notes": "<string>" } }
```

### §7.6 Tool dispatch order

When OpenAI emits multiple tool calls in a single assistant turn, the
server dispatches them in the order received. Failures in one tool
emit a `tool_result` with `success: false` but do not abort subsequent
tools. After all tool calls complete, the server re-prompts OpenAI
with the accumulated tool results so the model can produce a final
text response (matching the source-pin behavior: "Done! I've updated
that for you." is emitted only if the model produced no own content
after the tools fired).

---

## §8 Image generation

Per ADR-0004, the iOS port stores image **bytes** at Supabase Storage
URLs (not data URLs), and generates only at deliberate save / explicit
regeneration moments — never on a list view.

### §8.1 Generator interface (Go backend, internal)

The Go backend exposes an internal `image.Generate(ctx, prompt)
(bytes, contentType, error)` function. Implementation calls OpenAI:

```
model:  gpt-image-1   (or current equivalent)
size:   1024x1024
n:      1
format: PNG (returned as b64_json by OpenAI, decoded server-side)
```

The Go backend then uploads to Supabase Storage and returns the public
URL.

### §8.2 Cookbook image flow

- **At save** (`POST /api/kitchen/cookbook` or the auto-save path in
  `generate-recipe`): generate bytes, upload to
  `cookbook-images/{user_id}/{recipe_id}.png`, set
  `cookbook_recipes.image_url`.
- **On explicit regeneration** (`POST /api/kitchen/cookbook/{id}/
  regenerate-image`): regenerate bytes, overwrite the Storage object,
  update `image_url` with a `?v=<unix>` cache-buster.
- **No passive regeneration**: viewing the cookbook does not trigger
  generation.

### §8.3 Meal-plan-day image flow

Meal-plan-day images are **not stored**. The day row carries a
`recipe_image_prompt`. The iOS client, when the user taps the
placeholder on the recipe page, calls a one-shot endpoint:

```
POST /api/kitchen/regenerate-image
Authorization: Bearer <jwt>
Body: { "prompt": "<string>" }
→ 200 { "imageData": "data:image/png;base64,<...>" }
```

The bytes are returned **inline** (as a data URL — this is the one
remaining data-URL path in the contract). They are not persisted
server-side. The iOS client may cache them locally (NSCache + disk LRU
per ADR-0004); cache is per-day-id keyed.

This split (cookbook images persisted, meal-plan-day images transient)
matches the behavior spec's user model: cookbook recipes are the
"saved" set the user comes back to; meal-plan days are one-week
ephemeral and may swap.

### §8.4 Error handling

- OpenAI image-generation failure: return `502 Bad Gateway` with
  `{ "error": "ai_provider_error" }`. iOS shows a "couldn't generate
  image, tap to retry" affordance.
- Supabase Storage upload failure: return `503 Service Unavailable`
  with `{ "error": "internal_error" }`; the cookbook save **does not
  roll back** (the recipe row is persisted; `image_url` stays null).
  iOS shows the recipe with a placeholder and a "tap to regenerate"
  affordance.

---

## §9 Open behavior rules pinned by this contract

Behaviors the behavior spec left ambiguous and that the Architect locks
here.

### §9.1 Recipe-page chat statefulness — stateless (ADR-0008)

`POST /api/kitchen/recipe-message` writes nothing to
`kitchen_messages`. The iOS client discards in-page chat state on
navigation away. No `recipe_day_id` discriminator column is added.

### §9.2 `planned` / `ingredient` CFO roles — defined, not materialized (ADR-0009)

The `usage_context.role` CHECK constraint admits all four enum values.
The Go backend writes only `inventory` and `shopping`. Read paths
filter by role explicitly.

### §9.3 `getMealPlan` timezone semantics — client supplies Monday (ADR-0010)

The iOS client computes Monday-of-current-week from
`Calendar.current` with `firstWeekday = 2` (Monday). Sends as
`YYYY-MM-DD` on the wire. The server validates Monday-ness and treats
the date as opaque. The chat-send request carries
`clientWeekStartDate` so server-side tool dispatch always has the
user's "this week".

### §9.4 Day-of-week wire convention — JS-Sunday-first 0..6 (§3.3)

Wire and storage use `day_of_week ∈ 0..6` where `0 = Sunday`. UI
ordering (Monday-first on the plan tab) is a client concern; the
client computes its display order from the storage integers.

### §9.5 Shopping-list-checked-items endpoint — requires list id (ADR-0007)

The endpoint moves to
`DELETE /api/kitchen/shopping-lists/{id}/checked-items`. The source-
pin behavior (operating on the most-recent list regardless of view)
is treated as a defect and **not** preserved.

### §9.6 `update_meal` tool variants — both supported, registered per endpoint (§7.4 and §7.5)

The main chat endpoint registers the seven-day-name variant (§7.4);
the recipe-message endpoint registers the no-day variant (§7.5). The
two tool definitions share a name but differ by schema, which is
permitted because the iOS client never sees the OpenAI tool
registration — only the SSE stream.

### §9.7 Conversation freshness on session start

The source pin's "fresh chat on every session start" behavior
(behavior spec §3.2) is **preserved** by client behavior, not server
behavior. The iOS client calls `POST /api/kitchen/conversations` on
first foreground after launch (or after a configurable inactivity
window) to create a fresh chat. There is no server-side endpoint that
"creates if needed and returns" — every conversation is created
explicitly.

### §9.8 Auto-save case-insensitive de-duplication

The recipe-generation auto-save (§5.4) compares `meal_name` to
`cookbook_recipes.title` using `lower()` equality (using the
`cookbook_recipes_user_title_lower_idx` from §4.4). If the lowered
title matches any existing row, the auto-save is **skipped** —
behavior spec §6.2.

### §9.9 `getMealPlan` (no-week-supplied) behavior

There is **no** `GET /api/kitchen/meal-plan` endpoint that returns
"the current week" without an explicit date. Every meal-plan read
takes `weekStartDate` (§5.2). This is the wire-level enforcement of
ADR-0010 — the server never asks "what week is it?".

### §9.10 `ingredient_memory` references removed from the source

The source pin's `update_ingredients` AI prompt rule
("when the user mentions having or buying an ingredient, immediately
call `update_ingredients`") survives. What changes is the
server-side handler: it writes only `food_items`, not
`ingredient_memory` (ADR-0006).

---

## Cross-references

- Source behavior: `domain/sous-chef-behaviors.md` (the Analyst's
  spec — every claim here is sourced from there, with section
  citations inline).
- Coding standard: `/CODING_STANDARDS.md` (`dc-00`–`dc-07`).
- ADRs gating this contract:
  ADR-0001 (voice cut), ADR-0002 (OpenAI direct),
  ADR-0003 (SIWA + Supabase OTP), ADR-0004 (Supabase Storage images),
  ADR-0005 (NavigationStack), ADR-0006 (drop legacy tables),
  ADR-0007 (shopping list id required),
  ADR-0008 (recipe chat stateless), ADR-0009 (CFO roles preserved),
  ADR-0010 (client-supplied week-start).

---
