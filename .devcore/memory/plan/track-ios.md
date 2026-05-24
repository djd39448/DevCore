---
type: plan
title: iOS track plan — sous-chef-ios
status: accepted
owner: builder.ios
workload: sous-chef-ios
last_updated: 2026-05-24
contract: contract/contract.md
---

# iOS Track Plan — sous-chef-ios

## §1 What this plan is

The Phase 4 implementation plan for the **iOS track** of the sous-chef-ios
workload. It is the bridge document between:

- the shared contract (`contract/contract.md` — the authoritative wire spec
  the iOS app consumes), and
- the SwiftUI code the iOS Builder will write in Phase 4.

It commits the iOS track to a concrete shape — module layout, view-model
pattern, navigation values, API client design, SSE consumer, image cache,
task ordering, and acceptance criteria — that is independently buildable
against the contract without consulting the backend or data tracks.

**Readers, in order:**

1. The Conductor, at the `track_plan` gate. Confirms the plan is coherent
   with the contract (`contract/contract.md`), the behavior spec
   (`domain/sous-chef-behaviors.md`), and the ADRs (0001–0010), and is
   buildable in parallel with the backend and data tracks.
2. Dave, who approves the gate. Sees the architecture choices, the
   acceptance criteria, and the open questions.
3. The iOS Builder in Phase 4. Uses §3 as the architecture map, §5 as the
   ordered task tree, and §6 as the cross-track interface checklist.

It does **not** contain Swift code. Phase 4 produces the code. It does
**not** redefine the wire — every claim about HTTP, SSE, JSON shape, or
date convention is sourced from the contract and ADRs by citation.

---

## §2 Scope

### What the iOS app delivers

A native iOS application — iPhone only (workload spec §9) — implementing
the nine surfaces of the behavior spec §3, rendered in SwiftUI, consuming
the Go backend at `/api/kitchen/*` (contract §5) via a Supabase JWT
(contract §2, ADR-0003).

**Surfaces delivered** (behavior spec §3 mapping):

- §3.1 Landing / auth entry — Sign in with Apple primary, Supabase
  email + OTP fallback (ADR-0003). One SwiftUI screen with two buttons;
  the SDK handles the rest.
- §3.2 Chat — multi-conversation chat against the kitchen sous-chef.
  SSE consumption per contract §6. Suggestion chips for first-message UX.
  Sidebar conversation list grouped by recency. Auto-fresh chat on
  session start (contract §9.7).
- §3.3 Meal-plan week — sticky week navigator, seven `MealPlanCard`s in
  Mon→Sun order (UI ordering only; storage stays 0=Sunday per contract
  §3.3). Edit-mode toggle with per-day checkboxes; selective regeneration
  of unchecked days. "Add to shopping list" affordance.
- §3.4 Recipe detail — auto-fetch cached recipe markdown or stream-
  generate via SSE on `POST /meal-plan-days/{dayId}/generate-recipe`.
  Three-state image area (placeholder → spinner → image), tap-to-generate
  per ADR-0004. Stateless follow-up chat via `/recipe-message` (contract
  §5.3, ADR-0008). Save-to-cookbook affordance.
- §3.5 Cookbook index — list view, client-side filter on title, delete
  per recipe, navigate into a single recipe.
- §3.6 Cookbook recipe detail — view + edit modes, ingredient-helper
  widget (per behavior spec §3.6) with CFO suggestion chips from
  `/ingredient-suggestions`. Explicit regenerate-image action (ADR-0004).
- §3.7 Calendar — week view (read-only mirror of plan tab) and month
  view (calendar grid with dots for weeks that have plans/lists). Drill
  into a week or a shopping list.
- §3.8 Shopping — index of lists newest-first, drill into a list's
  items grouped by category in fixed display order (produce, meat,
  dairy, bakery, frozen, pantry, beverages, other; behavior spec §3.8).
  Per-item check toggle (`PATCH /shopping-items/{id}`). "Clear Done"
  acts on the **viewed list** id per ADR-0007.
- §3.9 Not-found — a static screen for navigation dead-ends.
- Cookbook image LRU cache (ADR-0004) — on-device, 128 MB cap, LRU by
  access time, keyed by `cookbook_recipes.id`. Honors the cache-buster
  on `?v=<unix>` regenerations.
- State restoration — last-viewed tab and last `NavigationPath` per tab,
  via `@SceneStorage`.
- Accessibility — meaningful labels on every interactive element,
  Dynamic Type, Reduce Motion respected (dc-03).

### What the iOS app does NOT deliver

- **Any backend logic.** The iOS app never talks to OpenAI, never
  computes a meal plan, never writes to Postgres. It consumes
  contract §5 endpoints; the backend owns business logic.
- **Any database write.** RLS lives on Supabase; the Go backend is the
  only writer to Postgres. The iOS app does not use the Supabase
  client SDK for data — only for auth (contract §2, ADR-0003).
- **The four AI tool calls themselves.** The iOS app **renders** the
  streamed `tool_result` events from the SSE wire (contract §6.1)
  but never dispatches a tool call. Tool dispatch is server-side
  (contract §7).
- **Image generation.** The iOS app requests generation via
  `POST /api/kitchen/regenerate-image` (contract §8.3 — the one
  remaining data-URL path) for meal-plan-day images, and reads
  Supabase Storage URLs from `cookbook_recipes.image_url` for
  cookbook images (ADR-0004). It never calls OpenAI.
- **Voice features.** Cut per ADR-0001.
- **iPad layouts.** iPhone only (workload spec §9).
- **Universal Links / deep linking.** Out of scope per ADR-0005.
- **Push notifications, widgets, App Clips, shortcuts.** Not in v1.
- **User data migration from the web app.** Workload spec §7 — the
  Replit-issued user IDs are not migrated; new SIWA users start fresh.

