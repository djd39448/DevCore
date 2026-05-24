---
type: decision
title: AI provider for the Go backend — Direct OpenAI
status: accepted
owner: conductor
workload: sous-chef-ios
last_updated: 2026-05-24
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# ADR-0002 — AI provider for the Go backend: Direct OpenAI

This ADR resolves workload-spec §6.2. The Conductor captured this decision at
the `behavior_spec` human gate; the Architect does **not** re-litigate it.
Subsequent contract work depends on it.

The ADR depends only on the workload spec and the behavior spec
(`domain/sous-chef-behaviors.md`, §4 *AI behaviors*) at source-pin
`d884efae9cc150df2a58afc255b3e631d31b5d2b`.

---

## Context

The web app talks to OpenAI through the **Replit AI Integrations proxy**
(`process.env.AI_INTEGRATIONS_OPENAI_BASE_URL`). The wire format is OpenAI
chat-completions with tool calling, SSE streaming, and `gpt-image-1` image
generation. The Replit proxy is platform lock-in and must go (workload-spec
§5). The replacement provider is open.

Options weighed at the gate:

- **Direct OpenAI** — Identical tool-call shape, SSE format, image
  generation model. Every behavior in the spec ports unchanged.
- **OpenAI-compat (Azure / OpenRouter)** — Same wire format, different
  vendor. Useful for data-residency or multi-model A/B, but adds
  configuration without changing v1 behavior.
- **Anthropic direct** — Different tool-call schema. The four tool calls
  (`update_ingredients`, `create_meal_plan`, `create_shopping_list`,
  `update_meal`) would need translation. No first-party image generation.
- **AWS Bedrock multi-model** — One endpoint, many models. Heavier
  infra; markup over direct provider pricing. Overkill for a one-user app.

## Decision

**Use OpenAI directly** from the Go backend for v1.

The Go API wraps the OpenAI client behind a small internal interface
(`AIClient`) with the methods the four tool calls need plus image
generation. The interface — not the OpenAI library directly — is what the
rest of the backend depends on. Swapping the provider later is a contained
change in the implementation behind that interface; the contract does not
move.

## Status

**Accepted.** Locked by Dave at the behavior_spec gate, 2026-05-24.

## Consequences

### Positive

- The four tool-call schemas in the behavior spec port verbatim into the
  contract — no translation work.
- `gpt-image-1` is available as today; the recipe-image flow keeps the
  same prompt-not-bytes shape.
- SSE streaming protocol from the behavior spec §4.5 ports unchanged.
- One environment variable (`OPENAI_API_KEY`) and one base URL. No proxy
  to operate.

### Negative

- Single-vendor risk: an OpenAI outage takes the AI surface down. Mitigated
  by the `AIClient` interface — adding a fallback provider is a contained
  change later.
- Pricing follows OpenAI's list rates. For a personal app this is in the
  low single-digit dollars per month at expected usage.

### What this implies for the contract

- The `AIClient` interface is part of the backend track's internal design,
  **not** the wire contract. The contract specifies tool-call shapes and
  SSE message shapes only.
- The contract names the model implicitly: "OpenAI chat-completions API,
  current production tool-calling model". The specific model
  (`gpt-4o`, `gpt-4o-mini`, etc.) is a config value, not a contract value.
- Image generation in the contract: "OpenAI image generation
  (`gpt-image-1` or current equivalent), prompt input, base64 PNG output".
