---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: complete
feature:
  id: 0013
  slug: follow-logs-stop-command
  dir: 0013-follow-logs-stop-command
relationships:
  - type: builds_on
    target: 0012-simple-startup-modes
references:
  - id: issue-31
    name: Follow logs and stop command issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/31
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
    used_for: command and cursor validation
    status: active
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Make the two routine background-service operations direct: watch the audit log
with `ghostgc logs`, and stop the service with `ghostgc stop`.

## CONTEXT

Startup is already a single command, but inspecting ongoing audit activity
still requires an external `tail` command or repeated one-shot invocations.
Stopping also exposes the launch-service implementation detail through
`ghostgc service uninstall`. The primary CLI should own both common actions
while keeping the advanced service commands compatible.

## REQUIREMENTS

- `ghostgc logs` prints the current filtered history and follows new matching
  audit entries until interrupted.
- `--follow` and `-f` select following explicitly; `--follow=false` and
  `-f=false` provide a one-shot request.
- Follow mode preserves `--limit`, `--kind`, and `--subject` filters and emits
  every returned audit record once in insertion order.
- Follow-mode JSON is newline-delimited `LogsResponse` objects so each emitted
  batch remains independently decodable; one-shot JSON remains one object.
- Cancellation is a successful operator stop, while daemon/API failures remain
  errors.
- `ghostgc stop` uses the exact same service-uninstall implementation as
  `ghostgc service uninstall`; the existing command remains supported.
- Root help, README, and operator guidance expose `logs` and `stop` as the
  primary commands.

### Non-goals

- Streaming daemon stderr/stdout, adding a server-push protocol, changing audit
  retention, or changing cleanup authority.
- Removing or weakening the advanced `service` command.

### Observable acceptance

- A follow request emits initial entries oldest-first, then newly inserted
  entries once even when multiple entries share a timestamp.
- Cursor pagination drains bursts larger than one response without skipping
  older records.
- `ghostgc logs --follow=false` exits after its initial response.
- Interrupting follow mode exits cleanly.
- Both stop spellings uninstall `com.jamesonstone.ghostgc` through the platform
  abstraction and reject unexpected arguments.
- Focused tests and all repository checks pass.

## ACCEPTED PLAN

1. Add an audit-record ID cursor to the existing logs API and storage query;
   cursor requests return the oldest unseen page first.
2. Move logs command behavior into a focused CLI file, add default-on polling,
   short and long boolean flags, cancellation handling, and batch rendering.
3. Extract one shared background-service uninstall helper and register `stop`
   as its top-level alias.
4. Add focused storage, CLI, service and help tests plus concise usage docs.
5. Run complete validation and curate the resulting durable behavior.

## DECISIONS

### Follow uses the durable audit ID

Timestamps are not unique and the existing time filter is inclusive. Following
by timestamp would therefore need fragile de-duplication and could miss records
when one polling interval exceeds the response limit. The SQLite audit ID is a
monotonic insertion cursor and lets the client drain bounded pages without
duplicates or gaps.

### Stop remains service uninstallation

The supported service lifecycle already defines stopping as unregistering the
background service. The top-level alias delegates to that exact path rather
than introducing a second process-kill mechanism or leaving a disabled launch
registration behind.

## DISCOVERIES

- Existing audit responses include the durable SQLite ID but expose only an
  inclusive timestamp query.
- Existing log rendering reverses newest-first history for terminal reading;
  cursor pages need direct oldest-first rendering instead.

## VALIDATION

- Focused CLI, storage, API and daemon package tests pass.
- The follow test covers same-timestamp records, bounded multi-page draining,
  filter preservation, insertion order, cancellation, both opt-out flag names,
  and compact one-object-per-line JSON framing.
- The service fake proves the shared stop path removes the existing
  registration and rejects unexpected arguments without adding a process-kill
  path.
- `make check`, `make race`, `make lint`, `make size`, and `git diff --check`
  pass. The race build emits the existing non-failing macOS malformed
  `LC_DYSYMTAB` linker warnings.
- A built binary shows `start` and `stop` together in root help, both follow
  flags with their default in logs help, and a successful `ghostgc stop -h`.
- `kit check follow-logs-stop-command` passes.
- `kit check --project` remains non-green on the repository's unchanged legacy
  contract debt: the missing project progress summary, older V1/V2 spec
  sections, instruction-support warnings, and a due project refresh. No
  finding names a source or reference changed by this feature.

## OUTCOME

- `ghostgc logs` now shows recent filtered audit history and follows new records
  by default; `--follow=false` and `-f=false` retain one-shot operation.
- Follow mode advances by durable audit ID and drains oldest-unseen pages, so
  timestamp collisions and response-size bursts do not create gaps or
  duplicates.
- `ghostgc stop` and `ghostgc service uninstall` share one platform uninstall
  path; the advanced spelling remains available.
- Help, README, operator guidance, and dogfooding now lead with the direct logs
  and stop commands.

## REPOSITORY MEMORY

This spec owns default-follow audit-log behavior, cursor rationale, streamed
JSON framing, and the compatibility relationship between `stop` and
`service uninstall`. Reusable operator commands belong in the README and
operator guide; no safety-authority invariant is changed.
