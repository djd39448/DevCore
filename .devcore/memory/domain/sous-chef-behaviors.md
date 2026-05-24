---
type: domain
title: Sous Chef AI — Behavior Specification
status: accepted
owner: analyst
workload: sous-chef-ios
last_updated: 2026-05-24
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# Sous Chef AI — Behavior Specification

This document is the behavior contract the iOS rebuild must satisfy. It is the
sole reference an iOS developer needs to construct a feature-equivalent native
app from scratch; the original web codebase is not a dependency. Every claim
here is extracted from commit `d884efae9cc150df2a58afc255b3e631d31b5d2b` of
`~/sous-chef-ai`. Drift since that pin is out of scope.

What this doc is: a description of **what the product does** and the **rules
it follows**, scrubbed of platform mechanism wherever the mechanism is
disposable. What this doc is **not**: an architecture for the iOS rebuild
(see `architecture/contract.md` once the Architect drafts it), a design spec
for SwiftUI screens, or a transcription of the existing endpoints.

The doc is structured as: (1) product summary, (2) domain entities, (3)
behaviors per surface, (4) AI behaviors, (5) REST endpoint surface, (6)
state machines worth calling out, (7) auth flow, (8) must-preserve /
must-cut, (9) open questions for the Architect.

---

## 1. Product summary

Sous Chef AI is a personal kitchen assistant for a single household. It is
mobile-first and conversational. A user opens it to answer one or more of:

- "What can I cook tonight given what's in my kitchen?"
- "Plan dinner for the week, edit parts of the plan, swap individual days."
- "Show me the full recipe for the meal I planned, regenerate it, save it
  to my cookbook."
- "Generate a shopping list from the plan minus the things I already have."
- "Browse the cookbook of recipes I've kept."

