---
type: decision
title: Clear-checked-items requires explicit shoppingListId
status: accepted
owner: architect
workload: sous-chef-ios
last_updated: 2026-05-24
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# ADR-0007 — Clear-checked-items endpoint requires `shoppingListId`

Resolves behavior-spec §9 open question #5: the source pin's `DELETE
/api/kitchen/shopping-items/checked` deletes checked items from the
user's most-recent list, regardless of which list the user is viewing.
This is a latent bug. The Architect locks the contract to the corrected
shape.

Status is **Proposed** until Dave approves at the `contract` gate.

Depends on the behavior spec §3.8 *shopping* and §9 open question #5.
Affects contract §5 *REST API surface*.

---

## Context

At the source pin, the web app's "Clear Done" button on a shopping-list
detail page calls `DELETE /api/kitchen/shopping-items/checked` with no
body. The server resolves the target list with
`storage.getShoppingList(userId)`, which returns the user's **most-recent**
list. If the user is viewing an older list (via `?list=<id>` or
`?list=<YYYY-MM-DD>`), checking items and tapping "Clear Done" deletes
items from a **different list** — the most-recent one. The behavior spec
flags this as a "latent bug worth flagging".

On iOS, the user can navigate the calendar to past weeks and tap into
their shopping list. The same bug would be worse: the user's mental model
("I'm clearing this list") and the data action ("delete items from
whichever list happens to be newest") would diverge silently.

Two options:

1. **Preserve the bug.** Carry the broken contract for fidelity.
2. **Fix it in the contract.** Require `shoppingListId` and operate on
   that list only.

The iOS port is explicitly a "may improve" pass on every preserved
feature (workload-spec §4). This is not a feature; it is a defect.

---

## Decision

**The contract requires `shoppingListId` on the clear-checked endpoint.**
The endpoint operates only on the list identified by the caller.

Wire shape:

```
DELETE /api/kitchen/shopping-lists/{shoppingListId}/checked-items
Authorization: Bearer <jwt>
→ 200 { "deleted_count": <int> }
```

The endpoint is **moved** from the flat
`/api/kitchen/shopping-items/checked` path to a nested
`/api/kitchen/shopping-lists/{id}/checked-items` path so the resource it
acts on is explicit at the URL level. Authorization re-checks that the
list belongs to the JWT's `user_id` before deleting.

---

## Status

**Accepted.** Approved by Dave at the contract gate, 2026-05-24.

---

## Consequences

### Positive

- The action matches the user's mental model: "clear checked items on the
  list I am looking at."
- The endpoint URL itself documents the resource (`dc-00`).
- The iOS app's "Clear Done" button passes the list ID it already holds
  in state. No extra fetch.

### Negative

- Not wire-compatible with the web client. The web client is not in scope
  for this port — it continues to use its existing backend — so this is
  a non-issue for v1.

### Reversibility

If a future client needs a "clear checked across all my lists" verb, it
is a new endpoint, not a regression of this one.
