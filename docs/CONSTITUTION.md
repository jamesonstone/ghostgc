# CONSTITUTION

## PRINCIPLES

- Observe before acting. Audit is the default. Recommendation grants only a
  short-lived, single-use manual approval. Enforcement requires global consent
  plus one singular orphan-only automatic policy. Both paths fully revalidate,
  and each authorized action can send only one exact-key SIGTERM.
- Startup authority is capped by the command: `ghostgc start` is audit-only,
  while `ghostgc start --mode reconcile` permits manual recommendation but
  never automatic enforcement. Configuration may narrow either ceiling.
- Every classification carries the observations that produced it. A conclusion
  without evidence is a defect, not a shortcut.
- Fail closed. Where ownership or safety cannot be established, nothing happens.
- Prefer saying "unknown" to saying something convenient. A plausible value the
  daemon did not observe is a fabrication.
- Missing activity evidence is not inactivity. Deltas require two ordered,
  available samples for the same exact process key.
- Worktree staleness is continuous evidence, never age alone. Cleanup first
  retires the checkout reversibly; permanent finalization is later, manual,
  grace-gated, non-force, foreground-only, and branch-preserving.
- Record what was observed when it was observed. Do not re-derive a fact each
  cycle that the operating system can destroy between cycles.
- Run with the least privilege that can do the job, and inspect only what that
  privilege legitimately reaches.
- Keep everything local. No telemetry transport exists in the binary.
- Ship one `ghostgc` executable. `ghostgc daemon` owns the persistent process;
  ordinary commands remain short-lived clients over the local Unix socket.
- Product output describes current capabilities and safety boundaries.
  Delivery phases are development history and belong in documentation only.
- Filesystem lifecycle authority is provider-specific and independent from
  process signalling authority. Observation never grants deletion: one proven
  artifact may be quarantined reversibly, while permanent purge requires a new
  approval after a grace period.
- The persistent daemon never owns permanent filesystem deletion capability.
  Irreversible cache unlink and native worktree removal exist only in exact,
  short-lived foreground executors after full-ID confirmation; ambiguous results
  open a mutation circuit until restart and fresh observation.

## CONSTRAINTS

- A process is never identified by PID alone. Every process is keyed by
  `pid:start_time_ns`, and a parent that started after its child is not believed.
- Action authority binds the executable path and kernel name observed at manual
  preview or automatic selection; a changed or unavailable image is protected.
- Automatic authority may select only a current candidate from the newest
  committed evaluation and may attempt at most one action per evaluation.
- Unknown is protected. Attribution below the policy-eligible threshold, an
  uninspected process, and a process owned by another user are all protected.
- Confidence is combined from independent evidence and capped below 1.0.
  Heuristic agreement is never reported as certainty, and confidence alone
  never authorises anything.
- Evidence that a process belongs to a session is not evidence that it *is* the
  agent. Environment variables are inherited by every descendant, so they
  establish lineage only, capped below the policy-eligible threshold.
- Relationships declare whether they may establish ownership. A shared terminal,
  process group or repository is context, never ownership.
- Recorded ownership is durable. Confidence may not fall, a `root` relation may
  not be downgraded, and the original parent is written once.
- Detection matches executable basenames exactly and path components as whole
  segments. Ownership is never established by matching a command-line substring.
- Source-code contents are never read or retained. The daemon records paths and
  metadata and reads version-control plumbing only; worktree inspection retains
  aggregate dirty evidence, never filenames.
- Cache artifact contents are never read. A cache provider must prove one exact
  pinned standard or explicitly configured root, file contract and exclusive
  session owner from primary-source-backed metadata. Roots are descriptor-walked
  without following components; an unproven, shared, linked, changed or
  incomplete artifact is protected.
- Expensive activity inspection is restricted to live, same-user processes
  already attributed to a coding-agent session. File paths and socket endpoints
  discovered during that pass never reach storage.
- Credentials are redacted before anything reaches storage or a log line.
- Failure never becomes action. A failed observation is recorded, no conclusion
  is drawn from it, and observation continues.
- In-memory state advances only after the write that persists it commits.
- Every queue, cache, worker pool, retained buffer and retention window is
  bounded by construction.
- Schema migrations only add. Recorded ownership cannot be recomputed from a
  fresh observation, so no migration may destroy it.
- A worktree is identified by its canonical common and administrative Git
  directories. Moving the registration preserves identity; recreating it does
  not.
- A worktree needs at least seven uninterrupted days of complete inactivity to
  become stale. Restart, scan gaps, activity, Git changes and unknown evidence
  reset the window.
- Primary, locked, missing, prunable, dirty, active, unreadable or operational
  worktrees are protected, as are local-only commits, unsafe detached commits,
  submodules, nested mounts and incomplete path-usage inspection.
- Worktree cleanup never uses force, prune, branch deletion, network access, a
  shell or recursive filesystem deletion. Durable attempting evidence precedes
  reversible retirement and foreground finalization, and the branch remains.
- Worktree retirement uses native non-force move to an absent same-filesystem
  sibling and durably binds original path, retirement path, inode, registration,
  branch and grace. Restore is separately approved. Finalization requires a new
  complete validation, full stable ID, and daemon verification of path and
  registration absence.
- Worktree Git commands revalidate their resolved executable identity for every
  invocation. User-writable Git is executed only from a private
  content-addressed snapshot of at most 128 MiB bound to the same approval;
  immutable system Git executes at its canonical path.
