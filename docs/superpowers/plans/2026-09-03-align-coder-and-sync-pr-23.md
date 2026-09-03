# Align CODER Agent and Sync PR 23 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align the CODER agent with the repository's invariant-focused gauntlet and synchronize PR 23 with the current `main`.

**Architecture:** Update only the CODER agent instructions, preserving its existing frontmatter and tool list. Fetch `origin`, then merge `origin/main` into the clean `pr-23` branch so existing PR work remains intact and conflicts are handled explicitly.

**Tech Stack:** Git, Markdown, Go project quality gates.

**Spec:** User request in the current task.

## Global Constraints

- Do not implement the PR 23 bounded-media-queue bug fix.
- Do not weaken tests or resolve races with sleeps.
- Preserve existing branch work and agent style/tools.

---

### Task 1: Update CODER instructions

**Files:**
- Modify: `.agents/agents/coder/agent.md`

- [ ] Add requirements to read the specification and acceptance criteria first, identify invariants, reason about concurrency/realtime/state-machine/queue/backpressure/cancellation/invalidation/ordering interleavings, and account for stale local copies, check-then-act, and queued-vs-in-flight state.
- [ ] Add deterministic invariant-focused regression tests, prohibition on weakened tests and sleeps, focused-change guidance, and the requested Go quality gates.

### Task 2: Synchronize PR 23

**Files:**
- Git history and working tree on branch `pr-23`.

- [ ] Fetch remote references.
- [ ] Merge `origin/main` into `pr-23`, resolving any conflict without dropping PR changes.
- [ ] Verify status, relevant commits, and the final agent diff.
