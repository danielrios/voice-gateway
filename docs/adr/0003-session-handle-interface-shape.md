---
status: accepted
---

# Use a long-lived Session Handle with attachment-aware client links

The Session Engine must preserve the domain distinction between a **Voice Session** and an individual client network connection while hiding Turn epochs, interruption, playback validity, provider resumption, Delegations, Announcements, and backpressure.

After a design-it-twice comparison of a minimal `Run` stream, a Session Handle, a reducer/effects machine, and public orchestration ports, we choose a **long-lived Session Handle with attachment-aware client links** as the external interface shape.

The caller-facing knowledge is limited to opening/resuming a Voice Session, attaching/detaching a Voice Client transport, sending typed session input, observing typed session output, and explicitly ending the Voice Session.

## Considered Options

- **Single `Run` command/stream** — deepest-looking surface, but couples call/connection lifetime too closely to Voice Session lifetime and pushes dormant/re-attachment conventions to callers.
- **Reducer + effects** — excellent internal testing technique, but exposes the event/effect state machine and splits orchestration locality between reducer and runner.
- **Public ports-and-adapters orchestration** — makes dependencies replaceable but leaks the Session Engine's dependency graph into callers.

## Consequences

- Transport attachment lifetime is explicit and separate from Voice Session termination.
- A Voice Session has at most one active `ClientLink`. A new attachment atomically supersedes the previous link so reconnect races do not strand a Voice Client; the superseded link becomes terminal and can no longer send or receive current-session output.
- Detach is safe to repeat and never implies Voice Session termination. Explicit `End` terminates the Voice Session and invalidates any active link.
- Media from an invalidated link/Turn is never replayed after re-attachment. Durable control state and pending Announcements may survive detach according to Session Engine policy.
- Voice Provider and Agent Runtime ports remain implementation dependencies, not caller-facing parameters; production and deterministic test adapters satisfy those seams.
- `SessionInput` and `SessionOutput` are gateway-owned, narrow typed domain variants; provider/runtime protocol objects and generic `map[string]any` extension points do not cross the interface.
- reducer-like transitions may be used internally where useful, but they are not the public interface.
- Phase 1 tests must exercise interruption, stale-output rejection, link supersession, detach/re-attach, Delegation completion, idempotent detach/end behavior, and termination through the Session Engine interface rather than internal state.
- the design-it-twice exploration is recorded in `docs/design/session-engine-interface.md`; exact Go type details may still be refined by the walking skeleton as long as this interface shape and its invariants remain intact.
