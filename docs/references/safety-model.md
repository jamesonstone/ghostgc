# Safety Model Reference

What ghostgc guarantees and where each guarantee is enforced. Project-wide
invariants are stated in `docs/CONSTITUTION.md`; this document is the map from
an invariant to the code and test that hold it.

This document lists what ghostgc guarantees, and where in the code and tests
each guarantee is enforced. Nothing here is aspirational: every item names the
mechanism.

## The one that matters most

**Each authorized action can send only one fully gated SIGTERM.**

The daemon can reach `Platform.SignalProcess` only after either a current exact
recommendation consumes a one-time manual approval or the newest committed
evaluation contains a candidate under the singular automatic policy. Every
fact is freshly revalidated in both paths. This is checked from both directions:

| Check | Where |
| --- | --- |
| Non-TERM and changed exact identities are refused | `internal/platform/signal_gate_test.go:TestPlatformRejectsNonTERMAndChangedIdentity` |
| A changed executable image is refused beside the signal syscall | `internal/platform/darwin/signal_test.go` |
| Exactly one literal SIGTERM primitive exists; alternatives are forbidden | `TestSignalPrimitiveIsSingleLiteralSIGTERM` — walks the whole repository |
| No source file shells out to `kill`, `pkill` or `killall` | `TestNoSourceFileShellsOutToATerminator` |
| The daemon proves both refusals at runtime | `ghostgc doctor`, check `signal-safety-gate` |
| Observation and policy evaluation never signal | daemon policy tests assert zero fake-platform attempts |
| Manual action is single-use and exact-key | `internal/daemon/action_test.go` covers success, replay and changed identity |
| Automatic action is singular, current and globally capped | `internal/daemon/enforcement_test.go` covers one/two candidates, global cap and changed image |

The durable action row is committed in `attempting` state before SIGTERM. The
post-call transaction records `signalled` or `failed`; a preflight refusal
records `rejected`. `signalled` means the operating system accepted the signal,
not that ghostgc has fabricated proof of process exit.

## Cache authority is independent and provider-specific

Cache handling does not reuse process policy, signal approvals or executable
authority. It is disabled by default, has no automatic mode, and accepts only
`disabled`, `audit` or `recommend`. Mutation needs cache-global `recommend`, one
enabled exact policy and a fresh single-artifact approval.

The sole real provider is pinned to Codex shell snapshots. It derives only
`CODEX_HOME/shell_snapshots` roots from observed Codex sessions, requires an
exact configured-root match, and accepts
only an immediate regular `<thread-id>.<generation>.sh|ps1` whose thread ID maps
to exactly one completed session with zero live claimants. It observes names and
filesystem metadata without opening file contents. Location, age, size or a
plausible name never establishes ownership.

| Check | Where |
| --- | --- |
| Exact provider names, ownership, ambiguity, links, UID/device and traversal limits | `internal/cacheprovider/codex/provider_test.go` |
| Two committed unchanged observations are required | `internal/cachepolicy/evaluate_test.go` |
| Component-walked descriptor observation, quarantine, restore, and link/destination refusals | `internal/cachefs/real_unix_test.go` |
| Approval expiry, replay, concurrency, config/identity/session drift and PID reuse | `internal/daemon/cache_*_test.go` |
| Full reversible lifecycle and grace-gated purge | `TestCacheLifecycleFixture` and `tests/end-to-end/local/cache-lifecycle-test.sh` |
| Interrupted and partial actions remain durable | `internal/storage/cache_test.go`, `TestPartialPurgeRemainsVisible` |
| One foreground cache unlink and native worktree removal; no broad deletion or shell `rm` | `TestIrreversibleFilesystemCapabilitiesAreForegroundOnly` |

Cleanup never deletes the observed path. After full fresh revalidation it first
commits `attempting`, then atomically renames exactly one inode into a private
0700 provider-local quarantine. `EXDEV` has no fallback. Quarantine is
reversible containment and explicitly reclaims zero bytes. Restore requires an
absent original destination and a separate approval.

Permanent deletion can name only an exact quarantined file. It requires elapsed
grace, unchanged policy/configuration/identity/manifest, a second approval, the
complete artifact ID, and a durable `purging` record. The daemon returns one
expiring exact plan but has no unlink capability. Only the foreground CLI calls
the descriptor-anchored `unlinkat`; the daemon independently verifies absence.
An ambiguous result opens a mutation circuit until daemon restart.

## Configuration cannot widen what the daemon does

