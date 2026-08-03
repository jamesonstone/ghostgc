# Phase 7: Narrow Automatic Enforcement

Status: accepted for implementation

GitHub: #13

## Purpose

Permit one deliberately narrow class of automatic cleanup while retaining the
same exact, evidence-backed authority boundary as manual cleanup. Audit remains
the default. Automatic action is explicit, bounded, local and fail-closed.

## Authority contract

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

## Bounded execution

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

## Configuration

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

## User surfaces

- `ghostgc status` reports manual and automatic cleanup authority separately.
- `ghostgc policies` shows the automatic flag and exact enforcement scope.
- `ghostgc candidates` separates enforceable current candidates from manual
  recommendations and audit-only/refused/cooldown decisions.
- `ghostgc actions` identifies manual versus automatic authority.
- audit logs and metrics preserve pre/post action evidence and counts.
- `ghostgc doctor` verifies the active authority boundary and final signal gate.

## Validation

Pending implementation. Required gates are `make check`, `make race`,
`make lint`, `git diff --check`, complete eligible source-size audit, exact-head
CI, independent PR review and a fully cleaned fixture-owned live automatic run.

## Live acceptance

Use the native `fixture-helper` action child in the fixture's isolated POSIX session.
Prove one exact orphaned target receives one automatic SIGTERM after at least
five minutes of continuous stable evidence; prove every non-target remains
alive before teardown; prove action/evaluation/audit ordering, authority,
metrics, target exit, exact source/deployed revision and complete cleanup. Keep
failed attempts literally and never weaken a safety gate to obtain a pass.

## Outcome

Pending implementation.

## Repository memory

This specification is the canonical Phase 7 rationale and authority contract.
After validation, update the constitution, safety model, architecture, testing,
README and dogfooding guide to describe only behavior proven by code and tests.
