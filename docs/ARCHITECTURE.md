# Architecture

## Purpose

Voice Gateway is a small, provider-neutral runtime between realtime voice clients and agent runtimes. It owns conversational transport and realtime voice lifecycle. It does not own the agent's memory, tools, planning, or long-running work.

The first target is:

```text
M5StickS3 / desktop / mobile
            |
            v
      Voice Gateway
       /         \
      v           v
Gemini Live    Hermes
Voice Provider Agent Runtime
```

The architecture deliberately keeps both sides replaceable so future Voice Providers (for example another realtime speech model) and Agent Runtimes (for example Quark) do not require rewriting the session core.

## Design principles

1. **Voice Session is the central deep module.** Callers should not coordinate provider callbacks, barge-in, tool waits, reconnection, playback, and announcements themselves.
2. **Transport is not conversation.** PCM/WebSocket is initially supported, but audio transport must not define the Voice Session model.
3. **Delegation is asynchronous.** Agent Runtime work can continue while the realtime conversation remains responsive.
4. **Realtime voice is provider-neutral.** Gemini Live is the first adapter, not the architecture.
5. **Agent work is runtime-neutral.** Hermes is the first adapter, not the architecture.
6. **Events are observable facts.** Provider and Agent Runtime events become typed gateway events before influencing session state.
7. **Interruption is first-class.** A stale provider completion must never complete a newer Turn.
8. **Session lifetime differs from connection lifetime.** Reconnect/resumption can preserve a Voice Session across provider or client connections.
9. **Keep seams earned.** External seams exist for dependencies that already need production and test adapters, or where a second production adapter is an explicit near-term goal.

## Runtime shape

```text
                    +----------------------+
Voice Client <----> |   Transport Adapter  |
                    +----------+-----------+
                               |
                               v
                    +----------------------+
                    |    Session Engine    |
                    |                      |
                    | turns                |
                    | interruption         |
                    | playback lifecycle   |
                    | announcements        |
                    | resumption policy    |
                    +-----+-----------+----+
                          |           |
            VoiceProvider|           |AgentRuntime
                          |           |
                  +-------v--+     +--v------------+
                  | Gemini   |     | Hermes        |
                  | adapter  |     | adapter       |
                  +----------+     +---------------+
```

### Session Engine

The Session Engine is the core deep module. Its interface should expose a small number of operations around starting/resuming a Voice Session, accepting client input, observing output, and ending the session. Provider protocol details, turn epochs, provider tool correlation, buffering, and reconnection policy stay inside the implementation.

The exact Go interface is intentionally not frozen in Phase 0. We will use a design-it-twice pass before committing the public interface.

### Voice Provider seam

A Voice Provider adapter translates between typed gateway events and an external realtime model protocol.

Responsibilities hidden behind the seam include:

- connect/reconnect;
- realtime audio/text input;
- streaming audio/text output;
- provider-native VAD and interruption signals;
- tool/function calls;
- session resumption handles;
- provider-specific errors and retry hints.

Initial adapters:

- production: Gemini Live;
- test: deterministic in-memory fake.

Likely later production adapter: OpenAI Realtime.

### Agent Runtime seam

An Agent Runtime adapter translates Delegations and Interactions into the runtime's native protocol and emits normalized lifecycle events.

Responsibilities hidden behind the seam include:

- submit/cancel delegated work;
- stream lifecycle and tool events;
- receive clarification/approval Interaction responses;
- retrieve terminal result/status;
- map runtime-specific sessions to gateway correlation identifiers.

Initial adapters:

- production: Hermes;
- test: deterministic in-memory fake.

Likely later production adapter: Quark.

### Transport seam

Transport connects a Voice Client to the gateway. It is deliberately below the Session Engine and does not receive provider- or agent-specific concepts.

Phase 1 begins with a simple binary WebSocket transport carrying PCM frames plus small control messages. This fits constrained clients and is easy to implement on ESP32. WebRTC is deferred until browser/mobile requirements justify its added signaling and media complexity.

## Internal event model

The core should reason in normalized events rather than provider callbacks. A representative vocabulary is:

