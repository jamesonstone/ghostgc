---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: complete
feature:
  id: 0014
  slug: operational-diagnostics
  dir: 0014-operational-diagnostics
relationships:
  - type: builds_on
    target: 0012-simple-startup-modes
  - type: builds_on
    target: 0013-follow-logs-stop-command
references:
  - id: issue-33
    name: Operational diagnostics and task lifecycle issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/33
    relation: guides
    read_policy: must
    used_for: scope and acceptance criteria
    status: active
  - id: safety-model
    name: Ghostgc safety model
    type: reference
    target: docs/references/safety-model.md
    relation: constrains
    read_policy: must
    used_for: ownership and action authority
    status: active
  - id: testing-contract
    name: Testing and environment validation rule
    type: ruleset
    target: docs/references/rules/testing-and-environment-validation.md
    relation: constrains
    read_policy: must
    used_for: validation boundaries
    status: active
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Make audit-mode dogfooding explain what Ghostgc is waiting for, while using
Codex task lifecycle evidence instead of treating the long-lived desktop app
backend as the lifetime of every task.

## CONTEXT

Codex for Mac keeps one `codex app-server` process alive across many tasks.
Process ancestry therefore proves that descendants belong to the app backend,
but the backend's continued existence does not prove that an individual task
is active. Codex's rollout JSONL provides a narrower provider-owned signal: an
exact thread ID in `session_meta`, followed by `task_started` and
`task_complete` lifecycle events.

That provider file is evidence, not new authority. Ghostgc may map a process
to a task only when the process carries the same native thread ID and retains
intact ancestry to an already identified Codex root. The rollout must be an
owner-controlled regular file under the exact physical `CODEX_HOME/sessions`
tree, its metadata ID must match, and its latest lifecycle event must fit
within a bounded tail read. Any missing, ambiguous, malformed, linked,
foreign-owned, oversized, or conflicting evidence leaves the process on the
existing host session and protected.

## REQUIREMENTS

- Discover Codex task sessions only from exact native UUID thread IDs already
  observed in process environments or previously committed task sessions.
- Validate one exact rollout as a regular same-user file beneath a physical,
  non-linked Codex home; read only bounded metadata and tail bytes and never
  retain rollout content.
- Treat only the latest valid `task_started` or `task_complete` event as task
  lifecycle evidence. Unknown evidence cannot end a task.
- Reassociate an intact descendant from the host session to the provider task
  only when ancestry and its native ID agree. Preserve the host-root confidence
  and durable recorded ownership after later reparenting.
- A completed task may satisfy the session-ended classification input, but all
  existing detached, complete-activity, stability, exact-policy, protection,
  approval, revalidation, and signal gates remain mandatory.
- Generic `node`, shell, Python, `gh`, build, editor, server, and other broad
  executable classes remain non-overridable protections regardless of task
  lifecycle evidence.
- `ghostgc candidates` and `ghostgc status` expose a current candidate funnel:
  active sessions, exact policy-executable matches, orphaned classifications,
  and matched policy decisions.
- Followed `ghostgc logs` excludes `process.attributed` noise by default.
  `--verbose` and `-v`, or an explicit `--kind process.attributed`, restore it;
  one-shot log queries retain the complete historical default.
- macOS service status accepts the quoted, whitespace-padded keys emitted by
  `launchctl list` and reports its PID as running.
- `ghostgc config show` reports the active daemon's effective startup-profile
  configuration. When no daemon is reachable, it reports the effective audit
  profile that a default `ghostgc start` would load.
- No implementation or validation step restarts, stops, or mutates the user's
  installed Ghostgc service.

### Non-goals

- Treating a rollout filename, modification time, thread environment variable,
  or task completion event alone as ownership or cleanup authority.
- Scanning rollout contents, indexing every Codex thread, inferring lifecycle
  from inactivity, or depending on Codex's private SQLite schema.
- Broadening the default process policy beyond exact
  `chrome-headless-shell`, enabling automatic cleanup, or weakening any hard
  protection.
- Changing the live daemon or LaunchAgent during tests.

### Observable acceptance

- A fixture app backend with two task descendants produces distinct task
  sessions when their exact rollouts are active/completed; a completed task's
  intact descendant is still not orphaned.
- A later detached, known-idle exact helper can become orphaned only after the
  completed provider task and the existing five-minute window; a generic
  runtime remains protected under the same evidence.
- Symlinked, foreign-owned, mismatched, missing, ambiguous, malformed, and
  over-bound rollout fixtures create no provider task session.
- Candidate and status human/JSON output explain an empty funnel without
  requiring audit-log archaeology.