`config.Validate` accepts `disabled`, `audit`, `recommend` and `enforce` while
keeping audit as the default. Global mode is a hard upper bound, so a policy is
not manually or automatically actionable without corresponding global consent.
`privacy.storeSourceContents: true` and
`privacy.networkTelemetry: true` are likewise startup errors. A misspelled key
is an error rather than a silently ignored setting.

Tested in `internal/config/config_test.go`.

Cache and worktree roots add discovery, never general deletion authority. Cache
roots must exactly name canonical `shell_snapshots` provider paths and are
opened component by component without following links. Worktree validation permits at
most 32 absolute canonical directories, refuses symlinks, filesystem roots and
the invoking user's whole home, and limits traversal to four levels without
following symlinks. `staleAfter` cannot be shorter than seven days.

## Worktree retirement is reversible and finalization is foreground-only

No policy or daemon scan can mutate a worktree. A candidate must be a present,
registered secondary worktree with seven continuous days of complete evidence.
Primary, locked, missing, prunable, dirty, unreadable, symlinked, active or
operation-in-progress worktrees are protected. Local-only commits, unsafe
detached commits, submodule metadata, nested mounts, same-user path use and any
incomplete inspection also protect it.

The macOS inspector normally compares vnode paths. If SIP withholds a path, it
can compare the vnode's device/inode metadata with a bounded scan-local identity
set for the candidate without retaining a filename. If both forms are denied,
the inspection remains incomplete and removal is refused.

Preview accepts only a persisted ID or unambiguous prefix. Its random
memory-only token expires after two minutes and binds the directory inode,
common and administrative Git directories, HEAD/ref/branch, status
fingerprint, inactivity window, discovery authority, allowed environment links
and exact Git executable. Apply consumes it once, serializes with scans and
other actions, and repeats every check. Any changed or unknown fact is a
durable rejection.

If the resolved Git executable or one of its parent directories is writable by
the daemon user, ghostgc executes a private content-addressed snapshot instead
of the package-managed path. The SHA-256 digest is bound while the source and
private execution-object identities are revalidated for every Git command and
again immediately before mutation. Snapshot reads accept only regular files up
to 128 MiB and reject size changes. Immutable system Git executes at its
canonical path under the same identity checks.

The `attempting` action and audit evidence commit before native non-force
`git worktree move` retires the checkout to an absent same-filesystem sibling.
The original path, retirement path, inode, registration, branch, and grace
deadline are durable. A separate approval can restore it to the absent original
path.

Permanent finalization requires elapsed grace, another fresh validation, a
separate approval, and the complete worktree ID. The daemon commits `purging`
and returns one exact expiring plan but cannot invoke native removal. The
foreground CLI pins Git, removes only verified root environment links, and
calls non-force `git worktree remove`. Approved links are opened through the
exact worktree directory descriptor and rechecked immediately beside the one
unlink site, so replacing a link with a regular file is refused. The daemon
records `removed` only after proving path and registration absence. There is no
force, prune, branch deletion, network request, shell invocation, or
application-owned recursive filesystem delete.

Tested by `internal/worktree/*_test.go` and daemon worktree lifecycle, scan, and
approval tests using disposable real repositories.

## Residual filesystem authority

ghostgc cannot promise that deletion can never occur while a foreground purge
runs with the user's normal filesystem permissions. A malicious or compromised
same-user process already has comparable authority and can race filesystem
names or Git itself; native non-force `git worktree remove` also owns its
documented checkout removal behavior. The safeguards above make accidental
broad deletion structurally unavailable to the daemon, constrain foreground
execution to one exact approved object, preserve a reversible stage and stop
new mutation after an ambiguous result. They do not form an operating-system
sandbox or recover a kernel-accepted unlink. Mission-critical data should not
be placed in provider cache roots or retired disposable worktrees, and ordinary
backups remain necessary.

## Policies cannot widen authority

Policies accept `disabled`, `audit`, `recommend` or `enforce`. Their schema is strict and
non-Turing-complete: exact `orphaned`, `hung` or `crashed` states, agent IDs and
executable basenames, plus explicit booleans and durations. `suspicious` is
never eligible because it means progress or live resources remain. Validation
rejects broad protected runtimes, weak/unknown states, unknown agents, unsafe
windows and duplicates. Enforce additionally requires `automatic: true`, one
enabled enforce policy total, exactly one orphaned state, agent and executable,
detachment, an ended session, at least five stable minutes and a one-hour
cooldown. Global `disabled` caps every
individual policy. A policy is applied only after hard protections; every
triggered protection becomes an immutable refusal reason and cannot be
overridden. Cooldowns are keyed by policy plus `pid:start_time_ns`, so PID reuse
cannot inherit eligibility or suppression, and active cooldown rows survive
ordinary and aggressive retention.
Retention also preserves the latest evaluation as an indivisible projection;
compaction cannot make only part of a current decision set disappear.

