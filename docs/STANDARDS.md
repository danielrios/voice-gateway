# Engineering Standards

## Scope

These standards keep the first implementation small, observable, and safe under realtime concurrency. They are defaults, not an excuse to add infrastructure before a need exists.

## Go

- Go 1.26 module baseline.
- `gofmt` is mandatory.
- `go vet ./...` and `go test ./...` must pass in CI.
- Prefer the standard library unless a dependency removes meaningful protocol or platform complexity.
- No dependency-injection framework.
- No generic plugin framework in Phase 1.
- Avoid reflection in runtime paths unless required by an SDK.

## Package direction

The eventual package layout should follow domain ownership, not technical layers. A likely starting shape is:

```text
cmd/voice-gateway/
internal/session/
internal/provider/gemini/
internal/agent/hermes/
internal/transport/ws/
internal/observability/
```

This is not a mandate to create one package per noun. Packages should be deep modules; delete pass-through packages rather than preserve a diagram.

## Concurrency

- One owner goroutine mutates each Voice Session.
- No goroutine may mutate session state directly from a provider callback.
- External callbacks translate to typed events and enqueue them.
- All queues are bounded.
- Every goroutine must have a clear cancellation owner.
- Use `context.Context` for lifetime/cancellation, not as a bag of dependencies.
- Do not use sleeps for synchronization in tests.

## Errors

- Preserve the causal error with `%w` when wrapping.
- Normalize external/provider errors at adapter seams only when the Session Engine needs semantic behavior such as retryable, terminal, or authentication failure.
- Do not create a taxonomy of errors before behavior differs.
- Logs must not contain provider keys, Agent Runtime credentials, secret Interaction values, or raw authorization headers.

## Events

- Events describe facts that happened, not commands disguised as past tense.
- Event payloads carry stable correlation IDs when they cross asynchronous lifecycles.
- Provider-native event objects never cross the Voice Provider seam.
- Hermes-native event objects never cross the Agent Runtime seam.
- Control events must never be silently dropped.

## Audio

- The Session Engine treats audio as timestamp/order-sensitive binary media, not JSON payloads.
- Avoid base64 internally; encode only when an external protocol requires it.
- Avoid transcoding in the core.
- Phase 1 canonical client format is PCM16 mono; exact sampling negotiation belongs to transport/provider adapters.
- After interruption, queued audio from the invalidated playback epoch is discarded.

## Interfaces and adapters

- The interface is the test surface.
- Keep interfaces owned by the module that consumes the dependency.
- Introduce an external seam only when a production adapter plus a test adapter is justified, or multiple production adapters are real near-term requirements.
- Avoid one-method pass-through wrappers that add names but hide no complexity.
- Provider/runtime capability differences should be represented as narrow capabilities, not `map[string]any` escape hatches.

## Testing

- Prefer table tests for state-machine behavior.
- Use deterministic in-memory adapters for Session Engine tests.
- Inject clocks where time changes behavior.
- Run `go test -race ./...` in CI once implementation begins.
- Adapter integration tests are separate from core behavior tests.
- Real-provider smoke tests are opt-in and credential-gated.
- Tests should survive internal refactors; if an internal rename breaks many tests, the test surface is too low-level.

## Observability

Start with:

- structured `log/slog` logging;
- counters/gauges/histograms through a small internal metrics facade only when the first metrics are implemented;
- correlation fields: Voice Session, Turn epoch, Delegation, provider connection.

Do not add OpenTelemetry in Phase 1 solely for future-proofing. Add it when distributed traces can actually span multiple meaningful hops.

## Security

- Secrets are server-side by default.
- Client and Agent Runtime authentication are distinct concerns.
- Bind local development listeners to loopback unless explicitly configured otherwise.
- Validate all control-message sizes and enum values at transport ingress.
- Apply explicit max frame/message sizes.
- Sensitive Interaction values require an explicit policy before they may be sent to a Voice Provider.

## Compatibility

The first implementation is pre-1.0. Internal packages may evolve freely. Any protocol exposed to Voice Clients must be versioned from its first committed implementation so constrained devices can upgrade independently from the gateway.
