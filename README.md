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

The caller-facing Session Engine shape is a long-lived **Session Handle** with attachment-aware client links. A Voice Client transport can detach without ending the Voice Session, while provider/runtime lifecycle and Turn mechanics remain hidden inside the Session Engine.

The architecture intentionally does not expose Gemini or Hermes types through the core.

## Stack

- **Go 1.27** runtime baseline;
- official Google Gen AI Go SDK for the initial Gemini Live adapter (its Live surface is currently Preview and remains isolated behind the adapter);
- standard library first;
- one owner goroutine per Voice Session as the initial concurrency model;
- `log/slog` for structured logging;
- deterministic in-memory adapters for core tests;
- WebSocket/PCM transport for the first constrained-device path.

## Reference implementation: Iris

[ASHR12/iris](https://github.com/ASHR12/iris) is an MIT-licensed reference implementation we studied because it demonstrates a closely related interaction model: Gemini Live remains responsive as the realtime voice frontend while Hermes performs independent work and reports real lifecycle/results back into the conversation.

Voice Gateway is independent from Iris and does not aim to reproduce its application architecture. Iris is one research input alongside the official Gemini/OpenAI documentation and the LiveKit/Pipecat architectures. The gateway keeps the useful invariants — independent voice/agent lifecycles, interruption correctness, session-resumption thinking, and proactive results — while defining its own provider/runtime-neutral domain model.

## Documents

- [`CONTEXT.md`](CONTEXT.md) — canonical domain language;
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — runtime shape, seams, concurrency, backpressure, security, testing;
- [`docs/RESEARCH.md`](docs/RESEARCH.md) — primary-source research and reference implementations;
- [`docs/design/session-engine-interface.md`](docs/design/session-engine-interface.md) — design-it-twice comparison of Session Engine interface shapes;
- [`docs/STANDARDS.md`](docs/STANDARDS.md) — engineering standards and implementation discipline;
- [`docs/agents/domain.md`](docs/agents/domain.md) — how engineering agents consume domain docs;
- [`docs/agents/issue-tracker.md`](docs/agents/issue-tracker.md) — GitHub issue-tracker convention;
- [`docs/adr/0001-go-runtime.md`](docs/adr/0001-go-runtime.md) — why Go;
- [`docs/adr/0002-session-engine-and-adapters.md`](docs/adr/0002-session-engine-and-adapters.md) — why the Session Engine and adapter seams;
- [`docs/adr/0003-session-handle-interface-shape.md`](docs/adr/0003-session-handle-interface-shape.md) — why the caller-facing interface uses a Session Handle with attachment-aware client links.

## Roadmap

### Phase 0 — architecture

- [x] establish domain language;
- [x] research realtime voice architecture patterns against primary sources;
- [x] define Session Engine responsibility;
- [x] choose runtime stack;
- [x] define initial seams and testing strategy;
- [x] design the Session Engine public interface at least twice before committing it.

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

## Development approach

The project uses Matt Pocock's engineering-skills methodology as a development aid: primary-source research, explicit domain language, selective ADRs, design-it-twice for important interfaces, TDD at stable seams, and standards/spec review before merge.

The selected skills are installed under `.agents/skills/` and tracked by `skills-lock.json`. They are development-only tooling, not runtime dependencies and not part of the Voice Gateway architecture. Their upstream MIT license is preserved in `.agents/skills/LICENSE`.

## License

Voice Gateway is licensed under the MIT License. See [`LICENSE`](LICENSE).

Vendored Matt Pocock engineering skills remain under their upstream MIT license; see [`.agents/skills/LICENSE`](.agents/skills/LICENSE).

## Status

Phase 0 architecture is defined. The next step is the Phase 1 walking skeleton, implemented test-first through the selected Session Engine seam without baking Gemini, Hermes, Iris/Electron, or one device into the project model.
