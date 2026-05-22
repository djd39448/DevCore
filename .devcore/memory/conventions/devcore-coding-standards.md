# DevCore — Coding Standard (Stack Layer)

**Version:** 1.0
**Status:** Active
**Date:** 2026-05-21
**Applies to:** DevCore itself, and every project DevCore builds.
**Extends:** the TrustCore base coding standard (`cs-00`–`cs-10`). This document
does not replace it.

These standards are non-negotiable. Every module, every commit, every test, every
migration must follow them — whether written by a human or by a DevCore agent.

---

## dc-00 — Intent: The codebase stands on its own

In the project owner's words:

> "The goal is to be able to give this to another dev and say no words. The dev
> will know exactly what they are looking at without guessing. The code base
> needs to stand by itself. This applies to DevCore and any project it builds.
> It should be the default mode."

This is the governing principle. A developer handed any DevCore-built codebase —
with zero verbal explanation and zero prior context — must be able to answer,
**from the code alone**, what every part is, what it does, why it exists, how it
connects, and how to change it safely.

No tribal knowledge. No onboarding call. No "ask the person who built it." The
code, its annotations, its tests, its migrations, and its commit history ARE the
documentation. If a reader has to guess, the code failed this standard.

This is not a final polish step. It is the **default mode of writing** — every
file, every commit, from the first line.

---

## dc-01 — Scope & Precedence

This document is the **stack-specific layer**. The TrustCore base standard
(`cs-00`–`cs-10`) — no dark code, self-documenting modules, no silent failures,
test-everything, small reviewable commits, CLI-for-everything — applies in full
and is assumed here. This document adds what those general principles do not
cover: the concrete idioms of the DevCore stack.

**The stack:**

| Layer | Technology |
|---|---|
| Engine & backend services | Go |
| iOS application | Swift / SwiftUI |
| Backend data & auth | Supabase / PostgreSQL |
| API hosting | AWS |

**Precedence**, highest wins: TrustCore base (`cs-XX`) → this document (`dc-XX`)
→ per-project addenda. A lower layer never contradicts a higher one; if it does,
the higher layer wins and the lower one is corrected.

### Module self-documentation carries to every language

`cs-03` (every file states what it does, what it depends on, what depends on it,
and why it exists) is mandatory in Go, Swift, and SQL alike. The syntax changes;
the requirement does not.

```go
// Package taskstore persists agent task rows and emits status transitions.
//
// Depends on:     internal/db (shared Postgres pool); internal/events (bus).
// Depended on by: internal/api — the only path agents use to write task state.
// Why it exists:  all task durability routes through here so the audit trail is
//                 guaranteed; direct DB writes from agent code are disallowed.
package taskstore
```

```swift
//  CheckoutFlow.swift
//
//  Holds the multi-step checkout state and the rules that advance it.
//
//  Depends on:     APIClient (order submission); CartModel (line items).
//  Depended on by: CheckoutView and its step subviews.
//  Why it exists:  checkout spans several screens; this is the single owner of
//                  that shared state, so no screen mutates it out of sequence.
```

```sql
-- Migration: create food_items — the canonical food object.
--
-- Depends on:     auth.users (user_id foreign key).
-- Depended on by: meal_plans, shopping_lists (reference food_items.id).
-- Why it exists:  one table backs the inventory, shopping, planned, and recipe
--                 roles, so food data is never duplicated across feature tables.
```

---

## dc-02 — Go

Baseline: **Go 1.26**. Pin it in `go.mod` (`go 1.26.0`).

### Project layout
- `cmd/<binary>/main.go` holds each executable. `main` does only wiring — flags,
  config, signal handling — then delegates immediately. *Entrypoints stay thin
  and the real logic stays testable.*
- `internal/` holds all non-public code and is the default home for everything.
  The compiler import-walls it. *Nothing couples to unstable code by accident.*
- `pkg/` only for code genuinely meant for external import. Most services need
  none — an absent `pkg/` is correct. It is not a dumping ground.
