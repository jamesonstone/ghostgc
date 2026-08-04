# Testing Reference

## Purpose

- Record the project's durable commands, suites, environments, automation, and evidence expectations
- Follow `rules/testing-and-environment-validation.md` for the mandatory cross-project testing and production-safety contract
- Keep feature-specific testing details in the current feature's `SPEC.md` VALIDATION and OUTCOME sections; legacy staged flows may still use `PLAN.md` or `TASKS.md`

## Code-Level Validation

| Layer | Command | PR workflow or check | Required | Notes |
| --- | --- | --- | --- | --- |
| Format + vet + unit/integration | `make check` | `CI / Check` | yes | Whole module; no external services |
| Race detector | `make race` | `CI / Race detector` | yes | Concurrency in the collector and daemon loop |
| Lint | `make lint` (`golangci-lint run ./...`) | `CI / Lint` | yes | Must report zero issues |
| Source file size | `make size` | included in `CI / Check` | yes | Enforces the 300-line gate on `cmd/`, `internal/`, `fixtures/` |
| Coverage summary | `make cover` | manual | no | Informational |

## High-Level Suites

| Suite | Type | Environment | Command | Automation | Evidence |
| --- | --- | --- | --- | --- | --- |
| Live process tree | end-to-end | local macOS | `fixtures/fixture-agent.sh start`, then `orphan`, then `stop` | manual | `ghostgc sessions`, `ghostgc session show <id>`, `ghostgc explain <pid>` |
| Activity evidence | live-integration | local macOS | start fixture and daemon, wait for two activity cadences | manual | `ghostgc activity`; periodic worker has positive delta, idle worker has known zero |
| Deterministic classification | integration + live | local macOS | wait for two activity cadences, then `ghostgc classifications` | manual | active periodic worker, known-idle worker, independent detachment, freshness and evidence |
| Policy audit | integration + live | local macOS | enable a fixture-scoped audit policy and wait through its stable window | manual | `ghostgc policies`, `ghostgc candidates`, policy audit log; zero enforceable entries and signals |
| Manual cleanup | live-integration | local macOS | orphan the fixture, wait for `action-child` to be classified orphaned, preview and apply its exact recommendation | manual | one SIGTERM, exact target exits, durable action evidence, all other fixture pids survive until teardown |
| Narrow enforcement | live-integration | local macOS | enable the singular fixture-only enforce policy, orphan the fixture and wait through the stable window | automatic by daemon | one automatic action per evaluation, exact target exits, durable authority/evidence, all non-target fixture processes survive |
| Session cache lifecycle | high-level integration | local macOS or Linux | `tests/end-to-end/local/cache-lifecycle-test.sh` | deterministic, no daemon or real cache | settle, quarantine, replay refusal, restore, re-settle, grace-gated purge and durable transition evidence |
| Resource budget | live-integration | local macOS | run `ghostgc daemon`, then `ghostgc metrics` | manual | scan duration, CPU, RSS, database size |

## Environment Preflights

- Requires Go 1.25 or newer and the Xcode command line tools; the macOS
  collector uses `libproc` through cgo.
- The unit and integration suites need no daemon, no network, and no database
  server. The platform is faked via `internal/platform/platformtest`.
- Cache tests use `t.TempDir` SQLite files, a deterministic metadata-only
  filesystem and controlled clock. The real-filesystem tests create only
  fixture-owned temporary roots and never inspect or mutate a user cache.
- The live suites need a macOS host and observe only the invoking user's own
  processes. They never require `sudo`.
- Linux is not applicable until delivery phase 9; the `/proc` collector is a
  compiling stub that returns `ErrNotImplemented`.
- The fixture starts in its own POSIX session and signals only processes it
  started itself after matching each PID to its recorded start time. Action
  validation may direct ghostgc only at the dedicated recorded `action-child`;
  every other process is out of scope.

## Safety Evidence

- `internal/platform/signal_gate_test.go` walks the whole repository and fails
  unless there is exactly one authorized literal SIGTERM system-call site; it
  also rejects alternate primitives and shell terminators.
- The same repository safety test requires exactly one cache `unlinkat` in
  `internal/cachefs/purge_unix.go`, rejects production `os.RemoveAll`, shell
  `rm` and alternate cache deletion primitives, and leaves the SIGTERM gate
  unchanged.
- Never weaken a safety test to make it pass. If a safety condition blocks a
  change, the change is wrong.

## Credentials And Test Data

- List credential and secret names without values
- Document synthetic-data naming, rate and cost limits, cleanup, and retention

## Evidence And Retention

- Keep `tmp/` ignored and record CI artifact locations and retention
- Keep `tests/RUN_STATUS.md` curated at meaningful validation milestones

## Automation And Fallbacks

- Map code-level checks to pull-request jobs and high-level suites to PR or post-deployment jobs when feasible
- Document ordered operator commands when safe automation is unavailable

## Known Gaps

- Record partial, blocked, skipped, and unavailable validation literally
