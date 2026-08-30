# ADR-0002: Center the runtime on a Session Engine with provider and agent adapters

- Status: Accepted
- Date: 2026-08-29

## Context

The gateway sits between two independently evolving external systems:

1. realtime Voice Providers such as Gemini Live;
2. Agent Runtimes such as Hermes and Quark.

A naive implementation could expose provider callbacks directly to clients or model the gateway as a generic processor pipeline. Both approaches make callers coordinate Turn lifecycle, interruption, provider tools, agent delegation, and reconnection themselves.

Iris demonstrates that the hard part is not forwarding audio. It is keeping realtime conversation coherent while independent agent work runs and while provider events can overlap or arrive after interruption.

## Decision

The core runtime is one deep Session Engine module.

The Session Engine owns:

- Turn lifecycle and epochs;
- interruption/barge-in state;
- playback validity;
- normalized provider events;
- delegation correlation;
- announcements;
- Voice Session lifecycle and resumption policy;
- media/control backpressure policy.

Two external seams are accepted:

- `Voice Provider`: production Gemini Live adapter + deterministic in-memory test adapter;
- `Agent Runtime`: production Hermes adapter + deterministic in-memory test adapter.

Transport is separate from the Session Engine. Phase 1 uses a binary WebSocket/PCM adapter.

The public Session Engine interface is not fixed by this ADR. It will be designed separately using a design-it-twice exercise before implementation.

## Consequences

Positive:

- high leverage for clients: they do not coordinate provider/runtime protocols;
- high locality: race-sensitive Turn state lives in one module;
- Gemini and Hermes remain replaceable;
- tests exercise the same Session Engine interface callers use;
- long-running Delegations do not block realtime conversation.

Negative:

- the Session Engine has meaningful internal complexity;
- normalized events require careful semantic mapping from each provider/runtime;
- provider-specific advanced features may need capability negotiation rather than leaking directly through the core interface.

## Rejected alternatives

### Generic processor/frame pipeline as the public architecture

Rejected for Phase 1. It maximizes flexibility but creates a broad, shallow interface and pushes orchestration knowledge into application composition. Voice Gateway has a specific job and should provide a deeper interface.

### Gemini-shaped core

Rejected. Gemini Live is the first adapter, not the domain model. Session resumption handles, SDK callbacks, model names, and MIME details stay inside the adapter.

### Hermes-shaped core

Rejected. Hermes is the first Agent Runtime adapter. Its JSON-RPC methods and event names do not define Delegation or Interaction semantics for the gateway.

### Voice logic inside a Hermes plugin

Rejected as the primary architecture. Audio transport, Voice Provider lifecycle, and realtime session state should survive replacing Hermes. A Hermes plugin may later improve integration, but it is an adapter concern rather than the gateway runtime itself.
