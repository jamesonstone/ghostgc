```text
 ██████  ██   ██  ██████  ███████ ████████  ██████   ██████
██       ██   ██ ██    ██ ██         ██    ██       ██
██   ███ ███████ ██    ██ ███████    ██    ██   ███ ██
██    ██ ██   ██ ██    ██      ██    ██    ██    ██ ██
 ██████  ██   ██  ██████  ███████    ██     ██████   ██████

                    garbage collection for abandoned coding runtimes
```

ghostgc is a local background service for abandoned AI coding runtimes. It
attributes processes to Codex sessions, explains stale-process decisions, and
tracks recoverable lifecycle candidates for Codex shell snapshots and Git
worktrees.

<!-- BEGIN KIT-MANAGED README BADGES -->

[![Last commit](https://img.shields.io/github/last-commit/jamesonstone/ghostgc)](https://github.com/jamesonstone/ghostgc/commits) [![Open issues](https://img.shields.io/github/issues/jamesonstone/ghostgc)](https://github.com/jamesonstone/ghostgc/issues) [![Pull requests](https://img.shields.io/github/issues-pr/jamesonstone/ghostgc)](https://github.com/jamesonstone/ghostgc/pulls) [![CI](https://github.com/jamesonstone/ghostgc/actions/workflows/ci.yml/badge.svg)](https://github.com/jamesonstone/ghostgc/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/jamesonstone/ghostgc)](https://github.com/jamesonstone/ghostgc/releases)

<!-- END KIT-MANAGED README BADGES -->

Audit is always the default. Unknown evidence is protected, actions require a
fresh exact preview, and permanent filesystem operations run only in a
short-lived foreground command after full-ID confirmation. See the
[safety model](docs/references/safety-model.md) for the complete boundary.

## Install

Requires Go 1.25 or newer. On macOS, install the Xcode command line tools, then:

```bash
make install
```

## Run

Start safely in audit/shadow mode with no configuration required:

```bash
ghostgc start
```

Start and immediately follow the high-signal audit log:

```bash
ghostgc start --logs
```

Ctrl-C stops the foreground log view; the background service keeps running.

Enable live, manually approved reconciliation:

```bash
ghostgc start --mode reconcile
```

Both modes include narrow defaults for Codex CLI and the macOS Codex app under
`~/.codex`. Reconcile mode permits recommendations and exact manual actions; it
does not enable automatic cleanup.

## Use

```bash
ghostgc status
ghostgc sessions
ghostgc processes
ghostgc candidates
ghostgc cache candidates
ghostgc worktrees
ghostgc logs             # follows until Ctrl-C
ghostgc doctor
ghostgc stop
```

Followed logs show lifecycle and policy signal by default. Add `--verbose` to
include process-attribution entries, or `--follow=false` for a complete
one-time audit-log snapshot. `ghostgc config show` reports the active daemon's
effective profile and overrides.

Run `ghostgc --help` for all commands. To override defaults, generate the
optional strict configuration and restart in either mode:

```bash
ghostgc config init
ghostgc start                         # audit ceiling
ghostgc start --logs                  # audit ceiling and followed logs
ghostgc start --mode reconcile --logs # manual-action ceiling and followed logs
```

## References

- [Operator guide](docs/references/operator-guide.md) — commands, configuration,
  lifecycle procedures, paths, and troubleshooting
- [Dogfooding guide](docs/references/dogfooding.md) — safely evaluate audit and
  reconciliation modes
- [Safety model](docs/references/safety-model.md) — guarantees and residual risk
- [Architecture](docs/references/architecture.md) — runtime and package design
- [Testing](docs/references/testing.md) — development and validation

## Maintainers

Maintained with 🪖 and ❤️ by [Jameson](https://github.com/jamesonstone) (`jamesonstone`).