---

## §3 Architecture

### §3.1 App shape

A single Xcode project at `~/sous-chef-ios/` with the bundle id
`com.dobbins.souschef` (or as configured at Apple Developer enrollment).
One iOS app target plus one Swift Package (in-repo, local) hosting the
domain code that the app target consumes.

The package split exists to enforce dc-03's "model and service types
**never** `import SwiftUI`" rule at the compiler level — the package
target does not link SwiftUI, so a stray import becomes a build error.

```
sous-chef-ios/
├── SousChefApp/                  # the iOS app target (SwiftUI views, app entry)
│   ├── SousChefApp.swift         # @main
│   ├── Features/
│   │   ├── Auth/                 # SignInView + the SIWA/OTP plumbing
│   │   ├── Chat/                 # ChatView, ConversationSidebar, MessageBubble
│   │   ├── Plan/                 # PlanView, MealPlanCard, EditModeBar
│   │   ├── Recipe/               # RecipeView, RecipeMarkdownView, RecipeChat
│   │   ├── Cookbook/             # CookbookListView, CookbookRecipeView, IngredientHelper
│   │   ├── Calendar/             # CalendarView (week/month), MonthGrid
│   │   ├── Shopping/             # ShoppingListView, ShoppingItemsView
│   │   └── Common/               # Tab shell, NotFoundView, shared modifiers
│   └── Resources/                # Assets.xcassets, Info.plist, entitlements
└── Packages/
    └── SousChefKit/              # local Swift Package
        ├── Package.swift
        └── Sources/
            ├── SousChefDomain/   # CFO, MealPlan, CookbookRecipe, Shopping, Conversation models
            ├── SousChefAPI/      # APIClient actor, codable DTOs, error types, SSE consumer
            ├── SousChefAuth/     # SupabaseAuth wrapper, JWT provider
            ├── SousChefMarkdown/ # canonical recipe markdown parser/renderer (see §3.7)
            └── SousChefImageCache/ # NSCache + disk LRU
```

`SousChefKit` is a single product exposing five library targets so the
app code imports only what it needs and so each library has its own
unit-test target (Swift Testing per dc-03).

### §3.2 Tab + navigation shell

A `TabView` with five tabs matching behavior spec §3:

| Tab | Default surface | NavigationStack root |
|---|---|---|
| Chat | `ChatView` | conversation list / messages |
| Plan | `PlanView(week:)` | `PlanRoute.week(Date)` per ADR-0005 |
| Cookbook | `CookbookListView` | `CookbookRoute.recipe(UUID)` |
| Calendar | `CalendarView(.month)` | `CalendarRoute.month(Date)` |
| Shopping | `ShoppingListView` | `ShoppingRoute.list(UUID) \| .week(Date)` |

Each tab owns its own `NavigationStack(path: ...)` with a typed
`NavigationPath`. The route enums are `Hashable` and `Codable` — the
latter is required for `@SceneStorage` to serialize the path across
relaunches (per ADR-0005). The recipe route lives **inside** the Plan
and Calendar stacks rather than a top-level tab — the user reaches a
recipe by tapping a day card, and the back stack matches that mental
model.

### §3.3 View-model pattern — Model–View per dc-03

dc-03 is explicit: do not introduce `…ViewModel` classes. Each screen
that owns state uses the `@Observable` macro on a domain-named state
object (e.g. `PlanWeekModel`, `ChatSessionModel`, `RecipeDetailModel`).
The view holds it as `@State` if it creates it, or as a plain `let`
property if a parent passes it down. Shared models (the API client,
the auth model, the image cache) are injected via `.environment(_:)`
and read with `@Environment(SousChefAPI.self)` etc.

Concretely, the state objects this track produces:

| Model | Owns | Lifecycle |
|---|---|---|
| `AuthModel` | the current Supabase session + JWT | app-scope, `@Environment` |
| `APIClient` | one `URLSession` + the JWT provider closure | app-scope, `@Environment` |
| `ImageCache` | NSCache + disk LRU per ADR-0004 | app-scope, `@Environment` |
| `ConversationListModel` | sidebar list (`GET /conversations`) | Chat tab root |
| `ChatSessionModel` | active conversation, message stream, SSE task | per-conversation view |
| `PlanWeekModel` | one week's plan + edit-mode state machine (§6.1) | per-week view |
| `RecipeDetailModel` | one meal-plan-day's recipe + image + in-page chat | per-day view |
| `CookbookListModel` | the full cookbook list | Cookbook tab root |
| `CookbookRecipeModel` | one cookbook recipe + edit state | per-recipe view |
| `CalendarModel` | calendar drill-down summaries | Calendar tab root |
| `ShoppingListsModel` | the index of lists | Shopping tab root |
| `ShoppingListModel` | one list with its items | per-list view |

dc-03 also bans `ObservableObject`/`@Published`/`@StateObject`/
`@EnvironmentObject` in new code — none appear in this plan.

### §3.4 The API client — one actor wrapping URLSession

`SousChefAPI.APIClient` is an `actor` (Swift 6 strict concurrency, dc-03)
that wraps a single `URLSession.shared`-style session configured with the
JWT provider. Its surface mirrors contract §5 one-method-per-endpoint:

- `func listConversations() async throws -> [Conversation]`
- `func createConversation(title: String?) async throws -> Conversation`
- `func fetchConversation(id: UUID) async throws -> ConversationWithMessages`
- `func sendMessage(conversationId: UUID, content: String,
   clientWeekStartDate: Date) -> AsyncThrowingStream<ChatEvent, Error>`