- One module per repo, `go.mod` at the root.
- Package name = directory name: lowercase, no underscores, no plurals. Package
  granularity follows behavior, not layers. **No `utils`, `helpers`, `common`,
  or `models` packages** — they reveal nothing at the call site.

### Error handling
- Errors are values: handle them or return them, **never ignore them**.
  `errcheck` enforces this; an explicit `_ =` requires an inline comment saying
  why discarding is correct.
- Wrap with `fmt.Errorf("doing X: %w", err)` to preserve the chain. Inspect with
  `errors.Is` (sentinels) and `errors.As` (typed errors). *Callers branch on
  cause, never on string matching.*
- Sentinel errors (`var ErrNotFound = errors.New(...)`) for simple conditions;
  typed errors when callers need structured fields. Aggregate with `errors.Join`.
- Error strings are lowercase, no trailing punctuation, no `"failed to"` prefix —
  they get nested. Each layer wraps with what it was doing.
- `panic` only for unrecoverable programmer-error states (a violated invariant,
  impossible config) — never for control flow or expected failures. `recover`
  only at a process boundary (e.g. one recover in HTTP middleware so a single bad
  request cannot kill the server).

### context.Context
- Any function doing I/O, blocking, or cross-goroutine work takes `ctx
  context.Context` as its first parameter. Propagate the received `ctx` —
  never create a fresh `context.Background()` mid-chain.
- Honor cancellation: pass `ctx` into every DB and HTTP call; `select` on
  `ctx.Done()` inside loops.
- A context carries only request-scoped transit data (request ID, trace span,
  auth principal) via typed unexported keys. **Never** put config, loggers, or
  optional parameters in a context — that hides the real API.

### Concurrency
- Every goroutine has a defined exit condition, and the code starting it knows
  how and when it stops. *An unbounded or blocked goroutine is a memory leak.*
- Use `golang.org/x/sync/errgroup` for grouped goroutines that can fail — it
  propagates the first error and cancels the shared context. Cap fan-out with
  `g.SetLimit(n)`.
- Channels transfer ownership and signal; mutexes protect in-place shared state.
  Do not mix paradigms for one piece of state.
- Only immutable values, or values whose ownership is transferred, cross a
  goroutine or channel boundary. Never share a mutable struct without a mutex.
- CI runs `go test -race ./...`. *Data races are undefined behavior; the detector
  is the only reliable catch.*

### Interfaces
- Accept interfaces, return concrete structs.
- Define an interface in the **consumer** package, sized to what that consumer
  uses — often one or two methods. No shared `interfaces` package.
- No interface until there are two implementations or a test needs a seam. No
  "just in case" interfaces.

### REST / JSON API design
- A handler does four things: decode → validate → call a service method →
  encode. **No business logic in handlers.** Dependencies live on a `Server`
  struct; handler methods return `http.Handler`.
- Validate all input at the boundary. Reject unknown fields
  (`decoder.DisallowUnknownFields()`). Client JSON is untrusted past the handler.
- One consistent error envelope (`{"error":{"code","message"}}`). Internal error
  detail is logged, never returned to the client.
- `http.ServeMux` method+path patterns (`"POST /v1/items/{id}"`) are sufficient
  routing; add a third-party router only with a stated reason.

### Testing
- Table-driven tests, subtests via `t.Run`. Call `t.Parallel()` unless shared
  state forbids it.
- Tests live in `_test.go` beside the code, in `package <name>_test` (black-box)
  by default — test the public API.
- Golden files in `testdata/` with an `-update` flag for large structured output.
- Default to the stdlib `testing` package. `testify/assert` and `testify/require`
  are permitted to cut assertion noise; **`testify/mock` and heavy frameworks are
  not** — hand-write fakes against consumer interfaces.

### Logging
- `log/slog` exclusively. No `fmt.Println`, no third-party loggers. JSON handler
  in production, text in development.
- Structured key/value attributes only — never format data into the message
  string. Pass a `*slog.Logger` explicitly as a constructor argument; no package
  global. Use the `…Context` methods so request and trace IDs attach.