Recommendation still grants no automatic authority. Preview binds the unique
evaluation and decision IDs, exact process key, executable path and kernel
name, session, classification evidence and canonical policy. The bearer token
is random, memory-only, single-use and valid for two minutes. Apply serializes
with scans and then freshly snapshots, reconciles, samples, classifies,
protects and evaluates. The platform repeats exact-key and executable-image
validation immediately beside the sole signal primitive. Any unknown or
changed fact fails closed.

Automatic selection uses only `candidate` from the newest committed evaluation,
never a refusal or cooldown, and returns after the first deterministic match.
The scan lane remains held through revalidation and both action transactions.
The same cooldown suppresses a retry after success, refusal or platform failure.
Durable actions identify `manual` or `automatic` authority.

Tested in `internal/config/policy_test.go`, `internal/policy/policy_test.go` and
`internal/daemon/policy_test.go`.

## A process is never identified by PID alone

Every process is keyed by `pid:start_time_ns` (`process.Key`). The consequences:

- a recycled PID produces a **new** database row, and cannot inherit the
  previous process's session, evidence or history
  (`storage_test.go:TestRecycledPIDIsADifferentRow`);
- a session ends when its exact root *process* disappears, not when its root PID
  stops appearing — a recycled PID cannot keep a finished session alive
  (`sessions_test.go:TestRecycledRootPIDDoesNotResurrectASession`);
- a session identifier derived from a root process changes when the root is a
  different process with the same PID
  (`codex_test.go:TestDeriveSessionIDIsStableAndStartTimeSensitive`).

## A parent link is only believed when it is chronologically possible

`process.BuildTree` classifies each parent link as intact, reparented,
impossible, missing or root. A child whose claimed parent started *after* it did
is `impossible`: the original parent exited and the PID was recycled, so the
link is a coincidence. Impossible and missing links do not create parent/child
edges at all.

Tree walks are cycle-safe and depth-bounded, so inconsistent data cannot spin
the daemon (`tree_test.go`).

## Ownership survives reparenting

When a process's parent exits, the kernel reparents it to `launchd`. The live
process tree no longer shows the relationship. The daemon records ownership the
first time it observes it, together with the original parent PID, and
`session_processes` rows are never downgraded: relation `root` stays `root`,
confidence only rises, and the original parent is written once and never
updated.

A process whose live attribution has disappeared falls back to that record with
relation `recorded`, and the transition is written to the audit log.

Tested in `sessions_test.go:TestOwnershipSurvivesReparenting`,
`TestSeedRestoresOwnershipAcrossRestart` and
`storage_test.go:TestOwnershipIsDurableAndNeverDowngraded`.

**Detached is not orphaned.** Nothing in the codebase concludes that a
reparented process is disposable. Phase 4 records detachment independently;
`orphaned` requires five continuous minutes of complete known-idle evidence
after the owning session ended. Even that classification grants no authority.

## Unknown is protected

`internal/protection` evaluates the hard protections from specification section
12.4. A process triggers a protection when:

- it is the daemon itself, or PID 1;
- it is owned by another user;
- it has a controlling terminal, or is a terminal session leader;
- it is an agent session root, or its session is still active;
- it has live descendants that would be orphaned;
- its attribution confidence is below 0.95;
- it was never inspected in detail;
- its executable belongs to a protected class: editors, language servers,
  container runtimes, database servers, build and test tools, development
  servers, or the broad runtime names specification section 14 rules out
  (`node`, `python`, `go`, `java`, `git`, `bash`, `zsh`, …);
- an agent adapter contributed a rule that matches it.

Every protection carries a human-readable reason, asserted by
`protection_test.go:TestEveryProtectionExplainsItself`. Protections are
evaluated and reported today by `ghostgc explain`, so the refusal a future
policy engine would produce is already visible.

## Confidence is bounded and is not the only control

Independent evidence weights combine with a noisy-or, capped at 0.99: heuristic
agreement is never reported as certainty (`TestConfidenceNeverReachesCertainty`).
The thresholds are:

| Threshold | Value | Meaning |
| --- | --- | --- |
| `ConfidencePolicyEligible` | 0.95 | the floor for a policy to consider a process at all |
| `ConfidenceAttributable` | 0.75 | the floor for displaying an attribution |
| `ConfidenceRootDetection` | 0.50 | the floor for treating a process as a session root |

Confidence gates nothing on its own. A process at 0.99 with a controlling
terminal is still protected.

### Identity evidence and membership evidence are not the same thing

