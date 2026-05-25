---
type: review
title: Reviewer pass — sous-chef-ios Phase 4 Week 1
status: complete
owner: reviewer
workload: sous-chef-ios
last_updated: 2026-05-24
scope: Sous-Chef-Claude2@c622e78 (Week 1 foundations across all three tracks)
---

# Reviewer pass — sous-chef-ios Phase 4 Week 1

## §1 What was reviewed

- **Backend track** (`~/Sous-Chef-Claude2/backend/`, commits ea0207e and
  decca9f, merged at c622e78). 8 source + 4 test files plus
  `Makefile`, `Dockerfile`, `.golangci.yml`, `.dockerignore`, `go.mod`,
  `README.md`. Reviewed against `CODING_STANDARDS.md` §dc-00/dc-01/dc-02/dc-05/dc-07,
  contract `§2`/`§3`/`§3.5`/`§3.6`, ADR-0011, and `track-backend.md` §5
  Phase A (A1–A7) and §6.1 (cross-track data-access stance).

- **Data track** (`~/Sous-Chef-Claude2/data/`, commits 4bd97df, 09102e5,
  4791458, 14a84d2, 8a4b244, merged at 7736004). 6 schema files
  (`00-extensions.sql`…`05-triggers.sql`), `config.toml`, `seed.sql`,
  one pgTAP test, Makefile, README. Reviewed against `dc-00`/`dc-01`/`dc-04`,
  contract `§2`/`§4`/`§9`, ADR-0003, ADR-0004, ADR-0006, ADR-0007,
  ADR-0009, ADR-0010, ADR-0011, and `track-data.md` §5 tasks 1–14, 18, 19.

- **iOS track** (`~/Sous-Chef-Claude2/ios/`, commits 505a7c9, cc9d429,
  de37108, merged at 8dbec0c). `project.yml`, generated `.xcodeproj`,
  `Info.plist`, `SousChefApp.swift`, `RootView.swift`, `LoginView.swift`,
  five tab placeholder views + `PlaceholderTabContent`, `SousChefKit`
  Package with five library targets and five test targets (~29 Swift
  files total), `Makefile`, `.swiftformat`, `.swiftlint.yml`, README.
  Reviewed against `dc-00`/`dc-01`/`dc-03`/`dc-06`, contract `§2`,
  ADR-0003, ADR-0005, and `track-ios.md` §3.1, §3.2, §5 Phase A (A1
  partial, A5 mock), and §6.3 (clientWeekStartDate prep).

---

## §2 Verdict per track

- **Backend: APPROVED WITH FINDINGS.** Phase A1–A7 acceptance for the
  foundation phase is met: builds clean, lints clean (`golangci-lint`
  v2 config with the dc-02 required linters), tests with `t.Parallel`
  and table-driven `Run` patterns, every file carries a dc-01 header,
  error envelope matches contract §3.5, /healthz returns the contract
  §3.6 shape, slog JSON output, request-ID propagation, panic recovery
  that does not leak. None of the findings below are blocking for Week
  2; they should land before Phase B closes.

- **Data: APPROVED WITH FINDINGS.** Schema is faithful to contract §4
  byte-for-byte (every CHECK, every named index, every JSONB sub-document
  comment). RLS is enabled AND forced on every table; one policy per
  operation; ownership-via-parent helpers are `SECURITY DEFINER` with
  explicit `search_path`; storage.objects RLS scopes by path prefix.
  Cache-clear and image-cascade triggers honor the contract's atomicity
  guarantees. One **Must-fix-Week-2** finding (the `seed.sql` production
  guard is a no-op) and one Should-fix (no pgTAP test yet for
  `kitchen_messages` append-only). The "initial migration not cut yet"
  is acknowledged in the README and is gated on tooling Dave hasn't
  installed; that defers, not blocks.

- **iOS: APPROVED WITH FINDINGS.** Foundation phase A1 acceptance is
  met: SousChef Xcode target + SousChefKit package builds clean, five
  library targets with five passing test targets (Swift Testing), strict
  concurrency on, warnings-as-errors, SwiftFormat/SwiftLint configs
  committed. The Package boundary enforces dc-03's "no `import SwiftUI`
  in service code" at compile time. The LoginView is a true fail-closed
  mock (the `onSignInSucceeded` callback is never invoked). One
  **Must-fix-Week-2** finding (bundle id diverged from `track-ios.md`
  §3.1 without an ADR or change_log entry) and one Should-fix (the
  app target's Swift mode is set to `SWIFT_VERSION: "6.0"` while
  `dc-03` baselines Swift 6.2 — Swift 6.0 is the strict-mode language
  baseline, not the toolchain).

