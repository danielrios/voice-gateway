# Voice Gateway

Provider-neutral realtime voice gateway for connecting constrained devices and applications to realtime voice models and agent runtimes.

Voice Gateway is **not an agent runtime** and **not a TTS wrapper**. It owns the realtime conversational path — media, turns, interruption, provider sessions, and the handoff between spoken interaction and independent agent work.

## Why

Realtime speech models are getting good at the conversation loop, while agent runtimes are getting good at tools, memory, coding, files, research, approvals, and long-running work. Coupling those two concerns inside one application makes both harder to replace.

Voice Gateway keeps them separate:

```text
M5StickS3 / desktop / mobile
            |
            v
      +---------------+
      | Voice Gateway |
      +-------+-------+
              |
        +-----+-----+
        |           |
        v           v
  Voice Provider  Agent Runtime
  Gemini Live     Hermes
  OpenAI RT (*)   Quark (*)
```

`(*)` planned adapters, not Phase 1 commitments.

## Initial architecture

The core is a deep **Session Engine**. It owns:

- Turn lifecycle and interruption/barge-in correctness;
- streaming playback validity;
- Voice Session lifetime independent of network connections;
- normalized provider events;
- asynchronous Delegations to an Agent Runtime;
- Interactions and proactive Announcements;
- bounded media queues and backpressure.

External protocols sit behind adapters:

- **Voice Provider** — Gemini Live first;
- **Agent Runtime** — Hermes first;
- **Transport** — binary WebSocket + PCM first.

The architecture intentionally does not expose Gemini or Hermes types through the core.

## Stack

- **Go** runtime;
- official Google Gen AI Go SDK for Gemini Live;
- standard library first;
- one owner goroutine per Voice Session;
- `log/slog` for structured logging;
- deterministic in-memory adapters for core tests;
- WebSocket/PCM transport for the first constrained-device path.

## Strong inspiration: Iris

[ASHR12/iris](https://github.com/ASHR12/iris) is the closest working reference for the target interaction model: Gemini Live remains responsive as the realtime voice frontend while Hermes performs independent work and reports real lifecycle/results back into the conversation.

Voice Gateway adopts that separation, Turn/interruption discipline, session-resumption thinking, and proactive result announcements — while extracting them from Iris's Electron/Gemini/Hermes-specific application architecture into a small provider/runtime-neutral gateway.

## Documents

- [`CONTEXT.md`](CONTEXT.md) — canonical domain language;
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — runtime shape, seams, concurrency, backpressure, security, testing;
- [`docs/RESEARCH.md`](docs/RESEARCH.md) — Iris, Gemini Live, OpenAI Realtime, LiveKit, and Pipecat comparison;
- [`docs/STANDARDS.md`](docs/STANDARDS.md) — engineering standards for the first implementation;
- [`docs/adr/0001-go-runtime.md`](docs/adr/0001-go-runtime.md) — why Go;
- [`docs/adr/0002-session-engine-and-adapters.md`](docs/adr/0002-session-engine-and-adapters.md) — why the Session Engine and adapter seams.

## Roadmap

### Phase 0 — architecture

- [x] establish domain language;
- [x] research realtime voice architecture patterns;
- [x] define Session Engine responsibility;
- [x] choose runtime stack;
- [x] define initial seams and testing strategy;
- [ ] design the Session Engine public interface at least twice before committing it.

### Phase 1 — walking skeleton

- [ ] Go module + CI;
- [ ] Session Engine with deterministic fake adapters;
- [ ] Gemini Live adapter;
- [ ] simple WebSocket/PCM Voice Client transport;
- [ ] Hermes Agent Runtime adapter;
- [ ] one end-to-end spoken delegation and result announcement.

### Later

- OpenAI Realtime Voice Provider adapter;
- Quark Agent Runtime adapter;
- richer transport options such as WebRTC when justified;
- device examples/SDKs, starting with ESP32/M5StickS3;
- production observability and deployment packaging.

## Status

Early architecture phase. The goal is to make the first implementation small without accidentally baking Gemini, Hermes, Electron, or one device into the project model.
