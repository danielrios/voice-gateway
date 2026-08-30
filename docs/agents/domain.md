# Domain Docs

Voice Gateway is currently a single-context repository.

Before architectural or implementation work:

- read the root `CONTEXT.md` for canonical domain language;
- read relevant decisions under `docs/adr/`;
- use terms from `CONTEXT.md` consistently in code, tests, issues, and design notes;
- if a needed domain term is missing, treat that as a domain-modeling question rather than silently inventing a synonym;
- if proposed work contradicts an ADR, surface the conflict explicitly instead of silently overriding the decision.

`CONTEXT.md` is a glossary, not a specification or implementation notebook. ADRs record only durable decisions that are costly to reverse, surprising without context, and the result of a real trade-off.

Do not create additional contexts or a `CONTEXT-MAP.md` unless the repository actually develops multiple independently meaningful domain contexts.