- Follow tests cover default noise suppression, verbose access, explicit-kind
  access, cursor progress, and one-shot compatibility.
- Service parsing and active/fallback effective-config reporting have focused
  tests.
- `make check`, `make race`, `make lint`, `make size`, `git diff --check`, and
  `kit check operational-diagnostics` pass; affected source and test files stay
  within 300 physical lines.

## ACCEPTED PLAN

1. Add an optional adapter lifecycle contract and a bounded Codex rollout
   reader that emits task sessions only after exact path, ownership, metadata,
   lifecycle, and host-root checks.
2. Persist provider task sessions beside their long-lived host session and
   prefer the task mapping only when native identity and intact ancestry agree;
   keep durable ownership and all protection gates unchanged.
3. Add one shared candidate-funnel projection to status and candidate APIs and
   render the exclusion counts concisely.
4. Add followed-log noise exclusion with explicit verbose/filtered escape
   hatches, fix launchctl key parsing, and expose the daemon's effective config.
5. Validate with deterministic fixtures and the complete repository gates,
   then curate durable rationale and user-facing behavior.

## DECISIONS

### Rollout lifecycle supplements process ownership

The rollout can say when a provider task starts or completes, but it cannot say
that an arbitrary process belongs to that task. Task association therefore
requires both an exact native ID and intact ancestry to the Codex host root.
Once recorded, the existing exact-process ownership record survives a later
reparenting event.

### Unknown lifecycle preserves the host-session model

Failure to find or validate lifecycle evidence is not a daemon failure and is
never converted into task completion. The process remains attributed through
the existing host ancestry, whose active state keeps it protected.

### Log filtering advances on the server-side audit cursor

Default follow mode excludes attribution rows in the storage query, so a burst
of noise cannot hide older high-signal entries behind the initial limit. The
durable audit ID remains the cursor and explicit filters retain their existing
semantics.

## DISCOVERIES

- Real Codex rollout files emit `task_started` and `task_complete`; completion
  is not inferred from file modification time.
- Rollouts can exceed 100 MiB, so full-file parsing on each 15-second process
  scan would be unsafe and unnecessarily expensive. The latest lifecycle event
  is near the file tail and can be read within an explicit byte ceiling.
- `launchctl list` pads quoted dictionary keys before `=`, so trimming quotes
  before surrounding whitespace leaves the closing quote in the parsed key.

## VALIDATION

- Focused Codex rollout, provider-session, daemon policy, API, storage, CLI,
  protection and macOS service tests pass. Fixtures cover active/completed
  tasks, unsafe or ambiguous rollouts, intact ancestry, generic-runtime
  protection, log filters, candidate diagnostics and effective configuration.
- `make check`, `make race`, `make lint`, `make size`, and `git diff --check`
  pass. The race build emits the existing non-failing macOS malformed
  `LC_DYSYMTAB` linker warnings.
- `kit check operational-diagnostics` passes.
- `kit check --project` remains non-green on unchanged repository contract
  debt: the missing project progress summary, legacy feature sections,
  instruction-support warnings, and a due project refresh. No finding names
  implementation or reference behavior introduced by this feature.
- An isolated temporary daemon discovered the current Codex task as a distinct
  active provider session, reported the four-stage candidate funnel and active
  audit startup profile, and left the installed service unchanged. The new
  service-status parser also read the existing LaunchAgent as running with its
  PID. The fixture database contained zero policy decisions or cleanup
  candidates.

## OUTCOME

- Codex task lifecycle now supplements the long-lived app backend: exact,
  bounded rollout evidence creates active or completed task sessions only when
  native identity and host ancestry agree.
- Candidate and status output expose the first unsatisfied policy stage instead
  of requiring audit-log inference.
- Followed logs suppress repetitive attribution entries by default while
  verbose, explicit-kind and one-shot modes preserve access to complete detail.
- Service status correctly parses launchctl output, and configuration display
  reports the active daemon's effective startup profile or a labelled default
  audit preview when no daemon is reachable.
- `gh` joins the non-overridable generic-runtime protections; every existing
  detachment, activity, stability, policy, approval, revalidation and exact
  signal gate remains intact.

## REPOSITORY MEMORY

This spec owns the distinction between long-lived Codex hosts and provider task
lifecycle, the bounded rollout-evidence contract, candidate-funnel diagnostics,
followed-log signal defaults, and effective configuration reporting. The
project-wide lifecycle authority boundary is promoted to
`docs/CONSTITUTION.md`; its implementation map lives in
`docs/references/safety-model.md`, and the optional adapter flow lives in
`docs/references/architecture.md`.