- `func fetchWeek(date: Date) async throws -> WeekResponse`
- `func fetchCalendar() async throws -> CalendarResponse`
- `func generateMealPlan(weekStartDate: Date) async throws -> MealPlanWithDays`
- `func regenerateDays(weekStartDate: Date, days: [Int]) async throws -> MealPlanWithDays`
- `func fetchMealPlanDay(id: UUID) async throws -> MealPlanDay`
- `func generateRecipe(dayId: UUID) -> AsyncThrowingStream<RecipeStreamEvent, Error>`
- `func sendRecipeMessage(dayId: UUID, content: String, mealName: String,
   dayName: String, currentRecipe: String?) -> AsyncThrowingStream<RecipeChatEvent, Error>`
- `func listShoppingLists() async throws -> [ShoppingListSummary]`
- `func fetchShoppingList(identifier: ShoppingListIdentifier) async throws -> ShoppingListWithItems`
- `func generateShoppingList(weekStartDate: Date?) async throws -> ShoppingListWithItems`
- `func toggleShoppingItem(id: UUID, checked: Bool) async throws -> ShoppingListItem`
- `func clearCheckedItems(shoppingListId: UUID) async throws -> Int`
- `func fetchIngredients() async throws -> [FoodItem]`
- `func fetchIngredientSuggestions() async throws -> [IngredientSuggestion]`
- `func listCookbook() async throws -> [CookbookRecipeSummary]`
- `func fetchCookbookRecipe(id: UUID) async throws -> CookbookRecipe`
- `func saveCookbookRecipe(title: String, content: String,
   imagePrompt: String?) async throws -> CookbookRecipe`
- `func updateCookbookRecipe(id: UUID, title: String?, content: String?,
   imagePrompt: String?) async throws -> CookbookRecipe`
- `func deleteCookbookRecipe(id: UUID) async throws`
- `func regenerateCookbookImage(id: UUID) async throws -> URL`
- `func generateRecipeImage(prompt: String) async throws -> Data` (the
  one remaining data-URL path per contract §8.3; the client decodes
  the base64 server-side return into raw PNG `Data`).

Each method:

1. Builds the `URLRequest` with `Authorization: Bearer <jwt>` from the
   injected JWT provider closure (the closure asks `AuthModel` for the
   current token, refreshing if expired).
2. For JSON endpoints: encodes the request body via `JSONEncoder`
   configured with `keyEncodingStrategy = .convertToSnakeCase` only
   for request bodies that use snake_case (currently the chat-send
   `clientWeekStartDate` field — see contract §5.1 — and the tool-call
   argument shapes the server-side dispatcher receives). Response
   bodies use camelCase per contract §3.1; `JSONDecoder` is configured
   accordingly.
3. Calls `URLSession.data(for:)`, validates the `HTTPURLResponse`
   status, branches on the contract's normative error codes
   (`missing_authorization`, `expired_token`, `not_found`, etc.) and
   throws a typed `APIError` (Swift 6 typed throws) so callers get
   exhaustive `catch`.
4. For SSE endpoints: see §3.5.

A separate `Codable`-conforming DTO layer (`SousChefAPI.DTO.*`)
matches the wire shape verbatim; a hand-written `init(domain:)`
mapping converts to the `SousChefDomain` types the rest of the app
uses. This is the dc-03 "keep wire DTOs separate from domain models"
rule.

### §3.5 The SSE consumer

Three contract endpoints stream SSE (§6): chat, recipe generation,
recipe-page chat. All share the contract §6.1 event shape; per-endpoint
terminal events differ (§6.2). The iOS client consumes via
`URLSession.bytes(for:)` returning an `AsyncBytes` sequence.

The consumer is `SousChefAPI.SSEStream<Event>`:

