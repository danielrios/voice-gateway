# Architecture Research

This document records the references that shaped the initial architecture. It is evidence and comparison, not a dependency list.

## Iris — strongest product reference

Repository: `ASHR12/iris`

Iris is the closest known implementation to the target experience: Gemini Live owns realtime conversation while Hermes owns tool-heavy agent work.

Patterns to adopt:

- realtime voice and long-running agent work have independent lifecycles;
- explicit Turn state that protects against stale completions after interruption;
- session resumption and idle disconnect are first-class concerns;
- agent results become proactive conversational announcements;
- agent status comes from runtime events/results, never invented from model speech;
- Hermes interaction/approval flows are explicitly correlated.

Patterns not to copy into the core:

- Electron/UI concerns;
- Gemini-specific types outside the provider adapter;
- Hermes-specific protocol outside the runtime adapter;
- desktop process-spawning assumptions;
- one large application module coordinating all concerns.

Relevant Iris files studied:

- `electron/liveSessionState.mjs` — Turn epochs, interruption, resumption handles, announcement ledger;
- `electron/hermesGatewayClient.mjs` — JSON-RPC/WebSocket bridge and runtime event stream;
- `electron/main.mjs` — Gemini Live connection/configuration, audio 16 kHz in / 24 kHz out, tool dispatch, auto-sleep/resume.

## Gemini Live

Gemini Live is the first Voice Provider because it already provides:

- bidirectional realtime audio;
- server-side VAD/interruption semantics;
- function/tool calling;
- input/output transcription;
- session resumption;
- native Go support in Google's Gen AI SDK.

The provider-specific protocol belongs entirely behind the Voice Provider seam. The rest of the gateway must not depend on Gemini model names, MIME conventions, resumption handles, or SDK callbacks.

## OpenAI Realtime

OpenAI Realtime is an important second-provider design reference because it supports realtime audio conversations over WebRTC and WebSocket and exposes a session/event model rather than a conventional request/response TTS interface.

We should not implement the adapter in Phase 1, but its existence is useful pressure against a Gemini-shaped core. A future adapter should be possible without redesigning Voice Session semantics.

## LiveKit Agents

LiveKit demonstrates a mature separation between transport/media infrastructure and agent/voice orchestration. Its architecture supports both pipeline-style STT→LLM→TTS and realtime speech models while keeping transport concerns separate.

Lesson for this project: Voice Client transport should sit below the Session Engine. WebRTC, WebSocket, or a future device protocol should not determine agent/provider semantics.

## Pipecat

Pipecat models realtime voice systems as streams of typed frames through processors and transports.

Lesson to adopt: normalize asynchronous media/control/model events into a coherent internal language.

Lesson not to adopt yet: Voice Gateway is not intended to become a generic graph/pipeline framework. A fixed, deep Session Engine gives callers more leverage and keeps orchestration local.

## Market pattern synthesis

Across these systems, several patterns recur:

1. **Streaming/event-driven lifecycle** rather than request/response.
2. **Transport separation** from conversational/model orchestration.
3. **Provider-specific adapters** around realtime model protocols.
4. **Barge-in/interruption as a protocol event**, not merely stopping a speaker.
5. **Session state independent of individual connections**.
6. **Bounded queues/backpressure** for media streams.
7. **Function/tool calls correlated by IDs**.
8. **Observability centered on latency and turn lifecycle**, especially time-to-first-audio.

Voice Gateway adopts these patterns while deliberately remaining narrower than a full voice-agent framework: the Agent Runtime continues to own tools, memory, planning, and durable agent work.
