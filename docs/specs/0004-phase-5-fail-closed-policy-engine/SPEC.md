---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: implement
feature:
  id: 0004
  slug: phase-5-fail-closed-policy-engine
  dir: 0004-phase-5-fail-closed-policy-engine
relationships:
  - type: builds_on
    target: 0003-phase-4-deterministic-classification
references:
  - id: issue-9
    name: Phase 5 policy engine issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/9
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
    used_for: evaluation and transaction boundaries
    status: active
  - id: safety-model
    name: Safety model
    type: documentation
    target: docs/references/safety-model.md
    relation: constrains
    read_policy: must
    used_for: protection and no-action guarantees
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

Turn Phase 4 classifications into explicit, inspectable audit candidates and
safety refusals through strict YAML policies, without adding recommendation or
action authority.

## CONTEXT

- A classification describes evidence; it does not grant permission.
- Hard protections are non-overridable and already explain why uncertain,
  interactive, active-session, broad-runtime and shared infrastructure
  processes must be refused.
- A policy match is a cleanup candidate, not an action. Phase 5 accepts only
  `audit` or `disabled` policy modes and retains the source-level no-signal gate.
- Cooldowns are keyed by policy plus exact process identity, persist in SQLite,
  and never transfer across PID reuse.

## REQUIREMENTS

- Parse policies from the strict main YAML configuration. Reject unknown fields,
  duplicate/invalid identifiers, unsupported modes, broad executable names,
  unknown states, empty scopes, unsafe stable windows and invalid cooldowns.
- Require exact agent IDs and executable basenames. Phase 5 policies may match
  only `orphaned`, `hung` or `crashed`; `suspicious` explicitly means progress
  or live resources remain and is never cleanup-eligible.
- Evaluate every current exact-key classification at the configured cadence.
  Match state, agent, executable, detachment, ended-session and stable-window
  conditions before applying all hard protections.
- Produce `candidate`, `refused` or `cooldown` audit decisions. Every decision
  carries policy/classification facts and every triggered protection.
- Persist decisions and audit entries in the same scan transaction. Advance
  evaluation cadence only after commit.
- Preserve candidate cooldowns across restart and isolate them by exact key.
- Expose policies and current live decisions through API, CLI, JSON, status,
  metrics and explain. Never list an exited/stale process as current.
- Preserve all prior privacy, retention, source-size and no-signalling gates.

### Non-goals

- Recommending or previewing a command, accepting manual approval, signalling a
  process, or enabling global `recommend`/`enforce` modes.
- Arbitrary expressions, regexes, command-line substring rules, negation, shell
  execution, policy hot-reload or remote policy sources.
- Allowing a policy to override a hard protection or manufacture missing facts.

### Observable acceptance

- Config tests prove strict safe parsing and reject broad/ambiguous policies.
- Engine tests cover match, mismatch, every refusal, cooldown and PID reuse.
- Daemon tests prove transactional persistence, live-only candidate projection,
  cadence and no signals.
- `ghostgc policies`, `ghostgc candidates`, `ghostgc explain`, status and metrics
  expose complete evidence and zero enforceable entries.
- All handwritten source/test files remain at or below 300 lines.

## ACCEPTED PLAN

1. Define and validate a deliberately small policy schema in configuration.
2. Add a pure fail-closed evaluator and table-driven tests.
3. Add additive decision/evaluation history, cooldown queries, retention and
   counts.
4. Evaluate current classification results in the scan and persist decisions
   plus audit evidence transactionally.
5. Replace placeholder API/CLI surfaces, update durable docs and validate live.

## DECISIONS

- Keep policies non-Turing-complete: exact lists, booleans and durations make a
  safety decision readable and testable.
- Restrict executable matching to exact basenames and reject the existing broad
  protected runtime classes during validation.
- A cooldown suppresses repeated candidate noise; it never converts a refusal
  into eligibility and never extends itself when merely observed.
- Global `disabled` is a hard cap: the daemon transactionally advances an empty
  current evaluation and emits no decisions even if individual policies are
  enabled in audit mode.
- Retention never deletes the candidate row that grants an active cooldown,
  including during aggressive compaction.
- Current candidate views join decisions to live exact process rows so stale
  database history cannot look actionable.

## DISCOVERIES

- Current policy views need an explicit committed evaluation watermark. Querying
  merely by the newest decision would leave an old candidate visible after the
  next classification no longer matched. A unique autoincremented evaluation
  identity makes even an empty or same-timestamp result durable and keeps API
  projection transactional.
- Classification samples can contain several independent evidence items that
  explain the state. Copying that evidence into each policy decision makes the
  durable refusal/candidate record self-contained instead of requiring a later
  join to reconstruct its basis.
- `suspicious` means that progress or live resources remain after a session
  ends. Treating it as cleanup-eligible would invert that evidence, so policy
  validation rejects it. A fixture-owned zombie supplies a deterministic
  `crashed` conclusion for live refusal testing instead.
- macOS activity samples are taken after the selecting process snapshot, so a
  classification timestamp can be slightly later than the cadence timestamp.
  Each process is evaluated at its own classification timestamp so another
  process's later sample can never satisfy its stable window. Snapshot time is
  retained solely for cadence bookkeeping, while evaluation identity—not a
  timestamp—selects the current committed projection.
- Candidate cooldown lookup must consider only a prior `candidate`, not a prior
  `cooldown` or `refused` decision. It is keyed by policy ID plus exact
  `pid:start_time_ns`, so neither refusal observation nor PID reuse extends it.

## VALIDATION

- `make check` — PASS on local macOS; vet, source-size and all package tests.
- `make race` — PASS on local macOS; all package tests under the race detector.
- `make lint` — PASS with golangci-lint v2; zero issues.
- `git diff --check` — PASS.
- All handwritten Go source/test files remain at or below 300 lines; the largest
  tracked Go file is 296 lines.
- `TestPolicyValidationFailsClosed`, `internal/policy` table tests,
  `TestPolicyDecisionCooldownAndCurrentLiveProjection` and
  `TestPolicyCandidateCooldownAndLiveProjection` prove strict config, every
  protection, exact-key cooldown persistence, current-only projection,
  same-timestamp empty projection, per-process stable windows,
  candidate/cooldown transitions and zero signal attempts.
- Live fixture run `20260803T170659Z-p5f1` at exact source `e4f1d76` used an exact `codex` /
  `fixture-helper` / `crashed` policy. Seventeen matching samples were durably
  recorded as `policy.refused` because the owning fixture session was active.
  `ghostgc candidates`, `explain`, `logs`, `metrics` and `policies` exposed the
  complete scope, both freshness timestamps and evidence; attempted/rejected/
  completed actions stayed zero. Structured `result.json` and readable
  `output.txt` under `tmp/2026-08-03/phase5-policy-live/1/` satisfy the repository
  evidence schema and record source/deployed identity, assertions, artifacts and
  successful fixture cleanup.

## OUTCOME

Phase 5 is complete. ghostgc now loads strict audit-only YAML policies, evaluates
current exact-key classifications behind every hard protection, persists
self-contained candidate/refusal/cooldown records transactionally, retains
cooldowns across restart, and exposes current decisions through the CLI and
API. There is still no recommendation or process-termination path in the
binary; Phase 6 is the first delivery allowed to add a manually approved,
revalidated SIGTERM.

## REPOSITORY MEMORY

- Decision: created
- Rationale: policy syntax and evaluation order become safety-critical inputs to
  Phase 6 and Phase 7 and must be decision-complete before either action phase.
- Artifacts: `docs/specs/0004-phase-5-fail-closed-policy-engine/SPEC.md`
