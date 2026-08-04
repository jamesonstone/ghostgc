---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: complete
feature:
  id: 0009
  slug: stale-worktree-cleanup
  dir: 0009-stale-worktree-cleanup
relationships:
  - type: builds_on
    target: 0007-single-binary-runtime
  - type: builds_on
    target: 0008-product-messaging
references:
  - id: issue-23
    name: Stale worktree cleanup issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/23
    relation: guides
    read_policy: must
    used_for: scope and acceptance criteria
    status: active
skills:
  - name: github:yeet
    source: GitHub plugin
    path: github:yeet
    trigger: publish the validated feature as a ready pull request
    required: true
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Let an operator identify and remove genuinely abandoned local Git worktrees
without trusting age alone, deleting branches, consulting GitHub, or granting
the daemon automatic filesystem authority.

## CONTEXT

Coding agents create durable worktrees so unrelated changes remain isolated.
Those lanes can accumulate after sessions end, but a worktree may still contain
valuable dirty, ignored, detached, in-progress, mounted, active, or unpublished
state. Cleanup therefore needs the same evidence, expiring approval, fresh
revalidation, and durable audit model as process cleanup, with stricter
filesystem and Git boundaries.

## REQUIREMENTS

- Discover registered worktrees only from exact repository paths associated
  with observed sessions and up to 32 operator-configured canonical roots.
- Bound configured-root traversal to four levels, do not follow symlinks, and
  reject filesystem-root and whole-home authority.
- Classify stable worktree identities as `active`, `observing`, `stale`,
  `protected`, `unknown`, `missing`, or `removed`.
- Require at least 168 hours of continuous complete inactivity evidence.
  Restarts, incomplete scans, activity, Git-state changes, and clock anomalies
  reset the window.
- Protect primary, locked, prunable, missing, unreadable, symlinked, dirty,
  unpublished, unreachable-detached, in-operation, submodule, mounted, or
  actively used worktrees and every candidate with incomplete evidence.
- Treat staged, tracked, conflicted, untracked, and ignored content as dirty.
  Only root `.env` and `.envrc` symlinks resolving exactly to the matching
  primary checkout files may be temporarily unlinked for removal.
- Inventory on every supported platform. Permit removal only on macOS and only
  through a two-minute, memory-only, single-use preview approval followed by
  explicit `--yes` apply.
- Bind approval to path and inode identity, common and administrative Git
  directories, registration, HEAD/ref/branch, aggregate status fingerprint,
  inactivity evidence, configured authority, approved links, and the resolved
  Git executable identity.
- Execute user-writable Git only through a private content-addressed snapshot,
  and revalidate both source and execution identity before every command.
- Serialize apply with scans and actions, revalidate every fact, persist an
  `attempting` action before invoking ordinary non-force
  `git worktree remove`, restore approved links on failure, and verify both
  path and registration absence before recording `removed`.
- Retain only aggregate cleanliness and process-key evidence. Never persist
  filenames, discovered descriptor paths, file contents, approval tokens, or
  environment values.
- Expose inventory, detail, preview/apply, and action history through the local
  CLI and Unix-socket API, plus stale/protected status and metrics counts.

### Non-goals

- Automatic worktree removal or daemon enforcement.
- Branch deletion, remote or GitHub lookups, fetch, prune, reset, clean, stash,
  recursive deletion, shell execution, force removal, or repository mutation
  other than native non-force worktree removal.
- Requiring a worktree branch to be merged.
- Repairing or pruning missing administrative metadata.
- Linux removal authority in this iteration.

### Observable acceptance

- A clean secondary worktree becomes stale only after seven uninterrupted days
  of complete observations and can be previewed and removed once.
- The branch survives removal and can recreate the worktree.
- Any bound fact change, restart, replay, expiry, active process, unsafe Git or
  filesystem state, incomplete inspection, or unsupported platform refuses the
  action without removal.
- Durable action history proves `attempting` was committed before the side
  effect and accurately distinguishes `removed`, `rejected`, and `failed`.

## ACCEPTED PLAN

1. Add validated worktree configuration and a bounded, shell-free Git adapter
   with NUL-delimited registration parsing and aggregate evidence.
2. Add schema migration v9, inventory/action records, stable identity and the
   continuous-inactivity state machine.
3. Integrate discovery into daemon scans, session repository evidence, status,
   metrics, local API and CLI inventory surfaces.
4. Add same-user path-usage inspection, full protection evaluation, expiring
   approvals and macOS-only native non-force removal with link restoration and
   durable pre-side-effect actions.
5. Prove safety with unit, disposable-repository integration, CLI/JSON,
   cross-platform compile, race, lint, file-size and controlled macOS tests.
6. Document configuration, architecture, safety model, manual operation,
   dogfooding and recovery, then curate demonstrated invariants.

## DECISIONS

### Staleness is continuous evidence, not elapsed wall time

Age is insufficient authority. The inactivity clock advances only across
complete, unchanged observations and resets whenever ghostgc cannot prove the
entire interval safe.

### Registration identity survives moves but not recreation

Identity hashes the canonical common and administrative Git directory paths
plus their device/inode identities. Git preserves the administrative directory
when moving a registered worktree; recreating one allocates a new directory
identity even when Git reuses the same pathname.

### Removal is narrower than inventory

Inventory is portable and read-only. macOS alone initially supplies the
complete process/file-usage evidence required for removal authority; every
other platform fails closed.

