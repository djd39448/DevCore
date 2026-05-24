---
type: decision
title: Week-start semantics — client supplies the Monday, server trusts it
status: accepted
owner: architect
workload: sous-chef-ios
last_updated: 2026-05-24
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# ADR-0010 — Week-start semantics: client picks the Monday in its local zone

Resolves behavior-spec §9 open question #6. The web app's
`getWeekStartDate()` runs in the **server's local timezone** and snaps
to the Monday of the current week. On iOS, the user's device clock is
the canonical "current week" reference; the server has no local time
that means anything to the user.

Status is **Proposed** until Dave approves at the `contract` gate.

Depends on the behavior spec §2.2 *meal_plans*, §3.3 *meal-plan page*,
§4.3 `create_meal_plan` semantics, §6.1 *meal-plan state machine*, and
§9 open question #6. Affects contract §3 *Wire conventions* and §5
*REST API surface*.

---

## Context

At the source pin, every "current week" computation happens server-side
via `getWeekStartDate()`. The function snaps `new Date()` to Monday-of-
current-week in the **server's timezone**. The `meal_plans.week_start_date`
column is a Postgres `date` — a calendar date with no timezone.

This works on the web because both the user's browser and the Replit
server tend to live in the same regional timezone, and `date` columns
don't drift across DST. It breaks down on iOS:

- The Go backend runs on AWS (likely UTC). The user is in their local
  timezone. "What week is it?" diverges by up to ~17 hours.
- A single backend can serve users in multiple timezones eventually.
- The `meal_plans` uniqueness invariant `(user_id, week_start_date)`
  works correctly only if both client and server agree on which Monday
  is "this week" for a given moment.

Two options:

1. **Server computes the week** — Server uses UTC. Friday-evening user
   in PT would see the meal plan flip to "next week" at 5pm Friday
   (UTC midnight). Bad UX.

2. **Client supplies the Monday** — The iOS app computes Monday-of-
   current-week from the user's `Calendar.current` (the user's
   timezone) and sends the ISO date `YYYY-MM-DD` on the wire. The
   server stores that date verbatim. The server never independently
   asks "what week is it?" — it always operates on a client-supplied
   `week_start_date`.

Option 2 matches how the source pin actually behaves from the user's
perspective (they think in their local week) without inheriting the
server-local-time bug.

---

## Decision

**The iOS client computes Monday-of-current-week in the user's local
timezone and sends it as an ISO date string. The server treats the
date as opaque — it stores and queries on it, never recomputes it.**

Contract rules:

- The wire date format is ISO 8601 `YYYY-MM-DD` with no timezone
  suffix. It represents a calendar date — the Monday that anchors a
  week in the user's mental model.
- Every endpoint that operates on "a week" takes `weekStartDate` as a
  path or query parameter:
  - `GET /api/kitchen/week/{weekStartDate}`
  - `POST /api/kitchen/meal-plans` (body: `{ week_start_date }`)
  - `POST /api/kitchen/meal-plans/{weekStartDate}/regenerate-days`
  - `POST /api/kitchen/shopping-lists` (body: `{ week_start_date }`)
- The `create_meal_plan` AI tool **also** receives `week_start_date`
  in its arguments — added to the tool schema in contract §7. The
  server-side tool dispatcher passes through the user's current week
  (received with the original chat send) so the tool's behavior
  matches the user's expectations even when the tool fires.
- The chat-send wire shape (`POST /api/kitchen/messages`) gains a
  `client_week_start_date` field so server-side tool dispatch always
  knows the user's "this week".
- The server's only week-related computation is **validation**: reject
  a `week_start_date` that is not a Monday (return 400). The Go API
  computes "is this date a Monday?" via Go's `time.Weekday() ==
  time.Monday`.

The `meal_plans.week_start_date` column type is `date` in Postgres,
unchanged from the source schema. The uniqueness invariant
`(user_id, week_start_date)` is enforced by a unique index.

---

## Status

**Accepted.** Approved by Dave at the contract gate, 2026-05-24.

---

## Consequences

### Positive

- The user's "this week" is always **the user's `Calendar.current`'s
  Monday**. No DST surprises. No timezone-of-the-server surprises.
- The server has zero timezone configuration to maintain. It treats
  dates as labels, not as moments.
- The wire is timezone-agnostic. A future Android client or web client
  uses the same convention.
- The Go API's date validation is one line. No timezone library.

### Negative

- The client carries the responsibility of computing Monday-of-week.
  The iOS app uses `Calendar.current` (which already handles
  weekStart-Monday for the user's locale where applicable; for the
  US locale where Sunday is the first day, the iOS app explicitly
  selects Monday by setting `firstWeekday = 2` in a dedicated
  `Calendar` instance).
- The chat send carries a `client_week_start_date` it didn't before.
  Trivial — one extra field.

### Reversibility

Adding a server-side "default to today's week if no date is supplied"
fallback is a small change later if needed (e.g. for a server-driven
notification that has no client context). It is not part of v1.