- An absent worktree record preserves its last actual registration observation.
  Absent inventory is both hard-capped and age-retained; removed tombstones and
  non-attempting actions share the configured action-retention window. A record
  anchoring an unresolved `attempting` action cannot be pruned.

### Kit-Managed Baseline Rules

<!-- BEGIN KIT-MANAGED BASELINE RULES -->
- Treat `docs/CONSTITUTION.md` as the canonical project contract.
- Keep `AGENTS.md`, `CLAUDE.md`, and `.github/copilot-instructions.md` aligned with the repo-local docs tree.
- Treat `docs/notes/<feature>` as optional source material, not canonical truth; promote durable decisions into `SPEC.md`, `docs/CONSTITUTION.md`, or durable references.
- Use native agent planning for research, clarification, design, and implementation planning.
- Before implementation, inspect code and repository memory; create or adopt `SPEC.md` when material rationale exists.
- After validation, curate feature rationale, project invariants, reusable practices, and domain knowledge into their scope-appropriate canonical documents.
- Allow a justified `not required` repository-memory decision when code and tests preserve the complete durable truth.
- Keep every version-control-eligible handwritten implementation/source and test file at 300 physical lines or less.
- Before delivery, audit the complete affected source/test scope; whole-project reconcile and scheduled maintenance audit the entire repository.
- Exclude documentation files, all `docs/**`, all `.kit/**`, `.kit.yaml`, ignored files, vendored dependencies, and proven generated files.
- Split oversized files by semantic responsibility while preserving stable public entry points and behavior; never use minification or arbitrary numbered chunks to claim compliance.
<!-- END KIT-MANAGED BASELINE RULES -->

## CHANGE CLASSIFICATION

<!-- all work falls into one of two tracks — classify before acting -->

### Repository-Memory Work

<!-- use when: consequential product rationale, architecture, cross-component behavior, or historical decisions must survive -->
<!-- workflow: native plan → create/adopt SPEC.md before code → implement → validate → curate repository memory -->
<!-- legacy staged documents: BRAINSTORM.md, legacy SPEC.md, PLAN.md, TASKS.md only when explicitly chosen -->

### Ad Hoc (Lightweight)

<!-- use when: bug fixes, security reviews, refactors, dependency updates, config changes, small refinements -->
<!-- workflow: understand → implement → verify -->
<!-- docs: update practical canonical docs when behavior changes -->
<!-- do not create feature SPEC.md solely for ceremony; report a justified not-required memory decision -->

### Ad Hoc with Existing Specs

<!-- if change touches code with existing spec docs: update them when rationale, behavior, requirements, or approach changes -->
<!-- leave them unchanged when code and tests communicate the complete durable truth -->

## NON-GOALS

- A general macOS or cache cleaner. ghostgc does not scan broad cache roots,
  optimise settings, invoke package-manager cleanup or manage unattributed
  files. Cache lifecycle is limited to explicitly implemented provider
  contracts with exclusive coding-session ownership; worktree inventory is
  limited to coding-agent sessions and explicitly configured local roots.
- Monitoring user activity outside coding-agent sessions. Unattributed
  processes are counted during a scan and then forgotten.
- Replacing Activity Monitor, `ps`, `top` or `launchctl`.
- Terminating a process on the strength of its executable name or its age.
- Restarting, reconfiguring or recovering agents automatically.
- Using a language model to make a termination decision. Classification is
  deterministic rule evaluation.
- Requiring elevated privileges, a cloud service, or any network transport.
- Attempting automatic recovery from every detected problem.

## DEFINITIONS

- **Agent** — a coding-agent runtime such as Codex or Claude Code.
- **Session** — one logical execution of an agent, identified by its root
  process and, when the agent exposes one, its own session identifier.
- **Root process** — the entry point of a session. A session has exactly one;
  when several processes claim it, the earliest-started wins.
- **Descendant** — a process launched directly or indirectly by a session root.
- **Detached** — a process no longer attached to its original parent or
  terminal. Detached does not imply orphaned.
- **Orphaned** — a process whose session has ended *and* for which strong
  evidence indicates no useful work remains.
- **Hung** — a process unable to make progress while its session is active.
- **Protected** — a process that must never be terminated automatically.
- **Cleanup candidate** — a process matching an enabled cleanup policy, which
  is not the same as a process that will be acted on.
- **Cache artifact** — one provider-contracted regular file whose metadata maps
  exactly to one known agent session; a name or location alone is insufficient.
- **Quarantine** — a reversible same-filesystem rename into a private
  provider-root location. It is containment and does not reclaim disk space.
- **Worktree** — one registered checkout identified by its canonical common and
  administrative Git directories, independently of its current path.
- **Stale worktree** — a present, registered, unprotected secondary worktree
  with at least seven continuous days of complete inactivity evidence.
- **Retired worktree** — a registered checkout moved intact to its deterministic
  sibling retirement path and still recoverable until separately finalized.
- **Identity evidence** — evidence that a process *is* an agent program.
  Executable and argument derived.
- **Membership evidence** — evidence that a process is *inside* an agent
  session's lineage. Environment derived, and capped accordingly.
- **Attributing relationship** — a graph edge permitted to establish ownership,
  as distinct from a context edge that merely describes circumstance.