### Tooling
- `gofumpt` (stricter `gofmt`) is mandatory; CI fails on any diff. `go vet ./...`
  always runs.
- `golangci-lint`, version-pinned, with a committed `.golangci.yml`. Enable at
  minimum: `errcheck`, `govet`, `staticcheck`, `ineffassign`, `unused`,
  `revive`, `gocritic`, `bodyclose`, `errorlint`, `contextcheck`, `noctx`,
  `gosec`, `nilerr`.
- Zero lint errors before any commit (`cs-04`). Suppressions require an inline
  reason.

### Dependencies
- Minimal. Every direct dependency needs a justification; prefer the stdlib and
  `golang.org/x/*`. *Each dependency is supply-chain surface.*
- Run `go mod tidy` on every change; commit `go.mod` and `go.sum`. No `replace`
  directives in mainline.
- Do not vendor by default — the module proxy plus `go.sum` give reproducibility.
- CI runs `govulncheck ./...`.

---

## dc-03 — Swift / SwiftUI

Baseline: **Swift 6.2**, Xcode 16+, iOS 17+ deployment target (required for the
Observation framework).

### Architecture
- Use plain **Model–View (MV)**. Do **not** add a `ViewModel` class for every
  screen — with the `@Observable` macro, views observe models directly at
  property granularity, so a pass-through ViewModel adds a layer with no benefit.
- Business logic lives in plain model types and service objects, **never in a
  `View` struct**. A `View` is a declarative description of UI for given state:
  layout, bindings, event forwarding — nothing else.
- Introduce an `@Observable` state object only when state is genuinely shared
  across views or has non-trivial lifecycle. Name it for its domain
  (`SearchModel`, `CheckoutFlow`), never `…ViewModel`.

### State management
- `@State` for view-local state the view owns; `@Binding` for state owned
  elsewhere and mutated here; `@Environment` for app-wide dependencies and shared
  models. **One owner per piece of state.**
- Model classes use the `@Observable` macro. **Do not use `ObservableObject`,
  `@Published`, `@StateObject`, or `@EnvironmentObject` in new code** — they
  invalidate every observer on any change; `@Observable` re-renders only views
  that read the changed property.
- A view that creates an `@Observable` object holds it with `@State`; a view that
  receives one takes a plain `let`. Inject shared instances with
  `.environment(_:)` and read with `@Environment(MyModel.self)`.
- A model or service type **never** `import SwiftUI`. *Logic stays
  UI-independent and testable.*

### Concurrency
- Enable Swift 6 complete data-race checking. The build is warning-free;
  concurrency warnings are treated as errors.
- Adopt Swift 6.2 "approachable concurrency": default `@MainActor` isolation for
  the app target. Most app code is UI code and belongs on the main actor.
- Off-main work is a deliberate, visible decision — opt in with `@concurrent`,
  an explicit `actor`, or `Task.detached`, and only on a measured need.
- Use `async`/`await` for all asynchronous work. No `DispatchQueue`, completion
  handlers, or Combine in new code. Protect shared mutable state with an `actor`.
- Every type crossing a concurrency boundary is `Sendable`. `@unchecked
  Sendable` and `nonisolated(unsafe)` are banned without an inline comment
  proving the manual guarantee.
- Every `Task` is tied to a lifecycle: prefer `.task { }` on views (auto-cancels
  on disappear); store and cancel manually created tasks; honor cancellation.

### Error handling
- Use `throws` with `do`/`catch`. For module- and layer-boundary APIs with a
  known finite error set, use Swift 6 typed throws (`throws(MyError)`) — callers
  get exhaustive `catch` and a documented contract.
- `Result` only for stored or deferred outcomes, not as a substitute for
  `throws` in call-and-handle code.
- Convert errors explicitly at layer boundaries — a low-level networking error
  never leaks into UI code.
- Internal precondition violations fail loudly (`precondition`, `fatalError`).
  User-facing errors surface in the UI via a type conforming to `LocalizedError`.
  Never swallow an error to show an empty screen.

### Views, networking, dependencies
- A `body` fits on screen without scrolling. Extract subviews aggressively — one
  reusable view per file. Repeated styling becomes a `ViewModifier`.