```text
ClientAudio
ClientText
ClientInterrupted
ProviderAudio
ProviderText
ProviderTurnStarted
ProviderTurnCompleted
ProviderInterrupted
ProviderToolRequested
ProviderToolCancelled
DelegationStarted
DelegationProgress
DelegationCompleted
DelegationFailed
InteractionRequired
AnnouncementQueued
SessionResuming
SessionClosed
```

This is inspired by frame/event pipeline architectures, but the gateway should not become a generic pipeline framework. Events exist to give the Session Engine one coherent language.

## Turn lifecycle

A Turn is not equivalent to one HTTP request or WebSocket message. Realtime events can overlap.

```text
idle
  |
  v
waiting-for-provider
  |
  v
generating ---- interruption ----> waiting-for-provider(new epoch)
  |
  +---- provider tool ----> tool-wait ----> generating
  |
  v
playback
  |
  v
idle
```

Each Turn receives a monotonically increasing epoch. Completion belonging to an interrupted older epoch cannot settle the current epoch. Iris implements the same essential safety property in its `LiveTurnState`; this is worth preserving as a gateway invariant rather than reproducing its exact implementation.

## Delegation lifecycle

Agent Runtime work is independent from a Turn:

```text
Voice Session
    |
    | delegation
    v
Agent Runtime -------------------+
    |                            |
    | progress                   | Voice Session continues
    |                            |
    +---- terminal result -------+
                 |
                 v
            Announcement
```

This is the most important architectural lesson from Iris: Gemini Live remains the conversational frontend while Hermes does long-running work independently. The gateway should preserve that capability without coupling it to either product.

## Session lifetime and resumption

Voice Session identity is gateway-owned. Provider connection identity is adapter-owned.

A provider adapter may supply a resumption token/handle. The Session Engine decides whether the Voice Session remains resumable and when a dormant session expires. Provider-specific TTLs remain inside the provider adapter as capability metadata or normalized events.

Idle connections should be closable without necessarily destroying the Voice Session. This matters both for resource usage and providers that charge/limit continuously streamed audio.

## Concurrency model

Each Voice Session should have one owner goroutine (actor-like event loop). External adapters push events into bounded channels; state transitions happen only in the owner goroutine.

Benefits:

- no shared mutable Turn state;
- ordering is explicit;
- interruption races become deterministic;
- backpressure can be defined per event class;
- tests can drive the same event loop deterministically.

Audio bytes should avoid unnecessary copies and should not pass through JSON internally.

## Backpressure

Realtime audio cannot use an unbounded queue.

Initial policy:

- audio input/output queues are bounded;
- stale playback audio is dropped immediately after interruption;
- control/tool/session events are never silently dropped;
- slow clients are disconnected rather than allowed to grow memory without bound;
- metrics distinguish dropped media from lost control events.

## Security baseline

- The gateway never exposes Agent Runtime credentials to a Voice Client.
- Provider API keys remain server-side.
- Client authentication is separate from Agent Runtime authentication.
- Sensitive approval/secret flows must be typed Interactions; providers must not receive secret values unless explicitly allowed by a future policy.
- Every Delegation and Interaction carries correlation identifiers; the gateway never infers completion from model speech.

## Observability

Phase 1 uses structured `slog` logs and Prometheus-compatible metrics. OpenTelemetry tracing is a planned extension once there are enough cross-process hops to justify it.

Important measurements:

- time to first provider audio;
- end-of-user-speech to first provider audio;
- interruption-to-playback-stop;
- reconnect/resumption success;
- media queue depth/drop count;
- Delegation duration and terminal status;
- active/dormant Voice Sessions.

## Testing strategy

The interface is the test surface.

- Session Engine tests use in-memory Voice Provider and Agent Runtime adapters.
- Protocol adapters have focused contract/integration tests against recorded or local protocol fixtures where possible.
- Race-sensitive Turn tests use deterministic events and clocks instead of sleeps.
- End-to-end smoke tests cover real Gemini Live and Hermes only when credentials/runtime are available; they are not required for the normal unit suite.

## Non-goals for the initial runtime

- generic workflow/pipeline framework;
- generic plugin marketplace;
- agent memory implementation;
- agent planning/tool execution implementation;
- WebRTC SFU;
- desktop UI;
- wake-word implementation;
- transcoding arbitrary codecs in the core;
- storing full conversation history as a source of truth.