Primary user: a busy home cook (the project owner's household). The product
is explicitly **not** a multi-tenant platform — it is per-user data, no
sharing, no roles. The voice and tone of the AI are codified in the system
prompt (Section 4.1): warm, practical, never judgmental, trusting of the
user's claims about what they have, family-friendly by default, time-aware,
30-minute meals unless asked otherwise.

The interaction loop is: **chat → plan → recipe → cookbook ↔ shopping**.
Every loop is anchored on the Canonical Food Object (CFO) — one schema
that backs inventory, planned ingredients, recipe ingredients, and the
shopping list.

---

## 2. Domain entities

Every persistent entity at the pin. Fields, types, invariants, and lifecycle.
Field names are given in the source's snake_case JSONB form where the JSONB
content is itself a contract; the surrounding column names are camelCase in
the Drizzle TS but snake_case in Postgres. Both are listed where it matters.

### 2.1 Canonical Food Object (CFO) — `food_items`

The single object schema for every food-related concept. Inventory, shopping
items, recipe ingredients, and planned ingredients are all **views over the
same row shape**, differentiated by `usage_context.role`.

**Columns** (table `food_items`):

| Field | Type | Required | Notes |
|---|---|---|---|
| `id` | serial PK | yes | |
| `user_id` | varchar (FK users.id) | yes | All CFOs are per-user. |
| `canonical_name` | text | yes | Lowercase, singular, generic. `"milk"`, `"chicken breast"`, not `"Whole Milk"`. |
| `display_name` | text | yes | Human-readable. `"Whole Milk"`. |
| `quantity` | jsonb `{ amount: number, unit: string }` | nullable | e.g. `{ amount: 2, unit: "lb" }`. |
| `category` | jsonb `{ primary, secondary? }` | yes | `primary` is one of the 9 standard categories below. |
| `attributes` | jsonb (open map) | default `{}` | Free-form key/value (`organic: true`, `brand: "X"`). |
| `flexibility` | jsonb | default `{ substitution_allowed: true, acceptable_variants: [], strict: false }` | |
| `usage_context` | jsonb `{ role, required, recipe_ids[], meal_plan_id?, shopping_list_id? }` | yes | `role` is the discriminator. |
| `inventory_state` | jsonb `{ status, on_hand_amount, last_confirmed }` | default `{ status: "unknown", on_hand_amount: null, last_confirmed: null }` | Confidence-aware soft inventory. |
| `sourcing` | jsonb `{ store_affinity, bulk_allowed, generic_ok }` | default `{ store_affinity: null, bulk_allowed: true, generic_ok: true }` | |
| `metadata` | jsonb `{ created_by: "ai" | "user", confidence: number }` | yes | |
| `created_at` / `updated_at` | timestamp | auto | |

**Enumerations:**

- `category.primary` ∈ `{ produce, dairy, meat, seafood, pantry, frozen,
  bakery, beverages, other }`.
- `usage_context.role` ∈ `{ inventory, shopping, planned, ingredient }`.
- `inventory_state.status` ∈ `{ confirmed, likely, unknown, out }`.

**Role semantics (the four faces of the CFO):**

| Role | Meaning | Created by | Lifecycle |
|---|---|---|---|
| `inventory` | Something the user actually has at home. | AI `update_ingredients` tool (`action: "add"`), or manual UI entry. | Status flips to `out` when the user says they're out (the row is **not** deleted — `out` is a state, not a delete). |
| `shopping` | Something on a generated shopping list. | AI `create_shopping_list` tool, or `POST /api/kitchen/generate-shopping-list`. | Tied to a `shopping_list_id`; lives as long as the list does. |
| `planned` | (Defined; not actively written by code at the pin.) Intended ingredient for a planned future meal. | Reserved for future feature work. | — |
| `ingredient` | (Defined; not actively written by code at the pin.) A recipe ingredient line, divorced from any plan or list. | Reserved. | — |

**Uniqueness invariant:**
`(user_id, canonical_name, usage_context.role)` is the de facto unique key.
The same canonical food (e.g. `"milk"`) MAY exist twice for the same user
if the roles differ — for example, `"milk"` as `inventory` (in fridge) and
`"milk"` as `shopping` (on this week's list). `upsertFoodItem` enforces this
by looking up by canonical_name + role and updating in place when found.
Storage rejects an upsert without `usage_context.role`.

**Inventory-state semantics (must preserve):**
- `confirmed` — the user explicitly said they have it.
- `likely` — inferred from context (e.g. AI deduced from a recipe).
- `unknown` — never confirmed.
- `out` — the user said they're out (row kept, status flipped).

`metadata.confidence` is a parallel numeric (0–1) signal: AI sets `1.0`
on `confirmed`, `0.8` on `likely`/inferred.

**Recipe inventory consumption is soft, not subtractive.** The AI is
instructed to **trust the user** about what they have. Nothing in the data
model deducts on-hand amounts when a meal is planned. `inventory_state` is
informational, not transactional.

### 2.2 `meal_plans` and `meal_plan_days`

A meal plan is one week's worth of planned dinners.

`meal_plans`:
- `id` (PK), `user_id`, `week_start_date` (date — Monday of that week),
  `created_at`, `updated_at`.
- **Invariant:** at most one meal plan per `(user_id, week_start_date)`.
  `createMealPlanForWeek` upserts by deleting any existing plan for that
  week (and its days, via cascade) before inserting. So a plan is
  **per-week-replaceable**, not appended.

`meal_plan_days`:
- `id` (PK), `meal_plan_id` (FK, cascade on delete), `day_of_week` (integer
  0–6 where 0=Sunday, 1=Monday, …, 6=Saturday — matches JS `Date.getDay()`),
  `recipe_id` (FK `recipes.id`, nullable — currently unused by the active
  code path), `meal_name` (text), `notes` (text, nullable),
  `recipe_content` (text, nullable — the cached full recipe markdown),
  `recipe_image_prompt` (text, nullable — the cached image prompt).
- A meal plan may have **zero or more days** (typically seven; nothing
  forbids fewer, the chat tool may add days incrementally).
- Updating `meal_name` clears `recipe_content` and `recipe_image_prompt`
  so the next recipe view regenerates fresh content. This is enforced at
  every meal-mutation site (`update_meal` tool, regenerate-days endpoint,
  recipe-chat swap tool).

### 2.3 `recipes`

A legacy structured-recipe table. Not actively written by the current code
paths — full recipes live as markdown on `meal_plan_days.recipe_content`
and `cookbook_recipes.content`. Kept here for backward compatibility, but
the rebuild may drop it.

- `id`, `user_id`, `name`, `description`, `ingredients` (jsonb string[]),
  `instructions` (jsonb string[]), `cook_time` (minutes), `servings`,
  `created_at`.

### 2.4 `cookbook_recipes` — the master cookbook

User-saved recipes that persist across meal plans.

- `id`, `user_id`, `title` (text), `content` (markdown text — the recipe in
  the canonical format defined in Section 4.4), `image_prompt` (text,
  nullable — the prompt used to generate the dish photo), `thumbnail_url`
  (text, nullable — a data-URL of the generated image, persisted after
  user-initiated regeneration), `created_at`.
- Saved by: explicit "save to cookbook" tap on the recipe page; **and**
  automatically by `POST /api/kitchen/generate-recipe/:dayId` after a meal's
  full recipe is generated for the first time (auto-save guarded by
  case-insensitive title match to avoid duplicates).
- The cookbook is the AI's **RAG context** for chat and meal-plan
  generation: titles + first ~200 chars of content are fed back into the
  system prompt so the AI prefers a user's existing recipes for consistency
  (see Section 4.2).

### 2.5 `shopping_lists` and `shopping_list_items`

`shopping_lists`:
- `id`, `user_id`, `name` (text), `week_start_date` (date, nullable —
  null means a general/non-week-tied list), `meal_plan_id` (FK
  `meal_plans.id`, ON DELETE SET NULL), `created_at`.
- **Invariant:** at most one shopping list per `(user_id, week_start_date)`
  when `week_start_date` is set. `createShoppingList` deletes the existing
  list for that week before inserting (cascading its items). General lists
  (null week) are not deduplicated; the user's most-recent is returned by
  `getShoppingList`.

`shopping_list_items`:
- `id`, `shopping_list_id` (FK, cascade), `name` (text), `quantity` (text,
  nullable — note: plain string here, **not** the `{amount, unit}` shape
  of the CFO), `category` (text — same 9-value enum as CFO category),
  `checked` (integer 0 or 1 — boolean stored as int).
- Items are written **alongside** the CFO `shopping`-role rows when the AI
  generates a list. Both representations are kept for backward
  compatibility. The check-off UI mutates `shopping_list_items.checked`
  only; the CFO is unaffected.

### 2.6 `kitchen_conversations` and `kitchen_messages`

The user's multi-conversation chat history.

`kitchen_conversations`:
- `id`, `user_id`, `title` (text, default `"New Chat"`), `created_at`,
  `updated_at`.
- Title auto-generation: on the first user message into a conversation
  whose title is still `"New Chat"` or `"Kitchen Chat"`, the server takes
  the first ~6 words (max 40 chars, ellipsized) of that message and saves
  them as the title.
- Lifecycle: created on `GET /api/kitchen/conversation` (a fresh
  conversation per session opening) **or** `POST /api/kitchen/conversation/new`
  (explicit New Chat button). Never auto-deleted; the user has no delete UI
  for individual conversations at the pin.

`kitchen_messages`:
- `id`, `conversation_id` (FK, cascade), `role` (`"user"` | `"assistant"`),
  `content` (text — the full message body), `metadata` (jsonb, nullable —
  unused at the pin), `created_at`.
- Append-only. The full message text (post-stream) is persisted; the
  per-token deltas are not stored.

### 2.7 `ingredient_memory` (legacy)

Pre-CFO soft inventory.

- `id`, `user_id`, `name` (text, lowercase), `quantity` (text — a
  free-form display string like `"2 lb"`, **not** structured), `confidence`
  (real, 0–1, default 1.0), `last_mentioned` (timestamp), `created_at`.
- Still written **in parallel** with CFO inventory rows by the
  `update_ingredients` tool for backward compatibility. The rebuild can
  drop it — the CFO subsumes its function.

### 2.8 `users` (Replit auth shape)

- `id` (varchar, UUID-as-string, PK — populated from the OIDC `sub` claim),
  `email`, `first_name`, `last_name`, `profile_image_url`, `created_at`,
  `updated_at`.
- The rebuild replaces the population mechanism (OIDC → Apple) but the
  conceptual identity contract — `id` is the user, every per-user table
  joins on it — is preserved.

### 2.9 `sessions` (Replit auth shape, web-only)

- `sid`, `sess` (jsonb — Passport session blob), `expire` (timestamp).
- The cookie-session mechanism is Replit-specific. The iOS port replaces
  cookie sessions with token-based auth; this table does not survive.

---

## 3. Behaviors per surface

The app exposes nine pages (`client/src/App.tsx`). For each, what the user
can do, what is rendered, and the state transitions.

### 3.1 `/` — `landing.tsx` (unauthenticated only)

Shown when the user is not authenticated. Marketing-style page with
"Sign In" / "Get Started Free" buttons. Both targets navigate to
`/api/login` (the Replit Auth OIDC entry — this is the surface that the
iOS rebuild replaces wholesale with Sign in with Apple).

After successful auth, the user lands on `/` (the chat page).

### 3.2 `/` — `chat.tsx` (authenticated)

The default screen post-login. A multi-conversation chat against the
kitchen sous-chef AI.

**Layout:**
- Sidebar with conversation list (hidden on mobile until toggled, always
  open on desktop). Conversations are grouped by recency: Today,
  Yesterday, This Week, This Month, Older.
- "New chat" button at the top of the sidebar.
- Main pane: header showing the active conversation title, scrolling
  message list, chat input at the bottom.
- A welcome state with four suggestion chips ("What can I make with
  what's in my fridge?", "Plan my dinners for the week", "Something quick
  and easy tonight", "I want to try something new") is shown when the
  conversation is empty.

**Behaviors:**
- **On entry:** server creates a fresh conversation via `GET
  /api/kitchen/conversation` (always a new one, per "fresh chat on login"
  rule). The user's list of past conversations is fetched separately via
  `GET /api/kitchen/conversations`.
- **Selecting a past conversation** loads its full message history via
  `GET /api/kitchen/conversation/:id`.
- **Sending a message:** `POST /api/kitchen/message` with `{ content,
  conversationId }`. The server responds with **SSE** (text/event-stream).
  See Section 4.5 for the wire shape. The client appends streaming
  content to the rendered assistant message in real time.
- **After a streamed reply completes:** the client invalidates the
  query caches for `meal-plan`, `shopping-lists`, `cookbook`, and the
  active conversation so any tool-driven side effects are reflected.
- **Conversation title:** the server sets it from the first user
  message on the first send into a `"New Chat"`/`"Kitchen Chat"`
  conversation (auto-titling).

State transitions: `idle → streaming → idle`. Sending a new message while
streaming is disabled.

### 3.3 `/plan` — `meal-plan.tsx`

The week-at-a-time meal-plan screen.

**Layout:**
- Sticky week navigator: ←, week label (`"Nov 4 - Nov 10"`), →. A "Back
  to this week" affordance appears when navigated away from the current
  week.
- A list of seven `MealPlanCard`s, one per day, ordered Mon → Sun (week
  starts Monday).
- Top-right "Edit Plan" / "Cancel" button toggles **edit mode**.
- Bottom button: in normal mode "Add To Shopping List"; in edit mode a
  pair: "Regenerate N Unchecked Days" and "Finalize Plan" (the
  Finalize button enabled only when all days are checked).

**Behaviors:**
- **Fetch:** `GET /api/kitchen/week/:weekStartDate` returns
  `{ mealPlan, shoppingList }` for that week. Date is Monday-of-week as
  `YYYY-MM-DD`.
- **No plan exists:** render an empty state with a "Create Meal Plan"
  button → `POST /api/kitchen/generate-meal-plan { weekStartDate }`.
  The server uses GPT-4.1 (temperature 0.9, randomized cuisine style and
  seasonal focus, JSON-mode) to produce seven `{ dayOfWeek, mealName,
  notes }` items, persists them, returns the populated plan. The
  random-cuisine and random-fallback behavior (Section 4.6) is part of
  the user-visible spec — same prompt produces materially different
  plans.
- **Plan exists, normal mode:** tap a day card → navigate to
  `/recipe/:dayId` (recipe detail). Tap "Add To Shopping List" →
  `POST /api/kitchen/generate-shopping-list` (this generates from the
  **current week's plan**, no body) and navigate to `/shopping`.
- **Plan exists, edit mode:** each card grows a checkbox. The user
  checks the meals they want to **keep**. "Regenerate N Unchecked Days"
  posts `POST /api/kitchen/regenerate-days { weekStartDate, daysToRegenerate
  }` with `daysToRegenerate` as integer-day-of-week indices for the
  un-checked days. Server preserves the checked days untouched and asks
  the AI for new meals only for the unchecked days, with "avoid these
  meals" constraints from the kept ones. "Finalize Plan" is a no-op on
  the data — it exits edit mode and resets the approval set. It is a
  UX commitment moment, not a data transition.

**State machine** (see also Section 6.1): `view (idle) ↔ edit-mode →
regenerate-some ↔ edit-mode → finalize → view`.

### 3.4 `/recipe/:dayId` — `recipe.tsx`

The detail page for a single meal plan day. Shows the full recipe, the
generated dish image, and a single-thread "questions and changes" chat.

**Layout:**
- Header: ← back, meal name + day label, "save to cookbook" button (with
  Check icon once saved), "regenerate" (refresh) button to reset and
  regenerate the recipe.
- Image area (above the recipe): three states — placeholder with
  "Tap to generate photo" before generation, spinner during generation,
  rendered image afterwards.
- Recipe markdown rendered inline.
- Chat input pinned to the bottom for follow-up questions or swaps.

**Behaviors:**
- **On entry:** fetch `GET /api/kitchen/meal-plan-day/:id`. Ownership is
  verified server-side (the join returns 403 if the day's plan belongs
  to another user).
- **Recipe content:** if `day.recipe_content` is already cached, it
  renders immediately. Otherwise the client opens an SSE stream to
  `POST /api/kitchen/generate-recipe/:dayId`. The server:
  - Looks up the user's cookbook first. If the meal name (case-insensitive
    exact-match on title) is already in the cookbook, the cookbook's
    `content` is streamed back directly **without re-asking the AI**, and
    that content is written to the meal plan day. (This is the
    "cookbook-first recipe loading" rule.)
  - Otherwise GPT-4.1 streams a fresh recipe in the canonical markdown
    format (Section 4.4). The server persists `recipe_content` and a
    pre-computed `recipe_image_prompt` (a templated string referencing
    the meal name); the cookbook is updated by an auto-save if the meal
    name doesn't already exist there.
  - SSE messages: `{ content: "..." }` deltas, then a terminal
    `{ imagePrompt: "...", done: true }`.
- **Image:** generated **only on user tap** of the placeholder. POST
  `/api/kitchen/regenerate-image { prompt }` → server calls
  `openai.images.generate({ model: "gpt-image-1", size: "1024x1024", n: 1 })`,
  returns a `data:image/png;base64,...` data URL. (The image is **not**
  saved to the meal plan day, but if the request includes
  `cookbookRecipeId`, the data URL is written to that cookbook recipe's
  `thumbnail_url`.)
- **Save to cookbook:** `POST /api/kitchen/cookbook { title, content,
  imagePrompt }`. Note: the recipe is also **auto-saved** by the recipe-
  generation endpoint, so this button is mostly idempotent — it serves as
  user reassurance.
- **Recipe-thread chat:** the chat input under the recipe posts
  `POST /api/kitchen/recipe-message` (a separate SSE endpoint, see Section
  4.7). This is a **stateless** conversation (no persisted history) scoped
  to the current dayId/mealName. The AI here can answer questions or call
  a restricted `update_meal` tool to swap the day's meal. On swap, the
  client resets recipeContent/imagePrompt and regenerates.

### 3.5 `/cookbook` — `cookbook.tsx`

A flat list of all saved cookbook recipes.

- Search box (client-side filter on title and content).
- Each recipe card shows title, saved-on date, thumbnail (if generated),
  and a trash icon to delete (`DELETE /api/kitchen/cookbook/:id`).
- Tap a card → navigate to `/cookbook/:id`.
- Empty state when no recipes are saved.

Data: `GET /api/kitchen/cookbook` returns the user's recipes,
newest-first.

### 3.6 `/cookbook/:id` — `cookbook-recipe.tsx`

A single saved recipe, viewable and editable.

**Layout:**
- Header: ← back, title (read-only or text input in edit mode), pencil
  (enter edit) or save+cancel buttons (in edit mode).
- Image area: same three states as the recipe page (placeholder, spinner,
  image). Regenerated images are persisted to `thumbnail_url` so they
  survive page reloads (this is what allows the cookbook list view to
  show thumbnails without re-generating).
- Body: markdown-rendered recipe in view mode; in edit mode a
  `Textarea` plus an **Ingredient Helper** widget above.

**Ingredient Helper (edit mode):**
- A small panel with quantity dropdown (`1/4`–`4` plus halves/thirds),
  unit dropdown (`cup`, `tbsp`, `tsp`, `oz`, `lb`, `each`, `clove`,
  `slice`, `can`, `pkg`), ingredient name text input, and an Add (+)
  button. Inserts `"{qty} {unit} {ingredient}"` as a `- ` bullet under
  the `## Ingredients` heading in the markdown.
- Shows up to 8 "Quick" chips of CFO suggestions from
  `GET /api/kitchen/ingredient-suggestions` (returns up to 20 deduped
  CFO `inventory`-role rows + legacy ingredient memory).

**Save:** `PUT /api/kitchen/cookbook/:id { title, content, imagePrompt? }`
with server-side validation that both `title` and `content` are non-empty
strings.

### 3.7 `/calendar` — `calendar.tsx`

Read-only browse of all weeks the user has touched.

- Two view modes: **week** (default — same week-view layout the plan tab
  uses but read-only) and **month** (calendar grid where each day cell
  shows a dot if its week has a plan, and each week-row has a "view plan"
  and "view list" affordance when those exist).
- Tapping a day cell in week view navigates to `/recipe/:dayId`.
- Tapping "view plan" or selecting a week navigates to that week in week
  view.
- Tapping "view list" navigates to `/shopping?week=YYYY-MM-DD`.
- The calendar **does not generate plans** at the pin — it is drill-down
  only. The empty-week state directs the user to the Plan tab.

Data: `GET /api/kitchen/calendar` returns `{ mealPlans[], shoppingLists[] }`
summaries used to dot the grid; `GET /api/kitchen/week/:weekStartDate`
gets the week detail.

### 3.8 `/shopping` and `/shopping?list=ID-or-DATE` — `shopping.tsx`

Two views in one page, switched by the `list` query param.

**Index (no param):** list of all the user's shopping lists, ordered
newest-first, each card showing list name and week (or created date if
not week-tied). Tap a card → navigate to `/shopping?list=<id-or-date>`
which becomes that list's detail. The identifier is the `week_start_date`
if present, else the numeric list id.

**Detail (`?list=...`):**
- Items grouped by category in the fixed display order: produce, meat,
  dairy, bakery, frozen, pantry, beverages, other.
- Each item shows name, optional quantity string, and a checkbox.
- Header: ← back (to index), list title, "Clear Done" button visible
  when any items are checked.
- Toggling an item: `PATCH /api/kitchen/shopping-item/:id { checked }`.
- "Clear Done": `DELETE /api/kitchen/shopping-items/checked` — deletes
  all checked items from the user's **most-recent** list (the server-side
  endpoint, at the pin, operates on `getShoppingList(userId)`, **not** on
  the currently-viewed list. This is a latent bug worth flagging — see
  Section 9 open questions).
- "All done!" celebration banner when every item is checked.

Lookup at `GET /api/kitchen/shopping-list/:identifier`: server interprets
a numeric identifier as a list id (scoped to the user), otherwise as a
`week_start_date`.

### 3.9 `not-found.tsx`

Static 404 card. Trivial.

---

## 4. AI behaviors

The conversational and generative core. Specified here in enough detail to
re-implement against any model that supports OpenAI-style tool calls and
streaming.

### 4.1 System prompt

The AI persona is fixed by `SOUS_CHEF_SYSTEM_PROMPT` in
`server/openai.ts`. The relevant rules:

- Persona: **warm, supportive, never judgmental** about cooking skills or
  food choices.
- Output style: **quick and practical** — give suggestions without
  overexplaining.
- Trust: **if the user says they have an ingredient, believe them.** Do not
  argue about the contents of their fridge.
- Defaults: family-friendly meals, 30-minute-or-less recipes unless the
  user asks for otherwise.
- Ingredient memory: when the user mentions having or buying an
  ingredient, immediately call `update_ingredients`.
- Meal-plan triggers: must call `create_meal_plan` whenever the user
  asks for a "weekly plan", "meal plan", "plan my week", or accepts a set
  of suggestions to schedule.
- Shopping-list triggers: must call `create_shopping_list` when the user
  asks for a shopping list.
- Single-day swap: must call `update_meal` when the user wants to change
  one specific day (`"make beef fajitas for Tuesday"`, `"swap Wednesday
  for pizza"`).
- Ingredient normalization: canonical names are lowercase, singular,
  generic (`"chicken breast"`, not `"Chicken Breasts"`).

The prompt is augmented at runtime with:

- **Ingredient context** — the user's current `ingredient_memory` (name +
  quantity string) inserted as a "User's current ingredients on hand"
  block. Empty case is stated explicitly: "User has not mentioned any
  ingredients yet."
- **Cookbook RAG context** — up to 15 cookbook recipes, each with title +
  first ~200 chars of content. The system prompt explicitly says: "When
  the user asks for a recipe that matches one in their cookbook, use the
  EXACT recipe from their cookbook to maintain consistency. Don't create
  new versions of saved recipes."

Conversation history fed to the model is the **last 10 messages** of the
active conversation (sliced server-side before the call).

### 4.2 Model and parameters

| Use | Model | Temperature | Other |
|---|---|---|---|
| Chat (`/api/kitchen/message`) | `gpt-4.1` | (default) | streaming, `tool_choice: "auto"`, `max_completion_tokens: 2048` |
| Meal plan generation | `gpt-4.1` | `0.9` | `response_format: json_object`, `max_completion_tokens: 1024` |
| Days regeneration | `gpt-4.1` | `0.95` | `response_format: json_object`, `max_completion_tokens: 512` |
| Single-recipe stream | `gpt-4.1` | (default) | streaming, no tools |
| Recipe-page chat | `gpt-4.1` | (default) | streaming, single tool: `update_meal` (different shape — see 4.7) |
| Shopping list (fallback) | `gpt-4.1` | (default) | `response_format: json_object`, `max_completion_tokens: 1024` |
| Images | `gpt-image-1` | n/a | `size: "1024x1024"`, `n: 1`, returns `b64_json`. |

These are the source-pin choices. The iOS port may swap providers (open
decision §6.2 in the workload spec); the **tool-call shape and the
overall capability** are what must survive.

### 4.3 The four tool calls

These are the AI's structured side effects. The exact argument schemas:

#### `update_ingredients`

Update the user's CFO inventory.

```
{
  ingredients: [
    {
      canonical_name: string,   // required, lowercased before write
      display_name?: string,    // defaults to canonical_name if absent
      quantity?: { amount: number, unit: string },
      category: "produce"|"dairy"|"meat"|"seafood"|"pantry"|"frozen"|"bakery"|"beverages"|"other",   // required
      status?: "confirmed"|"likely"|"out",  // defaults to "confirmed"
      action: "add"|"remove"    // required
    }, ...
  ]
}
```

Semantics:
- `add`: upsert a CFO row with `usage_context.role = "inventory"`,
  `inventory_state.status = status` (default `"confirmed"`),
  `metadata.created_by = "ai"`, `metadata.confidence = 1.0` if confirmed
  else `0.8`. Also writes the legacy `ingredient_memory` row.
- `remove`: looks up the existing inventory-role row by canonical_name +
  role, flips `inventory_state.status` to `"out"` and zeros
  `on_hand_amount`. The row is **kept**. The legacy ingredient_memory
  row, if present, is deleted.
- Called when the user says things like "I have chicken" or "I bought
  tomatoes" or "I'm out of milk".

#### `create_meal_plan`

Create a weekly plan from a user-driven set of meals.

```
{
  meals: [
    { dayOfWeek: 0..6, mealName: string, notes?: string },
    ...
  ]
}
```

- `dayOfWeek`: 0 = Sunday, 1 = Monday, …, 6 = Saturday. (Matches JS
  `Date.getDay()`. Note the plan-tab UI **renders** Mon → Sun, but the
  storage key is JS-Sunday-first.)
- Creates a meal plan for the current week (per `getWeekStartDate()`,
  which is Monday-of-this-week). Replaces any existing plan for the
  same week.
- Called when the user expresses a desire to schedule meals.

#### `create_shopping_list`

```
{
  items: [
    {
      canonical_name: string,
      display_name?: string,
      quantity?: { amount: number, unit: string },
      category: "produce"|...|"other",
      substitution_allowed?: boolean,  // default true
      generic_ok?: boolean             // default true
    }, ...
  ]
}
```

- Creates a new shopping list for the current week (with `meal_plan_id`
  if the current week has a plan), then for each item:
  - Writes a CFO row with `usage_context.role = "shopping"`,
    `flexibility.substitution_allowed`, `sourcing.generic_ok`,
    `metadata.created_by = "ai"`, `confidence: 0.9`.
  - Writes a parallel `shopping_list_items` row using
    `display_name || canonical_name` for the legacy `name` field and
    formatting `quantity` as `"{amount} {unit}"`.

#### `update_meal`

Swap one day in the **current week's** plan.

```
{
  day: "monday"|"tuesday"|"wednesday"|"thursday"|"friday"|"saturday"|"sunday",
  mealName: string,
  notes?: string
}
```

- Maps `day` to its `dayOfWeek` integer.
- If no plan exists for the current week: creates one with just this day.
- If a plan exists and the day already has a row: updates `mealName` and
  `notes`, **clears** `recipeContent` and `recipeImagePrompt` so the
  recipe regenerates on next view.
- If a plan exists but the day has no row: inserts the row.

### 4.4 Canonical recipe markdown format

Every full recipe — generated by `/api/kitchen/generate-recipe/:dayId`,
streamed back by `/api/kitchen/recipe-message`, saved in `cookbook_recipes`
— follows this exact shape:

```
# {Meal Name}
[1-2 sentence appetizing description]

**Prep Time:** X minutes | **Cook Time:** X minutes | **Serves:** X

## Ingredients
- [quantity] [ingredient in lowercase singular form]
- ...

## Instructions
1. [Step with specific temperatures and times]
2. ...

## Tips (optional)
- [Optional tip]
```

The client renders this with a hand-rolled markdown subset (`#`, `##`,
`###`, `**bold**`, `- bullets`, `1. ordered`). It does **not** use a
full Markdown library.

### 4.5 Streaming protocol (chat SSE)

The chat send (`POST /api/kitchen/message`) responds with
`Content-Type: text/event-stream`. Each event is a line:
`data: <JSON>\n\n`.

Message shapes the client must handle:

| Shape | When | Client action |
|---|---|---|
| `{ content: "..." }` | per content-token delta | Append to the rendered assistant message. |
| `{ done: true }` | end of stream | Stop streaming; final message has been persisted server-side. |
| `{ error: "..." }` | server error mid-stream | Surface error, end stream. |

Tool-call mechanics happen **server-side** inside `streamChatCompletion`
(see `server/openai.ts`). The client never sees raw tool-call deltas —
the server consumes the model's tool_call deltas, executes them via
`handleToolCall`, and then either emits an assistant content delta with
a confirmation (`"Done! I've updated that for you."` when no model
content was generated) or just lets the model's accompanying narration
flow through. The server-side generator yields a `tool_result` event
internally but does **not** forward it to the wire today — the route
handler only writes `content` and `done`. The wire protocol effectively
exposes only `content` and `done`/`error`.

**Buffer rule:** the parser must accumulate incomplete chunks across
network reads (split on `\n`, keep the final non-terminated line as a
pending buffer for the next chunk). This is explicit in the source
client code and was a fixed bug; the iOS rebuild must do the same.

### 4.6 Direct meal-plan generation (no tools)

`POST /api/kitchen/generate-meal-plan { weekStartDate? }` does **not**
go through tool-calling. It is a one-shot completion in JSON mode with:

- A random pick from a cuisine-style list: `Italian`, `Mexican`, `Asian`,
  `American comfort`, `Mediterranean`, `Southern`, `Tex-Mex`, `Greek`,
  `Indian-inspired`, `French bistro`.
- A seasonal focus: "hearty, warming" Oct–Feb, else "fresh, lighter".
- The user's known ingredients (from `ingredient_memory`) and up to 20
  cookbook titles as RAG context.
- Temperature 0.9, asks for a `{ meals: [{ dayOfWeek, mealName, notes }] }`
  envelope and tolerates four alternate keys (`meals`, `mealPlan`, `plan`,
  or a bare array) when parsing.
- **Fallback if the AI returns nothing parseable:** a hand-coded variety
  table is sampled. Seven slots, each with a small pool of 3–4
  alternatives, plus a notes pool of cooking-time strings — randomized
  per call. This guarantees a plan even if the model is unavailable.

`POST /api/kitchen/regenerate-days { weekStartDate, daysToRegenerate[] }`
follows the same pattern at temperature 0.95, with an "avoid these
meals" clause for the kept days, and a smaller fallback list. It updates
only the requested day rows.

### 4.7 Recipe-page chat (separate endpoint and tool surface)

`POST /api/kitchen/recipe-message { content, dayId, mealName, dayName,
currentRecipe? }` is a **stateless** stream (no `kitchen_messages`
persistence) scoped to one recipe page. The system prompt is rebuilt
per call from the meal name and day name; conversation history is **not**
forwarded.

The tool surface here is **only** `update_meal`, but with a *different*
schema — no `day` parameter, because the day is implicit from `dayId`:

```
{ mealName: string, notes?: string }
```

When called, the server updates the specified `dayId`'s row (clearing
`recipe_content` and `recipe_image_prompt`) and sends a terminal
`{ done: true, updatedMeal: { mealName, notes } }`. The client then
resets its in-page recipe state and re-runs `/api/kitchen/generate-recipe`.

