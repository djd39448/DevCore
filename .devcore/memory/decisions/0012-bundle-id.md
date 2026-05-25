---
type: decision
title: iOS bundle identifier — com.djd39448.souschef
status: accepted
owner: conductor
workload: sous-chef-ios
last_updated: 2026-05-25
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# ADR-0012 — iOS bundle identifier: `com.djd39448.souschef`

This ADR pins the iOS app's bundle identifier. The iOS track plan
called it "provisional, confirmed at Apple Developer enrollment"
(track-ios §3.1, §4); the iOS Builder picked `com.djd39448.souschef`
during Week 1 (`ios/project.yml:18,57`). Reviewer-pass 0001 flagged
the unrecorded pivot and the three places the value is referenced
must agree.

Depends on workload spec, the iOS track plan §3.1 / §4, the
integration synthesis §2.4 (Apple Developer Dave-action), and the
data track plan §5 task 17 (Supabase Auth Apple-provider config).

---

## Context

A bundle identifier is one of those values that propagates: once the
iOS project, the Apple Developer Service ID, and the Supabase Auth
Apple provider all reference it, **changing it later means a
re-enrollment**. So picking it now, in writing, is cheaper than
fixing it after Dave's Apple Developer enrollment lands.

The iOS Builder chose `com.djd39448.souschef` during Week 1 because
the Apple Developer account is registered to GitHub handle
`djd39448`, and matching the existing handle keeps the Apple Connect
admin UI obvious to operate. The track plan's `com.dobbins.souschef`
was a guess made before the Apple Developer account name was checked.

Considered alternatives:

- **`com.djd39448.souschef`** (Builder's choice) — matches the Apple
  Developer account handle. No ambiguity in the Apple Connect dashboard.
- **`com.dobbins.souschef`** — matches Dave's surname; intuitive
  outside the Apple ecosystem. But the Apple Developer account name
  determines what shows up in Apple's UI, so the GitHub-handle
  variant is what the dashboard already groups under.
- **`com.souschefclaude.app`** — independent of the developer's
  identity. Would survive a future transfer of ownership cleanly.
  But adds a "what does this prefix mean?" question for the first
  reader, which dc-00 dings.

## Decision

**The iOS bundle identifier is `com.djd39448.souschef`.**

This value is referenced in:

1. `ios/project.yml` — `PRODUCT_BUNDLE_IDENTIFIER` for the
   `SousChef` app target. Already set by the iOS Builder in Week 1.
2. Apple Developer Service ID — Dave's Apple Developer enrollment
   creates a Service ID whose **bundle identifier** field must
   contain this string. (Dave-action; integration.md §2.4.)
3. Supabase Auth Apple-provider configuration — the Supabase
   Dashboard's Apple provider page takes a Services ID, which itself
   carries this bundle identifier. (data track §5 task 17.)

All three references must stay in sync. If any future change to one
is needed, this ADR is amended and the others updated in the same
PR.

## Status

**Accepted.** Locked by Dave at the Reviewer-pass-0001 follow-up,
2026-05-25.

## Consequences

### Positive

- One canonical string for the bundle id, recorded with reasoning.
- Future readers (including Dave six months from now) see why the
  GitHub-handle prefix appears in Apple-side configuration.
- The iOS Builder's Week 1 choice is ratified, not rolled back —
  no churn.

### Negative

- If the Apple Developer account is ever transferred to a different
  GitHub handle or to a new team, the bundle id outlives the
  reasoning. The bundle id can be left in place (it is just a
  string; Apple does not care what comes before the leaf), or
  re-enrolled as part of the transfer. Either way the choice is
  reversible at moderate cost — Apple specifically supports bundle-
  id transfer between teams.

### What this implies for the iOS track plan

- `track-ios.md` §3.1 and §4 are amended in the same Conductor pass
  that records this ADR (or in a tracked follow-up if the plan
  amendment slips).

### What this implies for the data track plan

- `track-data.md` §5 task 17 ("Configure Supabase Auth Apple
  provider") references this ADR for the bundle id value. No code
  change today — the task itself is blocked on the Dave-action.

### What this implies for the integration synthesis

- `integration.md` §2.4 (Supabase Auth provider config Dave-action)
  references this ADR for the bundle id Dave types into Apple
  Developer's Service ID setup.
