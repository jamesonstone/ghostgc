---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: complete
feature:
  id: 0010
  slug: session-cache-lifecycle
  dir: 0010-session-cache-lifecycle
relationships:
  - type: builds_on
    target: 0005-phase-6-manually-approved-cleanup
references:
  - id: issue-24
    name: Session cache artifact lifecycle issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/24
    relation: guides
    read_policy: must
    used_for: scope and acceptance criteria
    status: active
  - id: codex-shell-snapshot-contract
    name: Codex 0.146.0 shell snapshot implementation
    type: external
    target: https://github.com/openai/codex/blob/rust-v0.146.0/codex-rs/core/src/shell_snapshot.rs
    relation: constrains
    read_policy: must
    used_for: exact provider root, filename attribution and artifact semantics
    status: active
  - id: safety-model
    name: Safety model
    type: documentation
    target: docs/references/safety-model.md
    relation: constrains
    read_policy: must
    used_for: authority, durability and revalidation boundaries
    status: active
  - id: backend-architecture
    name: Backend service architecture rule
    type: ruleset
    target: docs/references/rules/backend-service-architecture.md
    relation: constrains
    read_policy: must
    used_for: API, service and persistence responsibility boundaries
    status: active
  - id: testing-contract
    name: Testing and environment validation rule
    type: ruleset
    target: docs/references/rules/testing-and-environment-validation.md
    relation: constrains
    read_policy: must
    used_for: deterministic fixtures and validation evidence
    status: active
skills:
  - name: github:yeet
    source: GitHub plugin
    path: github:yeet
    trigger: publish the validated feature as a ready pull request under repo-local delivery rules
    required: true
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Add a fail-closed lifecycle for one proven family of coding-agent cache
artifacts without widening ghostgc into a general filesystem cleaner.

## CONTEXT

Codex 0.146.0 creates disposable shell snapshots beneath the exact
`CODEX_HOME/shell_snapshots` root. Its primary source constructs each regular
file name as `<thread-id>.<generation>.sh|ps1`, where `thread-id` is one Codex
`ThreadId`; temporary names are `<thread-id>.tmp-<generation>`. Codex deletes
the current snapshot on drop and has a three-day stale-snapshot cleanup path.
The local installed binary and filesystem match that contract.

ghostgc already records Codex `CODEX_THREAD_ID` values as native session IDs.
That provides the second half of the contract: a snapshot is eligible for
observation only when its parsed thread ID exactly matches one known Codex
session. Names and filesystem metadata are sufficient; contents are never
read. Attachments, rollouts, visualizations, ambient suggestions, plugin
caches, shared caches and every other Codex directory remain out of scope.

The active `GH-23` lane owns feature number `0009`, so this separate feature is
numbered `0010` to preserve lane isolation and avoid a future merge conflict.

## REQUIREMENTS

- Cache observation is disabled by default and cache mutation is impossible
  unless cache-global authority is `recommend` and one exact policy is enabled.
- Cache authority is independent from process `globalMode`. Initial cache modes
  are only `disabled`, `audit` and `recommend`; enforce and automatic modes are
  rejected.
- The only real provider is Codex shell snapshots beneath the exact current-user
  `CODEX_HOME/shell_snapshots` root established by an observed Codex session.
- Provider discovery accepts only regular `.sh` or `.ps1` files whose basename
  contains one valid UUID thread ID and numeric generation. Temporary,
  malformed, shared, unknown-session and uncontracted entries are protected.
- Observation records provider, agent, session, kind, relative path, UID,
  device, inode, file type, size, timestamps, first observation, exact identity,
  manifest digest and attribution evidence without opening file contents.
- Lifecycle is explicit: observed, protected, settling, stale candidate,
  recommended, quarantined, restored, purged, partial or failed as applicable.
- A stale candidate requires one strongly attributed completed session, no live
  process claiming that native session ID, and two committed unchanged
  observations spanning the configured stable window.
- Any unknown lifecycle, incomplete scan, traversal exhaustion, missing
  metadata, root entry, symlink, unexpected hard link, UID mismatch, mount
  crossing, changed identity or exceeded bound is protected.
- Policy selectors are exact provider, agent, artifact kind and completed
  lifecycle criteria. Paths and globs are not configurable authority.
- Recommendation produces a short-lived, random, single-use approval for one
  artifact and binds provider, session, policy, configuration digest,
  evaluation, exact identity, manifest digest and destination.
- Apply serializes with observation, commits `attempting` before mutation,
  freshly revalidates every bound fact, and atomically renames exactly one file
  into a private provider-root quarantine on the same filesystem. `EXDEV`
  refuses; copy-and-delete is absent. Quarantine does not reclaim space.
- Restore requires the original destination to be absent and both the source
  and bound identity to remain exact. It uses one separately previewed approval.
