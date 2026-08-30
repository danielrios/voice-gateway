# Domain Docs

Voice Gateway is a single-context repository.

## Before exploring

- Read the root `CONTEXT.md` for canonical domain language.
- Read relevant decisions under `docs/adr/`.
- If either is absent, proceed silently.

## Vocabulary

Use terms from `CONTEXT.md` consistently in code, tests, issues, and design
notes. If a needed term is missing, treat it as a domain-modeling question
instead of silently inventing a synonym.

## Decisions

Surface any conflict with an existing ADR explicitly rather than silently
overriding it.

`CONTEXT.md` is a glossary, not a specification or implementation notebook. ADRs record only durable decisions that are costly to reverse, surprising without context, and the result of a real trade-off.

Do not create additional contexts or a root `CONTEXT-MAP.md` unless the
repository develops multiple independently meaningful domain contexts.
