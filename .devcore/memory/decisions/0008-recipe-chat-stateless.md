---
type: decision
title: Recipe-page chat stays stateless
status: accepted
owner: architect
workload: sous-chef-ios
last_updated: 2026-05-24
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# ADR-0008 — Recipe-page chat stays stateless

Resolves behavior-spec §9 open question #10. The recipe-page follow-up
chat (`POST /api/kitchen/recipe-message`) is **stateless** at the source
pin: no `kitchen_messages` row is written, no history is forwarded to the
model, and the conversation evaporates on leaving the page. The Architect
must decide whether the iOS port preserves that or promotes the
recipe-page chat to a first-class persisted conversation.

Status is **Proposed** until Dave approves at the `contract` gate.

Depends on the behavior spec §3.4 *recipe page*, §4.7 *recipe-page chat*,
and §2.6 *kitchen_conversations / kitchen_messages*. Affects contract
§5 *REST API surface* and §6 *Streaming*.

---

## Context

At the source pin, the recipe-page chat is a single-shot SSE stream
scoped to one meal-plan day. The system prompt is rebuilt per call from
`mealName` and `dayName`; the only tool is a restricted `update_meal`
variant (no `day` parameter — the day is implicit from `dayId`); no
history is persisted; nothing carries across page navigations.

Two coherent options:

1. **Keep it stateless** — match the source. The recipe-page chat is a
   quick "tweak this recipe / swap this day" affordance, not a thread to
   come back to. The user already has the main chat at `/` for persisted
   discussion.

2. **Promote to persisted** — write rows into `kitchen_messages` with a
   new `recipe_day_id` (or analogous) discriminator. Each recipe day
   gets its own threaded sub-conversation visible in the conversation
   sidebar. This is a behavior **change**, not a port.

The behavior of the source is option 1. The workload spec's "must
preserve" list (§4) preserves the **feature set documented in
`replit.md`** and the SSE protocol shape; it does not call for promoting
this chat surface.

Promoting would also expand the contract's surface: a new endpoint
discriminator, sidebar UX, a "delete recipe-page conversation" path, and
RAG context choices. None of that is in scope for v1.

---

## Decision

**The recipe-page chat stays stateless in the iOS port.**

- No history is persisted server-side.
- No `recipe_day_id` discriminator is added to `kitchen_messages`.
- The wire endpoint `POST /api/kitchen/recipe-message` accepts the same
  inputs as the source pin (`content`, `dayId`, `mealName`, `dayName`,
  `currentRecipe?`).
- The single tool exposed is the recipe-page `update_meal` variant
  (`{ mealName, notes? }`, no `day` field) — contract §7.
- The terminal SSE event is `{ "type": "done", "updated_meal": {
  "meal_name": "...", "notes": "..." }? }` when a swap fired; otherwise
  `{ "type": "done" }`.
- The iOS client discards the in-memory chat history on navigation away
  from the recipe page.

---

## Status

**Accepted.** Approved by Dave at the contract gate, 2026-05-24.

---

## Consequences

### Positive

- The contract is smaller — no extra discriminator column, no extra
  endpoints.
- Behavior fidelity to the source pin is exact for this surface.
- The mental model is clear: the main chat at `/` is the persisted
  thread; the recipe-page chat is a per-page tweak surface.
- The Go backend's `kitchen_messages` table stays simple — one shape,
  one discriminator (conversation_id), no per-feature special cases.

### Negative

- A user who wants to "remember what I asked the AI about this recipe
  last week" cannot. We accept this; it is a v2 feature, not a port
  regression.
- The recipe-page chat's last-N-turn coherence is unmodeled: each call
  starts fresh. The source has this same limitation.

### Reversibility

Promoting later is straightforward: add `recipe_day_id INT8 NULL
REFERENCES meal_plan_days(id) ON DELETE CASCADE` to `kitchen_messages`,
write rows on each turn, fetch on entry. The CFO and tool-call contracts
are unaffected.
