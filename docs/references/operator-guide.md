# Operator Guide

This guide contains the operational detail intentionally omitted from the
README. ghostgc is local-only: its control API is an owner-only Unix socket and
it opens no TCP port.

## Startup modes

`ghostgc start` installs or refreshes the background service in audit mode.
`ghostgc start --mode reconcile` enables recommendations and exact manually
approved actions. Neither command enables automatic cleanup, and neither needs
a configuration file. `ghostgc stop` stops and unregisters the background
service.

The built-in profile enables Codex CLI and the macOS Codex app, the exact
physical `~/.codex/shell_snapshots` and `~/.codex/worktrees` directories when
they exist, and narrow Codex process and cache policies. Audit mode records
matches only. Reconcile mode permits the existing preview/apply workflows.

## Commands

| Command | Purpose |
| --- | --- |
| `ghostgc start [--mode audit\|reconcile]` | start or refresh the background service |
| `ghostgc stop` | stop and unregister the background service |
| `ghostgc status` | daemon health, mode, counts, and last scan |
| `ghostgc sessions` | observed agent sessions |
| `ghostgc session show <id>` | one session's evidence, processes, graph, and audit trail |
| `ghostgc processes` | processes attributed to sessions |
| `ghostgc explain <pid>` | current conclusion and protections for any PID |
| `ghostgc activity` | bounded CPU, disk, file, and socket evidence |
| `ghostgc candidates` | current process recommendations, refusals, and cooldowns |
| `ghostgc cleanup --dry-run ...` | exact process-cleanup preview |
| `ghostgc actions` | durable process action history |
| `ghostgc cache artifacts\|explain\|candidates` | cache metadata and decisions |
| `ghostgc cache cleanup\|restore\|purge` | quarantine, restore, or foreground purge one artifact |
| `ghostgc cache quarantined\|actions` | quarantine inventory and durable history |
| `ghostgc worktrees` | registered worktree inventory and state |
| `ghostgc worktree show <id>` | one worktree's evidence and protections |
| `ghostgc worktree remove` | retire one stale worktree reversibly |
| `ghostgc worktree restore` | return a retired worktree to its original path |
| `ghostgc worktree purge` | grace-gated foreground native removal |
| `ghostgc worktree actions` | durable worktree lifecycle history |
| `ghostgc classifications\|policies\|metrics` | decisions, configuration, and health evidence |
| `ghostgc logs [-f]` | show current audit history and follow new entries until interrupted |
| `ghostgc doctor` | diagnose configuration and installation, even with no daemon |
| `ghostgc config init\|path\|show` | manage configuration |
| `ghostgc service install\|uninstall\|status` | advanced background-service controls |

Add `--json` for machine-readable output. Following logs emit one compact JSON
response per line; use `ghostgc logs --follow=false` for one response and exit.

## Optional configuration

Built-in settings are sufficient for standard Codex installations. For custom
roots, policies, bounds, cadence, privacy, or storage paths, run
`ghostgc config init`, edit `~/.config/ghostgc/config.yaml`, then run either
startup command again. The strict YAML file overlays the selected profile. It
can narrow the mode, but audit startup cannot become actionable and reconcile
startup cannot become automatic. Unknown fields and unsafe values stop startup.

## Process cleanup

The built-in exact-match policy audits orphaned `chrome-headless-shell`
processes attributed to ended Codex sessions. Inspect it in audit mode:

```bash
ghostgc policies
ghostgc candidates
ghostgc logs --kind policy.candidate
```

When the matches are correct, enable manual reconciliation and request one
exact preview:

```bash
ghostgc start --mode reconcile
ghostgc cleanup --dry-run --process '<pid:start-time-ns>' --policy '<policy-id>'
```

The preview's approval is random, memory-only, single-use, and valid for two
minutes. Apply takes a fresh snapshot and repeats ownership, lifecycle,
activity, policy, protection, and executable checks. A passing action sends one
SIGTERM. See [manual-cleanup.md](manual-cleanup.md) and
[dogfooding.md](dogfooding.md).

## Session cache artifacts

The startup profile observes only the existing physical
`~/.codex/shell_snapshots` root and supports no automatic mode. Custom Codex
homes must explicitly list every exact canonical `CODEX_HOME/shell_snapshots`
directory that may be observed:

