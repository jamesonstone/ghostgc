---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: complete
feature:
  id: 0015
  slug: startup-log-following
  dir: 0015-startup-log-following
relationships:
  - type: builds_on
    target: 0012-simple-startup-modes
  - type: builds_on
    target: 0013-follow-logs-stop-command
references:
  - id: issue-35
    name: Startup log following issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/35
    relation: guides
    read_policy: must
    used_for: scope and acceptance criteria
    status: active
  - id: testing-contract
    name: Testing and environment validation rule
    type: ruleset
    target: docs/references/rules/testing-and-environment-validation.md
    relation: constrains
    read_policy: must
    used_for: command and service-boundary validation
    status: active
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Let an operator start Ghostgc and immediately watch its useful audit stream with
one command.

## CONTEXT

`ghostgc start` safely installs or refreshes the background service, while
`ghostgc logs` follows its high-signal audit entries. Running both is the normal
dogfooding workflow, but it currently requires two commands. Service
registration is asynchronous, so immediately opening the control socket can
briefly fail even after installation succeeds.

The combined command must reuse the existing log command rather than introduce
a second filter, cursor, or rendering path. It may retry only the expected
daemon-unreachable startup race for a bounded interval; other log errors must
remain visible.

## REQUIREMENTS

- `ghostgc start --logs` installs or refreshes the audit-mode service, then
  follows the same default high-signal stream as `ghostgc logs`.
- `ghostgc start --mode reconcile --logs` preserves the reconciliation startup
  ceiling before following logs.
- Without `--logs`, startup behavior and output remain unchanged.
- The combined command retries only `api.ErrDaemonUnreachable` for a bounded
  readiness interval after successful service installation.
- Non-readiness errors return immediately. Exhausted readiness and installation
  failures return errors rather than pretending logs are being followed.
- Ctrl-C stops only the foreground log view successfully; the registered
  background service continues running.
- Help, README, operator guidance, and dogfooding guidance expose the shortcut.

### Non-goals

- Adding log flags to `start`, changing default log filters, or implementing a
  new streaming protocol.
- Stopping the service when the foreground log view ends.
- Changing audit, reconciliation, automatic cleanup, or filesystem authority.
- Mutating the user's installed service during validation.

### Observable acceptance

- Parsing accepts `--logs` with audit, reconcile, shadow, and live mode aliases
  and still rejects automatic enforcement.
- A fake service install followed by one transient unreachable response enters
  the existing default-follow stream with attribution noise excluded.
- Cancellation exits successfully and leaves the fake service installed.
- A persistent unreachable response times out with a useful error, while an
  unrelated log error is not retried.
- Focused tests and all repository validation gates pass.

## ACCEPTED PLAN

1. Parse startup mode and log-following intent together without changing the
   service arguments.
2. After successful installation, delegate to the existing logs command and
   retry only its initial daemon-unreachable result within a short bound.
3. Add deterministic fake-service and fake-log tests for mode preservation,
   transient readiness, cancellation, timeout, and unrelated errors.
4. Update concise user guidance, run complete validation, and curate the final
   behavior into repository memory.

## DECISIONS

### The shortcut delegates to the existing logs command

After installation, `start --logs` invokes the same command path as
`ghostgc logs` with no additional flags. Default filtering, initial history,
cursor progression, human rendering, JSON framing, cancellation, and future
log-command fixes therefore remain owned by one implementation.

### Readiness retry is narrow and bounded

The combined command retries only `api.ErrDaemonUnreachable` for ten seconds.
This covers the expected interval between service registration and Unix-socket
readiness without turning configuration, API, storage, or rendering failures
into an indefinite wait. Once the first log request succeeds, the ordinary
unbounded follow lifecycle owns the foreground command until cancellation.

## DISCOVERIES

- Service registration returns before the daemon's Unix socket is guaranteed to
  accept a request, so direct delegation without readiness handling would make
  the convenience flag intermittently fail.
- Installing before entering the foreground follow loop naturally leaves the
  background service registered when Ctrl-C cancels log viewing.

## VALIDATION

- Focused CLI tests cover mode aliases, the new flag, default high-signal log
  options, one transient readiness failure, clean cancellation, service
  survival, installation failure, bounded timeout, unrelated log errors, and a
  daemon failure after readiness without startup retry or relabeling.
- `make check`, `make race`, `make lint`, `make size`, and `git diff --check`
  pass. The race build emits the existing non-failing macOS malformed
  `LC_DYSYMTAB` linker warnings.
- A built command reports `ghostgc start [--mode audit|reconcile] [--logs]` and
  describes the flag as following the audit log after startup.
- `kit check startup-log-following` passes.
- `kit check --project` retains the existing repository-document baseline of 24
  blocking findings; none concern this feature spec or a changed file.

## OUTCOME

- `ghostgc start --logs` now starts the safe audit service and follows the
  ordinary high-signal audit stream in the same foreground invocation.
- `ghostgc start --mode reconcile --logs` preserves manual-reconciliation
  authority while providing the same log shortcut.
- The startup-to-socket race is retried narrowly for ten seconds; every other
  error remains visible and Ctrl-C leaves the background service running.
- README, operator, dogfooding, and command help now expose the shortcut.

## REPOSITORY MEMORY

This spec owns the cross-command startup/readiness contract. Existing startup
authority remains canonical in feature 0012, and log cursor/filter behavior
remains canonical in feature 0013. No project-wide safety invariant changed,
so `docs/CONSTITUTION.md` remains unchanged.
