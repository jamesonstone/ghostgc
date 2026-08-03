---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: implement
feature:
  id: 0006
  slug: phase-7-narrow-enforcement
  dir: 0006-phase-7-narrow-enforcement
relationships:
  - type: builds_on
    target: 0005-phase-6-manually-approved-cleanup
references:
  - id: issue-13
    name: Phase 7 narrow automatic enforcement issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/13
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
    used_for: automatic authority and revalidation boundaries
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

Permit one deliberately narrow class of automatic cleanup while retaining the
same exact, evidence-backed authority boundary as manual cleanup. Audit remains
the default. Automatic action is explicit, bounded, local and fail-closed.

## AUTHORITY CONTRACT

Automatic SIGTERM exists only when every gate below is simultaneously true:

1. `globalMode` is exactly `enforce`.
2. Exactly one enabled policy has `mode: enforce` and `automatic: true`.
3. That policy contains exactly one state (`orphaned`), agent and executable.
4. It requires detachment and an ended owning session.
5. Classification has at least five continuous minutes of complete known-idle
   evidence, and the exact policy/process pair is not cooling down.
6. The decision belongs to the newest committed policy evaluation and is a
   current `candidate`, never a refusal or cooldown.
7. Every non-overridable protection is absent.
8. A new snapshot, ownership reconciliation, lifecycle check, activity sample,
   classification and policy evaluation reproduce the same decision.
9. PID/start time, UID, executable path and kernel name still match immediately
   beside the one literal SIGTERM system call.

Unknown, unavailable, stale or changed evidence revokes authority. There is no
override, retry escalation, SIGKILL, process-group action, shell terminator or
remote control path.

## BOUNDED EXECUTION

- At most one automatic action is attempted per committed policy evaluation.
- Selection is deterministic from the ordered current decision projection.
- Only `candidate` is actionable. Cooldown prevents an immediate retry even
  when revalidation or the system call fails.
- The scan lane is held continuously from the committed evaluation through
  fresh revalidation, the durable `attempting` transaction, the platform gate
  and the durable completion transaction.
- Each action stores authority (`manual` or `automatic`) and structured evidence.
- `attempting` commits before the side effect. `signalled`, `failed` or
  `rejected` follows; `signalled` means only that the OS accepted SIGTERM.

## CONFIGURATION

The policy schema gains `automatic`, default false. Validation rejects:

- `automatic: true` outside `mode: enforce`;
- enforce without `automatic: true`;
- more than one enabled enforce policy;
- enforce scopes with multiple states, agents or executables;
- enforce states other than `orphaned`;
- missing detachment/session-ended requirements;
- enforce stability below five minutes or cooldown below one hour;
- every existing unsafe state, broad runtime, path-like executable, unknown
  agent, duplicate or misspelled field.

Global mode remains a hard upper bound. `enforce` continues to permit manual
recommendation policies, but an enforce policy is automatic only under global
enforce. Disabled, audit and recommend behavior remain compatible.

## USER SURFACES

- `ghostgc status` reports manual and automatic cleanup authority separately.
- `ghostgc policies` shows the automatic flag and exact enforcement scope.
- `ghostgc candidates` separates enforceable current candidates from manual
  recommendations and audit-only/refused/cooldown decisions.
- `ghostgc actions` identifies manual versus automatic authority.
- audit logs and metrics preserve pre/post action evidence and counts.
- `ghostgc doctor` verifies the active authority boundary and final signal gate.

## VALIDATION

- `make check` — PASS; formatting, vet, complete source-size audit and all
  package tests.
- `make race` — PASS; all packages under the race detector.
- `make lint` — PASS with zero issues.
- `git diff --check` — PASS.
- All eligible handwritten Go, C and shell implementation/test files remain at
  or below 300 physical lines; the largest is 296 lines.
- Configuration, policy, daemon, API, CLI, storage and platform tests prove the
  singular enforce scope, global cap, current-candidate selection, one-action
  bound, exact automatic authority, changed-image refusal, schema migration and
  manual compatibility.
- Platform source tests continue to prove exactly one literal SIGTERM system
  call site, with no SIGKILL, process-group or shell terminator path.
- Live run `20260803T184936Z-p7e1` at exact source
  `8a1147ea40464a7978d97414578cd5ad0865491c` and deployed version `8a1147e`
  selected only fixture target `57820:1785783014974826000` after 5m1s of
  continuous stable orphan evidence. Evaluation 59 preceded the automatic
  action request, durable `action.attempting` preceded one exact-key SIGTERM,
  and durable `action.signalled` followed it. The next observation proved the
  target absent; all five non-target controls survived until exact teardown.
  Metrics reported 386 scans, zero scan failures, one policy decision, one
  attempted action, zero rejected actions and one completed action. Structured
  evidence is under `tmp/2026-08-03/phase7-narrow-enforcement/1/`.
- `ghostgc doctor` passed the owner-only socket, schema version 8, privacy,
  observation and both local/daemon signal-safety gates. Teardown removed every
  fixture PID, fixture state, daemon process and Unix socket.

Exact-head CI and independent pull-request review are delivery gates and must
pass before merge.

## LIVE ACCEPTANCE

Use the native `fixture-helper` action child in the fixture's isolated POSIX session.
Prove one exact orphaned target receives one automatic SIGTERM after at least
five minutes of continuous stable evidence; prove every non-target remains
alive before teardown; prove action/evaluation/audit ordering, authority,
metrics, target exit, exact source/deployed revision and complete cleanup. Keep
failed attempts literally and never weaken a safety gate to obtain a pass.

## OUTCOME

Phase 7 is implemented and locally accepted. ghostgc can now automatically act
only under global enforce and one explicitly automatic, singular orphan-only
policy. It selects at most one current candidate per committed evaluation,
binds automatic authority to the exact decision and executable image, repeats
the complete manual safety revalidation, commits the attempt before one
SIGTERM, and records its outcome. Audit remains the default; manual approval
and every prior observation surface remain available.

## REPOSITORY MEMORY

This specification is the canonical Phase 7 rationale and authority contract.
After validation, update the constitution, safety model, architecture, testing,
README and dogfooding guide to describe only behavior proven by code and tests.
