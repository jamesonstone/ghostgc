# Testing Reference

## Purpose

- Record the project's durable commands, suites, environments, automation, and evidence expectations
- Follow `rules/testing-and-environment-validation.md` for the mandatory cross-project testing and production-safety contract
- Keep feature-specific testing details in the current feature's `SPEC.md` VALIDATION and OUTCOME sections; legacy staged flows may still use `PLAN.md` or `TASKS.md`

## Code-Level Validation

| Layer | Command | PR workflow or check | Required | Notes |
| --- | --- | --- | --- | --- |
| Format + vet + unit/integration | `make check` | not yet wired to Actions | yes | Whole module; no external services |
| Race detector | `make race` | not yet wired to Actions | yes | Concurrency in the collector and daemon loop |
| Lint | `make lint` (`golangci-lint run ./...`) | not yet wired to Actions | yes | Must report zero issues |
| Source file size | `make size` | not yet wired to Actions | yes | Enforces the 300-line gate on `cmd/`, `internal/`, `fixtures/` |
| Coverage summary | `make cover` | manual | no | Informational |

## High-Level Suites

| Suite | Type | Environment | Command | Automation | Evidence |
| --- | --- | --- | --- | --- | --- |
| Live process tree | end-to-end | local macOS | `fixtures/fixture-agent.sh start`, then `orphan`, then `stop` | manual | `ghostgc sessions`, `ghostgc session show <id>`, `ghostgc explain <pid>` |
| Resource budget | live-integration | local macOS | run `ghostgcd`, then `ghostgc metrics` | manual | scan duration, CPU, RSS, database size |

## Environment Preflights

- Requires Go 1.25 or newer and the Xcode command line tools; the macOS
  collector uses `libproc` through cgo.
- The unit and integration suites need no daemon, no network, and no database
  server. The platform is faked via `internal/platform/platformtest`.
- The live suites need a macOS host and observe only the invoking user's own
  processes. They never require `sudo`.
- Linux is not applicable until delivery phase 9; the `/proc` collector is a
  compiling stub that returns `ErrNotImplemented`.
- The fixture signals only processes it started itself, whose pids it recorded
  at creation. ghostgc itself cannot signal anything in this build.

## Safety Evidence

- `internal/platform/signal_disabled_test.go` walks the whole repository and
  fails if any package references a signalling primitive or shells out to a
  terminator. Treat it as a delivery gate, not an ordinary test.
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