### 4.8 Image generation

- Endpoint: `POST /api/kitchen/regenerate-image { prompt,
  cookbookRecipeId? }`.
- Calls `openai.images.generate({ model: "gpt-image-1", prompt, n: 1,
  size: "1024x1024" })`, expects `b64_json` back, returns the data URL
  `data:image/png;base64,{base64}`.
- If `cookbookRecipeId` is provided, the data URL is persisted to that
  recipe's `thumbnail_url` (after re-confirming ownership).
- **Generation is always user-initiated** — image creation never happens
  passively in a list view, only when the user taps the placeholder.

### 4.9 Conversation memory

- The chat send always loads `ingredients` (CFO-backed via legacy
  ingredient memory) and `cookbookRecipes` and injects them into the
  system prompt.
- Conversation history fed to the model is the last 10 messages.
- Inter-conversation memory is **implicit**: ingredients and cookbook
  persist across all conversations; the per-conversation message
  history does not cross.

### 4.10 Ingredient memory rules summary

- The CFO `inventory` row is canonical.
- `confidence` and `inventory_state.status` together encode soft
  inventory.
- `out` is a state flip, not a delete (lets the user re-confirm later).
- `confirmed`/`likely` differ by whether the user explicitly said they
  have something vs. the AI inferring.
- The AI is instructed to trust the user; nothing in the data path
  contradicts a user claim.