An agent's environment variables are inherited by every descendant, including
long-lived daemons started from an agent shell weeks earlier. So
`CODEX_THREAD_ID` is evidence that a process is *inside* a Codex session's
lineage; it is never evidence that the process *is* the Codex program.

The Codex adapter keeps the two apart. Only identity evidence — executable
basename, package path segments, the script a JavaScript runtime was handed —
can promote a process to a session root. Membership evidence raises the recorded
confidence afterwards, and never on its own.

This distinction was not theoretical. Before it existed, running against a real
machine reported a `process-compose` service manager, started from a Codex
session days earlier and since reparented to init, as an active Codex session at
0.90 confidence. The regression test carries that exact process:
`codex_test.go:TestInheritedEnvironmentDoesNotCreateASessionRoot`.

Membership evidence does still attribute: a process carrying an agent's own
session identifier is recorded as belonging to that session, capped at
`ConfidenceEnvironmentMembership` (0.90), which sits deliberately below the
0.95 policy-eligible floor. A variable every descendant inherits forever can
establish lineage and must never establish eligibility for action
(`sessions_test.go:TestEnvironmentMembershipAttributesToTheOwningSession`).

### Not every relationship may establish ownership

The session graph records eight kinds of edge, and only four of them can
attribute a process to a session:

| May establish ownership | Context only |
| --- | --- |
| `parent-child`, `original-parent`, `environment`, `recorded` | `launch`, `terminal`, `process-group`, `repository`, `reparenting` |

The distinction matters most for `terminal`. A POSIX session leader is
normally the user's interactive shell, so every unrelated command that shell
has ever run shares the agent's session id; treating that as ownership would
attribute half a developer's machine to whichever agent happened to be running.
`repository` is the same trap one level up: a popular directory is not a
session. Both are recorded, both are displayed, neither attributes.

Enforced by `sessions.AttributingKinds` and
`sessions_test.go:TestTerminalAndProcessGroupEdgesAreContextNotOwnership`,
which also proves a process sharing only the terminal stays unattributed.

### A session has exactly one root

Several processes can name the same session: an agent that exposes its session
identifier passes it to every helper it starts, and a helper built from the same
executable is detected on its own identity evidence too. The earliest-started
claimant is the root and the rest are recorded as members, with the demotion
explained in the evidence. Two roots for one session would leave "which process
*is* this session" unanswerable, and every later phase depends on that answer
(`sessions_test.go:TestOnlyTheEarliestClaimantBecomesTheSessionRoot`).

### An unobserved parent is reported as unknown, not as 1

A process first seen after its parent exited has been reparented, and the
kernel reports its parent as `1`. Recording that as the creator would present
the init process as having started it, which nobody observed. The daemon stores
whether the original parent was actually seen alive and reports "unknown" when
it was not — in `explain`, in `session show`, and in the stored row
(`sessions_test.go:TestOriginalParentIsUnknownWhenItWasNeverObserved`).

### An unreadable environment is not an empty one

macOS redacts the environment section of `kern.procargs2` for SIP-protected
binaries, so `/bin/sh` and `/bin/sleep` return their arguments and nothing more.
Letting that stand in for "the agent set no variables" would silently convert a
permission limit into a negative finding. The collector records
`EnvReadable`, and `ghostgc explain` says the environment could not be read
rather than implying it was empty.

## Detection does not pattern-match command lines

Regular expressions over raw command lines are not used for ownership. The
adapter matches executable basenames exactly and path components as whole
segments. The negative cases in `codex_test.go` are recorded from a real
machine and would all match a substring search for "codex":

- `/Applications/ChatGPT.app/…/Codex Framework.framework/…/Codex (Renderer)`
- `/Applications/ChatGPT.app/Contents/Resources/codex-code-mode-host`
- `mcp-server-darwin-arm64 --app desktop --agent codexCLI`
- `node /Users/dev/scratch/codex.js`

None of them is detected. A near miss inside a macOS application bundle records
a *conflict*, so `ghostgc explain` shows why it was refused rather than being
silently absent.

## Secrets never reach disk

`process.RedactArgs` runs before any command line is stored or logged. It
redacts by flag name (`--api-key`, `--token`, `--password`, and compound forms),
by value shape (`sk-`, `ghp_`, `github_pat_`, `xoxb-`, `AKIA`, `AIza`, `glpat-`,
JWTs, `Bearer …`), by rewriting URLs that embed passwords, and by redacting
presigned-URL query parameters. It errs toward over-redaction.

`process.RedactEnv` drops every variable outside the adapters' allowlist
entirely, rather than storing the fact that it existed, and redacts the values
of allowlisted names that look sensitive.

