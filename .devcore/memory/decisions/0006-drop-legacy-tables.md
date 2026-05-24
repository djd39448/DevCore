---
type: decision
title: Drop legacy recipes and ingredient_memory tables
status: accepted
owner: architect
workload: sous-chef-ios
last_updated: 2026-05-24
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# ADR-0006 — Drop legacy `recipes` and `ingredient_memory` tables

Resolves behavior-spec §9 open question #8 (and the recommendation in the
behavior spec to consolidate under the CFO + cookbook + `meal_plan_days`).
The Architect captures this decision before sealing the contract. Status is
**Proposed** until Dave approves at the `contract` gate.

Depends on the behavior spec (`domain/sous-chef-behaviors.md`) §2.3
*recipes*, §2.7 *ingredient_memory*, and §4.10 *ingredient memory rules
summary*.

Affects contract §4 *Data model — the Supabase schema*.

---

## Context

The source pin carries two legacy tables that predate or duplicate live
storage:

- **`recipes`** (behavior spec §2.3) — A structured-recipe table
  (`name`, `ingredients[]`, `instructions[]`, `cook_time`, `servings`).
  Not actively written by any code path at the pin. Full recipes live as
  markdown on `meal_plan_days.recipe_content` and `cookbook_recipes.content`.
  The `meal_plan_days.recipe_id` FK to `recipes.id` exists in Drizzle but
  is never populated.

- **`ingredient_memory`** (behavior spec §2.7) — Pre-CFO soft inventory
  with `name`, free-form `quantity` text, `confidence`, `last_mentioned`.
  Written **in parallel** with CFO `inventory`-role rows by the
  `update_ingredients` tool for backward compatibility; read by the chat
  context builder and the ingredient-suggestions endpoint. The CFO
  `food_items` row with `usage_context.role = "inventory"` subsumes every
  field (`name` → `canonical_name`; structured `quantity`;
  `confidence` → `metadata.confidence` + `inventory_state.status`;
  `last_mentioned` → `inventory_state.last_confirmed`).

Both tables are pure carry-over from the web app's evolution. A clean
schema for the iOS port is the right moment to drop them — `dc-00` (the
codebase stands on its own) is hostile to two tables that exist only for
historical reasons.

---

## Decision

**Drop both tables from the new Supabase schema.** They do not appear in
contract §4.

- The CFO `food_items` row with `usage_context.role = "inventory"` is the
  sole source of truth for the user's on-hand ingredients.
- Recipe content lives on `meal_plan_days.recipe_content` (the active
  weekly plan's cache) and `cookbook_recipes.content` (the user's saved
  master cookbook).
- The Go backend's `update_ingredients` tool writes only the CFO row; it
  does **not** mirror to a legacy table.
- The Go backend's chat context builder reads ingredients from the CFO
  (filter: `user_id = $1 AND usage_context->>'role' = 'inventory' AND
  inventory_state->>'status' != 'out'`).
- The `meal_plan_days.recipe_id` FK column is removed; the structured
  recipe is gone.

---

## Status

**Accepted.** Approved by Dave at the contract gate, 2026-05-24.

---

## Consequences

### Positive

- Two fewer tables; no dual-write code path; one canonical place for
  ingredients and recipe content.
- The CFO genuinely earns its name — every inventory query has exactly
  one source.
- The new Go backend is simpler — the `update_ingredients` tool writes one
  row, not two.
- `dc-00` is satisfied: a reader sees one inventory model, not two.

### Negative

- The Drizzle schema in the source repo is no longer a 1:1 reference for
  the new schema. Mitigation: the new schema is documented by the contract
  and migrations, not by the legacy Drizzle.
- If a future workload wants structured recipes (vs. markdown), it
  re-introduces a typed table — but that's a feature decision, not a
  history-preservation one.

### Reversibility

Adding either table back is a single migration. The CFO subsumes their
function for v1, so neither is on the critical path.