---

## 5. REST API surface

Every endpoint under `/api/kitchen/*` (plus `/api/auth/*`). The Architect
re-specs these as the contract; here they are listed for completeness.

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/auth/user` | Current authenticated user (or 401). |
| GET | `/api/login`, `/api/callback`, `/api/logout` | Replit OIDC dance (cut from iOS). |
| GET | `/api/kitchen/conversation` | Create and return a fresh conversation (empty messages). |
| GET | `/api/kitchen/conversations` | List all of the user's conversations, newest-updated first. |
| POST | `/api/kitchen/conversation/new` | Explicit new-conversation. |
| GET | `/api/kitchen/conversation/:id` | Conversation + messages, ownership-checked. |
| POST | `/api/kitchen/message` | Send a chat message. SSE stream. |
| GET | `/api/kitchen/meal-plan` | Current-week meal plan with days. |
| GET | `/api/kitchen/calendar` | All meal plans + shopping lists (summaries). |
| GET | `/api/kitchen/week/:weekStartDate` | `{ mealPlan, shoppingList }` for one week. |
| POST | `/api/kitchen/generate-meal-plan { weekStartDate? }` | Generate a full week. |
| POST | `/api/kitchen/regenerate-days { weekStartDate, daysToRegenerate[] }` | Regenerate subset of a week. |
| GET | `/api/kitchen/meal-plan-day/:id` | Ownership-checked single day. |
| POST | `/api/kitchen/generate-recipe/:dayId` | Stream + persist a full recipe. SSE. |
| POST | `/api/kitchen/recipe-message` | Recipe-page chat (with `update_meal` tool). SSE. |
| GET | `/api/kitchen/shopping-list` | User's most-recent list. |
| GET | `/api/kitchen/shopping-lists` | Index of all the user's lists. |
| GET | `/api/kitchen/shopping-list/:identifier` | By id or week_start_date. |
| POST | `/api/kitchen/generate-shopping-list` | Generate from current meal plan + ingredients. |
| PATCH | `/api/kitchen/shopping-item/:id { checked }` | Toggle item check state. |
| DELETE | `/api/kitchen/shopping-items/checked` | Delete checked items from user's most-recent list. |
| GET | `/api/kitchen/ingredients` | Legacy ingredient memory rows. |
| GET | `/api/kitchen/ingredient-suggestions` | Up to 20 deduped CFO inventory + ingredient_memory names. |
| GET | `/api/kitchen/cookbook` | All saved recipes, newest-first. |
| GET | `/api/kitchen/cookbook/:id` | One recipe (ownership-checked). |
| POST | `/api/kitchen/cookbook { title, content, imagePrompt? }` | Save a recipe. |
| PUT | `/api/kitchen/cookbook/:id { title?, content?, imagePrompt? }` | Update (validates non-empty title/content). |
| DELETE | `/api/kitchen/cookbook/:id` | Delete. |
| POST | `/api/kitchen/regenerate-image { prompt, cookbookRecipeId? }` | Generate a dish image (data URL). |

All `/api/kitchen/*` endpoints require auth (`isAuthenticated` middleware
at the pin). The iOS port replaces the auth mechanism but keeps the
per-user scoping of every endpoint.

---

## 6. State machines worth calling out

### 6.1 Meal-plan edit / approve / regenerate

States: `view`, `edit`, `regenerating`. Transitions:

- `view → edit`: tap "Edit Plan".
- `edit → edit` (toggle approval): tap a day's checkbox to add/remove from
  the `approvedDays` set.
- `edit → regenerating`: tap "Regenerate N Unchecked Days". POST with the
  unchecked day-of-week integers.
- `regenerating → edit`: server returns the updated plan; `approvedDays`
  is reset to empty and edit mode persists.
- `edit → view`: tap "Cancel", **or** tap "Finalize Plan" once all days
  are approved. Finalize is purely UX — no server call.
- `view → view` (no plan): tap "Create Meal Plan" → call generate-meal-plan
  with the current week → server returns a fresh seven-day plan.

### 6.2 Recipe auto-generation + caching

- On entry to `/recipe/:dayId`, fetch the day.
- If `day.recipe_content` is non-null: render it immediately. Save
  `day.recipe_image_prompt` to state but do **not** generate the image.
- If null: open SSE to generate-recipe.
  - Server check: cookbook hit (case-insensitive title equality)?
    - Yes: stream the cookbook content, update the meal plan day to
      cache it.
    - No: ask the AI; stream content; persist `recipe_content` and
      `recipe_image_prompt`; auto-save to cookbook (skip if title is
      already there).
- Image: never auto-generated. User taps placeholder → POST
  regenerate-image → set image URL in state.

### 6.3 Cookbook save and image-prompt regeneration

- Save: explicit (recipe page button) or auto (after recipe generation
  for a never-saved title).
- Image: in the cookbook view, recipes have a `thumbnail_url` only after
  the user has tapped "Tap to generate photo" at least once on the
  recipe-detail or cookbook-detail page. The thumbnail is persisted so
  the index view shows it on subsequent visits.

### 6.4 Conversation lifecycle

- `GET /api/kitchen/conversation` always **creates** a new conversation
  (titled `"New Chat"`). Session-start = fresh chat.
- Title becomes the first ~6 words of the first user message.
- A separate "New chat" button creates additional empty conversations.
- The sidebar groups by recency. Updates happen on every message
  (`updated_at` is touched).
- No delete UI at the pin. (Possible future feature.)

### 6.5 SSE chat with tool calls

- Client opens POST stream to `/api/kitchen/message`.
- Server builds the system prompt with ingredient + cookbook context,
  fetches last-10 messages, calls OpenAI with `stream: true` and the four
  tools.
- Server iterates model deltas:
  - Content deltas → forwarded to wire as `{ content }`.
  - Tool-call deltas → accumulated server-side; on stream end, each is
    JSON-parsed and dispatched via `handleToolCall` (CFO inventory writes,
    meal plan creation, shopping list creation, meal updates).
- If only tool calls fired and no content, server emits a confirmation
  string `"Done! I've updated that for you."` to the wire.
- Server persists the assistant message text and sends `{ done: true }`.
- Client invalidates cached queries that may have been mutated.

---

## 7. Auth flow

### 7.1 What it does today

- Web client redirects to `/api/login`. Server runs Passport's OIDC strategy
  pointing at Replit (`https://replit.com/oidc` by default), with
  `REPL_ID` as the client. On callback, a Postgres-backed Express
  session is created (`connect-pg-simple` writes to `sessions`), the
  user row is upserted from OIDC claims (`sub` → `users.id`, plus email
  and profile fields), and a cookie is set with a one-week TTL.
- Every `/api/kitchen/*` route is gated by `isAuthenticated`, which
  pulls `userId = req.user.claims.sub` and refreshes the OIDC token on
  expiry.
- `useAuth()` on the client polls `GET /api/auth/user` and treats 401 as
  "show landing".

### 7.2 What survives the iOS port

- **Identity**: every per-user row joins on `users.id`. That column
  remains the primary key.
- **Session lifetime**: a long-lived signed-in state (~1 week) — the
  user opens the app and is signed in unless they explicitly log out.
- **Route protection**: every API endpoint is per-user-scoped and
  rejects unauthenticated requests.
- **User profile fields**: email, first/last name, profile image URL,
  created/updated timestamps.

### 7.3 What does not survive

- Replit OIDC, Passport, cookie sessions, the `sessions` table, and the
  `ISSUER_URL`/`REPL_ID`/`SESSION_SECRET` environment contract.
- Cookie-credentialed `fetch` calls. The iOS client uses a token.

### 7.4 Implications for the iOS contract

- The user identifier on iOS comes from Supabase Auth + Sign in with
  Apple (per the workload spec re-platform map). The `users.id` column
  in the new schema must be populated from the Apple identity token
  `sub`, not from Replit's OIDC `sub`. Existing Replit-issued user IDs
  are **not migrated** (a separate decision, but no user-data migration
  is in scope for this port).
- Sign in with Apple is preferred for App Store submission. Whether
  Supabase email/OTP is **also** offered is an open decision (workload
  spec §6.3).

---

## 8. Must preserve / must cut

This section restates workload-spec sections 4 and 5 in the behavior
context this document establishes.

### 8.1 Must preserve

1. **The CFO shape (Section 2.1).** Inventory, shopping, planned, and
   ingredient are all the same row shape with `usage_context.role` as
   the discriminator. The four-state `inventory_state.status` and the
   confidence value preserve the **soft inventory** semantics that
   define the product feel ("trust the user about their fridge").
2. **The four tool calls (Section 4.3).** Names, argument shapes,
   semantics. `update_ingredients`, `create_meal_plan`,
   `create_shopping_list`, `update_meal`. Tool-arg field names are part
   of the contract.
3. **The product surfaces (Section 3).** All nine routes/screens, with
   the behaviors described — chat with multi-conversation history,
   plan with edit-mode + selective regeneration, recipe with stream +
   image, cookbook (browse / edit / delete), shopping by-week, calendar
   week + month views.
4. **Soft inventory semantics (Section 4.10).** `confirmed`/`likely`/
   `unknown`/`out`. `out` is a state, not a delete.
5. **Image-prompt-not-bytes storage (Section 2.4 / 4.8).** Recipe images
   are stored as prompts. Generation is user-initiated. Thumbnails are
   persisted only after explicit generation.
6. **Canonical recipe markdown format (Section 4.4).** Same headings,
   same time/serves line, same ingredient bullets, same instruction
   ordering. The renderer is reimplemented; the format is the contract.
7. **Cookbook-first recipe loading + auto-save (Section 6.2).** A
   meal whose name matches an existing cookbook recipe streams that
   recipe back instead of regenerating. A newly-generated recipe is
   auto-saved to the cookbook.
8. **SSE streaming with `{ content }` and `{ done }` framing (Section
   4.5).** The wire protocol is preserved as the model for native
   `URLSession.bytes` consumption.
9. **AI persona and prompt rules (Section 4.1).** The warmth, the trust
   rule, the 30-minute default, the trigger-on-keywords rules for tool
   calls.

### 8.2 Must cut

1. **Replit Auth (OIDC), Passport, `connect-pg-simple`, the `sessions`
   table.** Section 7 — replaced wholesale.
2. **The entire `server/replit_integrations/audio/` and
   `client/replit_integrations/audio/` trees.** They are dead code at
   the pin (`registerAudioRoutes` is never called; the React shell has
   no microphone UI). See ADR-0001 for the voice decision.
3. **The entire `server/replit_integrations/batch/`,
   `server/replit_integrations/chat/`, `server/replit_integrations/image/`
   trees if not referenced by the active code path** — verify on the
   way out; most are scaffolding. Anything that imports `@replit/*` is
   out.
4. **Replit Vite plugins** (`@replit/vite-plugin-*`) and the `.replit`
   runtime config. iOS toolchain replaces them.
5. **`scripts/` and `script/` shell helpers** driving the Replit dev
   loop.
6. **The legacy `ingredient_memory` table** (Section 2.7). The CFO
   subsumes it; the rebuild can drop the parallel writes.
7. **The legacy `recipes` table** (Section 2.3). Unused by the active
   code paths.
8. **Cookie-credentialed fetch and the `credentials: "include"`
   pattern.** iOS uses tokens.
9. **Replit AI Integrations proxy environment variables**
   (`AI_INTEGRATIONS_OPENAI_API_KEY`, `AI_INTEGRATIONS_OPENAI_BASE_URL`).
   Direct OpenAI (or compat — open decision) in the Go backend.

### 8.3 Additional platform leakage discovered while reading

- `useDocumentTitle` (browser tab title hook) — irrelevant in iOS.
- `theme-provider` / dark-mode via CSS variables — iOS uses native
  appearance, not CSS custom properties.
- `Wouter` route param destructuring — replaced by SwiftUI navigation.
- The data-URL image storage approach (`data:image/png;base64,...`)
  scales poorly. The contract should decide on byte-storage
  (Supabase Storage URL) vs. on-device caching. This is workload-spec
  open decision §6.4.

---

## 9. Open questions for Architect

These items need a decision before the contract is sealed. The first four
are workload-spec §6.2–§6.5 verbatim; the rest are ambiguities the
behavior reading turned up.

1. **OpenAI provider for the Go backend.** Direct OpenAI vs. a compat
   provider (Anthropic, Bedrock). Tool-call shape matches OpenAI today;
   if the provider changes, the shape moves with it. The behavior
   contract here assumes OpenAI-style tool calls but does not depend on
   the actual provider — re-targeting is mechanical, not behavioral.

2. **Auth: Sign in with Apple only, vs. Supabase email/OTP + SIWA.**
   Mobile apps almost always need SIWA to ship on the App Store. The
   behavior spec is provider-agnostic; the architect picks the
   provider mix.

3. **Cookbook image storage.** Today: prompts only, regenerate on view,
   thumbnail data-URL after first regeneration. On iOS this means a
   network round-trip per generation. Decide whether to cache generated
   image bytes on-device (and at what eviction policy) or push them to
   Supabase Storage.

4. **Mealplan-week navigation as URL state vs. SwiftUI navigation
   state.** The web app uses query params (`?list=...`,
   `?week=...`); iOS has a richer model. Deferred to the iOS track plan
   per workload spec; flagged here.

5. **Clear-checked-items endpoint targets the wrong list.** `DELETE
   /api/kitchen/shopping-items/checked` deletes checked items from the
   user's **most-recent** list via `storage.getShoppingList(userId)`,
   regardless of which list the user is viewing. The iOS rebuild
   should accept a `shoppingListId` and operate on that list. Flagged
   as both a UX issue and a port-time fix.

6. **`getMealPlan(userId)` returns only the current week.** The
   "current week" definition uses a Monday-start week and JS-Sunday-first
   day-of-week numbering. The Architect should pin the timezone
   semantics: today the server runs in the server's local time. On iOS
   this is decided per the device's clock — the contract should specify
   "user's local week" and how the server interprets the date string.

7. **Day-of-week numbering: 0=Sunday in storage, but rendered Mon→Sun.**
   Worth re-stating in the contract that storage uses JS-Sunday-first
   integers while UI orders Monday-first. The iOS side benefits from
   sticking to the storage convention to keep the wire compatible.

8. **`recipes` and `ingredient_memory` legacy tables.** Drop them in the
   new schema, or carry them along? Recommend drop (consolidation under
   CFO + cookbook + meal_plan_days).

9. **`planned` and `ingredient` CFO roles are defined but unused at the
   pin.** Whether the rebuild materializes them (e.g. expanding a recipe
   into `ingredient`-role CFO rows for the chatbot to reason about)
   should be an explicit yes/no in the contract.

10. **Voice features.** Resolved in ADR-0001 in this directory
    (`decisions/0001-voice-features.md`). The behavior spec assumes
    "cut, no voice surface" — if the human flips the decision at the
    gate, the chat surface (Section 3.2) needs a microphone affordance
    added back.