The collector applies the allowlist while parsing the kernel buffer, so
non-allowlisted values are never copied into the daemon's memory at all.

Tested in `redact_test.go`, and end-to-end in
`sessions_test.go:TestStoredCommandLinesAreRedacted`.

## Source contents never enter ghostgc

The repository metadata path reads only bounded Git plumbing. Worktree
inspection invokes local Git with machine-readable output and reduces status to
aggregate dirty counts plus a one-way fingerprint. Filenames and file contents
are never retained, returned, logged or persisted. The process activity path
likewise reduces discovered paths to counts before storage.

## Activity evidence fails closed

The expensive activity pass runs only for live processes whose attribution
clears the reporting threshold. The platform validates the complete
`pid:start_time_ns` key and same-user ownership before and after descriptor
inspection, so PID reuse or exit produces unavailable evidence rather than a
sample for the wrong process.

CPU and disk deltas require two ordered, available, monotonic samples for that
exact key. Availability is stored independently for CPU, disk, files and
sockets. Missing evidence therefore remains unknown and can never masquerade as
zero activity. File paths and socket endpoints are used only to derive bounded
counts and are intentionally not persisted. Socket and file-lock relationships
are context-only and cannot establish ownership.

Tested by `process/activity_test.go`, `daemon/activity_test.go` and
`storage/activity_test.go`.

## Failure never becomes action

A failed scan is recorded to `scans` and to the audit log with the summary
"observation cycle failed and was skipped; no conclusion was drawn", the daemon
reports itself degraded, and observation continues on the next tick
(`daemon_test.go:TestFailedScanIsRecordedAndObservationContinues`).

A database that cannot be opened or migrated is moved aside and recreated, and
the recovery is audited. Losing observation history is safe precisely because
nothing is ever concluded from absent history.

Cross-cycle in-memory state advances only after the transaction that persists it
commits, so a failed write cannot suppress the audit entry for a change that was
never recorded (`sessions_test.go:TestStateOnlyAdvancesOnCommit`).

## Migrations only add

Schema migrations run in order, each in its own transaction, with the version
recorded as each completes — so a partial sequence is not possible, and the next
start resumes from the last version that committed. Every migration so far only
adds columns and tables. Ownership is the one thing in this database that cannot
be recomputed from a fresh observation, and a migration that dropped it would
silently unattribute every process belonging to a session that has already
finished. A database written by a newer build is refused rather than downgraded.

Tested in `storage_test.go:TestMigrationPreservesRecordedOwnership` and
`TestDatabaseFromANewerBuildIsRefused`.

## Bounded by construction

| Bound | Value |
| --- | --- |
| Detail-pass worker pool | `min(NumCPU, 8)` |
| Arguments retained per process | 128 arguments, 4 KiB |
| Environment variables retained | the adapters' allowlist only |
| Tree walk depth | 128 |
| Repository root search depth | 48, with a 4096-entry cache dropped wholesale when full |
| Audit query default limit | 100 |
| Repository metadata re-read | once per repository per 30 s |
| `.git/HEAD` and `.git` pointer reads | 4 KiB |
| Configured worktree roots | 32 absolute roots; four traversal levels; 50,000 entries |
| Worktree registrations | 500 merged repository inputs and 500 non-removed inventory rows; registered rows outrank absent history |
| Local Git executable | regular file, at most 128 MiB; private snapshot for user-writable paths |
| Local Git commands | 5 s and 4 MiB output per invocation; no prompts, pagers or optional locks |
| Worktree inventory / manual validation | 30 s per inventory pass / 25 s per preview or apply validation |
| Manual worktree filesystem inspection | 100,000 entries; no symlink traversal |
| Manual same-user path-usage inspection | 4,096 processes; 131,072 descriptors total; 4,096 per process |
| Absent worktree inventory | hard current-row cap plus action retention; unresolved action subjects are preserved |
| Database | retention windows plus a hard byte ceiling that triggers an aggressive pass |
| SQLite connections | 1, so there is no lock-retry logic to get wrong |
| Cache roots | exact `CODEX_HOME/shell_snapshots` roots from known sessions only |
| Cache traversal | `maxEntriesPerScan`, immediate children only |
| Cache action | one artifact per approval, plus entry and byte bounds |
| Cache approvals | 128 memory-only tokens, each single-use and two minutes |
| Cache stable/grace windows | explicit `minStable` and `quarantineGrace` durations |

## Least privilege

The daemon runs as the invoking user. `SnapshotProcesses` filters to
`uid == SelfUID` before any detail pass, so another user's processes are counted
and never inspected. `ghostgc doctor` warns if it is run as root.