- Purge is possible only from quarantine after a configured grace period and a
  separate preview and approval. It revalidates the complete exact manifest,
  commits `purging` first, and records `purged`, `partial` or `failed`.
- Filesystem operations sit behind a narrow interface with a deterministic
  metadata fake. Production mutation has one authorized purge primitive and no
  `os.RemoveAll`, shell `rm`, path-prefix authority, symlink following or mount
  crossing.
- SQLite migrations only add artifacts, observations, evaluations, decisions,
  quarantine records and actions. Cache storage follows the shipped worktree v9
  schema as v10. In-memory state advances only after commits.
- API and CLI expose stable human and JSON forms for artifacts, explanation,
  candidates, cleanup, quarantine, restore, purge and action history.
- Cache scanning has an independent bounded cadence and metrics for duration,
  inspected/protected/candidate counts, quarantined/purged bytes, failures and
  bounded state.

### Non-goals

- Scanning a home directory, `~/Library/Caches`, `~/.cache`, `/Library`, an
  arbitrary configured root, package caches, shared application caches,
  repositories or unattributed files.
- Establishing ownership from age, name, location, size or modification time
  alone; reading file contents; following symlinks; or crossing filesystems.
- Direct deletion of an observed path, automatic cache removal, broad approval,
  shell execution, package-manager cleanup or reuse of process signal authority.
- Managing Codex rollouts, attachments, visualizations, state databases,
  memories, plugins, ambient suggestions or any provider family not pinned by a
  primary-source contract and validated fixture.

### Observable acceptance

- Default and audit-only configurations perform zero cache mutation; disabled
  performs no cache scan.
- Deterministic fixtures cover active, completed, unknown, ambiguous and shared
  ownership plus unchanged and changed second observations.
- Every symlink, hard-link, UID, mount, identity, manifest, policy,
  configuration, session, PID-reuse, limit, concurrency and replay change
  refuses before mutation with explainable evidence.
- Preview and apply quarantine exactly one fixture artifact, restore it without
  changing controls, re-quarantine it, then separately approve and purge it
  after the grace period with durable evidence for every transition.
- Crash-recovery tests preserve unresolved `attempting` and `purging` evidence;
  a partial purge stays visible and requires new approval.
- A repository safety test rejects `os.RemoveAll`, shell `rm`, deletion outside
  the single purge implementation and any weakening or duplication of the
  existing exact SIGTERM gate.
- `make check`, `make race`, `make lint`, `make size`, `git diff --check` and the
  local high-level fixture pass; affected source and test files stay within 300
  physical lines.

## ACCEPTED PLAN

1. Define cache identities, lifecycle, exact manifests, provider and filesystem
   contracts; implement the Codex shell-snapshot provider and deterministic
   fakes before any daemon integration.
2. Add strict cache configuration and exact policy evaluation, additive storage
   migrations, committed two-observation state and bounded retention.
3. Add the independent observation lane, protection evaluation, metrics and
   current API/CLI read projections.
4. Add one-artifact preview bindings and fully revalidated atomic quarantine,
   restore and quarantine-only purge services with durable pre-action evidence.
5. Add focused unit, storage, daemon, API, CLI, concurrency, crash-recovery and
   repository safety tests plus a temporary-directory high-level fixture.
6. Curate operator and architecture documentation, run all required validation,
   self-review and publish a ready `GH-24` pull request.

## DECISIONS

### Codex shell snapshots are the sole provider contract

The upstream source names the exact bounded root, derives every supported file
name from one `ThreadId`, identifies the files as snapshots and deletes them as
disposable state. The local 0.146.0 installation matches the pinned source.
This is enough to prove session exclusivity without contents. No adjacent Codex
directory inherits that authority.

### Files are individual artifacts

Each snapshot file is one artifact. This keeps one approval equal to one exact
filesystem identity, makes atomic quarantine a single rename and avoids
recursive deletion. Directories, provider roots and temporary files are always
protected.

### Quarantine is provider-root local

The quarantine directory is a private sibling under the exact
`shell_snapshots` root. A rename can therefore be required to stay on the same
device. Quarantine is reversible containment and never reported as reclaimed
space.

### A manifest is metadata-only

For the single-file provider, the manifest contains the relative path, type,
UID, device, inode, link count, mode, size and filesystem timestamps. Its
digest never includes contents. The complete manifest is rebound and compared
at every action boundary.

### Stability requires consecutive complete committed evaluations

Only an artifact in the immediately previous complete committed evaluation may
carry its stable window forward. Absence, an incomplete bounded scan, a
protected state or a failed newest observation resets action authority and the
next observation starts settling again. This avoids converting stale database
history into current filesystem authority.

### Cache action health is daemon-local and fail-closed

The daemon grants cache previews only after its newest enabled cache scan
commits completely. A provider or persistence failure revokes in-memory cache
authority even if SQLite cannot record a replacement evaluation. Restart also
starts without cache authority until the immediate cache scan succeeds.

