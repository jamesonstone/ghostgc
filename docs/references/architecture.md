# Architecture Reference

How the daemon is put together and why. Feature-specific rationale and
superseded decisions live in `docs/specs/<feature>/SPEC.md`.

```
                     ┌──────────────────────────────┐
                     │ Agent adapters               │
                     │ internal/adapters/codex      │
                     │  detect roots, attribute      │
                     └───────────────┬──────────────┘
                                     │ evidence
┌──────────────────────────┐   ┌─────▼──────────────────────┐
│ Platform                 │   │ Session reconciliation      │
│ internal/platform/darwin │──▶│ internal/sessions           │
│  sysctl + libproc        │   │  durable ownership, audit   │
└──────────────────────────┘   └─────┬───────────────────────┘
            │ snapshot                │ records
┌───────────▼──────────────┐   ┌──────▼──────────────────────┐
│ Process model            │   │ State store                 │
│ internal/process         │   │ internal/storage (SQLite)   │
│  keys, trees, redaction  │   │  WAL, retention, audit log  │
└──────────────────────────┘   └──────┬──────────────────────┘
                                      │
┌──────────────────────────┐   ┌──────▼──────────────────────┐
│ Protections              │──▶│ Daemon                      │
│ internal/protection      │   │ internal/daemon             │
└──────────────────────────┘   └──────┬──────────────────────┘
                                      │ unix socket
                               ┌──────▼──────────────────────┐
                               │ Control API and CLI         │
                               │ internal/api, cmd/cli       │
                               └─────────────────────────────┘
```

The deterministic classifier sits after activity collection. The policy engine
combines its current exact-key conclusions with hard protections to produce
audit, recommendation or narrowly enforceable decisions before the scan
transaction is persisted.

A separate bounded worktree lane uses `internal/worktree` to discover local Git
registrations from session repositories and configured roots. It reduces Git
and filesystem inspection to identity, counts and fingerprints, then persists
the inventory beside process state. Worktree removal is never part of policy
evaluation or automatic enforcement. A resolved Git binary that is writable by
the daemon user is copied once into a private, content-addressed execution
snapshot under the state directory; immutable system Git executes in place.
Every command revalidates the resolved source and the execution object.

The CLI and daemon are two process roles of the same `ghostgc` executable.
Short-lived commands use the Unix socket; `ghostgc daemon` owns the persistent
observation loop. Keeping the process boundary preserves isolation while one
artifact makes installation and service registration self-contained.

## The observation cycle

Every 15 seconds by default:

1. **Snapshot.** One `sysctl(kern.proc.all)` call returns the entire process
   table in a single syscall — 1 ms for 1464 processes. Processes not owned by
   the current user are counted and dropped.
2. **Detail pass.** A bounded worker pool resolves the executable path
   (`proc_pidpath`), the argument vector and allowlisted environment variables
   (`kern.procargs2`), the session id (`getsid`), the working directory and task
   counters (`proc_pidinfo`) for the remaining processes — 24 ms for 1121.
3. **Tree.** Parent links are classified and only believable ones become edges.
4. **Detection.** Each adapter identifies its session roots from the snapshot.
5. **Attribution.** Each inspected process is offered to each adapter; the
   highest-confidence answer wins. A process nothing claims falls back to
   ownership recorded at an earlier observation.
6. **Classify.** Complete exact-key activity evidence becomes an activity state;
   missing evidence remains unknown and detachment remains an independent fact.
7. **Evaluate policy.** Strict exact-match policies produce candidates,
   non-overridable refusals or cooldowns. A decision grants no authority until
   the global and per-policy mode gates are applied after commit.
8. **Persist.** Sessions, processes, ownership, observations, classifications,
   policy decisions, exits and audit
   entries are written in **one transaction**, so a crash mid-cycle cannot leave
   a session recorded without its processes.
9. **Commit.** Only after that transaction succeeds do the reconciler and
   bounded classification windows advance their in-memory views.
10. **Narrow enforcement.** Under global enforce, select at most the first
    current candidate from the singular automatic policy, then hold the scan
    lane through fresh revalidation, the pre-action transaction, the exact
    platform gate and completion evidence. Refusals and cooldowns never enter
    this step.

On the separate 60-second activity cadence, the daemon adds a targeted pass
between attribution and persistence. It validates the exact process key before
and after collection, then samples cumulative CPU and disk counters plus bounded
file and socket counts. Only derived counts and deltas enter the same transaction
as the scan; discovered paths and socket endpoints remain scan-local.

