---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: complete
feature:
  id: 0007
  slug: single-binary-runtime
  dir: 0007-single-binary-runtime
relationships:
  - type: builds_on
    target: 0006-phase-7-narrow-enforcement
references:
  - id: issue-19
    name: Single-binary runtime issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/19
    relation: guides
    read_policy: must
    used_for: scope and acceptance criteria
    status: active
skills:
  - name: github:yeet
    source: GitHub plugin
    path: github:yeet
    trigger: publish the validated single-binary change as a ready pull request
    required: true
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Ship ghostgc as one executable without collapsing the existing process or
privilege boundaries. Operators install and invoke only `ghostgc`; the same
artifact runs either a short-lived control command or the persistent daemon.

## CONTEXT

The current build produces `ghostgc` and `ghostgcd`. Service installation must
discover the second artifact, so a globally linked CLI cannot install its own
daemon unless the daemon happens to be adjacent, separately on `PATH`, or
provided with `--binary`. That exposes an implementation boundary as a user
installation requirement and makes the normal first-run path incomplete.

The separate long-running process remains intentional. It owns observation,
storage, policy evaluation and the Unix socket while ordinary CLI invocations
remain short-lived clients. Only the executable boundary changes.

## REQUIREMENTS

### Runtime contract

- `ghostgc daemon` runs the existing persistent daemon lifecycle.
- It retains foreground debug logging, one-cycle execution and version output.
- Ordinary commands continue to communicate with the daemon over the existing
  local Unix socket.
- Signal handling, storage recovery, policy authority and the sole SIGTERM gate
  remain unchanged.

### Service contract

- `ghostgc service install` resolves the executable that is currently running.
- Symlink invocation resolves to the canonical executable target before the
  service definition is written.
- The service manager receives the executable plus explicit arguments:
  `daemon --config <path>`.
- The existing service label is preserved. Reinstall overwrites the definition,
  stops any registration under that label and bootstraps the new definition, so
  a legacy `ghostgcd` registration migrates without deleting configuration,
  state or audit history.
- After service registration succeeds, reinstall removes only the sibling
  legacy `ghostgcd` executable. A registration failure leaves it in place so the
  legacy runtime remains available for recovery or manual fallback.
- The obsolete `--binary` option and daemon-discovery fallback are removed.

### Distribution contract

- Build and install produce only `ghostgc`.
- Foreground, launchd and systemd examples invoke `ghostgc daemon`.
- User documentation describes one required artifact and one user-facing
  command.

### Non-goals

- Running the daemon in-process with an ordinary client command.
- Replacing the Unix socket, database, service label or configuration schema.
- Adding Linux service-management support beyond correcting its packaging
  example.
- Deleting legacy log files or user state.

## ACCEPTED PLAN

1. Move the existing daemon entrypoint behavior into a `daemon` command in the
   `ghostgc` command package and remove the separate command entrypoint.
2. Generalize platform service options to carry explicit program arguments.
   Render launchd arguments safely and test the exact ordering and XML escaping.
3. Resolve and canonicalize the running `ghostgc` executable during service
   installation, then register it with `daemon --config <path>`.
4. Build and install only `ghostgc`; update packaging examples and all current
   operator, architecture, testing and dogfooding references.
5. Validate focused command and service behavior, the full Go suite, race
   detector, lint, build artifact set and the complete source-file-size scope.

## DECISIONS

### One executable does not mean one process

The CLI and daemon retain different lifetimes and communicate through the same
socket boundary. Reusing the executable removes packaging friction without
making transient CLI commands own monitoring state.

### Service arguments belong to the platform contract

The platform registration interface accepts an argument vector instead of a
daemon-specific configuration field. This keeps launchd rendering generic and
makes the exact launched command independently testable.

### Reinstall is the migration mechanism

The existing launchd installer already writes the definition, boots out the
stable label and bootstraps it again. Keeping that label makes a normal
`ghostgc service install` the bounded migration from a legacy two-binary
registration.

### Legacy executable retirement follows service migration

Removing `ghostgcd` during `make install` could strand an existing service if
the replacement registration then failed. The service installer therefore
retires only the exact sibling legacy file and only after launchd accepts the
new single-binary command.

## DISCOVERIES

### Existing launchd replacement is sufficient

The installer already writes the property list, boots out the stable service
label and bootstraps the definition again. The single-binary migration needs no
new state migration or service label; changing the rendered command is enough.

### Historical validation remains historical

The Phase 3 spec truthfully records that its original validation built two
executables. Current architecture, tooling and operator references now describe
the superseding single-binary contract; the old validation record is retained
rather than rewritten.

## VALIDATION

- `go test ./cmd/cli ./internal/platform/darwin ./internal/platform/platformtest`
  — passed focused command, executable resolution and launchd rendering tests.
- `make check` — passed formatting, vet, the complete repository source-size
  gate and all native package tests.
- `make race` — passed all packages. The macOS linker emitted its existing
  malformed `LC_DYSYMTAB` warnings for cgo test objects without failing.
- `make lint` — passed with zero issues.
- `GOOS=linux CGO_ENABLED=0 go test -exec=/usr/bin/true ./...` — compiled every
  package and test for Linux without attempting to execute Linux binaries on
  macOS.
- `make build` — produced executable `bin/ghostgc` and removed a stale
  `bin/ghostgcd` artifact.
- A temporary `make install` installed only `ghostgc`; that installed artifact
  completed `ghostgc daemon --once` in an isolated audit-mode home and created
  its SQLite state.
- Upgrade regressions prove successful service migration removes a sibling
  legacy `ghostgcd`, while failed registration preserves it.
- `kit check 0007-single-binary-runtime` — passed.
- `kit check --project` — the issue branch and unchanged `main` both report the
  same 20 pre-existing blocking findings in legacy specs and the missing project
  progress summary. The new spec passes its focused feature check.

## OUTCOME

ghostgc now builds, installs and runs as one executable. The persistent runtime
is `ghostgc daemon`; ordinary commands still use the existing Unix socket.
Service installation canonicalizes the running executable, supplies explicit
`daemon --config <path>` arguments and reuses the stable service label, so an
in-place reinstall migrates a legacy registration without touching user state.

The separate command entrypoint and daemon discovery path are gone. Launchd,
systemd, README, architecture, testing, tooling and dogfooding documentation
now describe the single-binary contract. New logs use `ghostgc.out.log` and
`ghostgc.err.log`; legacy log files are intentionally left untouched. A
successful service reinstall removes the adjacent legacy executable after the
new service is registered.

## REPOSITORY MEMORY

- This spec preserves the executable-boundary rationale, rejected one-process
  interpretation, service migration decision and observed validation.
- `docs/CONSTITUTION.md` now records one executable with two process roles as a
  project-wide invariant.
- `docs/references/architecture.md` records the runtime topology, while the
  tooling, testing and dogfooding references own their reusable commands.
