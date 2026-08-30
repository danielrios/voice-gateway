---
status: accepted
---

# Use Go 1.27 for the gateway runtime

Voice Gateway is a long-lived realtime process expected to run on small servers and ARM64 machines while maintaining streaming network connections, bounded media queues, and independent Agent Runtime work. We choose Go 1.27 because it provides a small deployable runtime, straightforward cross-compilation, a concurrency model that fits per-session ownership, and an official Google Gen AI SDK that exposes Gemini Live sessions.

The Gemini Live surface in the Go SDK is currently marked Preview. That strengthens, rather than weakens, the need to keep provider SDK/protocol types behind the Voice Provider seam so upstream churn remains local to an adapter.

## Considered Options

- **TypeScript/Node.js** — excellent realtime-AI ecosystem fit and proven by Iris, but a larger runtime footprint and less attractive operational shape for a small standalone gateway binary.
- **Kotlin/JVM** — strong language/runtime and aligned with Quark, but would couple the gateway's technology choice to an Agent Runtime without a domain reason and increase the deployment footprint.
- **Rust** — excellent runtime characteristics, but materially higher implementation complexity with little expected product-level latency advantage for this network-bound system at the current phase.

## Consequences

- Initial module/toolchain baseline is Go 1.27, the current stable major release when this decision was recorded.
- Standard library is preferred where practical; the official Google Gen AI Go SDK is the initial Gemini adapter dependency.
- One owner goroutine per Voice Session is the default concurrency model, subject to validation in the Session Engine interface/design pass.
- Browser/device clients remain separate implementations and do not need to use Go.
- Revisit the language choice only if a required provider cannot be supported cleanly in Go or measurement shows the runtime itself is a material resource/latency blocker.
