```text
 ██████  ██   ██  ██████  ███████ ████████  ██████   ██████
██       ██   ██ ██    ██ ██         ██    ██       ██
██   ███ ███████ ██    ██ ███████    ██    ██   ███ ██
██    ██ ██   ██ ██    ██      ██    ██    ██    ██ ██
 ██████  ██   ██  ██████  ███████    ██     ██████   ██████

                    👻 garbage collection for abandoned AI coding runtimes
```

ghostgc is a local daemon that attributes operating-system processes to AI
coding sessions, tracks them across reparenting and PID reuse, and explains its
conclusions. It also inventories registered Git worktrees and narrowly scoped
session cache artifacts.

<!-- BEGIN KIT-MANAGED README BADGES -->

[![Last commit](https://img.shields.io/github/last-commit/jamesonstone/ghostgc)](https://github.com/jamesonstone/ghostgc/commits) [![Open issues](https://img.shields.io/github/issues/jamesonstone/ghostgc)](https://github.com/jamesonstone/ghostgc/issues) [![Pull requests](https://img.shields.io/github/issues-pr/jamesonstone/ghostgc)](https://github.com/jamesonstone/ghostgc/pulls) [![CI](https://github.com/jamesonstone/ghostgc/actions/workflows/ci.yml/badge.svg)](https://github.com/jamesonstone/ghostgc/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/jamesonstone/ghostgc)](https://github.com/jamesonstone/ghostgc/releases)

<!-- END KIT-MANAGED README BADGES -->

Audit is the default. Unknown or incomplete evidence is protected. Process
cleanup sends only one exact SIGTERM after fresh revalidation; it never
escalates. Cache cleanup quarantines before a separately approved foreground
purge. Worktree cleanup first moves a checkout to a restorable retirement path;
the daemon has no permanent cache-unlink or native worktree-removal capability.

See the [safety model](docs/references/safety-model.md) for the complete boundary
and residual risks.

## Install and run

ghostgc requires Go 1.25 or newer. macOS collection also requires the Xcode
command line tools.

```bash
make install
ghostgc config init
ghostgc service install
ghostgc status
```

`make install` installs the single binary at `~/.local/bin/ghostgc`. To run it
without a background service:

```bash
ghostgc daemon --log-level debug
```

## Use

Inspect sessions and processes:

```bash
ghostgc sessions
ghostgc session show '<session-id>'
ghostgc processes
ghostgc explain '<pid>'
ghostgc candidates
```

Every mutation starts with an exact, expiring preview. Run only the apply
command printed by that preview:

```bash
ghostgc cleanup --dry-run --process '<pid:start-time-ns>' --policy '<policy-id>'

ghostgc cache candidates
ghostgc cache cleanup --dry-run --artifact '<artifact-id>' --policy '<policy-id>'
ghostgc cache quarantined
ghostgc cache purge --dry-run --artifact '<artifact-id>' --policy '<policy-id>'

ghostgc worktrees --state stale
ghostgc worktree remove --dry-run --worktree '<id-or-prefix>'
ghostgc worktrees --state retired
ghostgc worktree restore --dry-run --worktree '<id-or-prefix>'
ghostgc worktree purge --dry-run --worktree '<id-or-prefix>'
```

Permanent cache or worktree purge additionally requires the complete opaque ID
through `--confirm` and runs only in the foreground after its configured grace
period. Add `--json` to commands for machine-readable output.

Use `ghostgc --help`, `ghostgc doctor`, and the
[operator guide](docs/references/operator-guide.md) for configuration, command
details, lifecycle procedures, storage paths, and troubleshooting.

## Documentation

- [Operator guide](docs/references/operator-guide.md) — commands, configuration,
  lifecycle workflows, local data, and metrics
- [Safety model](docs/references/safety-model.md) — guarantees, capability
  boundaries, and residual risk
- [Architecture](docs/references/architecture.md) — daemon, storage, and package
  design
- [Testing](docs/references/testing.md) — development and validation commands
- [Project history](docs/references/project-history.md) — rationale, measured
  behavior, and delivery phases
- [Constitution](docs/CONSTITUTION.md) — demonstrated project invariants

## Maintainers

Maintained with 🪖 and ❤️ by [Jameson](https://github.com/jamesonstone) (`jamesonstone`).
