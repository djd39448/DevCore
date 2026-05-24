---
type: decision
title: Meal-plan week navigation — SwiftUI NavigationStack, no deep links v1
status: accepted
owner: conductor
workload: sous-chef-ios
last_updated: 2026-05-24
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# ADR-0005 — Meal-plan week navigation: SwiftUI NavigationStack only (v1)

This ADR resolves workload-spec §6.5. The Conductor captured this decision
at the `behavior_spec` human gate; the Architect does not re-litigate it.

Depends on the workload spec, the behavior spec §3.3 *meal-plan page*,
§3.7 *calendar*, §3.8 *shopping*, and §5 *REST API surface* (the
`?week=YYYY-MM-DD` and `?list=...` query-param conventions).

---

## Context

The web app addresses weeks via URL query parameters: `/plan?week=...`,
`/shopping?list=YYYY-MM-DD`, `/calendar`. Wouter's lightweight router
makes those parameters part of the route's identity, which is how the web
client persists "which week am I looking at" across reloads.

On iOS, the platform offers a richer model: `NavigationStack` with typed
`NavigationPath`, value-driven `NavigationLink`, and (optionally) Universal
Links for deep-linking from outside the app.

Options weighed at the gate:

- **SwiftUI NavigationStack only, no deep links v1** — Idiomatic iOS;
  state lives in the navigation path; no Universal Links infrastructure.
- **SwiftUI state + Universal Links from day 1** — Set up
  `apple-app-site-association`, domain verification, deep-link parsing
  now. Enables shareable URLs to specific weeks/recipes.
- **URL-based routing inside the app** — Mirror the web model. Fights the
  platform; no platform-native back-stack behavior.

## Decision

**Use SwiftUI `NavigationStack` with a typed `NavigationPath` for v1.** No
Universal Links. No URL-style internal routing.

The Plan, Calendar, Shopping, and Recipe surfaces each define their own
navigation values:

- Plan tab: `PlanRoute.week(Date)` (the Monday of the week being viewed).
- Calendar tab: `CalendarRoute.month(Date)`, drills into
  `PlanRoute.week(Date)` on tap.
- Shopping tab: `ShoppingRoute.list(id: Int64)` *or* `ShoppingRoute.week(Date)`.
- Recipe screen: `RecipeRoute.day(mealPlanDayId: Int64)`.

State restoration uses SwiftUI's standard `SceneStorage` /
`NavigationPath` serialization for resuming the user's last position
after a relaunch.

## Status

**Accepted.** Locked by Dave at the behavior_spec gate, 2026-05-24.

## Consequences

### Positive

- The iOS app follows platform conventions — swipe-back works, the back
  stack is correct, animations are right.
- No `apple-app-site-association` to host, no domain to verify, no
  deep-link router to maintain in v1.
- State restoration on relaunch is one `@SceneStorage` line per tab.

### Negative

- No way to **share a week** as a URL ("hey, look at my meal plan for
  next week"). Acceptable for v1: the app is personal-use; sharing is
  not a required feature.
- No way to deep-link from an iOS widget or notification into a specific
  week/recipe. Acceptable for v1: no widgets or notifications are in
  scope.

### Reversibility

Adding Universal Links later is a contained v2 feature:
- Host `apple-app-site-association` at the Supabase project domain or a
  CloudFront distribution.
- Add an `App.urlScheme` handler.
- Map URL paths to existing `NavigationPath` values.

The contract is **not** affected — the wire protocol uses IDs and ISO
date strings either way. Only the iOS client's entry-point logic
changes.

### What this implies for the contract

- The contract's date conventions stay as the behavior spec describes
  them — ISO date strings, week-start = Monday, day-of-week JS-Sunday-
  first integers. URL-vs.-state choices do not change the wire.
- The contract does **not** specify any iOS routing structure. The iOS
  track plan owns this.