- Pass a subview the minimum data it needs, not a whole model.
- All HTTP access goes through a single protocol-defined API client backed by
  `URLSession`. No `URLSession` calls scattered in views or models. Use the
  `async` methods (`data(for:)`); validate the `HTTPURLResponse` status and throw
  a typed error on non-2xx. Keep wire DTOs separate from domain models.
- Prefer initializer injection. Every injected dependency is a protocol so tests
  and previews can substitute fakes. No global singletons, no ad-hoc DI
  containers — hidden dependencies break dc-00.

### Project layout, testing, accessibility, naming
- Group by feature (`Features/Checkout/…`), not by type. Shared code lives in a
  `Core` module. One primary type per file; filename equals type name.
- Swift Package Manager is the only dependency manager. Local packages for
  reusable code.
- New tests use the **Swift Testing** framework (`@Test`, `#expect`, `#require`,
  `@Suite`) with parameterized cases. XCTest is retained only for existing
  suites, UI automation, and performance tests.
- Unit-test models, services, and the API client with injected fakes. Every view
  ships `#Preview`s covering its key states — loading, empty, error, populated.
- Accessibility is a default, not a follow-up: meaningful labels on every
  interactive element, decorative images hidden, Dynamic Type supported (no fixed
  font sizes), Reduce Motion respected, contrast minimums met.
- Follow the Swift API Design Guidelines — clarity at the point of use over
  brevity. `UpperCamelCase` types, `lowerCamelCase` everything else, booleans
  read as assertions (`isEmpty`).
- Pin the Swift toolchain version. SwiftFormat and SwiftLint configs are
  committed and enforced as a pre-commit hook and a CI gate. Warnings-as-errors
  on. No checked-in derived data or `xcuserdata`.

---

## dc-04 — Supabase / PostgreSQL

Baseline: Supabase on PostgreSQL 17+.

### Schema design
- `snake_case` everywhere. Tables are plural nouns (`order_items`), columns are
  singular. *Unquoted identifiers fold to lowercase — mixed case forces quoting
  forever.*
- Primary keys default to `uuid` generated with `uuidv7()`. UUIDv7 embeds a
  timestamp so inserts stay index-ordered (v4 fragments the B-tree); UUIDs also
  stop a mobile client from enumerating resources. Store as native `uuid`, never
  `text`.
- Every table has `created_at timestamptz NOT NULL DEFAULT now()` and
  `updated_at timestamptz NOT NULL DEFAULT now()`. Always `timestamptz`, never
  `timestamp`. Maintain `updated_at` with a trigger, not application code.
- Use the narrowest correct type: `text` (never `varchar(n)`), `numeric` for
  money, `boolean` for flags, native `enum` or a lookup table for closed sets.
- Normalize to 3NF by default — a normalized schema is its own data dictionary.
- JSONB **only** for genuinely schemaless data (third-party payloads,
  user-defined fields). It is an anti-pattern for anything you filter, join, or
  aggregate on — Postgres keeps no statistics inside JSONB, and it hides
  structure from the next developer.
- Soft-delete with a nullable `deleted_at timestamptz`, not an `is_deleted`
  boolean. Hard-delete only where regulation requires it.

### Constraints — enforce in the database
- `NOT NULL` is the default; every nullable column is justified.
- Declare every foreign key explicitly, with an `ON DELETE` action chosen
  deliberately (`CASCADE`, `RESTRICT`, `SET NULL`).
- Use `CHECK` constraints for domain rules (`price >= 0`, non-empty strings).
- Prefer a lookup table over a native `enum` when values change over time —
  altering an enum needs a migration and cannot remove values.

### Row-Level Security
- RLS is **default-on**. Enable it on every table in an exposed schema the moment
  the table is created (`ALTER TABLE … ENABLE ROW LEVEL SECURITY`) — RLS is not
  automatic for SQL-created tables, and a forgotten table is a public data leak.
  Add `FORCE ROW LEVEL SECURITY`.
