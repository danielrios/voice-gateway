# Issue tracker: GitHub

Issues and specs for this repository live in GitHub Issues for
`danielrios/voice-gateway`. Use the `gh` CLI for all operations.

## Conventions

- Create issues with `gh issue create`.
- Read issues and comments with `gh issue view <number> --comments`.
- List and filter issues with `gh issue list`.
- Comment with `gh issue comment <number>`.
- Apply or remove labels with `gh issue edit`.
- Close issues with `gh issue close`.
- Link implementation tickets to their originating specification or decision.
- Use GitHub-native dependencies for blocking relationships when available.
- Record meaningful investigation results and decisions in comments.
- Include enough context when closing an issue to explain the outcome.

Infer the repository from `git remote -v`; `gh` does this automatically inside
the clone.

## Pull requests as a triage surface

**PRs as a request surface: no.**

Pull requests represent proposed changes. Planning and unresolved work belong
in issues.

## Skill operations

When a skill says "publish to the issue tracker," create a GitHub issue.
When a skill says "fetch the relevant ticket," read the GitHub issue and its
comments.

For wayfinding, use one `wayfinder:map` issue with linked child issues. Prefer
GitHub sub-issues and native issue dependencies; use task-list and `Blocked by`
fallbacks only when those features are unavailable.
