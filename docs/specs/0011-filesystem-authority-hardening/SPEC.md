---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: complete
feature:
  id: 0011
  slug: filesystem-authority-hardening
  dir: 0011-filesystem-authority-hardening
relationships:
  - type: builds_on
    target: 0009-stale-worktree-cleanup
  - type: builds_on
    target: 0010-session-cache-lifecycle
references:
  - id: issue-27
    name: Filesystem authority hardening issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/27
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
    used_for: destructive authority, recovery and fail-closed behavior
    status: active
  - id: backend-architecture
    name: Backend service architecture rule
    type: ruleset
    target: docs/references/rules/backend-service-architecture.md
    relation: constrains
    read_policy: must
    used_for: API, service, executor and persistence boundaries
    status: active
  - id: testing-contract
    name: Testing and environment validation rule
    type: ruleset
    target: docs/references/rules/testing-and-environment-validation.md
    relation: constrains
    read_policy: must
    used_for: adversarial filesystem and recovery validation
    status: active
skills:
  - name: security-best-practices
    source: Codex
    path: /Users/jamesonstone/.codex/skills/security-best-practices/SKILL.md
    trigger: filesystem trust-boundary hardening in Go
    required: true
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Make accidental permanent deletion structurally unavailable to the long-running
daemon, make worktree cleanup recoverable, and narrow every filesystem action
to explicit operator-approved roots and exact identities.

## CONTEXT

The cache lifecycle already quarantines one exact regular file before a
grace-gated purge, and stale-worktree removal already performs complete
process, filesystem and Git revalidation before native non-force removal.
Those controls fail closed, but both irreversible calls still execute inside
the daemon with the invoking user's ambient filesystem permissions.

The remaining risk is therefore capability-shaped rather than validation-
shaped. More path checks cannot prove that a future bug, alternate call site or
same-user control client will never reach a destructive primitive. The durable
boundary must make the daemon incapable of permanent deletion and must retain
a recoverable state before any foreground finalization.

## REQUIREMENTS

- The daemon may observe, recommend, quarantine, restore and retire, but it may
  not own or call a permanent cache unlink or native worktree removal method.
- Cache purge apply becomes a prepare/execute/complete protocol. The daemon
  consumes the approval, repeats all checks, commits `purging`, and returns one
  exact short-lived execution plan. The foreground CLI requires the complete
  artifact ID as an independent confirmation, executes the sole descriptor-
  anchored unlink, and asks the daemon to verify absence before committing the
  result.
- A purge plan binds the action, artifact, exact configured root, quarantine
  basename, complete root and entry identity, configuration digest, expiration
  and a single-use completion capability. It never grants a raw arbitrary path
  or recursive authority.
- Enabled cache observation requires at least one explicitly configured,
  absolute, canonical, physical provider root. A session-derived
  `CODEX_HOME/shell_snapshots` root is observed only when it exactly matches
  that allowlist.
- Cache filesystem roots are opened by descriptor-walking every path component
  from the filesystem root with no symlink following. Revalidation and mutation
  stay relative to the resulting provider and quarantine descriptors.
- Any ambiguous foreground result, failed post-action verification, expired
  plan, configuration change or unexpected filesystem state trips a daemon-
  local mutation circuit breaker. Further cache or worktree mutation requires
  a daemon restart and fresh observation.
- Stale worktree `remove` becomes a reversible retirement. After the existing
  preview/apply revalidation, native `git worktree move` relocates the exact
  registered secondary checkout to a deterministic absent sibling path on the
  same filesystem. The original path, retirement path, identities and grace
  deadline are durable.
- A retired worktree can be restored to its exact absent original path after a
  separately previewed approval. Final removal requires a later separately
  previewed foreground plan, the complete worktree ID as confirmation, fresh
  no-use/clean/published/identity checks, elapsed grace and native non-force
  `git worktree remove`. The branch remains the verified recovery point.
- The foreground worktree finalizer binds and verifies the exact pinned Git
  executable identity. The daemon records intent and verifies path and
  registration absence; it cannot invoke native removal itself.
- The owner-only socket remains a transport boundary, not independent human
  confirmation. Irreversible CLI execution requires both the daemon approval
  and a separately supplied exact artifact or worktree ID.
- Repository safety enforcement uses Go syntax rather than substring matching,
  rejects recursive deletion and shell deletion, and proves that permanent
  cache unlink and native worktree removal each have one foreground-only call
  site unreachable from `internal/daemon`.
- Focused tests cover allowlist refusal, component symlink replacement,
  identity change, approval replay/expiry, completion replay, daemon restart,
  foreground partial failure, circuit breaking, retirement restore, grace,
  canary survival outside approved roots, and the absence of daemon imports or
  calls to irreversible executors.
- README contains only project purpose, safety boundary, installation/running,
  primary commands and links. Detailed command catalogs, storage paths,
  operational lifecycle instructions, measurements, delivery history and
  development detail live under `docs/references`.

### Non-goals

- General filesystem cleanup, configurable path/glob deletion, recursive
  deletion, forceful Git operations, branch deletion or automatic filesystem
  mutation.
- Claiming an absolute guarantee against a malicious process already running
  as the user; that process already possesses the user's filesystem rights.
- Requiring undocumented or deprecated macOS sandbox facilities, elevated
  privileges, biometrics, network services or a second binary. The portable
  application capability split and exact foreground confirmation are the
  supported boundary.
