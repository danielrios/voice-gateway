# Architecture Research

This document records the primary sources that shaped the initial architecture. It is evidence and comparison, not a dependency list or a specification.

## Research discipline

Architecture claims should be traced to primary sources whenever possible: official provider documentation, official SDK source, specifications, or the source code of the implementation being studied. Secondary summaries are useful for discovery but should not become design authority.

Voice Gateway is intentionally independent from every reference below. Studying a pattern does not make the referenced project a dependency or its internal architecture our specification.

## Iris — reference implementation studied

Reference: https://github.com/ASHR12/iris

Iris is an existing MIT-licensed application with a closely related interaction model: Gemini Live owns realtime conversation while Hermes owns tool-heavy agent work. We study it as concrete evidence of problems that appear in a working system, not as a codebase to reproduce.

Patterns worth preserving as gateway invariants:

- realtime voice and long-running agent work have independent lifecycles;
- Turn state must reject stale completions after interruption;
- session resumption and idle disconnect are first-class concerns;
- agent results can return as proactive conversational announcements;
- agent status/results come from runtime events and responses, never invented model speech;
- approvals and interactions need explicit correlation.

Patterns deliberately not copied into the core:

- Electron/UI concerns;
- Gemini-specific types outside a Voice Provider adapter;
- Hermes-specific protocol outside an Agent Runtime adapter;
- desktop process-spawning assumptions;
- a single application module coordinating unrelated concerns.

Relevant Iris source studied:

- `electron/liveSessionState.mjs` — Turn epochs, interruption, resumption handles, announcement ledger;
- `electron/hermesGatewayClient.mjs` — JSON-RPC/WebSocket bridge and runtime event stream;
- `electron/main.mjs` — Gemini Live connection/configuration, audio streaming, tool dispatch, auto-sleep/resume.

License: https://github.com/ASHR12/iris/blob/main/LICENSE

## Gemini Live and the Go SDK

Primary sources:

- Live API capabilities: https://ai.google.dev/gemini-api/docs/live-api/capabilities
- Google Gen AI Go SDK Live implementation: https://github.com/googleapis/go-genai/blob/main/live.go
- Go SDK changelog: https://github.com/googleapis/go-genai/blob/main/CHANGELOG.md

Gemini Live is the first Voice Provider because it provides bidirectional realtime audio, provider-side voice activity handling, function/tool calling, transcription features, and session-oriented realtime behavior.

The official Go SDK exposes a Live module with `Connect`, `SendRealtimeInput`, `SendToolResponse`, and `Receive` over a WebSocket session. The SDK source currently marks the Live module and Session as **Preview**. That is both an enabling factor and a risk: Voice Gateway should use the SDK initially, but Gemini protocol/SDK types must remain behind the Voice Provider seam so SDK churn is local.

Provider-specific model names, MIME conventions, resumption handles, callbacks, and API-version details do not belong in the Session Engine.

## Go runtime baseline

Primary source: https://go.dev/doc/devel/release

Go 1.27.0 was released on 2026-08-19 and is the current stable Go release at the time of this architecture pass. Because Voice Gateway is a new project with no existing compatibility contract, the initial implementation should target Go 1.27 rather than start with an older language/toolchain baseline.

## ESP32 WebSocket transport feasibility

Primary source: https://docs.espressif.com/projects/esp-protocols/esp_websocket_client/docs/latest/index.html

Espressif maintains an official WebSocket client for ESP32-family devices with `ws` and `wss` support, TCP/TLS transport, multiple client instances, and binary-frame sending. This is sufficient evidence that a simple binary WebSocket transport is a practical first path for ESP32-class Voice Clients without making WebSocket part of the Session Engine semantics.

The transport decision remains deliberately narrow: WebSocket/PCM is the Phase 1 constrained-device path, not a claim that WebRTC or other media transports are unnecessary for richer browser/mobile clients.

## OpenAI Realtime

Primary source: https://developers.openai.com/api/docs/guides/realtime

OpenAI Realtime is the main second-provider design pressure. Its official documentation exposes realtime sessions over multiple connection methods, including WebRTC and WebSocket, and separate concepts for conversations, VAD, tools, and server-side controls.

We do not need an OpenAI adapter in Phase 1. Its value now is architectural: if adding it later would require changing Voice Session semantics, the initial Voice Provider seam is too Gemini-shaped.

## LiveKit Agents

Primary source: https://docs.livekit.io/agents/models/realtime/

LiveKit treats realtime speech models as interchangeable integrations at the agent-session level and supports multiple realtime providers. It also keeps media/transport infrastructure conceptually distinct from voice-model orchestration. Its documentation additionally shows that a realtime model can be combined with separate TTS when a product wants speech understanding from the realtime model but independent speech generation.

Lesson for Voice Gateway: transport and Voice Provider are separate concerns, and direct speech-to-speech must be an adapter capability rather than an assumption baked into the whole runtime.

## Pipecat

Primary source: https://docs.pipecat.ai/pipecat/learn/pipeline

Pipecat demonstrates a mature frame-processing model for realtime media systems: typed frames move through processors and transports rather than being treated as conventional request/response calls.

Lesson to adopt: normalize asynchronous media, control, provider, and runtime facts into a coherent internal event language.

Lesson not to adopt yet: Voice Gateway is not intended to become a generic graph/pipeline framework. A fixed, deep Session Engine should hide orchestration rather than requiring callers to assemble it.

## Matt Pocock engineering skills

Primary source: https://github.com/mattpocock/skills

The engineering workflow used for this project follows the repository's design vocabulary and process guidance, especially:

- `codebase-design` — deep modules, interfaces, seams, adapters, leverage, locality;
- `domain-modeling` — keep `CONTEXT.md` as a domain glossary and use ADRs only for durable trade-offs;
- `research` — prefer primary sources and record findings in-repo;
- `tdd` — red/green/refactor during implementation;
- `code-review` — review both standards compliance and spec fidelity;
- `to-spec`, `to-tickets`, and `implement` — turn resolved design into small executable slices.

The selected skills are installed under `.agents/skills/` as development-only tooling and tracked by `skills-lock.json`. They are not runtime dependencies and do not define Voice Gateway's product architecture. Their upstream MIT license is preserved in `.agents/skills/LICENSE`.

## Market pattern synthesis

Across the primary references, several patterns recur:

1. **Streaming/event-driven lifecycle** rather than request/response.
2. **Transport separation** from conversational/model orchestration.
3. **Provider adapters** around provider-specific realtime protocols.
4. **Barge-in/interruption as a protocol/state event**, not merely stopping a speaker.
5. **Session state independent of an individual network connection**.
6. **Bounded queues/backpressure** for realtime media.
7. **Tool/runtime work correlated by stable identifiers**.
8. **Observability centered on latency and Turn lifecycle**, especially time-to-first-audio.
9. **Provider SDK churn belongs behind an adapter**, especially while realtime SDK surfaces remain preview/fast-moving.

Voice Gateway adopts these patterns while deliberately remaining narrower than a full voice-agent framework: the Agent Runtime continues to own tools, memory, planning, and durable agent work.
