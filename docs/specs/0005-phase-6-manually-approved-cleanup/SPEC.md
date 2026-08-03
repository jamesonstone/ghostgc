---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: implement
feature:
  id: 0005
  slug: phase-6-manually-approved-cleanup
  dir: 0005-phase-6-manually-approved-cleanup
relationships:
  - type: builds_on
    target: 0004-phase-5-fail-closed-policy-engine
references:
  - id: issue-11
    name: Phase 6 manually approved cleanup issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/11
    relation: guides
    read_policy: must
    used_for: scope and acceptance criteria
    status: active
  - id: safety-model
    name: Safety model
    type: documentation
    target: docs/references/safety-model.md
    relation: constrains
    read_policy: must
    used_for: authority and revalidation boundaries
    status: active
skills:
  - name: github:yeet
    source: GitHub plugin
    path: github:yeet
    trigger: publish the validated phase as a ready pull request
    required: true
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Turn an exact, current Phase 5 policy decision into a short-lived manual
approval and a fully revalidated SIGTERM, while preserving fail-closed behavior
at every unknown, stale or changed fact.

## CONTEXT

- Recommendation is not approval, and approval is not execution authority.
- A process is identified by PID plus start time. PID alone is never an action
  target and the platform must recheck the exact identity immediately before
  signalling.
- Hard protections remain non-overridable. A changed session, owner,
  classification, process tree, policy, executable or identity invalidates the
  approval.
- Phase 6 permits only an operator-requested SIGTERM. Automatic enforcement,
  SIGKILL, shell terminators and arbitrary signals remain absent.

## REQUIREMENTS

- Accept global and per-policy `recommend` mode while continuing to reject
  every `enforce` configuration. Global mode is a hard upper bound.
- Expose current recommendations separately from audit-only and enforceable
  decisions. No Phase 6 decision is enforceable.
- Preview one exact policy/process recommendation. Return a single-use,
  short-lived approval plus the exact CLI command and the revalidation contract.
- Bind approval to the current committed decision, exact process identity,
  observed executable path and kernel name, session, classification evidence
  and canonical policy definition. Keep the bearer token only in daemon memory,
  redact its CLI flag from observed command lines, and never persist it.
- Consume approval once. Reject expired, replayed, stale, malformed or changed
  approvals without signalling.
- Serialize action revalidation with scans. Take a new process snapshot,
  reconcile ownership and lifecycle, resample activity, reclassify, rerun hard
  protections and reevaluate the exact policy before acting.
- Make the platform signal method accept an exact process key plus the bound
  executable path and kernel name, validate all of them inside the platform
  implementation immediately before the system call, and accept only SIGTERM.
- Persist the action attempt before the side effect, then durably record
  signalled, rejected or failed completion evidence. Expose action history via
  API, CLI, audit log and metrics.
- Preserve retention, privacy, owner-only local socket and the 300-line source
  file limit.

### Non-goals

- Background or automatic action, policy `enforce`, SIGKILL, signal escalation,
  shell execution, process-group signalling, remote approval or token storage.
- Acting on `suspicious`, active-session, interactive, broad-runtime,
  low-confidence, agent-root, descendant-owning or otherwise protected targets.
- Treating a successful `kill(2)` return as proof that a process exited.

### Observable acceptance

- Config and policy tests prove recommend is bounded by global mode and enforce
  is rejected.
- Approval tests prove exact binding, expiry, replay refusal and invalidation on
  every changed fact.
- Platform source tests permit exactly one literal SIGTERM system-call site and
  reject all SIGKILL or shell-terminator paths.
- Daemon tests prove rejected actions never increment signal attempts and an
  eligible fixture-owned target receives exactly one SIGTERM.
- Live evidence previews and terminates only the dedicated fixture-owned idle
  child, proves exact identity disappearance, and cleans up every fixture.

## ACCEPTED PLAN

1. Add strict recommend-mode configuration and recommendation projection.
2. Add ephemeral approval binding and preview API/CLI.
3. Add durable action records and pre/post-side-effect audit transactions.
4. Add serialized full pre-action revalidation and exact-key SIGTERM platform
   support.
5. Extend fixtures, docs, metrics and live validation; then publish for an
   independent pull-request review.

## DECISIONS

- Approval is an opaque, random, single-use bearer secret with a two-minute
  lifetime. Only its digest and binding are held in memory; neither is durable
  authority after daemon restart.
- The current evaluation identity is part of the binding. A later policy scan
  invalidates the approval even when it reaches the same conclusion.
- Cooldown decisions remain previewable recommendations because cooldown
  suppresses repeated audit noise rather than revoking an otherwise current
  match. Fresh revalidation ignores the old cooldown and must independently
  prove eligibility.
- An action row in `attempting` state is committed before SIGTERM. If the daemon
  cannot record completion after the system call, the durable record remains
  conservatively unresolved instead of claiming success.
- A `signalled` result means only that the exact-key SIGTERM call succeeded.
  Later observation, not the action endpoint, proves exit.

## VALIDATION

- `make check` — PASS; vet, source-size and all package tests.
- `make race` — PASS; all packages under the race detector.
- `make lint` — PASS with zero issues.
- `git diff --check` — PASS.
- All handwritten Go source and test files remain at or below 300 lines; the
  largest remains 296 lines.
- Action unit and socket tests prove exact recommendation projection, preview,
  single use, expiry, replay rejection, changed-key and changed-image refusal,
  token redaction/non-persistence, pre-side-effect durability, metrics and
  structured history.
- Platform source tests prove exactly one literal SIGTERM system-call site,
  reject alternate signalling primitives, SIGKILL and shell terminators, and
  exercise non-TERM, changed-key and changed-image refusal against the running
  collector.
- Live run `20260803T180100Z-p6a5` at exact source/deployed `1e3cb06` used only
  fixture target `44801:1785780083631995000`. It reached orphaned after more
  than five continuous idle minutes, appeared only as recommended, issued a
  memory-only preview, committed `action.attempting` before one SIGTERM,
  recorded `action.signalled`, observed the target absent, and proved every
  non-target survivor still alive before complete teardown. Metrics reported
  1 attempted, 0 rejected and 1 completed action. Structured evidence is under
  `tmp/2026-08-03/phase6-manual-cleanup/5/`.
- Failed live attempts 1-4 are preserved literally. They prove that absent
  survivors, inherited TTY, broken cadence continuity and real periodic CPU
  activity each prevent the positive action path rather than being waived.

## OUTCOME

Phase 6 is complete. ghostgc now distinguishes recommendations from audit-only
decisions, issues exact short-lived single-use approvals, revalidates every
identity, ownership, lifecycle, activity, classification, policy and hard
protection fact, and durably brackets one exact-key SIGTERM with pre/post action
evidence. Audit remains the default and no automatic or SIGKILL path exists.

## REPOSITORY MEMORY

- Decision: created
- Rationale: Phase 6 is the first action-capable delivery and its manual
  authority, revalidation and side-effect audit ordering must be explicit.
- Artifacts: `docs/specs/0005-phase-6-manually-approved-cleanup/SPEC.md`
