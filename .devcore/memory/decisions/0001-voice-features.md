---
type: decision
title: Voice features — cut from the iOS port
status: accepted
owner: analyst
workload: sous-chef-ios
last_updated: 2026-05-24
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# ADR-0001 — Voice features: cut from the iOS port

This ADR resolves workload-spec §6.1: keep voice features in the iOS port,
or cut them? It is one of two artifacts produced by the Analyst at Phase 3
(the other is the behavior spec at `domain/sous-chef-behaviors.md`). The
human gates this decision at the behavior_spec gate; until approved, the
status remains **Proposed**.

The ADR depends only on a reading of source-pin commit
`d884efae9cc150df2a58afc255b3e631d31b5d2b` of `~/sous-chef-ai`. No external
input is required to act on it.

---

## Context

The workload spec calls out voice features as an open decision because the
web app's voice path is Replit-specific (`server/replit_integrations/audio/`
uses `gpt-audio` and `gpt-4o-mini-transcribe` through the Replit AI
Integrations proxy, and the client tree under
`client/replit_integrations/audio/` defines a `useVoiceRecorder` hook,
`useAudioPlayback` worklet hook, audio utils, and a streaming helper).

The decision is whether the iOS rebuild should re-implement voice with
native `Speech` / `AVFoundation` and a native audio capture/playback path,
or drop the surface entirely.

### What the source actually does

Inspecting the pin:

- **Server.** `server/replit_integrations/audio/routes.ts` exports a
  `registerAudioRoutes(app)` function defining
  - `GET /api/conversations`, `GET /api/conversations/:id`,
  - `POST /api/conversations`, `DELETE /api/conversations/:id`,
  - `POST /api/conversations/:id/messages` — a base64-audio-in,
    SSE-audio-out endpoint using `gpt-audio` with
    `modalities: ["text", "audio"]`.
  The supporting `client.ts` provides `voiceChat`, `voiceChatStream`,
  `textToSpeech`, `textToSpeechStream`, `speechToText`,
  `speechToTextStream`, and an `ensureCompatibleFormat` helper that
  shells out to `ffmpeg` via `child_process.spawn` to convert
  WebM/MP4/OGG to WAV.

- **Critical observation: `registerAudioRoutes` is never called.**
  `grep -n "registerAudioRoutes" ~/sous-chef-ai/server/` shows only the
  module's own export and definition — nothing in `server/index.ts` or
  `server/routes.ts` wires it in. The audio routes are dark code at the
  pin.

- **Client.** `client/replit_integrations/audio/` contains
  `useVoiceRecorder.ts` (MediaRecorder API → `audio/webm;codecs=opus`
  blob), `useAudioPlayback.ts` (AudioWorklet-based PCM16 playback with
  out-of-order chunk reordering), `useVoiceStream.ts`, `audio-utils.ts`,
  and a worklet file. The shipped React app (`client/src/App.tsx`, the
  page components, `chat-input.tsx`) imports **none of these**. The chat
  input is text-only — no microphone button, no MediaRecorder usage, no
  audio playback on the rendered surfaces.

So at the source pin, voice is:
- **A complete server module that no router includes**, and
- **A complete client hook library that no page imports**.

The integration was scaffolded but never connected. There is no
user-facing voice surface in the running app.

### What "voice" would mean in the iOS port

If we chose to *re-implement* voice natively, we would build:

1. A push-to-talk or VAD-driven recording path using `AVAudioEngine` /
   `SFSpeechRecognizer` for on-device transcription, **or** a native
   record-and-upload path to a backend STT endpoint (today: gpt-4o-mini-
   transcribe; with provider freedom, also Whisper-1 or Apple's
   on-device speech).
2. A response path that either speaks the AI's text via
   `AVSpeechSynthesizer` (cheap, low latency) or streams a generated
   audio response from a model like `gpt-audio` and plays it via
   `AVAudioPlayer` / `AVAudioEngine` (expensive, network-heavy).
3. A new screen or pinned mic affordance in the chat input.

None of this exists today as a working user experience. It would be a
**new feature**, not a port of an existing one.

---

## Decision

**Cut voice features from the iOS port.** No native voice surface in the
v1 iOS app.

The `client/replit_integrations/audio/` and
`server/replit_integrations/audio/` trees are deleted alongside the rest
of the Replit-integration trees. The four `gpt-audio` / transcription
SDK functions in `client.ts` are not ported. No microphone affordance is
added to the chat input. No new endpoints are added to the contract.

---

## Status

**Accepted.** Approved by Dave at the behavior_spec gate, 2026-05-24.

---

## Consequences

### Positive

- **Scope contraction matches reality.** Voice is not part of the product
  the user uses today; cutting it cuts only dead code, not a working
  feature.
- **Faster v1.** No `AVAudioEngine`/`SFSpeechRecognizer` integration, no
  microphone permission flow, no audio-streaming network spec, no audio
  playback worklet equivalent. The iOS app ships sooner.
- **No new infrastructure obligations on the Go backend.** Skipping
  voice means no audio-format conversion service (the current `ffmpeg`
  shell-out is non-trivial to operate in production), no STT integration,
  and no audio SSE wire format.
- **Privacy and permissions surface stays small.** No microphone
  entitlement, no Speech Recognition usage description in the iOS Info
  plist, no audio storage discussion.
- **The behavior spec stays clean.** Chat is text-only; the contract for
  `POST /api/kitchen/message` doesn't need an audio variant.

### Negative

- **No voice convenience in the kitchen.** Hands-free use (literally
  with raw chicken on your hands) is a natural fit for this product.
  We accept this gap.
- **A real existing user expectation may go unmet.** Mitigation: the
  source's voice path was never wired to a UI, so no documented user
  expectation exists. We are deciding **not to add** a feature, not
  **removing** one.

### Reversible

This is a low-cost decision to reverse later. The surface area is small:
adding a microphone button to the chat input and a single audio
upload+SSE endpoint to the Go backend is a contained v2 feature. The
CFO and tool-call contracts are unaffected by the choice — voice would
be a UI/transport layer, not a domain change.

### What this implies for the workload spec must-cut list

- `server/replit_integrations/audio/` and the equivalent client tree are
  added to the must-cut list with confidence (workload spec §5 already
  implies this; this ADR confirms).
- No microphone-related entries appear in the iOS contract.

### What this implies for the behavior spec

The behavior spec (`domain/sous-chef-behaviors.md`) treats the chat
surface as text-only. If the human flips this decision at the gate, the
following sections need amendment:
- §3.2 (chat page) — add a microphone affordance and audio-mode state.
- §4.5 (SSE protocol) — add audio message shapes.
- §5 (REST API surface) — add an audio-in endpoint.
- §8.2 (must cut) — remove the audio-trees line.