### The cache migration follows the shipped worktree schema

The worktree lifecycle merged to `main` first and therefore owns schema v9.
Cache storage is the independent v10 migration. This ordering is required for
existing v9 databases to retain their worktree records while gaining every
cache table; folding both branches into one v9 migration would silently leave
already-upgraded databases without the cache schema.

## DISCOVERIES

- Codex snapshot file names can repeat one thread ID across generations; the
  generation is part of artifact identity while the thread ID establishes
  exclusive session ownership.
- Codex already attempts stale cleanup after three days. ghostgc still needs
  its own observation window because provider age is supporting evidence only,
  not action authority, and leaked files can survive failed provider cleanup.
- The primary branch contains specs through `0008`, while the separate dirty
  `GH-23` lane owns unmerged spec `0009`; feature numbering must account for
  active lanes, not only the protected checkout.
- POSIX rename changes ctime while preserving the file object. Post-rename
  verification therefore compares every metadata identity field except ctime,
  then persists the newly observed ctime and manifest for the next action.
- Reading an empty directory with a positive `ReadDir` bound returns `io.EOF`
  alongside the empty result; an empty exact provider root is a complete scan,
  not a provider failure.
- Retention must preserve the latest evaluation, active quarantine, partial
  state and unresolved `attempting` or `purging` actions while still removing
  stale non-current recommendations.
- Parallel feature branches can claim the same next schema number without a Git
  content conflict. Integration must compare migration identifiers as well as
  conflict-marked files and add an explicit upgrade test from the version that
  has already shipped on `main`.

## VALIDATION

Validated on local macOS at 2026-08-04T18:46:16Z:

Implementation source: `4fd32628163bdd9b0a54b7f9a9367c042d8e65c8`.

- `make check` — PASS; gofmt, vet, all package tests and the source-size gate.
- `make race` — PASS; all packages including concurrent single-use approval
  consumption. The host linker emitted its existing non-failing
  `LC_DYSYMTAB` warnings for cgo-linked test binaries.
- `make lint` — PASS; zero issues.
- `make size` — PASS; every eligible source and test file is at most 300 lines.
- `git diff --check` — PASS.
- `tests/end-to-end/local/cache-lifecycle-test.sh` — PASS using only
  temporary SQLite storage, a controlled clock and the deterministic
  metadata-only filesystem.
- `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -exec=true ./...` — PASS as a
  Linux compile-only portability check.
- `kit check 0010-session-cache-lifecycle` — PASS.
- `kit check --project` — FAIL on the pre-existing project baseline: the
  missing project progress summary, legacy spec-section gaps and instruction
  refresh warnings. No finding names this feature or a file introduced by it.

Conflict integration was revalidated on local macOS at
2026-08-04T20:50:49Z after merging `origin/main` at
`84a32dea0fb1bf33ccd41206ac3ed40cd4d32418`:

- Focused storage, API, daemon and CLI tests — PASS, including migration from
  the shipped worktree schema v9 to cache schema v10.
- `make check`, `make race`, `make lint`, `git diff --check` and the cache
  lifecycle high-level fixture — PASS. The race run emitted only the existing
  non-failing macOS cgo linker warnings.
- `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -exec=true ./...` — PASS as a
  compile-only portability check.
- `kit check 0010-session-cache-lifecycle` — PASS.
- `kit check --project` — FAIL with the same pre-existing 19 errors and five
  warnings: missing project summary, legacy spec gaps and instruction refresh
  debt. No finding names the cache feature or its changed files.

## OUTCOME

ghostgc now has a separate default-disabled lifecycle for the single proven
Codex shell-snapshot provider. Metadata-only bounded scans produce protected,
settling, candidate or recommended projections; two consecutive complete
committed observations and exact completed-session ownership are mandatory.

One-artifact previews bind the current evaluation, policy, configuration,
identity, manifest and destination to a two-minute memory-only approval. Apply
serializes with observation and persists pre-side-effect evidence before an
atomic provider-local quarantine, exact restore or separately approved
grace-gated purge. No automatic cache mutation or observed-path deletion path
exists. Interrupted and partial actions stay durable and the repository safety
gate admits exactly one quarantine-only `unlinkat` primitive.

The API, CLI, metrics, strict configuration, additive SQLite v10 migration,
retention, operator documentation and temporary high-level fixture expose and
validate the complete lifecycle. Quarantine is consistently described as
reversible containment that reclaims no disk space.

## REPOSITORY MEMORY

- This spec is the living source for the provider contract, the cache authority
  boundary, the reversible quarantine lifecycle and the rationale for refusing
  all other cache families.
- Project-wide invariants were curated into `docs/CONSTITUTION.md`; architecture,
  safety mechanisms, operator workflow and reusable test evidence were curated
  into `README.md` and `docs/references/{architecture,safety-model,testing}.md`.