- Write one policy per operation (`SELECT`/`INSERT`/`UPDATE`/`DELETE`), not a
  single `FOR ALL`.
- Always scope the role explicitly: `TO authenticated` or `TO anon`.
- Wrap `auth.uid()` / `auth.jwt()` in a subquery — `(select auth.uid())` — so it
  runs once per statement, not once per row.
- Index every column a policy references (`user_id`, `tenant_id`). Structure
  policies as `column IN (select … where user_id = (select auth.uid()))` so they
  use those indexes.
- Push multi-table logic into a `STABLE SECURITY DEFINER` function in a
  non-exposed schema; `GRANT EXECUTE` only to intended roles.
- Never trust a client-supplied user ID — derive identity solely from
  `auth.uid()`.

### Auth
- Use Supabase Auth; never roll your own. For Sign in with Apple use the native
  iOS flow (`signInWithIdToken`).
- Use asymmetric JWT signing keys (`sb_publishable_…` / `sb_secret_…`) so keys
  rotate without downtime.
- The publishable/anon key is the **only** key shipped to the mobile client. The
  secret/service-role key bypasses RLS entirely — it never reaches a client, a
  repo, or an app bundle.

### Migrations
- Use the Supabase CLI. Adopt declarative schemas: desired state lives in
  `supabase/schemas/*.sql`; `supabase db diff` generates versioned migrations in
  `supabase/migrations/`.
- **Never edit an applied migration** — add a new one. Editing breaks every
  environment that already ran it.
- Migrations are small, single-purpose, and reviewed like any other code. Avoid
  statements that take long table locks in production.

### Performance, Edge Functions, storage, secrets, local dev
- Index every foreign key (Postgres does not do this automatically) and every
  RLS policy column. Run `EXPLAIN ANALYZE` on policy-bearing queries before
  merge; a seq scan on a large table is a defect. Always paginate.
- Default to client-side queries through PostgREST (RLS-protected) for normal
  CRUD. Use database functions for transactional data logic. Use Edge Functions
  for third-party integrations, webhooks, and any secret-bearing logic — not as
  a proxy for plain CRUD.
- Storage buckets are private by default. Gate access with RLS policies on
  `storage.objects`, scoping by a path prefix named for `auth.uid()`. Constrain
  file size and MIME type at the bucket.
- Safe on the client: the project URL and the publishable key — RLS is the real
  security boundary. Server-side only: the service-role key, the Apple `.p8`,
  and all third-party keys.
- Develop against the local stack (`supabase start`), never against production.
  Commit `config.toml` so every developer's stack is identical.

---

## dc-05 — AWS Deployment (Go API)

- **Compute choice by traffic shape.** ECS/Fargate is the default for a
  steady-traffic REST API. Lambda (`provided.al2023` runtime, `lambda.Start`) for
  event-driven or spiky low-volume traffic. **App Runner is in maintenance mode —
  do not choose it for a new service.**
- **Build.** Multi-stage Docker from `scratch` or a distroless base,
  `CGO_ENABLED=0`, static binary, Linux target. Tag images immutably by git SHA.
  Scan every image (ECR scan + `govulncheck`).
- **Config.** All configuration via environment variables (twelve-factor). No
  config files baked into images.
- **Secrets.** AWS Secrets Manager or SSM Parameter Store, fetched at startup.
  Never in env vars committed to IaC, never in the image, never in the repo.
- **Observability.** `slog` JSON to stdout → CloudWatch Logs. Metrics and traces
  via OpenTelemetry. Expose a `/healthz` endpoint for the load balancer.
- **Infrastructure is code.** The cluster, service, and networking are defined as
  reviewed code (Terraform or CDK) — no console click-ops for anything that must
  be reproducible. IAM roles follow least privilege.

---

## dc-06 — macOS / Xcode Environment

DevCore is developed on macOS (Apple Silicon — M4 Mac Mini now, Mac Studio
later). These gotchas come from that environment. Each states the **symptom**,
the **cause**, and the **fix** — the format of `cs-09`, inverted from TrustCore's
Windows/WSL world.