```yaml
cache:
  enabled: true
  globalMode: recommend
  roots:
    - /Users/example/.codex/shell_snapshots
  scanInterval: 30m
  minStable: 24h
  quarantineGrace: 168h
  maxEntriesPerScan: 10000
  maxEntriesPerAction: 1
  maxBytesPerAction: 10GiB
  policies:
    - id: codex-snapshots
      description: completed Codex shell snapshots
      enabled: true
      mode: recommend
      provider: codex-shell-snapshot-v1
      agent: codex
      artifactKind: shell-snapshot
      sessionState: completed
```

Only immediate regular `<thread-id>.<generation>.sh|ps1` entries owned by one
completed observed session can settle. Contents are never opened.

```bash
ghostgc cache candidates
ghostgc cache explain '<artifact-id>'
ghostgc cache cleanup --dry-run --artifact '<artifact-id>' --policy codex-snapshots
ghostgc cache quarantined
ghostgc cache restore --dry-run --artifact '<artifact-id>'
ghostgc cache purge --dry-run --artifact '<artifact-id>' --policy codex-snapshots
```

Cleanup is a same-filesystem rename into private quarantine and reclaims no
space. Purge is separate, grace-gated, and requires the full artifact ID with
`--confirm`. The daemon prepares and verifies the action; only the short-lived
foreground CLI owns the one exact unlink capability.

## Worktree lifecycle

Session-associated repositories are inventoried by default. Optional roots add
bounded read-only discovery authority:

```yaml
worktrees:
  enabled: true
  scanInterval: 5m
  staleAfter: 168h
  retirementGrace: 168h
  roots:
    - /Users/example/worktrees/example-owner
```

Roots must be absolute, canonical physical directories and cannot be a
filesystem root or the whole home directory. A worktree needs seven continuous
days of complete inactivity before it can become `stale`.

```bash
ghostgc worktrees --state stale
ghostgc worktree show '<id-or-prefix>'
ghostgc worktree remove --dry-run --worktree '<id-or-prefix>'
```

Despite its compatibility name, `remove` retires the registered checkout with
native non-force `git worktree move` to a deterministic sibling. The branch and
all checkout contents remain. A retired checkout can be restored independently:

```bash
ghostgc worktrees --state retired
ghostgc worktree restore --dry-run --worktree '<id-or-prefix>'
```

After `retirementGrace`, permanent finalization needs a new preview and the full
worktree ID through `--confirm`:

```bash
ghostgc worktree purge --dry-run --worktree '<id-or-prefix>'
```

The daemon commits intent but cannot call native removal. The foreground CLI
pins the exact Git executable, invokes non-force removal, and reports completion;
the daemon records `removed` only after verifying both path and registration
absence. Branch deletion, force, prune, clean, reset, stash, and network access
are never used. See [manual-worktree-cleanup.md](manual-worktree-cleanup.md).

## Local files and stored data

| Item | macOS location |
| --- | --- |
| Configuration | `~/.config/ghostgc/config.yaml` |
| State and SQLite | `~/Library/Application Support/ghostgc/` |
| Logs | `~/Library/Logs/ghostgc/` |
| Control socket | `~/Library/Application Support/ghostgc/ghostgc.sock` |
| LaunchAgent | `~/Library/LaunchAgents/com.github.jamesonstone.ghostgc.plist` |

Only agent-attributed processes, session relationships, reduced activity,
cache metadata, and registered worktree evidence are persisted. Command lines
are redacted for credential flags, token shapes, credential URLs, and presigned
signatures. Cache contents and worktree file contents are never stored;
worktree status is reduced to counts and a one-way fingerprint.

`ghostgc metrics` reports scan duration, CPU-relevant counters, daemon memory,
and database size for the current machine. A `?` in activity output means the
metric was unavailable or lacked a valid baseline; it never means observed
zero.

## Troubleshooting

Run `ghostgc doctor` first. A failed or incomplete ownership, path-use,
filesystem, configuration, Git-identity, or post-action check is a refusal. An
ambiguous permanent action opens the daemon's filesystem mutation circuit;
restart the daemon, allow a fresh observation, and investigate the durable
action before requesting new authority.