---

## §3 Findings — per track, grouped by severity

### Backend

**Blocker** — none.

**Must-fix-Week-2** — none.

**Should-fix:**

- `backend/internal/server/server.go:88-92` — **dc-02 (consumer-side
  interfaces).** The middleware chain composes in a hardcoded order:
  `WithPanicRecovery(WithRequestID(WithAccessLog(mux)))`. There is no
  test that asserts the order (e.g. that a panic inside the access
  log is still recovered, or that the request id is present in the
  access log line). dc-07 "new logic has table-driven tests" applies.
  **Fix:** add a table-driven test that exercises the chain end-to-end
  for each layer's invariants (request-id-in-access-log; panic
  recovered with id attached). Won't gate Week 2 but lands before more
  middleware is added.

- `backend/go.mod` carries no dependencies and **no `go.sum` is
  committed.** dc-02 says "commit go.mod and go.sum". The current
  module imports only stdlib so there's nothing to lock, but as soon
  as Phase B adds `golang-jwt/jwt/v5` and `MicahParks/keyfunc/v3` the
  `go.sum` must appear in the same commit. **Fix:** treat this as a
  Phase B reminder; not a defect today.

- `backend/internal/server/middleware/middleware.go:135` — the
  `//nolint:contextcheck` directive is attached to a `defer func()`
  whose body does not propagate `ctx` to a downstream call (just
  reads it for logging). The directive is harmless but the cited
  reason ("closure intentionally captures r.Context() from the
  request scope") doesn't match the lint rule's intent. dc-02
  "Suppressions require an inline reason." **Fix:** drop the nolint
  if the lint doesn't trigger; otherwise restate the reason. The
  one in `server.go:160` is correctly justified.

**Nit:**

- `backend/internal/apierror/apierror.go:84` — the package's dc-01
  header states "log/slog (standard library only)" and the package
  docstring says "Write deliberately does not log", but the fallback
  branch calls `slog.Default().Error(...)`. The behavior is correct
  (header-after-write encode-fail must surface somewhere); the file
  header should mention the fallback to keep dc-00 honest.

- `backend/internal/buildinfo/buildinfo.go` — fine for now, but as
  soon as the build pipeline lands, double-check the `-ldflags`
  invocation in the `Makefile:23-25` and `Dockerfile:36-40` produce
  byte-identical `Version` strings. A divergence between local and
  Docker builds would show up at `/healthz` and is a small dc-00
  surprise.

### Data

**Blocker** — none.

**Must-fix-Week-2:**

- `data/supabase/seed.sql:38-43` — **dc-00 (zero-guessing bar) + dc-04
  (don't leak local artifacts into shared environments).** The
  production guard is effectively a no-op: `current_setting('app.environment', true)`
  returns NULL when unset (the `true` flag means "don't error on
  missing"), and `null = 'production'` is always false. Nothing in
  the repo ever SETs `app.environment` either. If a future operator
  ever runs `psql -f seed.sql` against the staging or prod project,
  the guard silently lets the dev user land in `auth.users`.
  **Fix:** either pivot the guard to check `current_database()` (Supabase
  cloud uses `postgres`; local uses `postgres` — that's not distinctive,
  so use `inet_server_addr() IS NULL` for "local socket only", or
  better, key off a `--var` passed at the `psql` boundary). The
  documented `supabase db reset` path doesn't load `seed.sql` against
  cloud projects today, so this is defense-in-depth — but the
  comment block currently advertises a guard that doesn't actually
  guard. dc-00: don't write a check that doesn't check.

**Should-fix:**

- `data/tests/01_rls_cross_user_isolation.sql` — the only pgTAP
  assertions are on `food_items`. The data plan §5 task 18
  acceptance check requires the cross-user test to cover:
  (a) `food_items` cross-user (covered), (b) `meal_plan_days`
  through-parent isolation, (c) `kitchen_messages` append-only
  (no UPDATE / no DELETE), (d) Storage bucket cross-user object
  access, (e) cookbook delete-trigger atomicity. (b)–(e) are
  outstanding. **Fix:** add four more pgTAP files (or extend this
  one) before merging Week 2 work; the policies are written, the
  asserts just need to land. Particularly (c) is the only check
  that exercises the "policy absence = denial" pattern the data
  Builder relies on for append-only.

- `data/supabase/schemas/01-app-private.sql:29-46` — the
  `app_private.uuidv7()` wrapper probes `to_regprocedure('extensions.uuid_generate_v7()')`
  on every call. dc-04 prefers UUIDv7 because the embedded timestamp
  keeps B-tree inserts ordered; the probe-on-every-call adds a
  catalog lookup per generated PK, which on a hot insert path matters.
  **Fix:** Memoize at function-definition time using a `DO` block in
  `00-extensions.sql` that picks the right body — or accept the
  catalog cost for v1 (the lookup is microseconds; a single-user
  workload won't notice). Acceptable as-is for v1; flag for
  future tuning.

- `data/supabase/schemas/03-rls.sql:31-51` — the `ENABLE / FORCE`
  block lists every table by name (8 tables × 2 statements). Per
  dc-04 R1, a forgotten table is a public data leak. Adding a future
  table requires the author to remember to add two lines here.
  **Fix:** add a pgTAP test asserting `pg_class.relrowsecurity` AND
  `pg_class.relforcerowsecurity` are true for every table in
  `public.*` (track-data §8 R1 even calls this out). Not blocking
  Week 2 since the current 8 tables are correctly handled, but the
  guard rail belongs in place before Week 3 introduces views or
  function-backed schemas.

**Nit:**

- `data/supabase/config.toml:97-104` — the Apple provider section
  is correctly disabled for local dev but the section comment says
  "wire this up against the real cloud projects". When the cloud
  projects come online (data §5 task 17), the `config.toml` should
  NOT be the place credentials land — the comment block could
  reaffirm "credentials never live in this file; the dashboard owns
  them" to stay dc-00-honest for a future reader.

### iOS

**Blocker** — none.

**Must-fix-Week-2:**

- `ios/project.yml:18, 57` — **track-ios.md §3.1 mismatch.** The
  iOS track plan explicitly names the bundle id as
  `com.dobbins.souschef` (§3.1 and §4 toolchain table); the
  project picks `com.djd39448.souschef`. The plan does call the
  bundle id "provisional; confirmed at Apple Developer enrollment",
  so this is not a contract break, but neither an ADR nor a
  `change_log.md` entry records the pivot. dc-00 fails when a
  reader has to ask "is the GitHub-handle prefix intentional or
  a typo?" **Fix:** either land an ADR ("ADR-0012 — bundle id is
  `com.djd39448.souschef` to match the Apple Developer account")
  or update `track-ios.md` §3.1 to match. The Apple Developer
  enrollment step in §4 §9 still depends on Dave; pick one bundle
  id and commit it in writing before SIWA wires up in Week 2.

**Should-fix:**

- `ios/project.yml:26, 60` — the project sets
  `SWIFT_VERSION: "6.0"` (the Swift language mode); `dc-03`
  baselines **Swift 6.2** as the toolchain (xcode 16+). Swift 6.0
  is the language version for strict-concurrency mode; the
  toolchain itself is governed by Xcode. The setting here is
  about the language mode, which is correct. **But:** the
  `.swiftformat` and `Package.swift` both pin `swift-tools-version:6.0`
  / `--swiftversion 6.0`, which is the language. If Dave's Xcode
  16.x ships with Swift 6.2 as the compiler, both the project
  and the package will build fine; if a future Xcode bumps the
  default to Swift 7, the explicit 6.0 setting becomes the brake.
  No fix today; flag for the "Xcode version drift" gotcha at
  dc-06.

- `ios/SousChefKit/Sources/{API,Auth,Domain,Markdown,ImageCache}/*.swift`
  — every "library" file is a single-line `public enum
  …Version { public static let current = "0.1.0-foundation" }`
  stub. This is fine **only because** every file's dc-01 header
  spells out what it will contain and which Phase 5 task lands
  the real content. dc-00 is honored: a reader knows what's
  missing and when. **Fix:** none for Week 1; verify that as
  the real code lands the dc-01 header is updated to reflect
  reality, not the original promise.

- `ios/SousChef/SousChefApp.swift:24` — the `@State private var
  isSignedIn = false` will never flip to true (the LoginView
  never calls `onSignInSucceeded`), so the entire `if isSignedIn
  { RootView() }` branch is unreachable in Week 1. dc-00:
  unreachable code is a guess for a future reader. The dc-01
  header DOES say "In Week 1 the entry point shows the mocked
  LoginView", so a reader knows the state; the unused branch is
  scaffolding for Week 2. Acceptable as-is.

**Nit:**

- `ios/SousChef/LoginView.swift:97-99` — the "Send code" button
  shows a mock alert AND advances to the `.enterCode` step. Two
  side effects per tap is slightly surprising for a mock. Not a
  bug; just a UX oddity for the gate review.

- `ios/.swiftlint.yml:17-18` — `trailing_comma` is disabled in
  favor of SwiftFormat's `trailingCommas`. The comment is good;
  add the same justification when other rules are toggled later
  so the rationale doesn't bit-rot.

---

## §4 Cross-track issues

### 4.1 Contract §8.4 vs `integration.md` §2.2 — cookbook save rollback

The contract (§8.4) says: *"Supabase Storage upload failure: return
503 … with `internal_error`; the cookbook save **does not roll back**
(the recipe row is persisted; `image_url` stays null)."*

`integration.md` §2.2 says: *"on image-gen failure, [backend] returns
502 and **does not** insert the cookbook row."*

These are mutually exclusive. The contract is the authoritative
artifact (the integration doc explicitly does not redefine the wire),
and the iOS track plan (§6.3 "Cookbook save UX during image generation")
correctly mirrors the contract: *"a 503 from POST /cookbook means the
recipe row was persisted but image_url stayed null."* So in practice
the contract wins.

**Affected tracks:** backend (J3 acceptance test will diverge from
integration.md), iOS (recipe-save model state machine), and the
Conductor's playbook.

**Fix:** Conductor decision — update `integration.md` §2.2 to mirror
the contract's persist-and-leave-null semantics, or open an ADR if
the integration doc's pattern is preferred and the contract should
change. The Reviewer's call: the contract's pattern is the safer one
(no silent loss of user-typed content because OpenAI hiccuped), so
update integration.md.

### 4.2 SSE error event shape — contract §6.1 vs apierror envelope

Contract §6.1 specifies the SSE error event as
`{ "type": "error", "error": "<code>", "details": <any>? }`. The
`apierror.envelope` struct emits the HTTP response shape
`{ "error": "<code>", "details": <any>? }` (no `type` discriminator).
The two are spec'd to be different and there is no defect today —
but the SSE writer (backend Phase F1) MUST emit the discriminator,
not the JSON-body envelope. **Affected tracks:** backend.

**Fix:** when Phase F1 lands, the SSE writer reuses the `apierror.Code`
constants but wraps them in an `sseErrorEvent` struct with `Type`
and `Error` fields. Easy to get right; easy to get wrong. Flag in
the F1 implementation prompt.

### 4.3 ADR-0011 ⟂ track-backend §6.1 — RLS as load-bearing or doc-only

ADR-0011 binds the backend to a JWT-aware connection (RLS enforces).
The backend track plan §6.1 (lines 690-691) still carries the older
text describing RLS as "defense-in-depth backstop, not the Go
service's primary scoping". The plan has a NOTE block at the top
pointing to ADR-0011, but §6.1's body has not been rewritten to
match. **Affected tracks:** backend.

**Fix:** Conductor follow-up — when the next round of plan
amendments lands, rewrite track-backend §6.1's "RLS… defense-in-depth
backstop" sentence to read "RLS is load-bearing per ADR-0011; the
Go service connects as the `authenticated` role and SETs
`request.jwt.claim.sub` per request." Not a code defect; a documentation
freshness gap that will mislead the Builder when they read §6 to
implement Phase C.

### 4.4 Bundle-id divergence (see §3 iOS Must-fix-Week-2)

Cross-track because the bundle id is referenced in three places:
`ios/project.yml`, the Apple Developer enrollment Dave-action
(integration.md §2.4 / iOS §4), and Supabase Auth's Apple-provider
config (data §5 task 17). All three must agree. The iOS Builder
picked one; the data and Conductor work hasn't.

**Fix:** pick the bundle id once and propagate to the iOS project
(YAML), an ADR (so Dave's Apple Developer step uses the same
string), and the data track's task-17 acceptance note.

---

## §5 Positive observations

Things the Builders got right and should keep doing.

- **dc-01 headers are universally excellent.** Every Go file, every
  Swift file, every `.sql` file carries the four-line shape (what /
  depends on / depended on by / why) and the headers are real, not
  boilerplate. Several go a step further and cite the contract
  section, the ADR, and the future task that lands the real
  content — that's the spirit of dc-00, not just the letter.

- **Tests are present even for stubs.** Each SousChefKit library
  target has a Swift Testing suite. Each is a single
  `versionMarkerExists` assertion, but the **test target plumbing
  is in place**, so Week 2 drops real cases in without scaffolding.
  Same on the backend: `apierror_test.go`, `config_test.go`,
  `server_test.go` are real tests with `t.Parallel` and table-driven
  subtests where appropriate.

- **Backend security defaults.** No request body logged
  (`access log` writes method/path/status only). Error envelope
  never leaks internal text. Panic recovery does not expose stack
  to client. Request ID is generated with `crypto/rand`, not
  `math/rand`. Header set with the request id is `X-Request-ID`,
  the standard. Boot fails closed if any required secret is unset.

- **RLS rigor.** `ENABLE + FORCE` on every table. One policy per
  operation. `(select auth.uid())` everywhere — never the bare
  function call. `SECURITY DEFINER` helpers have explicit
  `search_path`. The `kitchen_messages` append-only is enforced by
  *policy absence*, with an explanatory comment block on the table
  AND on the relevant policy file — that's not obvious and someone
  thought about it.

- **Storage RLS pattern.** The bucket-scoped policies use the
  documented `(storage.foldername(name))[1] = (select auth.uid())::text`
  form rather than rolling a custom check. The cookbook-delete
  trigger uses `SECURITY DEFINER` to bypass the storage.objects RLS
  inside the trigger body, with the reason in a function comment.

- **iOS package boundary as a compile-time guarantee.** The
  `SousChefKit` package's five library targets do not link SwiftUI;
  a stray `import SwiftUI` becomes a build error, not a code-review
  catch. This is the dc-03 "model and service types never `import
  SwiftUI`" rule realized as type-system enforcement.

- **iOS LoginView fail-closed.** The mocked OTP flow never
  invokes the `onSignInSucceeded` callback. A reviewer who taps
  every button cannot accidentally land in `RootView`. This is
  exactly what "auth flow placeholder fails closed" looks like.

- **Makefile + README ergonomics.** Each track ships a Makefile
  with a `help` target and clear verbs, plus a README that names
  the deferred work and why. The data README's "What was deferred
  (and why)" section is the gold standard — a reviewer reading
  cold knows what's there, what isn't, and why.

- **`updated_at` discipline.** Every table that carries the column
  has the trigger; the trigger function is in `app_private` (not
  `public`); the contract integration note ("backend MUST NOT write
  to `updated_at`") is captured in the trigger function's comment.

---

## §6 Conductor follow-ups

Not Builder defects; Conductor or workflow items.

- **Update `integration.md` §2.2** to mirror the contract's "save
  persists, image_url stays null on storage failure" semantics
  (Cross-track 4.1). Otherwise the backend Builder will follow the
  integration doc and ship a divergent J3.

- **Rewrite `track-backend.md` §6.1's RLS paragraph** to reflect
  ADR-0011's binding decision (Cross-track 4.3). The NOTE at the
  top of the plan catches a careful reader; the body still
  contradicts.

- **Decide the bundle id once, in writing** (Cross-track 4.4 +
  iOS §3 Must-fix-Week-2). One ADR or one `change_log.md` entry,
  whichever ladders cleaner. Pre-Phase-B-end so the SIWA Service
  ID and Supabase Apple provider config can land against a frozen
  value.

- **Conductor playbook capture.** `build_log.md` notes the
  worktree-isolation lesson learned ("Builder paths must not share
  a working directory"). That lesson is not yet reflected in any
  `prompts/conductor-playbook.md` (which doesn't exist). When the
  Conductor's prompt addendum lands, copy the lesson into a
  durable home so the next Phase-4 dispatch doesn't re-discover it.

- **Open pgTAP test plan.** The Data track's §5 task 18 acceptance
  check enumerates five cross-user / append-only scenarios; only
  the food_items scenario landed in Week 1. Conductor: schedule
  the remaining four tests as a discrete Week-2 task; do not let
  Week-3 work pile on top of an under-tested RLS surface.

- **No-secrets discipline.** No secret material is in the diff or
  commit history (`openai_api_key = "env(OPENAI_API_KEY)"` in
  `config.toml` is the Supabase CLI's documented env-reference
  syntax, not a leaked value; the test fixtures in
  `config_test.go` are clearly synthetic). Keep the pattern: real
  keys only ever in `.env` or AWS Secrets Manager.

---

## Final note

All three tracks are in a good place to take Week 2 commits on top.
The pattern Phase 4 Week 1 established — small commits, real tests
on stubs, dc-01 headers that name the cross-references — is the
shape this workload needs to keep through Weeks 2–4. The single
**Must-fix-Week-2** defect per track plus the four cross-track
items above are entirely tractable inside Week 2's start.
