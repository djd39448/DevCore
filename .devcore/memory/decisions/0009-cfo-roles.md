---
type: decision
title: Keep planned and ingredient CFO roles defined but unmaterialized
status: accepted
owner: architect
workload: sous-chef-ios
last_updated: 2026-05-24
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# ADR-0009 — `planned` and `ingredient` CFO roles: defined, not materialized

Resolves behavior-spec §9 open question #9. The CFO `usage_context.role`
enum has four members; only two (`inventory` and `shopping`) are written
by any code path at the source pin. The Architect decides whether the
iOS rebuild materializes the other two (writing `ingredient` rows from
recipe parses, `planned` rows from meal plan generation) or drops them
from the enum.

Status is **Proposed** until Dave approves at the `contract` gate.

Depends on the behavior spec §2.1 *CFO* and §9 open question #9. Affects
contract §4 *Data model — the Supabase schema* and §7 *AI tool-calling
contract*.

---

## Context

The four CFO roles (behavior spec §2.1):

| Role | Status at pin | If we kept the enum value | If we dropped it |
|---|---|---|---|
| `inventory` | Live — `update_ingredients` writes it. | — | — |
| `shopping` | Live — `create_shopping_list` writes it. | — | — |
| `planned` | Defined but never written. Intended for "ingredients reserved for a future planned meal". | Available; the AI could later expand a meal plan into reserved planned ingredients. | The intent disappears from the schema; future feature work has to add it back. |
| `ingredient` | Defined but never written. Intended for "a recipe ingredient line as a typed entity, divorced from any plan or list". | Available; future RAG queries could ask "show me all the recipes that use leeks". | Lost; recipe ingredients stay embedded in markdown text. |

Three options:

1. **Materialize both now** — expand every recipe-generation flow and
   every meal-plan generation flow to write `ingredient` and `planned`
   rows. Significant new behavior; useful AI context; v1 scope creep.

2. **Drop both from the enum** — `usage_context.role` becomes
   `inventory | shopping`. Cleaner now; reintroducing later is an enum
   migration and is reversible.

3. **Keep the enum but don't write them** — the schema documents the
   intent (`dc-00`: a future reader sees the design space); no current
   code writes those values; a `CHECK` constraint allows them; future
   feature work flips a switch in the AI tool implementations without a
   migration.

The workload spec's "must preserve" §4.1 mandates the **CFO shape**:
canonical_name, display_name, quantity, category, attributes,
flexibility, **usage_context.role**, inventory_state, sourcing, metadata.
All four roles are part of that shape. Dropping members from the role
enum to "two it currently writes" is a contraction of the preserved
shape.

The CFO is the architectural anchor of this product. The role enum
encodes "the four faces of the CFO" — that is the product idea, not an
artifact of the code that happens to write two of them.

---

## Decision

**Keep all four role values in the CFO `usage_context.role` enum;
materialize only the two the v1 port writes.**

- The schema's `CHECK` constraint on `usage_context->>'role'` accepts all
  four values.
- The Go backend writes only `inventory` and `shopping`. The contract's
  §7 *AI tool-calling contract* documents `update_ingredients` and
  `create_shopping_list` as the only writers.
- The contract §4 explicitly states: "`planned` and `ingredient` are
  reserved for future feature work; no v1 code path writes them. The
  enum admits them so the schema documents the design."
- Read paths that scan inventory (chat context, shopping-list generation,
  ingredient-suggestions endpoint) filter by `role = 'inventory'`
  explicitly — no path consumes a `planned` or `ingredient` row by
  accident.

---

## Status

**Accepted.** Approved by Dave at the contract gate, 2026-05-24.

---

## Consequences

### Positive

- The CFO design space is fully visible in the schema (`dc-00`).
- Future work that wants to expand a meal plan into reserved ingredients
  is an additive code change, not a schema migration.
- The workload spec's "must preserve the CFO shape" is satisfied
  exactly — no enum members are dropped.
- v1 scope stays bounded — no new tool calls, no new endpoints.

### Negative

- A reader of the live schema may briefly wonder why two enum members
  are unused. The contract and the schema comment will state the
  intent explicitly. This is a documentation cost, not a code cost.
- A bug that wrote a row with `role = 'planned'` would not be caught by
  the constraint. Mitigation: filter every read path on
  `role = 'inventory'` or `role = 'shopping'` explicitly; do not write
  default-empty `usage_context`.

### Reversibility

Materializing later is purely additive — a new AI tool, or an additional
write in the existing meal-plan generation path, plus the read paths
that consume the new role. No migration required.