### Branch preservation is deliberate

Removing a registered checkout leaves its branch and commits intact. Requiring
merge status would add network or remote-tracking assumptions without making
the local filesystem operation safer.

## DISCOVERIES

- Git can reuse the same administrative pathname when a removed worktree is
  recreated. Stable identity therefore includes the device/inode identities of
  the canonical common and administrative Git directories, not their paths
  alone; a real move preserves the ID and a real recreation changes it.
- A configured root sees the same repository through each linked worktree's
  `.git` marker. Inventory groups those sources by canonical common Git
  directory before detailed inspection, preventing quadratic status and
  reachability work while preserving merged source evidence.
- Newly observed missing/prunable registrations need their ID recovered from
  bounded administrative `gitdir` metadata because the checkout path is no
  longer inspectable. Without that enrichment they would disappear from first
  inventory instead of being reported `missing`.
- macOS may deny both pathname and metadata queries for an open vnode owned by
  a SIP-protected same-user system process. The inspector first tries the path,
  then a bounded scan-local device/inode comparison; denial of both remains an
  explicit fail-closed refusal naming only the process key.
- Ambient `GIT_*` variables can redirect otherwise explicit `git -C` commands.
  The adapter removes them all and supplies only its fixed no-prompt,
  no-pager, no-optional-lock environment.
- Path-bearing operating-system errors cannot cross the worktree boundary:
  traversal failures become bounded categories before audit or action storage,
  and sentinel-filename regressions prove the durable evidence is path-free.
- A check followed by pathname execution cannot bind user-writable Git across
  replacement. ghostgc therefore snapshots its exact opened bytes into a
  private content-addressed state-directory file, binds the SHA-256 digest, and
  revalidates source and snapshot identities for every command. Immutable
  system Git does not require a copy.
- Worktree selectors are literal lowercase hexadecimal IDs or prefixes; SQL
  wildcard characters never become authority.
- Registration absence is distinct from a registered-but-missing checkout.
  The former preserves its last actual observation and is hard-capped and
  age-retained, while the latter refreshes because Git still registers it.
- Every persistence failure after a possible native side effect includes the
  surviving-branch fact and conditional recreation command.

## VALIDATION

- Focused worktree, daemon, storage, configuration, API and CLI package tests
  passed, including disposable real-repository removal, branch preservation,
  stable move/recreation identity, every dirty/protection class, exact
  seven-day transitions, approval expiry/replay/restart/invalidation, link
  restoration, durable pre-side-effect ordering and unresolved post-removal
  persistence evidence.
- Independent-review regressions passed for path-free durable traversal
  failures, writable-Git source replacement with a pinned execution snapshot,
  literal selector validation, more than 500 absent identities with hard and
  age-based compaction, and combined verification/persistence recovery output.
- `make check` passed formatting, vet, all package tests and the repository-wide
  300-line source-size gate.
- `make race` passed every package. The macOS linker emitted its existing
  malformed `LC_DYSYMTAB` warnings for cgo test objects without failing.
- `make lint` passed with zero issues.
- `GOOS=linux CGO_ENABLED=0 go test -exec=/usr/bin/true ./...` compiled every
  package and test for Linux, including inventory and removal refusal surfaces.
- `GHOSTGC_LIVE_WORKTREE_TEST=1 go test ./internal/daemon -run
  TestLiveDarwinWorktreeRemoval -count=1 -v` passed the controlled macOS path.
  This host's `progressd` denied both vnode path and metadata inspection, and
  the test proved the disposable worktree and branch remained unchanged.
- `make build` plus direct human and JSON CLI checks against an isolated daemon
  passed for inventory, ID-prefix detail, status and metrics counters, empty
  action history, and a non-stale preview refusal. The disposable fixture was
  moved to Trash after validation.
- `kit check 0009-stale-worktree-cleanup` passed before completion curation.
- `kit reconcile --all --output-only` confirmed 174 eligible handwritten
  source/test files with zero over 300 lines. It also reported the established
  project baseline of 19 legacy-spec/missing-summary errors and five instruction
  or legacy-spec warnings outside this feature.

## OUTCOME

ghostgc now inventories registered local worktrees from observed session
repositories and optional bounded roots, persists stable identities and
aggregate protection evidence, and advances a stale classification only after
seven uninterrupted days of complete evidence. Status, metrics, CLI and the
local v1 API expose inventory, detail and durable action history.

On macOS, one exact stale ID can receive a two-minute memory-only approval.
Apply consumes it once, serializes with scans and other worktree actions,
revalidates every bound process, filesystem, Git, configuration and executable
fact, commits `attempting`, and calls native non-force `git worktree remove`.
Verified environment links are restored on failure; successful removal is
recorded only after both path and registration disappear. Branches remain and
every result provides a recreation command.

There is no automatic worktree removal, raw-path authority, force, prune,
branch deletion, network lookup, shell invocation or recursive-delete path.
Linux can inventory but refuses removal because complete targeted path-usage
evidence is not implemented there.

## REPOSITORY MEMORY

- This spec is the canonical rationale, discoveries and safety contract for
  manual stale-worktree cleanup.
- Demonstrated project-wide invariants are curated into
  `docs/CONSTITUTION.md`; architecture, safety, testing, dogfooding and manual
  operation are curated into their existing canonical references.
