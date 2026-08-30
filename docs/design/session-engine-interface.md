# Session Engine interface — design-it-twice

Issue: #3

This note applies the repository's `codebase-design` / `DESIGN-IT-TWICE` discipline to the Session Engine before Phase 1 commits a Go interface.

## Problem space

The Session Engine is intended to be a deep module. A caller should be able to run a realtime Voice Session without coordinating the implementation details that make realtime voice difficult:

- Turn epochs and stale-completion rejection;
- interruption / barge-in;
- playback validity;
- Voice Provider tool-call correlation;
- provider reconnect and session resumption;
- asynchronous Delegations to an Agent Runtime;
- Interactions and Announcements;
- bounded media queues and backpressure;
- client detach/re-attach while the Voice Session remains alive.

The external dependencies fall into two seam categories:

- **Voice Provider** — true external dependency. Production starts with Gemini Live; deterministic tests use an in-memory adapter. Provider SDK and protocol types stay behind this seam.
- **Agent Runtime** — remote dependency with a gateway-owned port. Production starts with Hermes; deterministic tests use an in-memory adapter. Runtime-native protocol types stay behind this seam.

Clock/timer behavior is an internal substitutable dependency and does not belong in the public Session Engine interface.

The client Transport is outside the Session Engine. It translates WebSocket/PCM (and later other transports) into domain-level client input and consumes domain-level session output.

The design must preserve a critical distinction: **Voice Session lifetime is not client connection lifetime**.

---

## Design A — minimal command/stream

### Interface shape

```go
type SessionEngine interface {
    Run(ctx context.Context, open OpenSession, input <-chan SessionInput) (<-chan SessionOutput, error)
}
```

`OpenSession` either creates a new Voice Session or carries a resumption reference. `SessionInput` and `SessionOutput` are gateway-owned typed unions; provider/runtime types never appear.

### Usage

```go
output, err := engine.Run(ctx, open, input)
for event := range output {
    // transport writes audio/control output to the Voice Client
}
```

### Invariants / ordering / errors

- one `Run` owns one active client attachment;
- input ordering is preserved;
- output ordering is session-defined;
- cancellation ends the attachment, not necessarily the underlying Voice Session;
- terminal setup/authentication failures are returned from `Run`;
- runtime/provider failures after startup become typed session output unless the Voice Session itself is no longer viable.

### What it hides

Almost everything: owner goroutine, Turn state, provider sessions, Delegations, queues, resumption, retry policy, and Announcements.

### Dependency strategy

The implementation receives Voice Provider and Agent Runtime ports at construction. Tests replace both with deterministic adapters.

### Trade-offs

**Depth:** extremely high — one entry point exposes substantial behavior.

**Locality:** strong while a client is attached.

**Weakness:** the lifetime of a `Run` call naturally looks like the lifetime of a Voice Session even though the domain explicitly says otherwise. Re-attachment, multiple transport connections over time, dormant sessions, and explicit session termination all require additional conventions or another registry interface. The apparent simplicity moves lifecycle knowledge into callers.

**Verdict:** reject as the primary interface. It optimizes method count at the cost of the most important lifecycle invariant.

---

## Design B — long-lived Session Handle with attachment-aware stream

### Interface shape

```go
type SessionEngine interface {
    Open(ctx context.Context, request OpenRequest) (SessionHandle, error)
}

type SessionHandle interface {
    ID() SessionID
    Attach(ctx context.Context) (ClientLink, error)
    End(ctx context.Context) error
}

type ClientLink interface {
    Send(ctx context.Context, input SessionInput) error
    Events() <-chan SessionOutput
    Detach() error
}
```

`OpenRequest` supports either a new Voice Session or a gateway-issued resume reference. `Attach` creates a transport-facing attachment without making the network connection the owner of the Voice Session.

### Usage

```go
session, err := engine.Open(ctx, OpenRequest{Resume: resume})
link, err := session.Attach(connectionCtx)

for {
    select {
    case input := <-clientInput:
        _ = link.Send(connectionCtx, input)
    case output := <-link.Events():
        // transport writes output
    }
}
```

When the network connection ends, the Transport detaches the `ClientLink`. Session policy decides whether the Voice Session remains dormant/resumable. `End` explicitly terminates the Voice Session.

### Invariants / ordering / errors

- a Voice Session may outlive any individual `ClientLink`;
- Phase 1 permits one active `ClientLink` per Voice Session; a second attach is rejected or replaces the stale link according to one explicit policy;
- `SessionInput` ordering is preserved per active link;
- every Turn is internally assigned an epoch; stale provider completion or playback from an older epoch is never emitted as current output;
- control events are never silently dropped;
- media may be dropped only according to documented bounded-queue policy;
- `Detach` stops delivery to that link but does not imply `End`;
- provider/runtime failures are normalized into domain behavior rather than leaking SDK/protocol errors.

### What it hides

- session registry and dormant-session policy;
- provider connection lifecycle and provider resumption handles;
- owner goroutine and channels;
- Turn epochs, interruption and playback invalidation;
- tool-call correlation;
- Delegation / Interaction / Announcement state;
- queue sizing and backpressure policy;
- provider/runtime error translation.

### Dependency strategy

`SessionEngine` is constructed with Voice Provider and Agent Runtime ports. Production adapters are Gemini Live and Hermes; deterministic adapters replace them in tests. Clock/timer dependencies remain internal seams.

### Trade-offs

**Depth:** high. Callers learn a small lifecycle vocabulary (`Open`, `Attach`, `Detach`, `End`) and gain the full realtime/session behavior.

