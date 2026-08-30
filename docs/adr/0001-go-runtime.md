# ADR-0001: Use Go for the gateway runtime

- Status: Accepted
- Date: 2026-08-29

## Context

Voice Gateway is a long-lived realtime process expected to run on small servers and ARM64 machines while maintaining multiple streaming network connections, bounded media queues, and independent agent-runtime work.

The first Voice Provider, Gemini Live, has an official Go SDK with Live support. The project also needs simple cross-compilation, predictable memory use, low operational overhead, and a concurrency model that makes per-session ownership straightforward.

Alternatives considered:

- TypeScript/Node.js: excellent Gemini ecosystem fit and proven by Iris, but higher runtime footprint and less attractive as a small standalone gateway binary.
- Kotlin/JVM: strong language/runtime and aligned with Quark, but makes this independent edge/gateway process heavier than necessary and couples two projects' technology choices without a domain reason.
- Rust: excellent runtime characteristics, but materially higher implementation complexity for the current team/project phase with little expected product-level latency advantage over Go for network-bound provider calls.

## Decision

Use Go as the implementation language for Voice Gateway.

Baseline:

- Go 1.26 language/module baseline;
- CI tests current baseline and the current stable Go release;
- standard library first;
- official Google Gen AI Go SDK for Gemini Live;
- goroutine-per-session owner loop rather than shared mutable state.

## Consequences

Positive:

- single small deployable binary;
- straightforward Linux/ARM64 builds;
- good fit for streaming I/O and session ownership;
- official Gemini Live SDK support;
- gateway technology remains independent from Agent Runtime technology.

Negative:

- Hermes and Iris examples cannot be reused directly because they are Python/JavaScript-oriented;
- future browser-facing code still needs a separate client implementation;
- some realtime AI SDK examples appear first in Python/TypeScript, requiring occasional protocol translation.

## Revisit when

Revisit only if a required Voice Provider lacks a viable Go SDK/protocol implementation, or measured gateway CPU/memory/latency shows Go itself to be a blocker.
