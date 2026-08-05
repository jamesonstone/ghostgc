# Project History and Measurements

## Why ghostgc exists

Coding agents spawn shells, test runners, headless browsers, MCP helpers, and
language servers. Some survive a failed session. Process-name matching cannot
distinguish abandoned children from a developer's editor, database, or build,
especially after reparenting or PID reuse. ghostgc solves attribution and
evidence first, then refuses action until the exact subject and protections are
freshly established.

The durable design commitments are in [../CONSTITUTION.md](../CONSTITUTION.md),
with enforcement detail in [safety-model.md](safety-model.md).

## Measured behavior

The original acceptance run observed 1,471 processes, including 1,127 owned by
the current user:

| Target | Measured |
| --- | --- |
| Base process scan under 250 ms | 17 ms mean, 26 ms max |
| Average CPU under 1% | 0.83% |
| Idle daemon memory under 50 MB | 35 MB RSS |
| Database under 250 MB at default retention | 4.4 MB after 36 scans |
| Runs without `sudo` | yes |

These are historical fixture results, not a guarantee for every host. Use
`ghostgc metrics` for current measurements.

## Delivery phases

| Phase | Scope | Status |
| --- | --- | --- |
| 1 | observation daemon, CLI, SQLite, macOS process collection, audit | complete |
| 2 | durable session graph, reparenting, repository and terminal context | complete |
| 3 | bounded activity evidence | complete |
| 4 | deterministic classification and hard protections | complete |
| 5 | strict policy engine and audit recommendations | complete |
| 6 | exact preview/apply manual SIGTERM | complete |
| 7 | singular-policy, one-candidate automatic SIGTERM | complete |
| 8 | notifications and richer reporting | future |
| 9 | Linux process collector and user systemd service parity | future |

Worktree inventory, reversible retirement, and the session cache lifecycle were
added as independent authority lanes after the original phase sequence. Their
accepted rationale and validation live under `docs/specs/`.
