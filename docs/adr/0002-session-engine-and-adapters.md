---
status: accepted
---

# Center the runtime on a deep Session Engine with adapters

Voice Gateway sits between independently evolving realtime Voice Providers and Agent Runtimes. We choose one deep Session Engine module to own Turn lifecycle, interruption, playback validity, Voice Session lifetime, Delegation correlation, Announcements, and backpressure, while provider/runtime protocols remain behind adapters. This keeps race-sensitive orchestration local instead of forcing every caller to assemble a generic pipeline or coordinate provider callbacks itself.

The decision is supported by recurring patterns in Gemini Live, OpenAI Realtime, LiveKit, Pipecat, and by Iris as a concrete reference implementation. Iris is evidence that the interaction model works; it is not the specification for this module.

## Considered Options

- **Generic processor/frame pipeline as the public architecture** — maximizes flexibility, but exposes orchestration composition to callers and risks a broad, shallow interface. Typed internal events may still be used inside the Session Engine.
- **Gemini-shaped core** — rejected because model names, MIME rules, SDK callbacks, API versions, and resumption handles belong to the Gemini adapter.
- **Hermes-shaped core** — rejected because Hermes JSON-RPC methods/events do not define gateway domain semantics.
- **Voice logic primarily inside a Hermes plugin** — rejected because media/session/provider lifecycle must survive replacing Hermes. A Hermes plugin may still exist later as a thin integration adapter if the runtime requires it.

## Consequences

- `Voice Provider` is an external seam with Gemini Live as the first production adapter and a deterministic in-memory adapter for tests; OpenAI Realtime is deliberate second-provider design pressure.
- `Agent Runtime` is an external seam with Hermes first and a deterministic in-memory test adapter; Quark is a likely later production adapter.
- client Transport stays below conversational semantics; Phase 1 begins with binary WebSocket/PCM.
- the Session Engine may have substantial internal implementation complexity, but its public interface should remain small and become the main test surface.
- the exact Go interface is deliberately not fixed by this ADR; it must go through a design-it-twice pass before the walking skeleton commits it.