**Locality:** highest of the candidates. Session lifecycle stays in the Session Engine while transport connection lifecycle stays in `ClientLink`.

**Seam placement:** matches the domain seam directly: Transport attaches to a Voice Session; Voice Provider and Agent Runtime remain implementation dependencies.

**Cost:** a few more methods and types than Design A. The distinction between `Detach` and `End` must be documented clearly. `SessionInput` / `SessionOutput` must remain narrow typed domain variants rather than becoming a generic event bus.

**Verdict:** strongest candidate.

---

## Design C — reducer + effects

### Interface shape

```go
type SessionMachine interface {
    Transition(state SessionState, event SessionEvent) (SessionState, []Effect, error)
}
```

Effects represent actions such as sending provider audio, starting a Delegation, cancelling playback, reconnecting a provider, or emitting client output. An outer runner executes effects and feeds resulting events back into the reducer.

### Usage

```go
state, effects, err := machine.Transition(state, ClientAudio{...})
for _, effect := range effects {
    runner.Execute(effect)
}
```

### Invariants / ordering / errors

The reducer can make state transitions completely deterministic. Ordering is explicit in the event sequence. Invalid transitions return domain errors.

### What it hides

Pure transition rules are hidden, but I/O orchestration, effect execution, queueing, retry, provider/runtime adapters and lifecycle ownership move into the runner/caller composition.

### Dependency strategy

The reducer itself has almost no external dependencies. Provider/runtime ports are consumed by an outer effect runner.

### Trade-offs

**Depth:** deceptively low. The type surface is small, but callers must understand the entire event/effect protocol and state machine.

**Locality:** poor for the gateway's hardest behavior because orchestration is split between reducer and runner.

**Strength:** outstanding for deterministic state-machine testing and useful as an **internal implementation technique**.

**Weakness:** turns the public Session Engine into a generic event machine, which ADR-0002 explicitly tries to avoid.

**Verdict:** reject as the external interface; keep as a possible internal pattern.

---

## Design D — explicit orchestration ports

### Interface shape

```go
type SessionEngine interface {
    StartSession(ctx context.Context, client ClientPort, voice VoiceProviderPort, agent AgentRuntimePort) error
}
```

Variants of this design expose callbacks or separate ports for media input/output, provider events, Agent Runtime events, persistence, clock, reconnect, and approval interactions.

### Usage

```go
err := engine.StartSession(ctx, wsClient, gemini, hermes)
```

### Invariants / ordering / errors

Each port documents its own ordering and failure modes. The Session Engine coordinates them.

### What it hides

Turn and session behavior can remain inside the implementation, but callers must assemble the dependency graph and understand which adapters participate in every Voice Session.

### Dependency strategy

Ports & adapters are explicit at the public seam. Tests pass fake implementations directly.

### Trade-offs

**Depth:** medium. The module hides orchestration logic but exposes too much dependency structure.

**Locality:** weaker than Design B because changes in provider/runtime capabilities can alter construction/composition call sites.

**Seam placement:** Voice Provider and Agent Runtime are real seams, but exposing all seams through the Session Engine's caller-facing interface confuses external interface with internal dependency injection.

**Strength:** straightforward dependency replacement.

**Weakness:** risks a shallow "orchestrator" where callers know nearly as much about composition as the implementation.

**Verdict:** use ports & adapters **inside** the Session Engine implementation, not as its caller-facing shape.

---

## Comparison

| Design | Depth / leverage | Locality | Voice Session != connection | Test surface | Leakage risk |
| --- | --- | --- | --- | --- | --- |
| A — `Run` stream | Very high superficially | High while attached | Weak / implicit | Simple | Lifecycle conventions leak |
| B — Session Handle | High | **Highest** | **Explicit** | Strong | Low if I/O variants stay narrow |
| C — Reducer/effects | Low externally despite tiny signature | Split | Possible | Excellent for transitions | Event/effect machine leaks |
| D — Public ports | Medium | Medium | Explicit | Strong | Dependency graph leaks |

## Recommendation

Adopt **Design B — long-lived Session Handle with attachment-aware stream** as the external interface shape.

Use ideas from the other designs internally:

- from **A**, keep the transport-facing link as a simple typed input/output stream;
- from **C**, allow the owner goroutine to use reducer-like deterministic state transitions internally where that improves race-sensitive tests;
- from **D**, inject Voice Provider and Agent Runtime ports into the Session Engine implementation, with production and deterministic test adapters.

This combination produces the deepest useful seam without hiding the domain distinction between a Voice Session and a network attachment.

### Public knowledge budget

A caller should need to know only:

1. how to open/resume a Voice Session;
2. how to attach/detach a Voice Client transport;
3. which typed client inputs it can send;
4. which typed session outputs it can receive;
5. how to explicitly end the Voice Session.

A caller should **not** know about provider connection IDs, provider resumption handles, provider tool-call IDs, Hermes JSON-RPC methods, Turn epoch mechanics, retry/backoff, queue internals, or goroutine ownership.

## Phase 1 constraints derived from the design

- implement the interface test-first with deterministic Voice Provider and Agent Runtime adapters;
- keep `SessionInput` and `SessionOutput` closed/narrow at first; do not add `map[string]any` extensibility;
- model `Detach` separately from Voice Session termination from the first test;
- write interruption tests through the public interface, proving stale playback/completion never crosses the seam;
- write re-attachment tests through the public interface, proving Voice Session state survives a client link replacement;
- do not expose provider/runtime SDK types even temporarily in the walking skeleton.
