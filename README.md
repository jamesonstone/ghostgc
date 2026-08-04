```text
 ██████  ██   ██  ██████  ███████ ████████  ██████   ██████
██       ██   ██ ██    ██ ██         ██    ██       ██
██   ███ ███████ ██    ██ ███████    ██    ██   ███ ██
██    ██ ██   ██ ██    ██      ██    ██    ██    ██ ██
 ██████  ██   ██  ██████  ███████    ██     ██████   ██████

                    👻 garbage collection for abandoned AI coding runtimes
```

ghostgc is a local daemon for developers who run coding agents. It works out
which operating-system processes belong to which agent session, tracks them
across reparenting and PID reuse, and explains every conclusion it reaches with
the evidence behind it. It also inventories registered Git worktrees and can
manually remove one only after seven continuous days of complete inactivity.

<!-- BEGIN KIT-MANAGED README BADGES -->

[![Last commit](https://img.shields.io/github/last-commit/jamesonstone/ghostgc)](https://github.com/jamesonstone/ghostgc/commits) [![Open issues](https://img.shields.io/github/issues/jamesonstone/ghostgc)](https://github.com/jamesonstone/ghostgc/issues) [![Pull requests](https://img.shields.io/github/issues-pr/jamesonstone/ghostgc)](https://github.com/jamesonstone/ghostgc/pulls) [![CI](https://github.com/jamesonstone/ghostgc/actions/workflows/ci.yml/badge.svg)](https://github.com/jamesonstone/ghostgc/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/jamesonstone/ghostgc)](https://github.com/jamesonstone/ghostgc/releases)

<!-- END KIT-MANAGED README BADGES -->

**Cleanup is available only when explicitly configured.** Manual cleanup needs
an exact one-time approval. Automatic cleanup additionally requires global
enforce plus one singular orphan-only policy and attempts at most one current
candidate per evaluation. Both fully revalidate, and each authorized action can
send only one SIGTERM.
Audit remains the default. See
[docs/references/safety-model.md](docs/references/safety-model.md).

Worktree removal is a separate manual-only authority. It never deletes a
branch, never uses force, and refuses dirty, active, uncertain, primary, locked,
or otherwise protected worktrees.

```
$ ghostgc sessions
ID        AGENT  REPOSITORY        STATE   CONF  AGE     PROCESSES   ROOT   LAUNCHED BY
8f2a1b3c  codex  labcore@main      active  0.96  18m     12          48102  zsh
2d1b77e0  codex  event-sink@fix-7  active  0.55  2h 14m  4 (3 live)  51993  Code Helper (Plugin)

$ ghostgc explain 48231
Classification: protected
Process: pid 48231 (chrome-headless-shell)
Identity: 48231:1785756660392212000 (pid plus start time, so a recycled pid is a different process)
Parent link: reparented
Created by: unknown — the process was already reparented when ghostgc first observed it
Session: 8f2a1b3c (codex, state completed), relation environment, confidence 0.90

Evidence:
- [environment, weight 0.90] environment carries codex session identifier
  "019fae08-329a-7bb1-946c-a90e9908c2ae", which names session 8f2a1b3c
- [environment] an environment variable is inherited by every descendant, so this
  establishes that the process descends from that session and nothing more; it is
  capped below the policy-eligible threshold for that reason

Relationships (4):
KIND           FROM   TO     OWNERSHIP    DETAIL
environment    48231  -      attributing  environment carries the agent's own session identifier
reparenting    48231  -      context      parent link lost; the process was reparented to pid 1
repository     48231  -      context      working directory is inside /Users/dev/src/labcore
terminal       48231  48102  context      shares POSIX session 47990 with the session root

Protections that apply:
- protected-uncertain-attribution-v1: attribution confidence is 0.90, below the 0.95
  required for any policy to consider the process; unknown ownership is protected
```

## Why it exists

Coding agents spawn processes: shells, test runners, headless browsers, MCP
helpers, language servers. When a session ends badly, some of them survive. The
usual reaction is to reach for `pkill` and a name pattern, which is how people
end up killing the language server their editor was using, or a database that
happened to be started from an agent shell.

The difficult part is not termination. It is knowing, with evidence, _which
session a process belongs to_ — after the operating system has reparented it to
`launchd`, after its PID has been recycled, and when several unrelated things on
the machine have the agent's name somewhere in their command line.

ghostgc solves that part first, and refuses to act until it is solved.

## Design commitments

The full set is in [docs/CONSTITUTION.md](docs/CONSTITUTION.md). The ones that
shape everything else:

**Observe before acting.** Audit is the default mode. Recommendation must be
enabled globally and on one exact policy; automatic cleanup additionally needs
global enforce and one explicitly automatic, singular orphan-only policy.

**Evidence over heuristics.** Every classification carries the observations that
produced it. "The node process is old" is not a reason. Confidence combines
independent signals and is capped below 1.0.

**Fail closed.** Where ownership or safety cannot be established, nothing
happens. Unknown is protected — including a process the daemon did not manage to
inspect.

**Never identify a process by PID alone.** Every process is keyed by
`pid:start_time_ns`, and a parent link whose "parent" started _after_ its child
is not believed at all.

**Ownership is written down, not re-derived.** When a parent exits and the
kernel reparents its children, the live process tree loses the relationship. The
daemon does not.

**A session is a graph, not a tree.** Each reason a process belongs is a typed
edge, and each edge declares whether it may establish ownership. Sharing a
terminal with an agent means a human was at the same keyboard; it never means
the agent owns your process.

**Say "unknown" rather than something convenient.** A process first seen after
its parent exited reports its creator as unknown, not as `launchd`. A process
whose environment the operating system refuses to show reports that, rather than
letting "unreadable" pass for "no agent variables set".

## Install

Requires Go 1.25 or newer and the Xcode command line tools (the macOS collector
uses `libproc` through cgo).

```bash
make install
ghostgc config init
ghostgc service install
ghostgc status
```

`make install` puts the single `ghostgc` executable in `~/.local/bin`.
`ghostgc service install` registers that same executable as a LaunchAgent so
the daemon starts at login and restarts after an unsuccessful exit with a 30
second throttle. Running the install command again migrates a legacy service
definition without deleting configuration or state. To run the daemon in the
foreground instead: `ghostgc daemon --log-level debug`. After the replacement
service is registered successfully, installation also removes a sibling legacy
`ghostgcd` executable left by an earlier release.

## Commands

| Command                                      | What it does                                                          |
| -------------------------------------------- | --------------------------------------------------------------------- |
| `ghostgc status`                             | daemon health, mode, session counts, last scan                        |
| `ghostgc sessions`                           | every observed agent session                                          |
| `ghostgc session show <id>`                  | one session: evidence, processes, relationship graph, audit trail     |
| `ghostgc processes`                          | processes attributed to a session                                     |
| `ghostgc explain <pid>`                      | what was concluded about a PID and why — works for _any_ PID          |
| `ghostgc activity`                           | bounded CPU, disk, file and socket evidence for attributed processes  |
| `ghostgc candidates`                         | current enforceable, recommended and audit/refusal/cooldown decisions |
| `ghostgc cleanup --dry-run ...`              | issue an exact, expiring manual cleanup preview                       |
| `ghostgc cleanup --apply ...`                | consume one approval after full fresh revalidation                    |
| `ghostgc actions`                            | durable attempted, rejected, signalled and failed actions             |
| `ghostgc worktrees`                          | registered worktree inventory and stale/protected state                |
| `ghostgc worktree show <id>`                 | one worktree's identity, inactivity evidence and protections           |
| `ghostgc worktree remove --dry-run ...`      | preview one exact stale worktree and issue an expiring approval        |
| `ghostgc worktree remove --apply ...`        | revalidate and remove one approved secondary worktree                  |
| `ghostgc worktree actions`                   | durable worktree removal attempts and outcomes                         |
| `ghostgc classifications`                    | latest deterministic process states and detachment                    |
| `ghostgc policies`                           | loaded YAML policies and their exact scope                            |
| `ghostgc logs`                               | the audit trail                                                       |
| `ghostgc metrics`                            | scan timings, counts, database size, daemon memory                    |
| `ghostgc doctor`                             | diagnose the installation; works when the daemon is down              |
| `ghostgc daemon`                             | run the persistent observer in the foreground                         |
| `ghostgc config init\|path\|show`            | manage the configuration file                                         |
| `ghostgc service install\|uninstall\|status` | manage the LaunchAgent                                                |

Add `--json` to any command for machine-readable output.

### Audit a policy

`ghostgc config init` includes a disabled exact-match example. To begin
dogfooding, edit the generated policy to `enabled: true` and `mode: audit`, restart the
daemon, then use:

```bash
ghostgc policies
ghostgc candidates
ghostgc logs --kind policy.candidate
```

Policies can match only exact agent IDs and executable basenames in strong
states. Hard protections cannot be overridden. A `candidate` under an audit
policy remains evidence only.

### Manually approve one cleanup

Change both `globalMode` and one exact policy to `recommend`, restart the
daemon, and inspect the recommendation before requesting authority:

```bash
ghostgc candidates
ghostgc cleanup --dry-run --process '<pid:start_time_ns>' --policy '<policy-id>'
```

The preview prints the only apply command, containing a one-time token and
`--yes`. It expires after two minutes. Apply takes a fresh snapshot, reconciles
ownership and lifecycle, resamples activity, reclassifies, reruns every hard
protection and re-evaluates the exact policy. Any changed or unknown fact is a
durable rejection. A passing request sends one exact-key SIGTERM and never
escalates:

```bash
ghostgc actions
ghostgc logs --kind action.signalled
```

See [the manual cleanup guide](docs/references/manual-cleanup.md) for a complete
configuration and fixture walkthrough.

### Remove one stale worktree

Worktree inventory is enabled by default for repositories associated with
observed sessions. Optional configured roots add operator-declared workspace
discovery. After seven uninterrupted days of complete inactivity evidence, an
unprotected secondary worktree can become stale:

```bash
ghostgc worktrees --state stale
ghostgc worktree show '<id-or-prefix>'
ghostgc worktree remove --dry-run --worktree '<id-or-prefix>'
```

Read the preview and paste its generated apply command within two minutes.
Apply consumes the approval once, repeats every filesystem, process, Git,
configuration and identity check, commits durable attempting evidence, and
invokes native `git worktree remove` without force. The branch remains. See
[the manual worktree cleanup guide](docs/references/manual-worktree-cleanup.md).

### Narrow automatic cleanup

Phase 7 accepts `globalMode: enforce`, but only one enabled enforce policy may
exist. It must set `automatic: true`, match exactly one agent and executable,
match only `orphaned`, require detachment plus an ended session, stay stable for
at least five minutes and cool down for at least one hour. Each committed policy
evaluation can attempt at most one exact current candidate. Fresh revalidation,
hard protections, durable pre-action evidence and the final exact-image
platform gate are identical to the manual path.

Start with [the dogfooding guide](docs/references/dogfooding.md), which moves
from audit to manual recommendation and then proves enforcement against the
fixture before suggesting any real policy.

## Where things live

|                    | macOS                                                            |
| ------------------ | ---------------------------------------------------------------- |
| Configuration      | `~/.config/ghostgc/config.yaml`                                  |
| State and database | `~/Library/Application Support/ghostgc/`                         |
| Logs               | `~/Library/Logs/ghostgc/`                                        |
| Control socket     | `~/Library/Application Support/ghostgc/ghostgc.sock` (mode 0600) |
| LaunchAgent        | `~/Library/LaunchAgents/com.github.jamesonstone.ghostgc.plist`   |

The socket is the only interface. No TCP port is opened.

## What is stored

Only processes attributed to an agent session and registered worktree inventory
are persisted. Everything else on the machine is counted during a scan and
then forgotten: monitoring activity outside coding-agent sessions is an
explicit non-goal.

Before anything reaches SQLite, command-line arguments pass through a redactor
that removes credentials by flag name, by value shape (`sk-`, `ghp_`, `AKIA`,
JWTs, …) and by rewriting URLs carrying passwords or presigned signatures;
environment variables are reduced to the small allowlist the adapters need.
Worktree inspection stores paths, Git identities, aggregate dirty counts and a
one-way status fingerprint; filenames and file contents are never persisted.

The separate activity pass runs once a minute by default and only for live
processes already attributed to an agent session. It persists deltas and counts,
not file paths or socket endpoints. A `?` in `ghostgc activity` means the metric
was unavailable or lacks a valid baseline; it never means observed zero.

## Measured behaviour

On a machine with 1471 running processes, 1127 of them the current user's:

| Target                                     | Measured              |
| ------------------------------------------ | --------------------- |
| Base process scan under 250 ms             | 17 ms mean, 26 ms max |
| Average CPU under 1%                       | 0.83%                 |
| Idle daemon memory under 50 MB             | 35 MB RSS             |
| Database under 250 MB at default retention | 4.4 MB after 36 scans |
| Runs without `sudo`                        | yes                   |

Run `ghostgc metrics` to see these on your own machine.

## Delivery phases

Each phase is completed, tested and documented before the next begins.

| Phase | Contents                                                                                                                               | Status   |
| ----- | -------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| 1     | Observation foundation: daemon, CLI, SQLite, macOS collection, process trees, Codex detection, audit log                               | **done** |
| 2     | Session graph: typed relationships, launch context, environment membership, repository and terminal association, session state machine | **done** |
| 3     | Activity tracking: CPU/IO/network deltas, open files, sockets                                                                          | **done** |
| 4     | Classification: active, idle, waiting, detached, suspicious, orphaned, unknown                                                         | **done** |
| 5     | Policy engine: YAML policies, audit evaluation, safety refusals, cooldowns                                                             | **done** |
| 6     | Recommended cleanup: manual approval, exact command preview, pre-action revalidation, SIGTERM only                                     | **done** |
| 7     | Narrow enforcement: one singular orphan-only automatic policy, one candidate per evaluation                                            | **done** |
| 8     | Adapters for Claude Code, Cursor, OpenCode                                                                                             |          |
| 9     | Linux: `/proc` collector, user systemd unit, parity tests                                                                              |          |

Phase 7 automatic termination is singular-policy, one-candidate-per-evaluation
and SIGTERM-only. Audit remains the default; manual recommendation remains
available under global recommend or enforce.

## Development

```bash
make check   # gofmt, go vet, source-file-size gate, tests
make race    # tests under the race detector
make lint    # golangci-lint
make run     # daemon in the foreground with debug logging
```

The suite includes a source-level check that exactly one literal SIGTERM site exists,
adversarial detection cases taken from a real machine, and tests for PID reuse,
reparenting, redaction, schema migration and every relationship kind that must
not establish ownership. Disposable real Git repositories additionally prove
the seven-day state machine, every removal protection, approval invalidation,
native non-force removal and branch preservation. A safety test is never
weakened to make it pass.

`fixtures/fixture-agent.sh` builds a real process tree — a session root, a
worker shell, idle and periodic-work children, a helper orphaned to `launchd`,
and a native exact action target — so the collector can be exercised against a
known shape:

```bash
fixtures/fixture-agent.sh start
ghostgc sessions
sleep 65
ghostgc activity
```

## Documentation

- [docs/CONSTITUTION.md](docs/CONSTITUTION.md) — project invariants
- [docs/references/architecture.md](docs/references/architecture.md) — how the pieces fit together
- [docs/references/safety-model.md](docs/references/safety-model.md) — every guarantee and how it is enforced
- [docs/references/testing.md](docs/references/testing.md) — commands, suites and evidence expectations
- [docs/references/dogfooding.md](docs/references/dogfooding.md) — immediate audit, manual and fixture enforcement walkthrough
- [docs/references/manual-worktree-cleanup.md](docs/references/manual-worktree-cleanup.md) — stale-worktree inventory, preview, removal and recovery
- [docs/specs/0001-session-aware-process-observation/SPEC.md](docs/specs/0001-session-aware-process-observation/SPEC.md) — feature rationale, discoveries and deferred risks
- [docs/specs/0006-phase-7-narrow-enforcement/SPEC.md](docs/specs/0006-phase-7-narrow-enforcement/SPEC.md) — automatic authority contract and acceptance evidence
- [docs/specs/0009-stale-worktree-cleanup/SPEC.md](docs/specs/0009-stale-worktree-cleanup/SPEC.md) — worktree staleness, protection and removal contract

## Maintainers

Maintained with 🪖 and ❤️ by [Jameson](https://github.com/jamesonstone) (`jamesonstone`).
