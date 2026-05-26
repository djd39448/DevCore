---
type: plan
title: Backend track plan — sous-chef-ios
status: accepted
owner: builder.backend
workload: sous-chef-ios
last_updated: 2026-05-24
contract: contract/contract.md
---

# Backend track plan — sous-chef-ios

The Go-on-AWS service that implements the shared contract for the Sous Chef
iOS port. This plan is **the** plan the backend Builder executes in Phase 4.
The contract (`memory/contract/contract.md`) is the spec; this document is
the route from contract to running service.

> **NOTE 1 (post-`track_plan`-gate):** §3 *Architecture* in this plan defaulted
> to a service-role connection with app-level `WHERE user_id = $1` filtering.
> **That default is overridden by ADR-0011 — JWT-aware connection, RLS
> enforced.** The Builder reads ADR-0011 before Phase 4 implementation and
> incorporates its task addendum (one new task: `internal/store.WithClaims`
> helper) into this plan's §5 task tree. The rest of the plan stands.
> See `memory/plan/integration.md` §1.1 for the cross-track reasoning.
>
> **NOTE 2 (mid-Phase-4 pivot, 2026-05-25):** §3.3 *AWS target* in this plan
> chose **ECS Fargate**. **That choice is overridden by ADR-0013 — Elastic
> Beanstalk Docker, load-balanced with ALB, us-east-1.** The Dockerfile,
> Makefile, distroless image, and SSE design all carry over unchanged
> (Beanstalk's Docker-on-AL2023 platform runs the same image behind an ALB).
> The Phase L task list gains two items: `eb init` against the
> `sous-chef-api` application in us-east-1 (commit `.elasticbeanstalk/
> config.yml`), and a `.ebextensions/01-alb-idle-timeout.config` setting
> the ALB idle timeout to 600s for SSE. The Dave-action in
> `integration.md` §3.2 "confirm AWS ALB idle-timeout 600s" is superseded
> by this ADR (the value is now version-controlled). The rest of the plan
> stands.

---

## §1 What this plan is

This file exists because three Builders (`backend`, `data`, `ios`) must
proceed in parallel against one contract, and each needs an independently
buildable plan the Conductor can clear at the `track_plan` gate.

- **Read by:** the Conductor at the `track_plan` gate (Phase 3 → 4 bridge),
  then the backend Builder during every Phase 4 implementation task, then
  the Reviewer when judging "does the code match the plan" and the
  Verifier when running the deploy gate.
- **Commits to:** every contract §5 endpoint reachable on AWS staging, the
  SSE chat path with tool dispatch, JWT middleware, the `AIClient`
  interface with an OpenAI implementation and a fake, and a single Go
  binary that boots from environment variables and serves `/healthz`.
- **Depends on:**
  - `memory/contract/contract.md` — the spec.
  - `memory/domain/sous-chef-behaviors.md` — context for why each
    endpoint exists.
  - `memory/decisions/0001-..0010-..` — ten ADRs that gate the contract.
  - The data track plan — for the Supabase schema and storage bucket
    this service queries (this plan does **not** depend on the data
    track's *implementation* — only on the contract §4 description).
  - `CODING_STANDARDS.md` §dc-00, §dc-01, §dc-02 (Go), §dc-05 (AWS).
- **Out of scope:** Supabase migrations and RLS SQL (data track); the
  iOS app (iOS track); the human-led DNS/TLS provisioning of the AWS
  staging domain — backend's responsibility ends at "the load balancer
  forwards to this binary and it answers".

---

## §2 Scope

### In scope

1. The Go HTTP service implementing every contract §5 endpoint:
   `/healthz`, `/api/kitchen/conversations*` (4), `/api/kitchen/weeks/*`
   (1), `/api/kitchen/calendar`, `/api/kitchen/meal-plans*` (2),
   `/api/kitchen/meal-plan-days/*` (2), `/api/kitchen/recipe-message`,
   `/api/kitchen/shopping-lists*` (4), `/api/kitchen/shopping-items/*`
   (1), `/api/kitchen/ingredients`, `/api/kitchen/ingredient-suggestions`,
   `/api/kitchen/cookbook*` (5), `/api/kitchen/regenerate-image`.
   Twenty-six handlers in total.
2. The Supabase JWT verification middleware (JWKS-cached, refresh on
   `kid` miss) — contract §2.2.
3. The SSE streaming layer used by three endpoints (main chat, recipe
   generation, recipe-page chat) — contract §6.
4. The `AIClient` interface (ADR-0002) with the production OpenAI
   implementation and a fake for tests.
5. Server-side tool-call dispatch for the five tool variants (contract
   §7): `update_ingredients`, `create_meal_plan`, `create_shopping_list`,
   `update_meal` (main chat), `update_meal` (recipe-page).
6. The data-access layer that talks to Supabase Postgres (via `pgx` —
   see §3) and Supabase Storage (via the storage REST API).
7. The image generation path (contract §8) — synchronous on cookbook
   save, asynchronous on `/regenerate-image` (transient bytes), explicit
   on `/cookbook/{id}/regenerate-image` (persisted bytes + cache
   buster).
8. Structured JSON logging via `log/slog`; structured error envelope
   per contract §3.5.
9. The Dockerfile, the AWS deployment artifact (ECS task definition and
   companion files), and a documented deploy procedure to the staging
   account.
10. CI: `gofumpt`, `go vet`, `golangci-lint`, `go test -race`,
    `govulncheck` — green on every commit.

### Out of scope (explicitly, with whose track owns it)

- The Supabase schema, the SQL migrations, the RLS policies, the
  storage bucket creation, the `updated_at` triggers — **data track**.
- Any client-side code in Swift — **iOS track**.
- AWS account provisioning, the VPC/subnets, the load balancer, ACM
  certificates, Route53 records — Dave provisions these once and hands
  the backend Builder the ARNs/hostnames. The Builder produces the
  task definition and the Dockerfile.
- Data migration from the existing web app — workload spec §9 places
  this out of scope for the port.
- Rate limiting and pagination — contract §3.4 defers both. The
  backend implements the defensive 1000-row cap; no per-user
  throttling at the edge.

---

## §3 Architecture

A single Go binary. No microservices, no message bus, no separate
worker. Concurrency happens inside the process via goroutines bound to
the HTTP request lifecycle.

### §3.1 Package layout

```
cmd/sous-chef-api/
  main.go                  -- wiring only: flags, slog, config, signals
internal/
  config/                  -- env-var loading, validation, secret hydration
  server/                  -- http.Server, mux, middleware chain
    middleware/            -- auth, request ID, panic recovery, logging
  auth/                    -- Supabase JWKS fetch + cache, JWT verify
  api/                     -- one file per resource: conversations.go,
                              meal_plans.go, cookbook.go, shopping.go,
                              ingredients.go, recipes.go, sse.go
  aiclient/                -- AIClient interface + openai impl + fake
    tools/                 -- the five tool schemas (single source of truth)
  toolschemas/             -- byte-identical JSON schemas, embedded via go:embed
  store/                   -- pgx-backed data access; one file per table family
  storage/                 -- Supabase Storage REST client (upload, delete,
                              signed URL)
  image/                   -- the image.Generate facade (calls aiclient,
                              uploads via storage, returns URL)
  sse/                     -- generic SSE writer: event encoder + flusher loop
  apierror/                -- the error code constants + envelope writer
  weekdate/                -- Monday-of-week validation; the only date helper
testdata/                  -- golden SSE traces, OpenAI fixture responses
```

### §3.2 Concurrency model

- Per-request goroutines as the Go HTTP server provides them. **No
  background workers in v1.**
- Image generation on cookbook save is synchronous in the request
  goroutine (latency budget: ~10s; contract documents the blocking
  behavior at §8.2 and §5.6). If the upstream image generation fails,
  the cookbook row still persists (`image_url` stays null); the
  request returns 503 only when the row write itself fails.
- The SSE handlers run a single goroutine per stream — they read from
  the OpenAI streaming response and write to the HTTP response flusher
  inline. **No fan-out**: one upstream chunk produces zero-or-one
  downstream events. Tool calls block the stream while the server
  dispatches them (matches source behavior; matches contract §6.3
  ordering).
- All goroutines respect `r.Context()`; cancellation propagates to the
  OpenAI client via `context.WithCancel` chains. Per contract §6.5,
  cancelling the client SSE drops the upstream OpenAI stream and
  persists whatever assistant text was emitted up to that point.

### §3.3 SSE implementation choice

**Standard library only.** `http.ResponseWriter` + `http.Flusher` + a
small internal helper. No third-party SSE library.

Reasoning:

- The wire shape is `data: <json>\n\n` per event (contract §6.1) —
  ~10 lines of code to emit, well within stdlib reach.
- A dependency would be supply-chain surface for an unchanging,
  trivially-specifiable wire format (`dc-02` minimum-dependency rule).
- The `sse.Writer` in `internal/sse` accepts `any` and writes
  `data: <json-encoded-any>\n\n`; ordering and flush semantics live in
  the handler, not in a library.

The handler pseudocode:

```go
flusher, ok := w.(http.Flusher)
if !ok { return apierror.Internal("streaming unsupported") }
w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
w.Header().Set("Cache-Control", "no-cache")
w.WriteHeader(http.StatusOK)
sseW := sse.NewWriter(w, flusher)
defer sseW.Close(ctx)
// then: sseW.Event(ctx, sseEvent{Type: "text", Content: chunk})
```

### §3.4 Configuration

Twelve-factor — environment variables only, validated at boot, fatal on
missing required values (`dc-05`). The full set:

| Variable | Required | Purpose |
|---|---|---|
| `PORT` | optional (default 8080) | HTTP listen port. |
| `LOG_LEVEL` | optional (default info) | slog level. |
| `SUPABASE_JWKS_URL` | required | `https://<project>.supabase.co/auth/v1/.well-known/jwks.json` |
| `SUPABASE_ISSUER` | required | `https://<project>.supabase.co/auth/v1` — JWT `iss` check. |
| `SUPABASE_DB_URL` | required (secret) | `postgres://...` connection string. |
| `SUPABASE_STORAGE_URL` | required | Storage REST base URL. |
| `SUPABASE_SERVICE_ROLE_KEY` | required (secret) | For Storage REST writes only; never reaches client. |
| `OPENAI_API_KEY` | required (secret) | Direct OpenAI (ADR-0002). |
| `OPENAI_CHAT_MODEL` | optional (default `gpt-4.1`) | Per contract §7 / ADR-0002. |
| `OPENAI_IMAGE_MODEL` | optional (default `gpt-image-1`) | Per contract §8. |
| `STORAGE_BUCKET_COOKBOOK` | optional (default `cookbook-images`) | |
| `AWS_REGION` | required in prod | For log shipping / secret hydration. |

**Secret loading:** secret-valued vars come from AWS Secrets Manager
at boot via the AWS SDK; the task IAM role gates access. Plain env
vars are non-secret. The config package resolves both transparently
(`dc-05`).

### §3.5 Logging

`log/slog` JSON handler in production, text in development. Every
handler attaches `request_id` and `user_id` (post-auth) to a derived
logger via `slog.With`. No `fmt.Println`. No request bodies logged
(contract §3.5 forbids leaking client data into logs unless
explicitly safe). Error chains logged at the layer that produced
them, never re-wrapped at the boundary just to re-log.

### §3.6 Stack & toolchain (summary)

See §4 for the full toolchain decision matrix.

---

## §4 Stack & toolchain

### §4.1 Language

**Go 1.26.** Pinned in `go.mod` (`go 1.26.0`). Per `dc-02`.

### §4.2 Key libraries

| Concern | Choice | Why |
|---|---|---|
| HTTP routing | `net/http` `ServeMux` | Go 1.22+ method+path patterns are sufficient; matches `dc-02` "no router without a stated reason". |
| Postgres | `jackc/pgx/v5` (pool) | Direct SQL — the schema is owned by the data track in plain SQL, and `pgx` gives us LISTEN/NOTIFY, JSONB support, and named-arg queries without an ORM layer. **Not** `database/sql`; pgx is the de facto Go Postgres driver. **Not** a Supabase Go client — the auth-track work happens at the JWT layer; for data we want the simpler, faster direct connection. |
| JWT | `golang-jwt/jwt/v5` + `MicahParks/keyfunc/v3` | Verified parser + cached JWKS with auto-refresh on `kid` miss; widely used Supabase pattern. |
| OpenAI | `sashabaranov/go-openai` v2 | Most-used Go OpenAI client; supports tool calling, streaming, images. Wrapped behind `AIClient` so we can swap. |
| Test framework | stdlib `testing` + `testify/require` for assertion noise | Per `dc-02`: stdlib first, `testify` allowed for `require`. **No** `testify/mock`. |
| Lint/format | `gofumpt`, `golangci-lint` with `.golangci.yml` | Per `dc-02`. Lints: `errcheck, govet, staticcheck, ineffassign, unused, revive, gocritic, bodyclose, errorlint, contextcheck, noctx, gosec, nilerr`. |
| Vulnerability scan | `govulncheck` | Per `dc-02`. Runs in CI. |

### §4.3 AWS target — ECS Fargate

**ECS Fargate** on a 512MB-CPU / 1GB-memory task, behind an
Application Load Balancer, in a single-AZ public subnet for staging
(multi-AZ for production). One service, one task definition, one
container, scaling 1→3 on CPU. ALB terminates TLS via ACM; routes
`/healthz` to the container's `/healthz`.

**Why ECS Fargate, not Lambda or EC2:**

- **Steady traffic shape.** The product is conversational and SSE-heavy
  — Lambda's cold-start (~500ms) and 15-minute hard ceiling on
  streaming responses (API Gateway tightens it to 30s for SSE in most
  configurations) makes it the wrong tool. `dc-05` calls ECS/Fargate
  "the default for a steady-traffic REST API"; we are exactly that.
- **SSE pinning.** API Gateway in front of Lambda terminates idle
  streams aggressively and buffers writes. The ALB → ECS path holds
  TCP open for the full stream duration and forwards chunks verbatim.
- **Stateful TCP to OpenAI.** A long-lived `*http.Client` with a
  reused TLS connection to api.openai.com cuts ~200ms off each chat
  send. Lambda's instance-recycle wipes this.
- **Configuration simplicity.** One task definition, one Dockerfile,
  one deploy verb. Lambda + API Gateway + custom-runtime + streaming
  shim is more pieces with worse SSE characteristics.
- **EC2 is over-provisioned.** Single-user-household workload doesn't
  justify ASG management; Fargate gives us "compute as a unit, billed
  per second, no OS to patch".

`dc-05` rules out App Runner (maintenance mode). The choice is
ECS/Fargate vs. Lambda; SSE pins it to ECS.

### §4.4 Build & deploy

- **Image:** multi-stage Dockerfile. Stage 1: `golang:1.26-alpine`,
  builds static binary with `CGO_ENABLED=0`. Stage 2: `scratch` with
  the binary, CA certs (`/etc/ssl/certs/ca-certificates.crt`), and
  zoneinfo (`tzdata`). Final image < 30MB.
- **Tagging:** every image is tagged with `:sha-<short-git-sha>` (and
  `:latest` only on `main`).
- **Registry:** AWS ECR private repository; image scanning on push.
- **Deploy:** `aws ecs update-service --force-new-deployment` after
  pushing the image. The task definition references the image by SHA
  tag (immutable; `dc-05`).
- **Observability:** stdout/stderr → CloudWatch Logs (the awslogs
  driver). Two log groups: `/sous-chef-api/staging`, `/sous-chef-api/
  prod`. Metrics: ECS native CPU/memory; per-request latency emitted
  as a CloudWatch Embedded Metric Format JSON line for percentiles.

---

## §5 Task tree

Ordered. Each task lists dependencies, the contract section it
satisfies, and the acceptance check the Builder confirms before the
task is "done". The Builder works strictly top-to-bottom within a
phase; phases gate each other.

### Phase A — Foundation (no auth, no AI)

**A1. Repo skeleton + module init.**
Dependencies: none.
Acceptance: `go mod init`, the package layout from §3.1 exists with
empty `dc-01` headers in each file, `go build ./...` succeeds, `gofumpt`
and `golangci-lint` run clean on the empty tree. Contract: none
directly — the structural foundation.

**A2. CI pipeline.**
Dependencies: A1.
Acceptance: `.github/workflows/ci.yml` runs `gofumpt -l`,
`go vet ./...`, `golangci-lint run`, `go test -race ./...`,
`govulncheck ./...` on every push; all five green on the empty tree.

**A3. Config loading.**
Dependencies: A1.
Acceptance: `internal/config` reads the env-var set from §3.4,
validates required vars, returns a typed `Config`. Unit tests cover
missing-var fatal-exit and override defaults. `dc-02`, `dc-05`.

**A4. `cmd/sous-chef-api/main.go` + `GET /healthz`.**
Dependencies: A3.
Acceptance: Service starts, listens on `$PORT`, `/healthz` returns
`200 {"status":"ok"}`. `slog` JSON output goes to stdout.
Contract: §3.6.

**A5. Structured error envelope helper.**
Dependencies: A1.
Acceptance: `internal/apierror` exposes typed error codes (the dozen
listed in contract §3.5) and a `Write(w, code, status, details)`
function. Unit-tested for response shape and status mapping.
Contract: §3.5.

**A6. Request ID + slog middleware + panic recovery.**
Dependencies: A4.
Acceptance: Every response carries `X-Request-ID`; every log line
includes `request_id`; a handler that panics returns
`500 {"error":"internal_error"}` (no stack trace on the wire); the
process keeps serving. Contract: §3.5 ("never leaks internal error
text").

**A7. Dockerfile + ECR push + initial ECS task definition.**
Dependencies: A4.
Acceptance: The image builds, pushes to ECR, deploys to staging ECS,
ALB routes `/healthz` to it, returns 200 from a public DNS name.
This is the **first deploy** — confirms the deploy pipeline before
any feature code lands. Contract: §3.6.

### Phase B — Auth

**B1. Supabase JWKS client.**
Dependencies: A3.
Acceptance: `internal/auth/jwks.go` fetches from
`$SUPABASE_JWKS_URL`, caches the keyset, refreshes on `kid` miss.
Race-tested. Backed by `keyfunc`. Contract: §2.2.

**B2. JWT verify middleware.**
Dependencies: B1, A5, A6.
Acceptance: `internal/server/middleware/auth.go` extracts the
`Authorization: Bearer <jwt>` header, verifies signature against
JWKS, checks `iss`, `exp`. Stuffs the verified `userID uuid.UUID`
into the request context via a typed key. Six unit tests covering
the contract §2.3 rejection table (missing/malformed/invalid/expired/
wrong-issuer + happy path). Contract: §2.2, §2.3.

**B3. Test harness: fake JWT minter.**
Dependencies: B2.
Acceptance: A test helper signs a JWT with a test RSA key whose
JWKS is served by an `httptest.Server`. Used by every subsequent
handler test. Contract: testability of §2.

### Phase C — Data access

**C1. pgx connection pool + Ping check.**
Dependencies: A3, A4.
Acceptance: `internal/store` exposes a `*pgxpool.Pool` constructed
from `$SUPABASE_DB_URL`, with pool sizing (max 10 conns) and a
boot-time `Ping`. The pool joins `/healthz` — a DB-unhealthy
process returns 503. **No SQL written yet.** Contract: §3.6.

**C2. Store package shape + interface seam.**
Dependencies: C1.
Acceptance: `internal/store` exposes interfaces per resource family
(`Conversations`, `MealPlans`, `Cookbook`, `Shopping`, `FoodItems`,
`Messages`). Each interface has its real `pgx`-backed
implementation and a fake in-memory implementation. **No reading
the data track's schema yet** — we define the methods the handlers
need; the data track's SQL must satisfy them at the contract level.
Tests use the in-memory fakes. Contract: §4 (we consume the schema
shape the contract documents; we don't author the SQL).

### Phase D — Read-side endpoints

(These are pure JSON reads; no streaming, no AI, no writes.)

**D1. `GET /api/kitchen/conversations`.**
Dependencies: B2, C2.
Acceptance: Lists user's conversations newest-updated-first; 401
for missing token; per-user row scoping by JWT `sub`. Contract:
§5.1.

**D2. `GET /api/kitchen/conversations/{id}`.**
Dependencies: D1.
Acceptance: Returns conversation + messages oldest-first; 404 on
non-owner per contract §2.3. Contract: §5.1.

**D3. `POST /api/kitchen/conversations`.**
Dependencies: D1.
Acceptance: Creates an empty conversation with default title;
returns 201. Contract: §5.1.

**D4. `GET /api/kitchen/weeks/{weekStartDate}`.**
Dependencies: B2, C2.
Acceptance: `weekdate.MustBeMonday` validates; returns
`{ mealPlan, shoppingList }`; 400 with `week_start_not_monday` for
non-Mondays. Contract: §5.2, §3.2.

**D5. `GET /api/kitchen/calendar`.**
Dependencies: D4.
Acceptance: Returns the calendar drilldown summary. Contract: §5.2.

**D6. `GET /api/kitchen/meal-plan-days/{dayId}`.**
Dependencies: D4.
Acceptance: Returns a single day with `recipeContent` and
`recipeImagePrompt` (possibly null). Contract: §5.2.

**D7. `GET /api/kitchen/shopping-lists` and `GET
/api/kitchen/shopping-lists/{identifier}`.**
Dependencies: D4.
Acceptance: Identifier-as-UUID OR identifier-as-YYYY-MM-DD; correct
404; categories on items returned verbatim. Contract: §5.4.

**D8. `GET /api/kitchen/ingredients` and
`/ingredient-suggestions`.**
Dependencies: D1.
Acceptance: Filtered by `usage_context.role = 'inventory'` AND
`status != 'out'` (ADR-0009); suggestions deduped, max 20.
Contract: §5.5, ADR-0006, ADR-0009.

**D9. `GET /api/kitchen/cookbook` and `GET /api/kitchen/cookbook/{id}`.**
Dependencies: D1.
Acceptance: List omits `content`; single returns full body
including `imageUrl`. Contract: §5.6.

### Phase E — AI client + tool schemas

**E1. `AIClient` interface + OpenAI implementation.**
Dependencies: A3.
Acceptance: `internal/aiclient` defines `AIClient` with three
methods: `Chat(ctx, req) (Stream, error)`,
`OneShot(ctx, req) (Response, error)` (for the JSON-mode meal-plan
generation), `GenerateImage(ctx, prompt) ([]byte, error)`. OpenAI
implementation wraps `sashabaranov/go-openai`. Contract: ADR-0002,
§7, §8.

**E2. Fake `AIClient` for tests.**
Dependencies: E1.
Acceptance: `aiclient/fake` returns scripted responses; SSE tests
drive deterministic chunked output and scripted tool calls.
Contract: testability of §6, §7.

**E3. Tool schemas — single source of truth.**
Dependencies: E1.
Acceptance: `internal/toolschemas/*.json` contains the five tool
JSON schemas (contract §7.1, §7.2, §7.3, §7.4, §7.5) embedded via
`//go:embed`. A test compares the embedded bytes against the
contract document's fenced-code blocks (parsed from
`memory/contract/contract.md`) — drift fails CI. The Architect
flagged this as a cross-track risk (§6 below); this addresses it
in one place. Contract: §7.

### Phase F — SSE foundation

**F1. `internal/sse.Writer`.**
Dependencies: A6.
Acceptance: Writer accepts typed events
(`text|tool_call|tool_result|done|error`) and emits `data: <json>\n
\n`; honors `ctx.Done()`; serializes with the discriminator
contract from §6.1. Unit tests against an `httptest.ResponseRecorder`
+ a captured `Flusher`. Contract: §6.1.

**F2. SSE round-trip integration test.**
Dependencies: F1, B3.
Acceptance: A test handler emits 5 events and `done`; a test client
consumes via the stdlib `bufio.Scanner` and asserts each event's
JSON shape. Documents the wire format byte-for-byte. Contract: §6.

### Phase G — Tool dispatch

**G1. Tool-call argument parsing + validation.**
Dependencies: E3.
Acceptance: For each of the five tool schemas, a strongly-typed Go
struct decodes the arguments via `json.Unmarshal`. Bad shapes
produce a typed error; the model can recover (contract §6.6).

**G2. `update_ingredients` handler.**
Dependencies: G1, C2.
Acceptance: Add upserts inventory-role food_items rows with the
correct defaults from contract §7.1; remove flips status to `out`
and zeros `on_hand_amount` without deleting. Per ADR-0006, no
`ingredient_memory` write. Contract: §7.1.

**G3. `create_meal_plan` handler.**
Dependencies: G1, C2.
Acceptance: Server-side dispatcher overrides any model-supplied
`week_start_date` with `clientWeekStartDate` from the chat-send
request (contract §7.2 note + ADR-0010); transaction replaces
existing plan; inserts day rows. Contract: §7.2.

**G4. `create_shopping_list` handler.**
Dependencies: G1, C2.
Acceptance: Dual-write `food_items` (`role=shopping`) and
`shopping_list_items` per contract §7.3 (behavior preserved from
behavior spec §2.5); transaction replaces existing list when
`week_start_date` is non-null. Contract: §7.3.

**G5. `update_meal` main-chat handler.**
Dependencies: G1, C2.
Acceptance: Day-name → integer mapping; upsert day row; **null
`recipe_content` and `recipe_image_prompt`** on any meal-name
change (cache-clearing invariant). Server injects
`clientWeekStartDate`. Contract: §7.4, §4.3.

**G6. `update_meal` recipe-page variant.**
Dependencies: G5.
Acceptance: Day lookup by `dayId`; same cache-clearing invariant.
Contract: §7.5.

### Phase H — SSE endpoints (write-side)

**H1. `POST /api/kitchen/conversations/{id}/messages` (main chat
SSE).**
Dependencies: F1, E1, E3, G2–G5, B2.
Acceptance: Persists the user message; opens an OpenAI chat
stream with the four tools (§7.1–§7.4); forwards `text` deltas
verbatim; on tool-call accumulation completion, dispatches to the
relevant handler and emits `tool_call` + `tool_result` events with
full argument objects (no deltas — contract §6.3); on stream end,
persists the assistant message text and emits `done` with
`assistant_message_id`. Auto-titles the conversation if title is
`"New Chat"` or `"Kitchen Chat"` and this is the first user
message. **Title auto-gen runs server-side as the behavior spec
§2.6 + contract §4.6 require.** Contract: §5.1, §6, §7.

**H2. `POST /api/kitchen/meal-plan-days/{dayId}/generate-recipe`
(recipe generation SSE).**
Dependencies: F1, E1, D6, C2.
Acceptance: Cookbook-first lookup by case-insensitive `title =
meal_name` (contract §9.8 + §4.4 index); on hit, stream the
cookbook content back and write it to the day; on miss, stream
OpenAI completion in canonical markdown format (behavior spec
§4.4); persist `recipe_content` and a templated
`recipe_image_prompt`; auto-save to cookbook if title is unique
(case-insensitive); **on auto-save, generate the image inline**
per ADR-0004; emit terminal `done` with `image_prompt`.
Contract: §5.2, §5.4, §6, §8.

**H3. `POST /api/kitchen/recipe-message` (stateless recipe-page
SSE).**
Dependencies: F1, E1, G6, B2.
Acceptance: No persistence to `kitchen_messages` (ADR-0008); the
recipe-page `update_meal` variant is the only tool; terminal
`done` carries `updated_meal` when the tool fired. Contract: §5.3,
§6.2, §7.5.

### Phase I — Non-streaming AI endpoints

**I1. `POST /api/kitchen/meal-plans` (generate a week).**
Dependencies: E1, D4, C2.
Acceptance: OpenAI JSON-mode call (temperature 0.9, randomized
cuisine and seasonal context per behavior spec §4.6); persists the
plan replacing any existing one for the same week; hand-coded
variety fallback if OpenAI returns unparseable output. Contract:
§5.2; behavior spec §4.6.

**I2. `POST /api/kitchen/meal-plans/{weekStartDate}/regenerate-
days`.**
Dependencies: I1.
Acceptance: OpenAI JSON-mode call at temperature 0.95 with "avoid
these meals" prompt context from kept days; updates only listed
`day_of_week` rows; null `recipe_content` and `recipe_image_prompt`
on updated days. Contract: §5.2.

**I3. `POST /api/kitchen/shopping-lists` (generate from meal
plan + inventory).**
Dependencies: I1, D8, C2.
Acceptance: Server-side generation (this is the non-tool path —
distinct from `create_shopping_list` tool); dual-write per
contract §5.4. Contract: §5.4.

### Phase J — Cookbook + image generation

**J1. Supabase Storage REST client.**
Dependencies: A3.
Acceptance: `internal/storage` exposes `Upload(ctx, bucket, key,
bytes, contentType) (url, error)` and `Delete(ctx, bucket, key)
error`. Uses the service-role key via `Authorization: Bearer
<service_role>`. Unit tests against an `httptest.Server`
simulating Supabase Storage. Contract: §4.4 storage bucket
description.

**J2. `image.Generate` facade.**
Dependencies: E1, J1.
Acceptance: One call: `image.Generate(ctx, prompt, opts)
(url, error)` — calls OpenAI for bytes, uploads to Storage at
`{user_id}/{recipe_id}.png`, returns the URL. Errors surface
per contract §8.4 (502 / 503 distinct). Contract: §8.

**J3. `POST /api/kitchen/cookbook` (save w/ inline image gen).**
Dependencies: J2, C2.
Acceptance: Validates non-empty title and content; persists the
recipe row; generates the image inline; returns the populated
recipe with `imageUrl`. **Returns 201 with `imageUrl = null` and a
500 inline-log entry on storage failure — the row still saves**
(contract §8.4 "the cookbook save does not roll back"). Contract:
§5.6 + §8.

**J4. `PUT`, `DELETE`, `POST /cookbook/{id}/regenerate-image`.**
Dependencies: J3.
Acceptance: PUT validates non-empty title/content if present, does
**not** regenerate the image. DELETE returns 204; the Storage
object is deleted by the data track's trigger (this handler does
nothing storage-side; the trigger runs on row delete). Regenerate
calls `image.Generate` against `cookbook_recipes.image_prompt`,
overwrites the Storage object, returns `imageUrl` with
`?v=<unix>` cache-buster. Contract: §5.6, §8.2.

**J5. `POST /api/kitchen/regenerate-image` (transient bytes).**
Dependencies: J2.
Acceptance: Calls `image.Generate` bytes-only path (no Storage
upload); returns `{"imageData":"data:image/png;base64,..."}`
inline. Contract: §8.3.

### Phase K — Final wiring + shopping toggles

**K1. `PATCH /api/kitchen/shopping-items/{id}` (toggle checked).**
Dependencies: D7.
Acceptance: Updates the row; verifies ownership through the
list FK. Contract: §5.4.

**K2. `DELETE /api/kitchen/shopping-lists/{shoppingListId}/
checked-items`.**
Dependencies: K1.
Acceptance: Nested path per ADR-0007; verifies list ownership;
returns `{ "deletedCount": <int> }`. Contract: §5.4.

**K3. Cross-handler integration test suite.**
Dependencies: K2 (last handler done).
Acceptance: A `go test ./...` suite that boots the server with
the in-memory store and fake `AIClient`, mints a JWT via B3,
walks every endpoint, and asserts the contract §5 response shape.
Coverage of every endpoint at least once. Contract: §5 totality.

### Phase L — Deploy gate

**L1. Connect to a real Supabase staging project.**
Dependencies: K3 + data track's schema landed.
Acceptance: The service points at the data track's staging
Supabase project; a smoke test using a real JWT from a Supabase
Auth login round-trips one chat send and exercises one tool call;
the cookbook image actually lands in Supabase Storage and the
URL fetches the bytes back.

**L2. Production task definition + ALB hookup.**
Dependencies: L1.
Acceptance: Service running on a public DNS name behind the ALB
and ACM cert; CloudWatch logs flowing; health checks green; one
chat round-trip from outside the VPC succeeds. Contract: §3.6.

**Task count: 41 tasks across 12 phases.** Phasing is foundation
→ auth → data → reads → AI → SSE foundation → tools → SSE
endpoints → non-streaming AI → cookbook + images → shopping +
final → deploy. The Builder gates phase-to-phase: no phase opens
until the prior phase's acceptance is met.

---

## §6 Integration points

### §6.1 Data track produces, this track consumes

| Artifact | Where this track touches it |
|---|---|
| The eleven tables in contract §4 (`food_items`, `meal_plans`, `meal_plan_days`, `cookbook_recipes`, `shopping_lists`, `shopping_list_items`, `kitchen_conversations`, `kitchen_messages`). | `internal/store` SQL queries reference these by name. The handlers never query through the Supabase REST API — direct `pgx` against the same Postgres instance. |
| RLS policies on every table. | **RLS is the source of truth per ADR-0011** (this section was rewritten on 2026-05-25 after Reviewer-pass 0001 flagged it as stale). The Go service connects as the **`authenticated`** Postgres role (never service-role for DB access) and executes `SET LOCAL request.jwt.claim.sub = '<user_id>'` at the start of every transactional handler. The data track's `(select auth.uid())` policies then enforce per-row filtering inside Postgres. Application-level `WHERE user_id = $1` is **not** the gate — RLS is. A Go bug cannot leak rows because the database refuses the query. See `internal/store.WithClaims` (new task per ADR-0011 amendment). |
| Storage bucket `cookbook-images` with RLS on `storage.objects`. | This track uploads via the Storage REST API with the service-role key (allowed past RLS by design — the per-user-path scoping is enforced in this track's URL construction at `{user_id}/{recipe_id}.png`). PostgREST is disabled at the project level (Conductor decision at the `track_plan` gate), so the iOS-direct-DB path the prior wording referenced does not exist; RLS on the `cookbook-images` Storage bucket remains load-bearing because the service-role key only bypasses RLS for the **storage** REST API path, not for Postgres queries. |
| Storage delete trigger on `cookbook_recipes` row delete. | This track issues plain `DELETE FROM cookbook_recipes`; the trigger handles the object cleanup. |
| `updated_at` triggers. | This track issues plain `UPDATE`; the trigger does the timestamp. |
| `uuidv7()` PK default. | This track does **not** generate UUIDs client-side; it lets the DB do it and reads `RETURNING id`. |

### §6.2 This track produces, iOS track consumes

| Artifact | Wire location |
|---|---|
| Every contract §5 endpoint at the AWS DNS name. | Path + method + JSON-or-SSE shape per contract. |
| The SSE wire format. | Contract §6.1 event types + §6.2 per-endpoint terminal events. |
| The JWT error response shape. | Contract §2.3 / §3.5 envelope. |
| `imageUrl` values pointing at Supabase Storage URLs. | The iOS client GETs the URL directly — this track does not proxy the bytes. |
| The cache-buster query string `?v=<unix>` after a regenerate. | The iOS client treats it as a new URL for cache purposes. |

### §6.3 Cross-track risks this track touches

The Architect flagged five risks in the contract review; three land on
this track.

**Risk A — `clientWeekStartDate` plumbing.** Contract §5.1, §7.2, §7.4
require the chat-send body to carry `clientWeekStartDate` and the
server-side tool dispatcher to override the model's `week_start_date`
on `create_meal_plan` and `update_meal`. **Plan resolution:** Phase H
task H1 wires `clientWeekStartDate` from the request body into the
tool-dispatch context object; Phase G tasks G3/G5 read it from that
context. Tested in K3 with an explicit case where the model emits a
wrong date — the test asserts the dispatched row uses the client's
date. Per ADR-0010.

**Risk B — Cookbook save blocks on image generation (2–10s).**
Contract §5.6 + §8.2 require image generation inline on
`POST /api/kitchen/cookbook` and on the auto-save inside the recipe
SSE. **Plan resolution:** for the explicit `POST /api/kitchen/cookbook`,
the request blocks; the iOS client expects a 5–10s response and
shows a spinner. For the auto-save inside `/generate-recipe`'s SSE,
the spec at §6 does not require streaming the image URL — the SSE
terminates with `image_prompt`, and the iOS client renders the
recipe immediately; the image becomes available on the next
`GET /cookbook/{id}` (which iOS triggers when the user visits the
cookbook). **The SSE does not emit a "generating image…" event —
the image is the cookbook row's concern, not the recipe stream's.**
This matches the contract's terminal-event shape verbatim.

**Risk C — Tool-call schemas must be byte-identical.** Contract §7
documents the five schemas. **Plan resolution:** Phase E task E3 puts
all five schemas in `internal/toolschemas/*.json`, embedded via
`//go:embed`, used both at OpenAI tool registration (production
code) and at every dispatch-handler unit test. A CI check parses
the fenced JSON blocks out of `memory/contract/contract.md` and
diffs against the embedded files; mismatch fails the build.
**Single source of truth: the contract document. The Go bytes are
a CI-verified copy.**

### §6.4 Other cross-track risks (flagged for completeness)

The remaining two Architect-flagged risks (image storage URL freshness
and JWT clock-skew between Supabase and AWS) are noted in §8 *risk
register* below, not §6 — they are internal to this track.

---

## §7 Acceptance criteria

"Backend track done" = the Conductor can run the following at the
deploy gate and every assertion passes:

1. **Every contract §5 endpoint is reachable** on the AWS staging DNS
   name; an HTTP smoke test of all 26 endpoints returns the documented
   shape (2xx) or the documented error code (4xx/5xx).
2. **The SSE chat round-trip works end-to-end.** A real JWT from a
   real Supabase login posts to `/conversations/{id}/messages`; at
   least one tool call dispatches (e.g. `update_ingredients`); the
   `tool_call` and `tool_result` events appear in order; the
   assistant message persists; the `done` event carries
   `assistant_message_id`.
3. **JWT middleware rejects per contract §2.3.** Six negative tests
   (missing/malformed/invalid/expired/wrong-issuer/not-owner) each
   return the documented status and error code.
4. **The `AIClient` interface has two implementations.** Production
   OpenAI client and a fake. The fake is used by every Phase H/I
   handler test; production is exercised in L1/L2.
5. **`gofumpt`, `go vet`, `golangci-lint`, `go test -race`,
   `govulncheck` all green in CI** on the merge commit.
6. **The service is deployed to AWS staging behind a real DNS name.**
   `curl https://<staging-dns>/healthz` returns 200; an authenticated
   round-trip works from outside the VPC.
7. **The tool-schema CI check is green** — the contract document and
   the embedded `toolschemas/*.json` files are in sync (Risk C).
8. **Every `.go` file carries its `dc-01` header.** `dc-00`-conformant
   to a reader with no other context.

---

## §8 Risk register (track-specific)

Risks visible only from inside this track. Each risk has a mitigation
the Builder owns in Phase 4.

**R1 — OpenAI rate limits during meal-plan generation.** A bulk
meal-plan generate fires up to 8 OpenAI calls in quick succession
(seven days + the streaming chat that initiates it). At free-tier
limits (3 RPM for some models), this is rate-limited. **Mitigation:**
the production account is paid-tier; the `aiclient/openai`
implementation parses 429 responses and exposes a typed
`RateLimitError`; the handler emits `ai_provider_error` to the wire
(contract §3.5). **Not v1-blocking** — single-user product, rare path.

**R2 — JWKS rotation.** Supabase rotates JWKS keys periodically.
Stale cache → false 401s. **Mitigation:** `keyfunc` library
auto-refreshes on `kid` miss (B1); cache TTL is 24h; on the rare
race, a single retry inside the middleware succeeds. Smoke test in
L1 includes a rotation simulation.

**R3 — Postgres connection pool saturation.** Default `pgx` pool
size 10 may be too low under SSE concurrency (a 60s chat stream
holds a connection only at message-persist boundaries, not for the
duration — so this is mostly a non-risk, but worth tuning).
**Mitigation:** instrument pool stats via `pgxpool.Pool.Stat()`;
expose as a CloudWatch metric; alert at >80% utilization. Pool size
is config-driven, default 10, can be raised at runtime via env var
without a code change.

**R4 — SSE behind ALB idle timeout.** ALB has a default idle
timeout of 60s; a long quiet stretch in an SSE response (rare but
possible during an OpenAI cold-start) drops the connection.
**Mitigation:** set ALB idle timeout to 600s in the task definition
companion; emit a heartbeat `data: \n\n` (an empty SSE comment line)
every 20s if no real event has been emitted, from the
`internal/sse.Writer`'s `Heartbeat()` goroutine. **Tested in F2.**

**R5 — Cold-start latency on first request after deploy.** ECS
Fargate cold-starts are ~5s for the container plus Go process boot
(~50ms — Go is fast here). **Mitigation:** ALB health checks
prevent traffic until `/healthz` returns 200; a minimum of 1 task
running ensures continuous availability. Acceptable.

**R6 — OpenAI streaming response truncation under cancellation.**
If the iOS client drops the SSE mid-stream, we want the partial
assistant message persisted (contract §6.5). **Mitigation:** the
SSE handler uses a `defer` on the persist-assistant-message call,
keyed off the accumulated `text` buffer; the OpenAI stream is
canceled via `context.WithCancel` chain. Tested in H1.

**R7 — Image generation prompt rejected by OpenAI safety filter.**
A meal name like "death by chocolate" triggers a false positive.
**Mitigation:** the `image.Generate` facade catches the
content-policy error code and emits `ai_provider_error`; the
cookbook row saves with `image_url = null`; the iOS client shows
the placeholder with "tap to retry". **Documented behavior per
contract §8.4.**

**R8 — Storage upload race on regenerate-image.** Two concurrent
regenerate requests for the same cookbook recipe could race on the
Storage object. **Mitigation:** per-`(user_id, cookbook_recipe_id)`
mutex in `internal/image`; second request blocks until first
completes. Acceptable: regenerate is user-initiated and rare.

---

## §9 Open questions

Items the contract does not pin and that this track needs answered
before the Builder writes code. Each is a candidate ADR; the
Conductor decides at the `track_plan` gate whether to add one.

**Q1 — OpenAI model version pinning.** Contract §7 references "OpenAI
chat-completions API, current production tool-calling model" and
ADR-0002 names `gpt-4.1` and `gpt-image-1` as the current choices.
Should the model name be a config value (current plan: yes — env
var `OPENAI_CHAT_MODEL`, default `gpt-4.1`)? Or should we pin a
specific dated snapshot like `gpt-4.1-2024-04-09`? Snapshot pinning
costs reproducibility wins behavior stability across the upstream's
silent improvements. **Recommendation:** pin a snapshot in
production; allow override in staging for testing.

**Q2 — Connection identity for Supabase Postgres.** *Resolved at
the `track_plan` gate as ADR-0011 — JWT-aware connection, RLS is
load-bearing.* The Go service connects as the **`authenticated`**
Postgres role and executes `SET LOCAL request.jwt.claim.sub =
'<user_id>'` at the start of every transactional handler. Data
track's `(select auth.uid())` policies are the gate. A new task,
"Implement `internal/store.WithClaims` helper; test against a
Supabase local stack with an RLS-protected table", lands in the §5
Phase A task tree as A6 (or wherever the Builder slots it).

**Q3 — SSE heartbeat interval and ALB idle timeout.** R4 above
suggests 20s heartbeat + 600s ALB idle. **Question:** confirm the
ALB idle timeout is settable on Dave's AWS staging account (some
accounts have org-level caps), and confirm 20s is short enough to
keep CloudFront / corporate proxies happy. **Recommendation:**
make heartbeat interval a config value (default 20s); document the
ALB setting in the deploy guide.

**Q4 — Meal-plan-day image transient endpoint shape.** Contract §8.3
documents `POST /api/kitchen/regenerate-image { prompt }` returning
`{ imageData: "data:image/png;base64,..." }`. The contract does **not**
say whether this endpoint accepts a `dayId` parameter for ownership
verification. **Question:** is `dayId` optional, required, or
absent? The behavior spec §4.8 shows the source pin accepts an
optional `cookbookRecipeId`. **Default plan:** the endpoint takes
no extra context — any authenticated user can generate an image from
any prompt; rate-limited per user via R1's mitigation. **Open**
because it widens the OpenAI cost exposure; the Conductor should
confirm.

**Q5 — Title auto-generation language model.** Contract §4.6 +
behavior spec §6.4: on first user message into a `"New Chat"`
conversation, set the title to "first ~6 words (max 40 chars,
ellipsized)". The source pin does this with deterministic string
truncation (no LLM call). **Question:** confirm we are NOT using an
LLM for title generation — the cost and latency of an extra call
per first-message are not justified. **Default plan:** deterministic
truncation, no AI call. **Confirm with Conductor.**

**Q6 — Recipe-generation cookbook-first lookup case folding.**
Contract §4.4 documents `cookbook_recipes_user_title_lower_idx`
on `(user_id, lower(title))`. Contract §9.8 specifies
case-insensitive equality. **Question:** is `lower()` enough, or
should we use full Unicode normalization (`NFKC`) too? Meal names
in English are unlikely to need NFKC; international users
might. **Default plan:** `lower()` only for v1 — matches the
documented index; document the limitation in the handler comment.

**Q7 — Hand-coded fallback variety list ownership.** Behavior spec
§4.6 describes a hand-coded variety table for meal-plan generation
when OpenAI returns unparseable output. **Question:** where does
this table live? **Default plan:** in
`internal/api/meal_plans/fallback.go` as a Go literal — it's
backend-only behavior, not part of the contract, and updating it
is a normal code change.

**Q8 — `EXPLAIN ANALYZE` evidence for the most-frequent queries.**
`dc-04` requires "Run `EXPLAIN ANALYZE` on policy-bearing queries
before merge". The Go service's queries aren't policy-bearing
(see Q2), but they are large per-user reads. **Plan:** include an
`EXPLAIN ANALYZE` capture for the five highest-frequency queries
(list conversations, list shopping items, list cookbook, list
inventory, get week with days) in `internal/store/queries.md`.
Confirm in code review.

---

## Cross-references

- Contract: `memory/contract/contract.md` (the spec).
- Behavior spec: `memory/domain/sous-chef-behaviors.md` (the why).
- Workload spec: `.devcore/tasks/sous-chef-port.md`.
- Coding standards: `CODING_STANDARDS.md` §dc-00 (intent), §dc-01
  (precedence), §dc-02 (Go), §dc-04 (Supabase consumer rules), §dc-05
  (AWS deploy), §dc-07 (pre-commit gate).
- ADRs: 0001 (voice cut), 0002 (OpenAI direct), 0003 (Supabase JWT),
  0004 (Supabase Storage), 0005 (no deep links), 0006 (drop legacy),
  0007 (shoppingListId required), 0008 (recipe chat stateless), 0009
  (CFO roles defined), 0010 (client week-start).