- Reading or copying cache contents to create a backup. Cache recovery remains
  the existing quarantine during grace; irreversible foreground purge applies
  only to the pinned disposable provider contract.

### Observable acceptance

- A compile-time interface and repository syntax gate make cache unlink and
  native worktree removal unavailable from daemon packages.
- Cache enablement with no approved roots fails configuration validation;
  unlisted session roots produce no artifacts or action authority.
- The daemon can prepare but cannot execute purge. Only the foreground CLI can
  execute a non-expired exact plan after full-ID confirmation, and completion
  succeeds only after the daemon proves the exact quarantine entry is absent.
- Worktree remove leaves a registered checkout at the retired path and exposes
  an exact restore command. Restore returns the same registration to the absent
  original path. Finalize refuses before grace and removes only the retired
  clean checkout afterward, while preserving its branch.
- Every out-of-scope canary remains present across successful and failed cache
  and worktree lifecycle tests.
- `make check`, `make race`, `make lint`, `make size`, `git diff --check`,
  focused lifecycle tests and the cache high-level fixture pass.

## ACCEPTED PLAN

1. Add exact cache-root configuration and provider allowlisting, then replace
   string path opening with component-wise descriptor traversal.
2. Split cache metadata/reversible mutation from the foreground purge executor;
   add bound prepare and completion API/service operations plus exact-ID CLI
   confirmation and mutation circuit breaking.
3. Add additive worktree retirement persistence and state, use native move for
   reversible apply, and add restore plus foreground grace-gated finalization.
4. Replace deletion substring checks with Go AST enforcement and add focused
   fault, race, replay, canary and recovery tests.
5. Curate the concise README, operator guides, architecture, safety, testing,
   constitution and this spec to the implemented behavior.
6. Run all required validation, self-review, explicitly stage, commit, push and
   publish a ready issue-linked pull request.

## DECISIONS

### Capability separation is stronger than another path predicate

The daemon's interfaces intentionally omit irreversible operations. Validation
still protects the foreground executor, but package ownership makes an
accidental daemon call a compile failure and the repository syntax gate makes
an alternate primitive a test failure.

### Foreground execution stays in the single binary

`ghostgc` remains one shipped executable. A short-lived CLI invocation is the
independent execution boundary: it has no observation loop, receives one exact
expiring plan, requires the full opaque identity again, and terminates after
reporting completion. This preserves the single-binary invariant without
placing permanent authority in the persistent daemon.

### Worktree retirement is a same-filesystem registered move

The retirement destination is a deterministic absent sibling of the original
checkout. Native `git worktree move` updates Git's registration while retaining
the exact checkout and branch. Cross-device movement is refused, and the
original path is durable for exact restoration.

## DISCOVERIES

- Limiting each irreversible lane to one outstanding plan is necessary for the
  mutation circuit breaker to revoke all new authority after an ambiguous
  foreground result.
- Approved worktree environment links required descriptor-anchored finalization
  in addition to ordinary path validation. The foreground executor now refuses
  a link replaced by a regular file immediately before unlink.
- Retired worktrees and unresolved `attempting`, `purging`, or `partial`
  actions must be protected from ordinary retention so recovery and audit
  evidence cannot disappear.
- The repository-wide Kit check still reports pre-existing workflow-v3 debt in
  legacy specs 0001, 0005, and 0006 and a missing project progress summary. The
  feature-scoped 0011 check passes; expanding this change to rewrite unrelated
  historical specifications would violate the issue boundary.

## VALIDATION

- `go test ./internal/worktree ./internal/platform ./internal/daemon ./cmd/cli`
  passes, including real disposable Git retirement, restoration, finalization,
  canary, approval replay, circuit-breaker, and link-replacement cases.
- `make check` passes: vet, complete source-size gate, and all package tests.
- `make race` passes all packages. The macOS linker emits its existing
  non-failing malformed `LC_DYSYMTAB` warnings.
- `make lint` passes with zero issues; `make size` and `git diff --check` pass.
- `tests/end-to-end/local/cache-lifecycle-test.sh` passes the high-level
  metadata-only quarantine, restore, and foreground purge fixture.
- `kit check filesystem-authority-hardening` passes. `kit check --project`
  remains non-green only for the pre-existing legacy-spec debt described above.

## OUTCOME

- The persistent daemon can no longer permanently unlink cache artifacts or
  natively remove worktrees. It prepares exact expiring plans and verifies
  completion; only a short-lived foreground CLI holds either irreversible
  capability after complete-ID confirmation.
- Cache discovery is restricted to explicitly configured exact
  `shell_snapshots` roots, and each physical path component is descriptor-opened
  without following symlinks.
- Worktree cleanup is now retirement-first, independently restorable, and only
  later purgeable after grace, full fresh validation, durable intent, and
  daemon verification. Ambiguity opens a cross-lane mutation circuit.
- The README is a concise project/run/use entrypoint. Operational details,
  historical context, architecture, testing, and the complete safety boundary
  are linked references.

## REPOSITORY MEMORY

- This spec owns the capability-separation and recoverable-retirement rationale.
- `docs/CONSTITUTION.md` records the demonstrated foreground-only irreversible
  authority and retirement invariants.
- `docs/references/operator-guide.md`, `safety-model.md`, `architecture.md`,
  `testing.md`, and `manual-worktree-cleanup.md` preserve the operating model;
  `project-history.md` preserves important README material that is not required
  for first use.
