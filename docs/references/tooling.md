# Tooling Reference

## Purpose

- Record durable repo-wide tooling notes, command references, and local development expectations
- Keep short-lived implementation notes in feature docs instead of here

## Toolchain

- Go 1.25 or newer. The `go` directive is held at the lowest version the
  dependency set allows so the module builds for as many people as possible.
- Xcode command line tools. The macOS collector calls `proc_pidpath`,
  `proc_pidinfo` and `devname_r` through cgo, so `CGO_ENABLED=1` is required on
  darwin. The Linux collector will be pure Go.
- `golangci-lint` for linting; the repository expects a zero-issue report.
- `goimports` is useful when moving declarations between files during a
  source-file-size split, because it prunes the imports the move leaves behind.

## Canonical Commands

- `make help` — list targets.
- `make build` — build `ghostgcd` and `ghostgc` into `./bin`.
- `make check` — format check, `go vet`, and the test suite. The default gate.
- `make race`, `make lint`, `make size`, `make cover` — individual gates.
- `make install` — install both binaries into `~/.local/bin`.
- `make run` — run the daemon in the foreground with debug logging.

## Local Conventions

- `bin/` is build output and is ignored. It is never transferred into a
  worktree or staged.
- Point the daemon at a scratch state directory when experimenting, using a
  config with a `paths.stateDir` override, so a test run never disturbs the
  real database or socket.
- Unix socket paths are capped near 104 bytes on macOS. Keep scratch state
  directories short; a long path fails at bind time.
