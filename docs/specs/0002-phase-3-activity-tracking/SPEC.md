---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: implement
feature:
  id: 0002
  slug: phase-3-activity-tracking
  dir: 0002-phase-3-activity-tracking
relationships:
  - type: builds_on
    target: 0001-session-aware-process-observation
references:
  - id: issue-5
    name: Phase 3 activity tracking issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/5
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
    used_for: staged collection and session graph boundaries
    status: active
  - id: safety-model
    name: Safety model
    type: documentation
    target: docs/references/safety-model.md
    relation: constrains
    read_policy: must
    used_for: fail-closed activity evidence and no-action guarantees
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

Add bounded, inspectable activity evidence for processes already attributed to
an agent session, so later classification and policy phases can distinguish
useful work from inactivity without guessing or inspecting unrelated processes.

## CONTEXT

- Phase 2 records cumulative CPU time, memory, status and process identity on
  every scan, but no query derives activity from consecutive observations.
- CPU inactivity alone is not enough: a waiting process may hold writable
  repository files, locks or live sockets while doing no CPU work.
- Open-file and socket enumeration is materially more expensive and more
  privacy-sensitive than the existing process scan. It must be limited to live,
  attributed, same-user processes at the separate activity cadence.
- Missing or unreadable counters are not zero. Later phases must be able to
  distinguish known inactivity from unavailable evidence.
- Socket and file-lock graph edges are already reserved as context-only
  relationships and may never establish ownership.

## REQUIREMENTS

- Model one activity sample against the exact `pid:start_time_ns` process key.
- Compute CPU and I/O deltas only across chronologically ordered samples for the
  same key. Counter resets, missing baselines and invalid intervals are unknown.
- Collect expensive file and socket evidence only for live processes attributed
  above the reporting threshold and only when the activity cadence is due.
- Record metric availability independently for counters, files and sockets.
- Persist only derived counts and safety-relevant booleans. Do not persist file
  contents, socket payloads, remote addresses, credentials or unrelated paths.
- Populate file-lock and socket relationships as context-only evidence.
- Keep activity storage bounded by raw-observation retention and the database
  byte ceiling through additive migrations.
- Expose latest activity and evidence through the Unix-socket API, CLI and JSON.
- Preserve all phase-2 safety, privacy, ownership and transactional guarantees.
- Add pull-request CI for the repository-required code-level checks.

### Non-goals

- Producing idle, waiting, suspicious, hung, crashed or orphaned classifications.
- Parsing or evaluating cleanup policies.
- Recommending, approving or sending process signals.
- Inspecting unattributed processes or reading source-code contents.
- Persisting socket endpoints or complete open-file inventories.

### Observable acceptance

- A periodic-work fixture reports a positive CPU or I/O delta while an idle
  fixture reports known zero only after two valid samples.
- A vanished, reused or unreadable process reports unavailable evidence rather
  than inactivity.
- API and CLI users can inspect freshness, deltas, file/socket counts and
  evidence completeness for each attributed process.
- Source-level tests still prove ghostgc cannot signal a process or read source
  contents, and all source/test files remain at or below 300 lines.

## ACCEPTED PLAN

1. Add a platform-neutral targeted activity contract and a deterministic fake
   implementation, then implement bounded macOS file-descriptor inspection.
2. Add a cadence-gated daemon activity pass after attribution, validating exact
   process keys and deriving deltas from the previous in-memory sample.
3. Add an additive activity table, bounded retention, queries and API/CLI views.
4. Populate context-only socket and file-lock edges from derived evidence.
5. Add focused unit, migration, retention, API and live-fixture coverage.
6. Wire repository-required checks to GitHub Actions and update durable docs.

## DECISIONS

- Persist derived activity facts instead of raw file paths or socket endpoints;
  later policy conditions need activity and safety state, not private inventory.
- Run the expensive pass only after session attribution. This avoids monitoring
  unrelated user activity and bounds work to the product's ownership domain.
- Keep activity separate from classification. Phase 3 records evidence; phase 4
  makes deterministic state conclusions from it.
- Treat socket and file-lock evidence as context-only regardless of confidence.

## DISCOVERIES

- macOS public `proc_pid_rusage` exposes cumulative disk bytes but not cumulative
  per-process network byte counters. Network evidence therefore uses bounded
  live socket counts, connection state and queue movement; it does not claim
  transmitted-byte totals the operating system did not provide.
- `PROC_PIDLISTFDS` can change between its sizing and read calls. A changed or
  over-4096 descriptor list is recorded as unavailable rather than retried
  without a bound or presented as complete.
- Real Codex app-server roots may clear the root-detection threshold while
  remaining below the activity/reporting threshold. They remain visible and
  protected, but targeted activity is limited to stronger attributions.

## VALIDATION

- `make check` — passed (format, vet, whole-module tests, 300-line source gate).
- `make race` — passed; Apple linker emitted malformed
  `LC_DYSYMTAB` warnings while all packages passed.
- `make lint` — passed with golangci-lint 2.11.2, zero issues.
- `make build` — passed for both `ghostgc` and `ghostgcd` on macOS.
- `GOOS=linux CGO_ENABLED=0 go build ./...` — passed; the phase-9 Linux
  collector remains an explicit not-implemented stub.
- Live macOS fixture with one-second sampling — passed. Six attributed fixture
  processes were sampled in 0.6 ms; the periodic writer repeatedly reported a
  4 KiB write delta while idle processes reported known zero after a baseline.
  The daemon remained audit-only and recorded zero attempted actions.
- Live resources were removed after validation; the scratch database was moved
  to Trash and the fixture removed every PID it created.

## OUTCOME

Phase 3 adds a cadence-gated macOS activity collector, exact-key delta model,
schema-v3 activity history, retention, API/CLI inspection and metrics. Expensive
inspection is restricted to live, strongly attributed same-user processes.
Availability is explicit at every layer, and raw file paths, socket endpoints
and cumulative counters are not persisted. Classification and all process
actions remain absent by design.

## REPOSITORY MEMORY

- Decision: created
- Rationale: collection gating, privacy boundaries and unknown-value semantics
  constrain every later safety decision and are not fully recoverable from code.
- Artifacts: `docs/specs/0002-phase-3-activity-tracking/SPEC.md`