- Wraps `URLSession.bytes(for:)`.
- Maintains a `String` buffer that accumulates incoming UTF-8 chunks
  across network reads — this implements the buffer rule of contract
  §6.4 ("split on `\n`, treat the trailing non-terminated line as a
  pending buffer for the next chunk"). This bug fix from the web
  client is **explicitly** ported.
- For each terminated line: if it starts with `data: `, parse the
  remainder as JSON, decode into the per-endpoint `Event` enum, yield
  to the `AsyncThrowingStream` continuation.
- On `{ type: "done" }`: complete the continuation with the
  endpoint's terminal payload.
- On `{ type: "error" }`: complete the continuation with `.failure`.
- On `Task` cancellation: cancel the underlying URLSession data task;
  the stream completes. The server detects cancellation per contract
  §6.5 and persists whatever content was emitted.

Each per-endpoint `Event` enum has its own `tool_result` decoding —
the rendered UI shows tool effects (e.g. "Updated 3 ingredients" when
`update_ingredients` fires). Per contract §6.1, the iOS client
**renders** `tool_result` events but does not act on them — the act
already happened server-side; the event is informational.

### §3.6 The image cache

`SousChefImageCache.ImageCache` per ADR-0004:

- An `actor` (Swift 6 strict concurrency).
- Backed by an `NSCache<NSString, NSData>` for in-memory access and
  a disk LRU in `~/Library/Caches/SousChef/Images/` keyed by
  `cookbook_recipes.id`.
- Hard cap **128 MB on disk** (ADR-0004 — this number is iOS-track
  design, not contract; the contract guarantees only "URL works on
  each call and bytes are stable until regeneration").
- Eviction policy: LRU by access time. On every read, touch the file's
  `contentAccessDate`; on insert when total > cap, delete oldest until
  under cap.
- Honors the cache-buster: the disk key is `{recipeId}_v{unixOrNone}`.
  When a regenerate returns a new `?v=<unix>` URL, the old key is
  evicted explicitly.
- Cooperates with iOS low-memory warnings: register for
  `UIApplication.didReceiveMemoryWarningNotification` and clear the
  NSCache (the disk cache is unaffected — it survives memory pressure).
- Falls back to network fetch on cache miss; the fetched bytes are
  written through to both NSCache and disk.

Meal-plan-day images (contract §8.3) are cached **transiently in
NSCache only**, keyed by `mealPlanDay.id`, with no disk write — they
are server-side transient too, and persisting them would diverge from
the server's "image is not stored on the day row" semantic.

### §3.7 Canonical recipe markdown renderer

Behavior spec §4.4 specifies a fixed markdown subset:

```
# {Meal Name}
[1-2 sentence description]

**Prep Time:** X minutes | **Cook Time:** X minutes | **Serves:** X

## Ingredients
- [quantity] [ingredient]
- ...

## Instructions
1. [Step]
2. ...

## Tips (optional)
- [Optional tip]
```

The chosen renderer: **a hand-written parser-and-SwiftUI-View pair in
`SousChefMarkdown`** that handles exactly this subset — `#`, `##`,
`###`, `**bold**`, `- bullets`, `1. ordered`, plain paragraphs.

This choice is deliberate, not lazy:

- The source web client also rolls its own subset (behavior spec §4.4)
  — porting the renderer is a small mechanical step against a known
  shape.
- SwiftUI's built-in `AttributedString(markdown:)` supports inline
  markdown only (no block-level `#`/`-`/`1.`). It would not render
  the canonical format.
- Third-party libraries (`MarkdownUI`, `swift-markdown-ui`) are
  full CommonMark renderers — overkill for a known fixed subset,
  add ~5–10k LOC of dependency, and bring SwiftUI-coupling that
  fights the `SousChefKit` package boundary.
- A hand-rolled parser keeps the renderer in `SousChefMarkdown` (no
  SwiftUI import) and a `RecipeMarkdownView` in the app target that
  consumes the parser's AST and emits SwiftUI views.

The parser produces a `[RecipeBlock]` AST (Swift enum:
`.heading(level: Int, text: String)`, `.paragraph(InlineSpans)`,
`.unorderedList([InlineSpans])`, `.orderedList([InlineSpans])`,
where `InlineSpans` is `[InlineSpan]` of `.text(String)` and
`.bold(String)`). The view walks the AST.

### §3.8 Concurrency model

Per dc-03 Swift 6.2 "approachable concurrency":

- The app target defaults to `@MainActor` isolation. Every SwiftUI
  view body and every state-object method that touches view state runs
  on the main actor.
- `APIClient` and `ImageCache` are `actor`s — off-main by definition.
  Methods that return data hop back to the main actor at the call site
  via `await`.
- SSE consumers run as `Task` children of a view's `.task { }` so the
  task auto-cancels on disappear (matches contract §6.5's cancellation
  protocol).
- Every DTO and every domain model is `Sendable`. No `@unchecked
  Sendable`, no `nonisolated(unsafe)`. Where a closure crosses an
  actor boundary, it is `@Sendable`.
- The build is warning-free under `-strict-concurrency=complete`.
  Warnings-as-errors is on (dc-03).

### §3.9 State restoration

Per ADR-0005: one `@SceneStorage` line per tab serializes its
`NavigationPath`. The shell `App` also `@SceneStorage`s the last-active
tab index. On relaunch the user sees the screen they left.

The chat tab's restoration is bounded: it restores the active
conversation id, but always **creates a fresh conversation** on first
foreground after a configurable inactivity window (contract §9.7).

---

## §4 Stack & toolchain

| Item | Choice | Reason |
|---|---|---|
| Language | Swift 6.2 | dc-03 baseline |
| Swift mode | strict concurrency, complete data-race checking | dc-03 + Swift 6 |
| Toolchain | Xcode 16.x (latest stable) | dc-03 — Xcode 16+ |
| Deployment target | iOS 17.0 | dc-03 — required for the Observation framework / `@Observable` macro |
| Dependency manager | Swift Package Manager only | dc-03 |
| External packages | `supabase-swift` (auth + storage URLs only); no UI library | minimal — every dep is supply chain |
| Test framework | Swift Testing (`@Test`, `#expect`, `#require`, `@Suite`) | dc-03 — XCTest only for UI automation |
| Formatter | SwiftFormat | consume DevCore's existing `.swiftformat` |
| Linter | SwiftLint | consume DevCore's existing `.swiftlint.yml` |
| Pre-commit | SwiftFormat + SwiftLint, fail on diff | dc-03 + dc-07 |
| CI | Xcode Cloud — deferred to deploy phase | dc-05 says CI is required; choice of host is a Phase 5 deploy concern |
| Bundle id | `com.dobbins.souschef` | provisional; confirmed at Apple Developer enrollment |
| Capabilities | Sign in with Apple | ADR-0003 |

### iOS 17 vs iOS 18 deployment target

iOS 17.0 is chosen, not iOS 18. Rationale:

- dc-03 baseline is **iOS 17+** explicitly.
- The Observation framework (`@Observable`) — the core of the
  view-model pattern (§3.3) — landed in iOS 17.
- `URLSession.bytes(for:)` for SSE (§3.5) ships in iOS 15.
- `NavigationStack` + typed `NavigationPath` (§3.2) ship in iOS 16.
- The Supabase iOS SDK supports iOS 13+; not a constraint.
- Sign in with Apple ships in iOS 13+; not a constraint.
- The hand-rolled markdown renderer (§3.7) needs only SwiftUI basics.

iOS 18 would add no capability this app uses. iOS 17 maximizes the
installed-base reach.

### SIWA setup steps (Phase 4 prerequisite)

The iOS Builder needs Dave to perform these in the Apple Developer
portal before SIWA can be exercised:

1. Enroll the bundle id (`com.dobbins.souschef`) and enable
   "Sign in with Apple" capability.
2. Create a Service ID for the Supabase callback.
3. Create a Key (.p8) for the Service ID and download it. The .p8
   never leaves Dave's machine — it goes into Supabase project
   settings (Auth → Providers → Apple), not the iOS bundle.
4. Configure Supabase Auth's Apple provider with the Service ID, Team
   ID, Key ID, and the .p8 contents.

These steps are listed in §9 as an open dependency.

---

## §5 Task tree

Twenty-three tasks across six phases. Each task lists short title,
dependencies, and the acceptance check (contract §X.Y or behavior spec
§3.x it satisfies).

### Phase A — Foundation (5 tasks)

| # | Task | Depends on | Acceptance check |
|---|---|---|---|
| A1 | Create Xcode project + `SousChefKit` package | — | Builds clean. dc-03 file headers in place. SwiftFormat + SwiftLint configs committed. |
| A2 | Add `supabase-swift` dependency; wire AuthModel | A1 | App compiles; AuthModel exposes `currentSession: Session?` and `signIn` / `signOut` async methods. |
| A3 | Implement SIWA flow (`SignInWithAppleButton` → Supabase `signInWithIdToken`) | A2, **Dave's Apple Developer steps (§4)** | Tapping SIWA returns a Supabase session; `auth.users.id` is the JWT `sub`. Contract §2.1. |
| A4 | Implement email/OTP flow (Supabase `signInWithOTP` → verify) | A2 | Six-digit code lands user with same Supabase session shape. ADR-0003. |
| A5 | Implement landing screen + session restoration | A3, A4 | App launches, shows landing if no session; restores session silently if present. Behavior spec §3.1. |

### Phase B — API client + codable models (3 tasks)

Phase B can start in parallel with Phase A4–A5 once A2 is in.

| # | Task | Depends on | Acceptance check |
|---|---|---|---|
| B1 | Define all DTOs + domain models (Conversation, MealPlan, CookbookRecipe, Shopping, FoodItem, etc.) per contract §4 and §5 response shapes | A1 | Every contract §5 response decodes from canned JSON fixtures (Swift Testing parameterized). |
| B2 | Implement `APIClient` actor with all REST methods (excluding SSE) | A2, B1, **backend track: auth endpoint reachable** | Each method, mocked against URL-stubbed responses, returns the expected domain object. Error codes from contract §3.5 map to typed `APIError`. |
| B3 | Implement `SSEStream` consumer with the line-buffer rule from contract §6.4 | B2 | Given canned SSE byte streams (split across arbitrary chunk boundaries), yields the correct event sequence. Includes a parameterized test for split-mid-line. Contract §6.4. |

### Phase C — Chat (3 tasks)

| # | Task | Depends on | Acceptance check |
|---|---|---|---|
| C1 | Implement `ConversationListModel` + sidebar UI grouped by recency | B2 | Sidebar shows conversations in Today/Yesterday/This Week/This Month/Older buckets. Behavior spec §3.2. |
| C2 | Implement `ChatSessionModel` + `ChatView` with optimistic user message, SSE-streamed assistant message, suggestion chips | B2, B3 | Sending a message streams text into the UI; `tool_result` events render an inline confirmation chip; final message id from `done` matches the persisted server message. Contract §5.1, §6.1, §6.2. Behavior spec §3.2. |
| C3 | Implement fresh-chat-on-session-start (`POST /conversations` on first foreground after inactivity window) | C2 | Behavior spec §3.2 / contract §9.7. State machine: `idle → streaming → idle`. |

### Phase D — Plan + Recipe (4 tasks)

| # | Task | Depends on | Acceptance check |
|---|---|---|---|
| D1 | Compute Monday-of-week in user's local timezone (`Calendar.current` with `firstWeekday = 2`); a `WeekDate` value type that wraps `Date` and serializes to `YYYY-MM-DD` | B1 | Unit-tested across DST transitions, all US timezones, and a few non-US (London, Sydney). ADR-0010. |
| D2 | Implement `PlanWeekModel` + `PlanView` read-only (week navigator, seven `MealPlanCard`s Mon→Sun) | B2, D1 | Navigating to a week renders the seven days in the correct UI order despite storage being 0=Sunday. Empty-state for missing plans with a "Create Meal Plan" button. Behavior spec §3.3 / contract §5.2 / contract §3.3. |
| D3 | Implement edit-mode + selective day regeneration (state machine §6.1: view ↔ edit → regenerating ↔ edit → view) | D2 | Per-day checkboxes, "Regenerate N Unchecked Days" calls `POST /meal-plans/{week}/regenerate-days`. Kept days are visually preserved during the regen. "Finalize Plan" is a UX-only no-op. Behavior spec §3.3, §6.1. |
| D4 | Implement `RecipeDetailModel` + `RecipeView`: SSE generate-recipe, three-state image area, tap-to-generate, save-to-cookbook, stateless `/recipe-message` follow-up chat | B2, B3, §3.7 markdown renderer | Tapping a day shows the cached recipe if present, else streams a fresh one. Image placeholder → spinner → image on tap. Save button persists via `POST /cookbook`. Recipe-chat swap (`update_meal` tool result) resets the recipe and re-streams. Behavior spec §3.4 / §6.2 / contract §5.2 / §5.3 / §7.5 / ADR-0008. |

### Phase E — Cookbook + Calendar + Shopping (5 tasks)

| # | Task | Depends on | Acceptance check |
|---|---|---|---|
| E1 | Implement `ImageCache` actor (NSCache + disk LRU, 128 MB) per §3.6 | A1 | Round-trips bytes; evicts on cap; honors `?v=<unix>` cache-buster; clears NSCache on memory warning. ADR-0004. |
| E2 | Implement `CookbookListModel` + `CookbookListView` with thumbnails (cache-aware) | B2, E1 | List renders newest-first with cached thumbnails. Client-side title/content filter. Delete swipe action. Behavior spec §3.5 / contract §5.6. |
| E3 | Implement `CookbookRecipeModel` + `CookbookRecipeView` with edit mode + ingredient helper widget + regenerate-image affordance | B2, E1, B3 not needed | Edit mode swaps `Markdown` for `Textarea`. Ingredient-helper inserts `- {qty} {unit} {name}` under `## Ingredients`. Quick chips come from `GET /ingredient-suggestions`. Regenerate updates `imageUrl` via cache-buster. Behavior spec §3.6 / contract §5.5 / §5.6. |
| E4 | Implement `CalendarView` with week and month subviews, drilling into PlanRoute and ShoppingRoute | B2, D2 | Month grid dots map to `GET /calendar` summaries. Week view mirrors PlanView but read-only. Behavior spec §3.7. |
| E5 | Implement `ShoppingListsModel` + `ShoppingListModel` + ShoppingView with category grouping (fixed display order), check toggle, Clear Done (per-list per ADR-0007) | B2 | Items grouped: produce, meat, dairy, bakery, frozen, pantry, beverages, other. PATCH on toggle. DELETE acts on the **viewed list id**, not most-recent. "All done!" celebration when every item is checked. Behavior spec §3.8 / contract §5.4 / ADR-0007. |

### Phase F — Polish + acceptance (3 tasks)

| # | Task | Depends on | Acceptance check |
|---|---|---|---|
| F1 | Implement `@SceneStorage` state restoration on every tab + last-active tab index | C2, D2, E2, E4, E5 | Cold-relaunch returns the user to the screen they left. ADR-0005. |
| F2 | Accessibility pass — labels on every interactive element, Dynamic Type, Reduce Motion respected, contrast minimums (dc-03) | all prior | VoiceOver walkthrough of every screen reads sensibly; Dynamic Type at largest setting does not clip critical UI; "Reduce Motion" disables non-essential animations. |
| F3 | Build passes Swift 6 strict concurrency, SwiftFormat, SwiftLint, all unit tests; archive to TestFlight | all prior | Acceptance criteria §7 satisfied; build uploaded to TestFlight; Dave installs on his iPhone via TestFlight invite. |

### Ordering & parallelism notes

- A1–A5 must complete before B can finish (B2 needs `AuthModel` for
  the JWT provider).
- B1 can begin immediately in parallel with A2; B2 and B3 need A2.
- C, D, E can proceed in parallel after B3 lands.
- F runs last.
- D1 (the Monday-of-week computation) is small but blocking — it
  belongs early in the D phase because every D and E task that hits
  a week-bearing endpoint depends on it.

---

## §6 Integration points

### §6.1 Data track produces → this track consumes (indirectly)

The data track stands up:

- Supabase Auth (the provider) — configured for SIWA + email/OTP per
  ADR-0003.
- Supabase Storage bucket `cookbook-images` with RLS scoped per user
  (contract §4.4).
- The `auth.users` table and every per-user table referenced by the
  contract §4 schema.

The iOS track consumes:

- **The Supabase Auth provider, via the `supabase-swift` SDK.** The
  iOS app calls `auth.signInWithIdToken` (SIWA) and
  `auth.signInWithOTP` (email/OTP) and receives a `Session` with a
  JWT. The JWT is attached to every backend call per contract §2.2.
- **Supabase Storage URLs from `cookbook_recipes.image_url`.** The
  iOS app never lists or writes storage objects — it dereferences the
  URLs the Go backend returns in `GET /cookbook/{id}` responses.

The iOS app does **not** use Supabase for data queries (no PostgREST
client, no realtime). All data flows through the Go backend.

### §6.2 Backend track produces → this track consumes

Every endpoint in contract §5. The iOS app's `APIClient` has one
method per endpoint (§3.4). The SSE wire format from contract §6 is
consumed by `SSEStream` (§3.5). JWT 401 responses (contract §2.3)
trigger an explicit `AuthModel.refreshSession()`; if refresh fails,
the user is bounced to the landing screen.

The iOS track has **one hard dependency on the backend track for
Phase 4 to start**: the auth endpoint must be reachable. Specifically:

- `GET /healthz` (contract §3.6) responding `200 { status: "ok" }`.
- At least one authenticated endpoint (e.g.
  `GET /api/kitchen/conversations`) responding either `200 [...]`
  or `401 { error: "missing_authorization" }` — enough to verify
  the JWT-bearer flow round-trips.

Without those, Phase B2 cannot be acceptance-checked end-to-end.
Mocked URL stubs let B1–B3 proceed in isolation, but C onward needs
a real backend reachable in dev.

### §6.3 Cross-track risks this track owns

The Architect flagged three risks that land on the iOS track. Each
is owned here with a concrete plan.

#### `clientWeekStartDate` plumbing (ADR-0010)

**Risk:** the iOS app must compute Monday-of-current-week in the
user's local timezone (`Calendar.current` with `firstWeekday = 2`)
and attach it on:

- `POST /api/kitchen/conversations/{id}/messages` — as
  `clientWeekStartDate` (contract §5.1) so server-side
  `create_meal_plan`, `create_shopping_list`, and `update_meal` tool
  dispatch knows the user's "this week".
- Every week-bearing path/body parameter — `weekStartDate` on the
  week, meal-plan, regenerate-days, and shopping-list endpoints
  (contract §5.2, §5.4).

**Plan:**

- A `WeekDate` value type in `SousChefDomain` wraps a `Date` and
  enforces "is a Monday" at construction. It serializes to
  `YYYY-MM-DD` via a dedicated `DateFormatter` with `timeZone =
  TimeZone.current` and `locale = .init(identifier: "en_US_POSIX")`
  — locale-safe ISO formatting.
- A single helper `WeekDate.currentInUserLocalZone()` returns the
  Monday of "today" per the user's calendar. It uses
  `Calendar.current` with `firstWeekday = 2` (forces Monday-first
  even in en_US, per ADR-0010).
- Every API call that takes a week threads this `WeekDate` —
  there is exactly one computation site per user action; no
  scattered `Calendar` math.
- Unit tests cover DST forward/back transitions, end-of-year
  rollover, and multi-zone correctness (Pacific, Eastern, London,
  Sydney).

#### Cookbook save UX during image generation

**Risk:** `POST /api/kitchen/cookbook` may take 2–10 seconds because
the backend generates the image inline before returning (contract
§5.6 / ADR-0004). The UI must not feel broken; the user must not
double-submit; an image-generation failure must not nuke the recipe.

**Plan:**

- The Save action enters a `saving` state in `RecipeDetailModel` or
  `CookbookRecipeModel` (depending on the entry point). The save
  button shows a `ProgressView` spinner with the label "Saving recipe
  and generating cover image…". The button is `disabled` while
  `saving`.
- A timeout of 30 seconds on the URL request. If exceeded, the model
  enters `saveFailed(.timeout)` and shows a retry affordance.
- Per contract §8.4: a 503 from `POST /cookbook` (Supabase Storage
  upload failure) means the recipe row **was persisted** but
  `image_url` stayed null. The model treats this case specifically:
  shows a success toast for the save, but a "Image generation failed
  — tap to regenerate" affordance on the recipe afterward.
- A 502 (`ai_provider_error`) is the same: recipe saved, image
  failed; retry via `POST /cookbook/{id}/regenerate-image`.
- No double-submit guard at the network layer — the model's
  `disabled` button is the guard.

#### Recipe markdown rendering

**Risk:** the canonical recipe format (behavior spec §4.4) needs to
render correctly. Choosing the wrong renderer means either
under-rendering (no `#` headings) or over-rendering (random Markdown
features the AI didn't intend).

**Plan:** the hand-written parser in `SousChefMarkdown` (§3.7). The
parser handles exactly the spec subset; anything outside it (e.g.
inline links, images, tables) is rendered as plain text. The
renderer is tested against canned recipes from the source web
client's actual generations (a `testdata/` directory of ~20 recipes
extracted from `~/sous-chef-ai`).

---

## §7 Acceptance criteria

The iOS track is "done" when **every** item below is true. The
Conductor checks this list at the `track_plan` gate (for the plan
itself) and at the end of Phase 4 (for the implementation).

1. The Xcode project builds clean under Swift 6 strict concurrency,
   warnings-as-errors, with SwiftFormat and SwiftLint as
   pre-commit hooks (zero diffs, zero violations).
2. App launches on an iPhone simulator and on Dave's iPhone via
   TestFlight; the launch screen renders within 1 second of cold
   start.
3. SIWA flow signs the user in and lands on the Chat tab; the
   resulting Supabase session is restored silently on relaunch.
4. Email/OTP fallback flow also signs the user in and lands on the
   Chat tab.
5. Sending one chat message receives a streamed reply with at least
   one rendered `text` chunk and (if the message triggers a tool)
   one rendered `tool_result` event; the conversation persists on
   the server (verified by reload).
6. The Plan tab renders an existing week's plan with seven cards in
   Mon→Sun UI order. The "Create Meal Plan" path generates a
   week. Edit-mode + selective regeneration round-trips correctly.
7. Tapping a day card navigates to the recipe detail; cached
   `recipe_content` renders instantly, missing content streams via
   SSE in real-time and renders progressively. The image placeholder
   transitions to spinner to image on tap.
8. Save-to-cookbook persists the recipe and returns a recipe with
   a populated `image_url`. The cookbook list then shows the new
   entry with its thumbnail.
9. Cookbook browse + save + edit + delete + regenerate-image all
   work end-to-end. The 128 MB disk LRU cache evicts as documented.
10. Calendar month view dots match weeks that have a plan; tapping a
    week drills into it. Shopping `view list` affordance navigates
    correctly.
11. Shopping list detail shows items grouped by category in the
    fixed order; toggling an item PATCHes; "Clear Done" deletes
    only the **viewed** list's checked items (ADR-0007).
12. Stateless recipe-page chat round-trips; an `update_meal` swap
    resets the recipe state and re-streams.
13. State restoration: relaunching from a deep navigation (e.g. inside
    a cookbook recipe) returns the user there.
14. VoiceOver walkthrough of every screen reads sensibly with no
    "unlabeled button" warnings.
15. Dynamic Type at the largest setting does not clip critical UI on
    iPhone 14 Pro and iPhone SE (smallest supported screen).
16. The build is uploaded to TestFlight and successfully installed on
    Dave's device.

A failure on any item is a blocker to closing the track. A partial
pass is reported back to the Conductor for triage.

---

## §8 Risk register (track-specific)

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| R1 | The Supabase iOS SDK has gaps in Swift 6 strict-concurrency annotations; the build fails with `Sendable` warnings. | Medium | Medium | If hit: pin to the latest tagged SDK version that compiles clean; if none, wrap SDK types in `@unchecked Sendable` adapters local to `SousChefAuth` with an inline comment proving the manual guarantee per dc-03. Report upstream. |
| R2 | `URLSession.bytes(for:)` SSE consumption has rare bugs around mid-line chunk boundaries on flaky networks. | Low | High | The `SSEStream` buffer rule (§3.5) is explicitly tested against parameterized chunk-split fixtures (Phase B3 acceptance check). Manual flaky-network test on cellular before TestFlight upload. |
| R3 | SIWA's "hide my email" relay address vs. Supabase's email field — a user who revokes Apple permission has their relay address invalidated; subsequent SIWA login may fail to match an existing Supabase user. | Low | Medium | Per ADR-0003 §"Consequences/Negative" — identity is keyed on Supabase `user_id`, not email. The iOS app never compares emails; the JWT `sub` is the user. If a re-link is needed, it's a v2 settings feature. |
| R4 | iOS image cache eviction under low-memory warning evicts cookbook thumbnails just as the cookbook list scrolls them in. | Low | Low | NSCache + disk LRU split: low-memory clears the NSCache (in-RAM tier); disk survives. Cookbook list re-reads from disk on cache miss — slight latency, no failure. |
| R5 | Hand-rolled markdown parser misses an edge case the AI emits (e.g. nested bold inside a list item). | Medium | Low | Test corpus from real recipes (§6.3 markdown plan). A miss renders as plain text — no crash. Add the case to the corpus and patch. |
| R6 | Backend track is not ready when Phase B2 wants to acceptance-check. | Medium | Medium | Phase B1–B3 acceptance is against URL-stubbed fixtures, not the live backend; the dependency is "auth endpoint reachable" not "every endpoint live". Phase C onward needs the live backend; if not ready, the iOS track parks at end of Phase B and uses canned fixtures to keep moving on UI work. |
| R7 | Apple Developer enrollment, Service ID, .p8 generation are not done in time for SIWA testing. | Medium | High | Listed as an explicit prerequisite for task A3 in §5. The Builder requests the steps from Dave at the start of Phase A; A4 (email/OTP) can proceed without them as a backstop for testing the rest of the app. |
| R8 | The contract changes mid-build because the backend or data Builder discovers an inconsistency. | Low | Medium | Per the Builder contract: stop and raise. The iOS Builder does not improvise around a wrong contract — the Architect updates the contract; the iOS plan is re-greened. |
| R9 | TestFlight reviewer rejects the build for App Store guideline reasons (e.g. SIWA implementation incorrect). | Low | Medium | Dave's personal-use builds typically distribute via TestFlight internal testing, which is exempt from App Review. If external testing is requested, the SIWA implementation follows Apple's HIG to the letter — `SignInWithAppleButton` from `AuthenticationServices`, the `.signIn` style. |

---

## §9 Open questions

The plan resolves every question the iOS track owns within the
contract's spec. The following items the iOS track cannot resolve
unilaterally — they need a decision from the Conductor or a step from
Dave before Phase 4 can fully start.

1. **Apple Developer enrollment & SIWA prerequisites.** The bundle id,
   Service ID, Key (.p8) creation, and Supabase Auth Apple provider
   configuration (§4) require Dave's action in the Apple Developer
   portal and the Supabase dashboard. The iOS Builder cannot do
   these steps. Without them, task A3 (SIWA flow) cannot reach
   acceptance — though A4 (email/OTP) and every non-auth task can.

2. **Final backend host URL for dev / TestFlight.** The contract's
   endpoints are rooted at `/api/kitchen/*`, but the host
   (`https://<aws-host>` for prod, `http://localhost:8080` for dev)
   is a deployment concern. The iOS Builder needs a configurable
   `APIBaseURL` per build configuration. The dev URL is needed at
   the start of Phase B2; the prod URL is needed at the start of
   Phase F3.

3. **Inactivity window for "fresh chat on session start"** (contract
   §9.7). The contract says "after a configurable inactivity
   window" without pinning the value. Suggested default: **15
   minutes** — short enough to feel session-like, long enough that
   a quick app-switch doesn't burn a new conversation. Conductor's
   call.

4. **Whether to ship a tab bar or a navigation-drawer shell.** The
   plan picks a `TabView` with five tabs (§3.2) because it matches
   iOS norms and the source web client's mental model (chat, plan,
   cookbook, calendar, shopping). The drawer-style sidebar from the
   web chat page is preserved **inside** the chat tab. Confirming
   the tab bar choice at the gate is a sanity check on the visual
   hierarchy.

5. **Image-cache disk location.** ADR-0004 leaves the on-device
   cache implementation to the iOS track. The plan picks
   `~/Library/Caches/SousChef/Images/` (§3.6) — iOS may purge this
   under storage pressure, which is the documented behavior for a
   cache and the right behavior here. Confirming at the gate that
   "purgeable under storage pressure" is acceptable closes this.

---

## §10 Cross-references

- Workload spec: `.devcore/tasks/sous-chef-port.md`.
- Contract: `.devcore/memory/contract/contract.md`.
- Behavior spec: `.devcore/memory/domain/sous-chef-behaviors.md`.
- ADRs gating this plan: 0001 (voice cut), 0002 (OpenAI direct),
  0003 (SIWA + Supabase OTP), 0004 (image storage),
  0005 (NavigationStack), 0006 (drop legacy tables),
  0007 (shopping list id required), 0008 (recipe chat stateless),
  0009 (CFO roles preserved), 0010 (client-supplied week-start).
- Coding standards: `/CODING_STANDARDS.md` — dc-00 (intent),
  dc-01 (precedence), dc-03 (Swift / SwiftUI), dc-06 (macOS / Xcode).
- Source pin (behavioral reference only): `~/sous-chef-ai` at commit
  `d884efae9cc150df2a58afc255b3e631d31b5d2b`.
- Source screenshots (visual reference only):
  `~/sous-chef-ai/attached_assets/Screenshot_*.png`.

---
