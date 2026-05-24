---
type: decision
title: Backend connects to Supabase as the user — JWT-aware, RLS-enforced
status: accepted
owner: conductor
workload: sous-chef-ios
last_updated: 2026-05-24
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# ADR-0011 — JWT-aware Postgres connection; RLS is the source of truth

This ADR resolves the cross-track conflict surfaced at the `track_plan`
gate: the **backend** track plan (§3 Architecture, §9 Q2) defaulted to a
**service-role** connection with app-level `WHERE user_id = $1` filtering;
the **data** track plan (§6 Integration points; §3 RLS strategy) wrote
RLS-aware policies assuming the user's JWT flows through to Postgres
and `auth.uid()` enforces per-row filtering.

The two designs are mutually exclusive. This ADR picks one and binds
both tracks to it.

Depends on the workload spec, the contract (§2 *Identity & auth* and §4
*Data model*), ADR-0003 (Supabase JWT bearer to the Go API), the data
track plan, and the backend track plan.

---

## Context

In a Supabase deployment two patterns are common:

- **JWT-aware** — the Go service, after verifying the bearer token, sets
  `request.jwt.claim.sub` (and friends) on the database session before
  issuing queries. Row-Level Security policies use `auth.uid()` to filter
  per user; the database is the enforcement point.
- **Service-role + app filtering** — the Go service connects as a
  superuser-equivalent and includes `WHERE user_id = $1` in every query.
  RLS policies become documentation of intent rather than enforcement.

| Dimension | JWT-aware | Service-role |
|---|---|---|
| Per-query overhead | One `SET LOCAL` per request | None |
| Where data leakage is prevented | Postgres (the database) | The Go code (the app) |
| Cost of a Go bug | Bounded by RLS | Whole-database leak surface |
| Cost of an RLS-policy bug | Whole-database leak surface | Bounded by Go's `WHERE` |
| Pool management | Sessions must reset GUCs cleanly | Standard pgxpool |
| Idiom match with Supabase docs | Native | Surprising |

The data track has already authored RLS policies under the JWT-aware
model. The backend track plan defaulted the other way without consulting
the data track — the divergence is precisely the kind of cross-track
seam the `track_plan` gate exists to surface.

## Decision

**The Go backend connects to Supabase using a JWT-aware pattern.**

Concretely:

1. The Go service maintains a `pgxpool` against the Supabase Postgres
   instance, connecting as the **`authenticated`** Postgres role (not
   the service-role).
2. On every request that has already passed the JWT-verification
   middleware, the handler executes
   `SET LOCAL request.jwt.claim.sub = '<user_id>'`
   (and any other claims the RLS policies reference) at the start of
   the request's database transaction. The `LOCAL` scope ensures the
   GUCs are dropped at transaction end — the connection returns to the
   pool clean.
3. All RLS policies use `(select auth.uid())` — Supabase's standard
   form — to read the claim back. The data track plan §3 already
   specifies this.
4. The service-role key is **not** loaded into the Go process. It is
   reserved for one-off operational scripts (data backfills,
   maintenance) and never reaches a request path.

The single exception is the Supabase JWKS fetch at startup, which uses
no database connection at all — it's an HTTPS call to Supabase's
public JWKS endpoint.

## Status

**Accepted.** Locked by Dave at the `track_plan` gate, 2026-05-24.

## Consequences

### Positive

- **Defense in depth.** A bug in the Go service cannot read or write
  rows belonging to a different user, because Postgres refuses the
  query. The RLS policies the data track wrote are load-bearing, not
  documentation.
- **Single source of truth for authorisation.** The policies are in
  one place (`supabase/schemas/*.sql`) and reviewed once; the Go code
  is not duplicating the authorisation logic per endpoint.
- **Matches Supabase's documented idiom.** New contributors land on
  the supported, expected pattern; no "surprising" architecture.
- **The audit story is simpler.** "What can user X see?" is answered by
  reading the RLS policies, not by reading every Go handler.

### Negative

- **One round trip of overhead per request.** The `SET LOCAL` adds a
  ~1ms RTT to localhost (less inside AWS). At the sous-chef-ios
  workload's expected QPS (single-user, low double-digit RPM), this is
  invisible. Mitigation if it ever matters: pipeline the `SET LOCAL`
  with the first query in a single round trip.
- **Pool hygiene must be tested.** A bug where `SET LOCAL` is missed
  on one code path would mean an unauthenticated query against the
  `authenticated` role, which fails closed (RLS returns zero rows).
  The failure mode is loud (empty results), not silent. A test fixture
  that runs against a real Supabase local stack catches this.

### What this implies for the backend track plan

The backend track plan's §3 *Architecture* is amended (a NOTE block at
the top of `plan/track-backend.md` cross-references this ADR). The
specific changes from the plan-as-written:

- The `internal/store` (or equivalent) data-access package uses the
  `authenticated` role connection string, not the service-role.
- A new helper sets the request's claims as `SET LOCAL` GUCs at the
  start of every transactional handler.
- The plan's task list grows by one task ("Implement
  `internal/store.WithClaims` helper; test it against a Supabase local
  stack with an RLS-protected table") — to be incorporated when the
  Builder begins Phase 4.

### What this implies for the data track plan

No changes. The data plan already specifies RLS-aware policies using
`(select auth.uid())`. This ADR confirms that design as binding.

### What this implies for the iOS track plan

No changes. The iOS app sends its Supabase JWT as a bearer token to
the Go API; how the Go API then talks to Postgres is invisible to iOS.

### Reversibility

Reversing to service-role + app filtering is contained:
- Drop the `SET LOCAL` calls in the Go handlers.
- Switch the connection string to the service-role key.
- Keep the RLS policies in place (they become belt-and-suspenders).

No data migration; no contract change; no iOS change. The reversal is
a Phase-4-internal refactor if the pattern turns out to be problematic.

We are unlikely to reverse: defense-in-depth almost never gets undone
once it's load-bearing. But the option exists.