On the separate five-minute worktree cadence, the daemon parses registered
worktrees, merges discovery sources, inspects Git and filesystem state, and
advances an inactivity window only when the entire observation is complete.
Restart, scan gaps, activity, Git changes or incomplete evidence reset that
window. The inventory is persisted in the enclosing scan transaction. An
absent registration keeps its last actual observation timestamp, old absent
rows expire with action retention, and a hard 500-row current-inventory bound
always preserves registered rows before the newest absent records.

Manual preview and apply use a second, serialized path. Preview accepts only a
stored ID or unambiguous prefix and binds two-minute memory-only authority to
every identity, Git, filesystem, process, configuration and inactivity fact.
Apply consumes the token once, holds the scan lane, repeats those checks,
commits `attempting`, then calls native non-force `git worktree remove`. It
verifies both the path and registration are absent before recording `removed`.

Retention compacts every 6 hours.

## Why staged collection

The specification requires it, and the numbers show why: the cheap scan sees
everything for 1 ms, while per-process inspection costs roughly 20 µs each. Had
the detail pass covered all 1464 processes including other users', it would have
bought nothing — those processes can never be attributed or managed — while
adding a third to the scan cost.

Phase 3 adds a genuinely expensive third stage (open files and sockets). It is
gated on successful attribution and its own cadence rather than run per cycle.

## Why only attributed processes are stored

A thousand user processes sampled four times a minute is 5.8 million rows a day.
Beyond the storage budget, persisting them would mean monitoring activity
outside coding-agent sessions, which the specification lists as a non-goal. The
daemon therefore keeps the full snapshot in memory — so `ghostgc explain` can
answer for *any* PID — and writes only what belongs to a session.

## The session graph

Specification section 9 requires a session to be modelled as a graph rather than
only a process tree, "because operating-system reparenting can destroy the
original process tree". Phase 1 handled the destruction by recording ownership.
Phase 2 records *why* each process belongs, as typed, timestamped edges:

| Relationship | Meaning | Attributes? |
| --- | --- | --- |
| `parent-child` | the live parent link, believed only when chronologically possible | yes |
| `original-parent` | who actually created the process, captured at first observation | yes |
| `environment` | the process carries the agent's own session identifier | yes, capped at 0.90 |
| `reparenting` | the moment a live parent link was lost | no |
| `launch` | the non-agent process that started the session root | no |
| `process-group` | shared process group with the session root | no |
| `terminal` | shared controlling terminal or POSIX session | no |
| `repository` | the repository a process is working inside | no |
| `socket`, `file-lock` | live socket counts and repository-lock context | no |

Several edges can support one attribution, so losing a reason does not lose the
session. A process whose parent exited still has its `original-parent` edge, its
`environment` edge and its recorded ownership.

