---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: implement
feature:
  id: 0003
  slug: phase-4-deterministic-classification
  dir: 0003-phase-4-deterministic-classification
relationships:
  - type: builds_on
    target: 0002-phase-3-activity-tracking
references:
  - id: issue-7
    name: Phase 4 deterministic classification issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/7
    relation: guides
    read_policy: must
    used_for: scope and acceptance criteria
    status: active
  - id: architecture
    name: Architecture reference
    type: documentation
    target: docs/references/architecture.md
    relation: constrains
    read_policy: must
    used_for: lifecycle, evidence and transaction boundaries
    status: active
  - id: safety-model
    name: Safety model
    type: documentation
    target: docs/references/safety-model.md
    relation: constrains
    read_policy: must
    used_for: unknown, protection and no-action guarantees
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

Turn Phase 3 activity samples into deterministic, inspectable process and
session classifications without confusing observed activity with lifecycle,
ownership, protection or permission to act.

## CONTEXT

- Session `state` already means lifecycle: starting, active or completed. An
  activity classifier must not overwrite that independent fact.
- Phase 3 stores independently available CPU, disk, file and socket evidence.
  Missing evidence is unknown, not inactivity.
- A detached process may be productive, idle, waiting or unknown. Detachment is
  therefore an orthogonal boolean and never a synonym for orphaned.
- Crashes are only observable when the kernel reports a zombie. A vanished root
  has no recoverable exit status and remains lifecycle-completed, not crashed.
- Orphaned and hung are strong conclusions. They require complete evidence to
  remain stable for a minimum five-minute window.

## REQUIREMENTS

- Classify exact-key, live, attributed processes from the current activity
  sample at the configured classification cadence.
- Produce `active` for CPU, disk or socket-queue movement; `waiting` for known
  inactivity while writable repository files or connected sockets remain;
  `idle` for complete known inactivity with neither; and `unknown` whenever a
  required evidence family or valid baseline is missing.
- Produce `suspicious` when a completed session retains a detached process that
  is active or waiting, `orphaned` only after detached known-idle evidence has
  remained stable for five minutes, `hung` only after a stopped process has
  remained known-inactive for five minutes, and `crashed` only for a zombie.
- Store `detached` separately from classification and explain its evidence.
- Reset stable windows across identity, attribution, availability, basis-state,
  lifecycle or detachment changes. Bound in-memory state to the current pass.
- Persist classification history, stable-window facts and evidence through an
  additive migration and the scan transaction; retain it with raw observations.
- Expose latest classifications through API, CLI, JSON, process/session views,
  explain output, status and metrics.
- Preserve every Phase 3 privacy, ownership, exact-key and no-signalling gate.

### Non-goals

- Parsing or evaluating policies, producing cleanup candidates or authorising
  any action.
- Guessing an exit cause when no exit status was observed.
- Treating age, executable name, detachment, lifecycle completion or protection
  alone as inactivity or disposability.
- Reclassifying unattributed processes or monitoring unrelated activity.

### Observable acceptance

- Deterministic tests cover each state, five-minute boundaries, evidence gaps,
  identity changes and detachment without orphaning.
- A fixture periodic worker is active and an idle worker becomes known idle;
  both keep the same lifecycle session state.
- API/CLI output includes evidence freshness, stable-since time and detachment.
- Candidates remain empty and source-level tests prove signalling is absent.
- All handwritten source/test files remain at or below 300 lines.

## ACCEPTED PLAN

1. Add a platform-neutral classification model with table-driven boundary tests.
2. Add an additive classification table, latest/history queries and retention.
3. Run a cadence-gated daemon classification pass after activity collection,
   replacing in-memory previous-state maps only after transactional persistence.
4. Add API/CLI views and enrich process, session, explain, status and metrics.
5. Update durable docs and validate deterministic, integration and live paths.

## DECISIONS

- Keep lifecycle `state`, activity `classification`, protection and future
  policy decisions as separate fields. Combining them would make a protected
  active process or a completed-but-working helper impossible to describe.
- Use a five-minute stable window for strong hung/orphaned conclusions. One
  quiet sample is enough to describe idle evidence, never abandonment.
- Restarting the daemon resets a stable window. Understating a strong conclusion
  is fail-closed and safer than reconstructing continuity across evidence gaps.

## DISCOVERIES

No additional information required before implementation.

## VALIDATION

- `make check`: pass (format, vet, source-file-size and full test suite).
- `go test ./internal/classification ./internal/storage ./internal/daemon`: pass.
- Classifier tests cover all immediate states, exact five-minute boundaries,
  detachment independence, PID reuse and evidence-gap reset.
- Storage tests cover schema v4, ordered history, latest-per-process queries and
  classification counts. Live fixture and pull-request CI evidence are recorded
  in `tests/RUN_STATUS.md`.

## OUTCOME

Phase 4 adds deterministic per-process classification without changing session
lifecycle state or introducing policy/action authority. Results preserve their
basis, detachment, stable-since time and evidence, are transactionally persisted
and retained, and are available through status, process/session/explain,
`/v1/classifications` and `ghostgc classifications`.

## REPOSITORY MEMORY

- Decision: created
- Rationale: separating lifecycle, activity, detachment, protection and policy
  is a durable safety boundary for every later phase.
- Artifacts: `docs/specs/0003-phase-4-deterministic-classification/SPEC.md`