### Case-insensitive filesystem
- **Symptom:** a file renamed only by case is not picked up; two files differing
  only in case collide; Go imports or Git behave strangely.
- **Cause:** APFS on macOS is case-insensitive by default.
- **Fix:** never rely on case to distinguish files or identifiers. CI runs on a
  case-sensitive Linux filesystem and *will* fail on a mismatch — a case bug that
  "works on my Mac" breaks the build.

### Cross-compiling Go for AWS
- **Symptom:** a binary built on the Mac will not run on AWS.
- **Cause:** the Mac is arm64/Darwin; the AWS target is Linux.
- **Fix:** build deploy artifacts with `GOOS=linux GOARCH=arm64 CGO_ENABLED=0`
  (Graviton) in CI, never by hand. Local `go build` output is for local runs only.

### Xcode code signing and device runs
- **Symptom:** the app runs in the simulator but will not install on a physical
  iPhone.
- **Cause:** device installs require a provisioning profile and an Apple
  Developer account; the simulator does not.
- **Fix:** document the signing setup (team ID, bundle ID, profile) in the iOS
  project's README — signing is not tribal knowledge. Never commit certificates
  or `.p8` keys.

### Tool version drift
- **Symptom:** a build passes for one developer and fails for another.
- **Cause:** Homebrew rolls tool versions forward independently per machine.
- **Fix:** pin every tool version (Go, Swift, the Supabase CLI, `golangci-lint`,
  SwiftLint) in a committed manifest. The Mac Studio migration must reproduce the
  exact toolchain.

### macOS filesystem cruft
- **Symptom:** `.DS_Store` or `xcuserdata` files appear in commits.
- **Cause:** Finder and Xcode write them into directories automatically.
- **Fix:** `.DS_Store`, `*.xcuserdata`, and Xcode derived data are in
  `.gitignore` from the first commit.

### Local secrets
- **Symptom:** an API key (Pinecone, Supabase service role, Apple `.p8`) lands in
  the repo.
- **Cause:** secrets kept in a tracked file for convenience.
- **Fix:** local secrets live in a gitignored `.env` or the macOS Keychain —
  never a tracked file. The repo carries a `.env.example` with the required keys
  and empty values, documenting what is needed without leaking anything.

---

## dc-07 — Pre-commit Checklist (stack additions to cs-10)

These extend the TrustCore `cs-10` checklist; that checklist still applies in
full.

```
[ ] Go: gofumpt clean, go vet clean, golangci-lint clean on touched packages
[ ] Go: go test -race passes; new or changed logic has table-driven tests
[ ] Go: errors wrapped with %w; no dropped errors; ctx propagated, never re-rooted
[ ] Swift: builds warning-free under Swift 6 complete concurrency checking
[ ] Swift: SwiftFormat + SwiftLint clean; new code uses @Observable, not ObservableObject
[ ] Swift: new tests use Swift Testing; every view ships #Previews for key states
[ ] Supabase: RLS enabled on every new table in an exposed schema, one policy per operation
[ ] Supabase: new migration is additive (no edits to applied migrations), small, single-purpose
[ ] Supabase: every foreign key and every RLS policy column is indexed
[ ] Secrets: no key, token, or service-role credential anywhere in the diff
[ ] AWS: config via env vars; deploy artifact built for the Linux target in CI
[ ] Every new file carries its dc-01 header: what / depends on / depended on by / why
```

---

## Sync points

- **Canonical copy:** this file, `CODING_STANDARDS.md`, at the DevCore repo root.
- **Agent copy:** `.devcore/memory/conventions/devcore-coding-standards.md` is a
  byte-identical copy, read by the Reviewer agent. When this file changes, that
  copy is updated in the same commit. The two must never diverge.
- **Relationship to TrustCore:** the base standard (`cs-00`–`cs-10`) remains the
  read-only reference in Pinecone (`trustcore-systems/coding-standards`). This
  document extends it; it does not modify or replace it.
- **Per-project addenda:** a project DevCore builds may add project-specific
  rules on top. Addenda extend — they never contradict this document or the base.