The attributing/context split is the safety-relevant part and is documented in
[safety-model.md](safety-model.md#not-every-relationship-may-establish-ownership).

## Why launch context is recorded

Six Codex servers started by six editor windows and six left behind by a crashed
script are indistinguishable from the agent process alone. The daemon records
the nearest ancestor of each session root that is not itself an agent process,
so a session reads as "launched by Code - Insiders Helper (Plugin)" or "launched
by zsh" or "launched by unknown, the root was already reparented".

That last case is the honest one. A root first seen after its parent exited has
no discoverable launcher, and reporting `launchd` would be presenting the init
process as the cause.

## Why ownership is a table and not a derivation

This is the load-bearing decision of the whole design.

When an agent session's root process exits, the kernel reparents its surviving
children to `launchd`. Every relationship that made those processes explicable
disappears from the live process tree. A daemon that re-derives ownership each
cycle sees an unattributed process with `ppid == 1` and no history — precisely
the situation in which a naive tool concludes "orphan, safe to kill".

`session_processes` records the association the first time it is observed, along
with the original parent PID. Nothing removes it. `UpsertOwnership` will not let
confidence fall or a `root` relation be downgraded. On restart the table is
reloaded into memory.

The result is that reparenting changes the *evidence* ("ownership was recorded
at first observation; the current parent link is reparented") rather than the
*conclusion*.

## Why the process key includes the start time

PIDs are recycled, often within hours on a busy machine. A stored PID is a
dangling reference. `pid:start_time_ns` is not: the kernel's recorded creation
time distinguishes a reused PID from the process that held it before.

This propagates everywhere — the primary key in `processes`, session root
identity, the derived session identifier, snapshot lookups, and the tree
builder's refusal to believe a parent younger than its child. Manual and
automatic pre-action revalidation plus the final platform gate depend on it too.

## Package boundaries

| Package | Responsibility | Depends on |
| --- | --- | --- |
| `internal/process` | process model, PID-safe keys, tree building, redaction | nothing |
| `internal/platform` | the OS interface; `darwin` and `linux` implementations behind it | `process` |
| `internal/adapters` | the adapter contract, evidence, confidence combination | `process` |
| `internal/adapters/codex` | Codex detection and attribution | `adapters`, `process`, `repository` |
| `internal/repository` | repository root, branch and lock metadata | nothing |
| `internal/worktree` | bounded Git discovery, stable identity, status reduction and staleness | nothing |
| `internal/storage` | SQLite schema, writes, queries, retention | nothing |
| `internal/sessions` | reconciliation, durable ownership, audit emission | `adapters`, `process`, `storage` |
| `internal/protection` | hard protections | `adapters`, `process` |
| `internal/classification` | deterministic evidence-to-state rules; no policy or action | `process` |
| `internal/policy` | bounded YAML policy matching, hard refusals and cooldown decisions | `config`, `protection` |
| `internal/config` | configuration, path validation and authority bounds | nothing |
| `internal/api` | socket transport, request and response types | `adapters`, `protection`, `storage` |
| `internal/daemon` | the loop, the API backend, diagnostics | everything above |

The platform implementations deliberately do not import `internal/platform`, so
the factory can live there without an import cycle; the adapting wrappers in
`factory_darwin.go` and `factory_linux.go` bridge the two.

`internal/api` defines a `Backend` interface that the daemon implements, so the
transport knows nothing about observation and the daemon knows nothing about
HTTP.

## Storage schema

| Table | Contents |
| --- | --- |
| `meta` | schema version, daemon key/values |
| `processes` | one row per attributed process, keyed `pid:start_time_ns` |
| `process_observations` | lightweight time series from every process scan |
| `process_activity` | bounded phase-3 deltas and availability flags |
| `process_classifications` | phase-4 state, basis, detachment, stable window and evidence |
| `policy_evaluations` | unique committed phase-5 projections, including empty results |
| `policy_decisions` | phase-5+ candidates, refusals, cooldowns and evidence |
| `actions` | phase-6+ pre-side-effect attempts, manual/automatic authority and final outcomes with evidence |
| `worktrees` | registered inventory, discovery sources, inactivity window, protections and removal tombstones |
| `worktree_actions` | manual worktree attempts and removed, rejected or failed outcomes |
| `scans` | one row per cycle, including failures |
| `sessions` | one row per detected session |
| `session_processes` | durable ownership, never downgraded |
| `session_relationships` | typed graph edges, first-seen preserved |
| `audit_log` | every state transition and decision, with evidence |

All tables are `STRICT`. WAL is on, `synchronous` is `NORMAL`, and the pool is
capped at one connection so there is no lock-retry logic to get wrong.

Migrations run in order, each in its own transaction, with the version recorded
as each completes; a partial sequence is therefore impossible. Every migration
so far only adds. A database written by a newer build is refused rather than
downgraded, because downgrading would silently drop columns it depends on.

## Session states

```
                 root younger than one scan interval
   (none) ─────────────────────────────────────────▶ starting
      │                                                  │
      │ root already running when first observed          │ root survives the window
      ▼                                                  ▼
   active ◀───────────────────────────────────────────────
      │
      │ the exact root process is no longer present
      ▼
  completed
```

Session lifecycle remains `starting`, `active` or `completed`. Phase 4 records a
separate per-process activity state: `active`, `waiting`, `idle`, `suspicious`,
`hung`, `crashed`, `orphaned` or `unknown`. Strong hung/orphaned conclusions need
five continuous minutes of complete exact-key evidence. Unknown remains
protected and no activity state authorises an action.

The transition out of a live state is by process *key*, never PID: a recycled
PID cannot keep a finished session alive, and the evidence for the transition
says so explicitly when the PID is in use by something else.

## Adding an agent adapter

Implement `adapters.AgentAdapter`:

- `EnvKeys` — the environment variables you need. The union across enabled
  adapters is all the collector extracts; ask for nothing and nothing is copied.
- `DetectRootProcesses` — find session entry points. Gate root detection on
  evidence that a process **is** the agent binary. Environment variables are
  inherited by descendants and prove membership, never identity.
- `ExtractSessionMetadata` — working directory, repository, terminal, invocation.
- `AttributeProcess` — which session owns a process.
- `NativeSessionID` — the agent's own session identifier carried by a process,
  when it carries one. Membership evidence only: environments are inherited.
- `ProtectedPatterns` — classes you refuse to see terminated.

Match executable basenames exactly and path components as whole segments. Do not
pattern-match raw command lines: a developer machine has many processes with an
agent's name somewhere in their arguments, and none of them is that agent.
