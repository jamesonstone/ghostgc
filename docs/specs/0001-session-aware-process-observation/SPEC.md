# SPEC: Session-Aware Process Observation

Issue: [#3](https://github.com/jamesonstone/ghostgc/issues/3)
Delivery phases: 1 (observation foundation) and 2 (session graph).

## CONTEXT

Coding agents spawn processes: shells, test runners, headless browsers, MCP
helpers, language servers. When a session ends badly some of them survive. The
usual reaction is `pkill` plus a name pattern, which is how people kill the
language server their editor was using or a database that happened to be
started from an agent shell.

The hard part is not termination. It is knowing, with evidence, *which session
a process belongs to* — after the operating system has reparented it to
`launchd`, after its PID has been recycled, and when several unrelated things
on the machine have the agent's name somewhere in their command line.

This feature solves that part and deliberately ships with no ability to act on
the answer.

## SCOPE

Delivered:

- A persistent local daemon (`ghostgc daemon`) and short-lived CLI commands from
  the same `ghostgc` executable, communicating over a Unix socket.
- macOS process collection via `sysctl(kern.proc.all)` and `libproc`.
- PID-reuse-safe process identity and process-tree reconstruction.
- Codex session detection with evidence-scored attribution.
- A typed session graph: parent-child, original-parent, reparenting, launch,
  process-group, terminal, repository and environment edges.
- Durable ownership, session state machine, repository and terminal association.
- SQLite state with migrations, retention and an audit trail.

Explicitly not delivered: any ability to signal a process. Activity tracking
(phase 3), classification beyond liveness (phase 4), the policy engine (phase
5), recommended cleanup (phase 6), enforcement (phase 7), further adapters
(phase 8) and the Linux collector (phase 9).

## DECISIONS

### Ownership is a table, not a derivation

This is the load-bearing decision.

When a session root exits, the kernel reparents its surviving children to
`launchd`, and every relationship that made those processes explicable
disappears from the live process tree. A daemon that re-derives ownership each
cycle sees an unattributed process with `ppid == 1` and no history — precisely
the situation in which a naive tool concludes "orphan, safe to kill".

`session_processes` records the association the first time it is observed,
along with the original parent. Nothing removes it; confidence cannot fall and
a `root` relation cannot be downgraded. Reparenting therefore changes the
*evidence* ("ownership was recorded at first observation; the current parent
link is reparented") rather than the *conclusion*.

### Process identity includes the start time

PIDs are recycled, often within hours. A stored PID is a dangling reference;
`pid:start_time_ns` is not. This propagates to the `processes` primary key,
session root identity, derived session identifiers, snapshot lookups, and the
tree builder's refusal to believe a parent that started after its child.

Phase 6's pre-action revalidation will depend on it, which is why it was built
in from the first commit rather than retrofitted.

### Identity evidence and membership evidence are different claims

Environment variables are inherited by every descendant forever. So
`CODEX_THREAD_ID` proves a process is *inside* a session's lineage; it never
proves the process *is* the agent program.

Only identity evidence — executable basename, package path segments, the script
a JavaScript runtime was handed — can promote a process to a session root.
Membership evidence attributes a process to a session but is capped at 0.90,
below the 0.95 policy-eligible floor, so an inherited variable can establish
lineage and never eligibility for action.

**Superseded:** the first implementation weighted `CODEX_SESSION_ID` at 0.90 as
ordinary detection evidence. See DISCOVERIES.

### Not every relationship may establish ownership

`sessions.AttributingKinds` splits graph edges into those that can attribute
(`parent-child`, `original-parent`, `environment`, `recorded`) and those that
are context only (`launch`, `terminal`, `process-group`, `repository`,
`reparenting`).

A POSIX session leader is normally the user's interactive shell, so every
unrelated command that shell ever ran shares the agent's session id. Treating
that as ownership would attribute half a developer's machine to whichever agent
happened to be running. `repository` is the same trap one level up: a popular
directory is not a session.

### Only attributed processes are persisted

Roughly 1,100 user processes sampled four times a minute is 5.8 million rows a
day. Beyond the storage budget, persisting them would mean monitoring activity
outside coding-agent sessions, which the specification lists as a non-goal.

The daemon keeps the full snapshot in memory — so `explain` can answer for *any*
PID — and writes only what belongs to a session.

### Detection never pattern-matches command lines

The adapter matches executable basenames exactly and path components as whole
segments. A substring search for "codex" on the development machine matches an
Electron framework inside ChatGPT.app, a `codex-code-mode-host` helper, and an
unrelated binary invoked with `--agent codexCLI`. None is a Codex CLI session.

### Say "unknown" rather than something convenient

Two cases where a plausible-looking value would have been a fabrication:

- A process first observed after its parent exited reports its creator as
  unknown, not as `launchd`. The kernel reports `ppid == 1`; nobody observed
  that process create anything.
- A process whose environment the operating system refuses to disclose reports
  exactly that, rather than letting "unreadable" pass for "no agent variables
  were set".

### A session has exactly one root

Several processes can name the same session. The earliest-started claimant is
the root and the rest are recorded as members, with the demotion explained in
the evidence. Two roots would leave "which process *is* this session"
unanswerable, and every later phase depends on that answer.

## DISCOVERIES

Each of these came from running against a real machine, not from review.

### An inherited environment variable created a false session

A `process-compose` service manager — started from a Codex session days earlier
and long since reparented to init — was reported as an **active Codex session at
0.90 confidence**, because it had inherited `CODEX_THREAD_ID`.

The weight was not the problem; the model was. This produced the identity /
membership split above. Regression test:
`TestInheritedEnvironmentDoesNotCreateASessionRoot`, carrying that exact
process.

### Six "mystery sessions" were not a defect

Six `codex … app-server` processes appeared as six separate one-process
sessions. Investigation showed six distinct parents — one
`Code - Insiders Helper (Plugin)` extension host per editor window — and
`cwd = /`. They are genuinely six servers; merging them would have been wrong.

What was missing was *launch context*. Recording the nearest non-agent ancestor
turned six mysteries into "launched by Code - Insiders Helper (Plugin)".

### macOS withholds the environment of system binaries

The kernel redacts the environment section of `kern.procargs2` for
SIP-protected binaries, so `/bin/sh` and `/bin/sleep` return arguments and
nothing more to an unprivileged caller. The collector was recording that as "no
agent variables set", silently converting a permission limit into a negative
finding. `process.Process.EnvReadable` now distinguishes the two.

### Two processes could claim the same session root

An agent that exposes its session identifier passes it to every helper it
starts, and a helper built from the same executable is detected on its own
identity evidence too. Both named the same session and both were recorded as its
root. Resolved by the earliest-claimant rule above.

### The unix socket path limit is a real constraint

`sun_path` is capped near 104 bytes on macOS and a long temporary directory
pushed the test socket over it, surfacing as an opaque `bind: invalid argument`.
Both `config.Paths.Validate` and `api.Server.Listen` now check the length and
say so.

## VALIDATION

| Layer | Command | Required |
| --- | --- | --- |
| Format, vet, tests | `make check` | yes |
| Race detector | `make race` | yes |
| Lint | `golangci-lint run ./...` | yes |
| File size gate | `make size` | yes |
| Live process tree | `fixtures/fixture-agent.sh start\|orphan\|stop` | manual |

127 tests pass under `-race`. Every safety condition has a test and none was
weakened to make it pass.

The fixture exercises the real macOS collector against a known-shaped tree: a
session root, a worker shell, an idle child, a periodic-work child and a helper
orphaned to `launchd`. Observed end to end:

1. Session detected, associated with its repository and branch.
2. `starting -> active` transition recorded with evidence.
3. Root killed; session moved to `completed`; three survivors kept `recorded`
   ownership with the reparenting visible and the original parent preserved.
4. A helper orphaned before ghostgc ever saw a parent for it was attributed
   through the inherited session identifier at 0.90.

Two fixture defects were found and fixed, both of which would have made the
fixture quietly stop testing anything: `setsid` does not exist on macOS, and
`select{}` trips Go's deadlock detector.

## OUTCOME

Measured at the default 15 s cadence against ~1,471 processes, ~1,127 of them
the current user's:

| Target | Measured |
| --- | --- |
| Base process scan < 250 ms | 17 ms mean, 26 ms max |
| Average CPU < 1 % | 0.83 % |
| Idle memory < 50 MB | 35 MB RSS |
| Database < 250 MB | 4.4 MB after 36 scans |
| Runs without sudo | yes |

All ten phase-1 acceptance criteria are met, and phase 2's six scope items are
delivered.

### Deferred, with rationale

| # | Item | Why it is safe | Resolves in |
| --- | --- | --- | --- |
| 1 | Environment membership only resolves for sessions ghostgc observed | Unattributed is protected; a daemon cannot observe the past | 8, via the agent's on-disk session records |
| 2 | Environments of system binaries are unreadable | Distinguished from empty; ancestry and recorded ownership still apply | no unprivileged fix |
| 3 | A session can display an exited root process | Nothing is gated on the root PID; liveness follows the live claimant | 4 |
| 4 | `crashed` indistinguishable from `completed` | Understates, never overstates | 4 |
| 5 | `idle`/`waiting`/`suspicious`/`hung` unproduced | Vocabulary and storage already in place | 3, 4 |
| 6 | Detail collection covers every user process | Well inside the scan budget | 3 must gate expensive inspection |
| 7 | Repository state is branch and locks only | The safety-relevant part (locks) is covered | 3, if needed |
| 8 | Arguments truncated at 4 KiB | Detection reads argv[0] and argv[1] | accepted |
| 9 | Storage recovery discards history | Failure mode is "knows less", never "acts wrongly" | before 7 |
| 10 | Codex adapter calibrated on one machine | An unrecognised layout yields no detection | 8 |
| 11 | Linux is a compiling stub | — | 9 |
| 12 | cgo required on macOS | Native APIs beat parsing `ps`/`lsof` | accepted |

`gosec` reports integer-conversion (G115), subprocess (G204) and file-inclusion
(G304) findings in the collector and service installer. These were reviewed and
accepted: the conversions are bounded kernel values crossing the cgo boundary,
the subprocess is `launchctl` with internal arguments, and the file reads are
the configuration path and `.git` plumbing. `gosec` is not a repository-required
check.
