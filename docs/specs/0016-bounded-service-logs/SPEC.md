---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: complete
feature:
  id: 0016
  slug: bounded-service-logs
  dir: 0016-bounded-service-logs
relationships:
  - type: builds_on
    target: 0007-single-binary-runtime
references:
  - id: issue-36
    name: Bound background service log size
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/36
    relation: guides
    read_policy: must
    used_for: scope and acceptance criteria
    status: active
  - id: safety-model
    name: Ghostgc safety model
    type: reference
    target: docs/references/safety-model.md
    relation: constrains
    read_policy: must
    used_for: filesystem authority
    status: active
  - id: testing-contract
    name: Testing and environment validation rule
    type: ruleset
    target: docs/references/rules/testing-and-environment-validation.md
    relation: constrains
    read_policy: must
    used_for: validation boundaries
    status: active
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Keep Ghostgc's macOS background-service diagnostic output within a hard 10 MB
filesystem bound without granting Ghostgc general log-cleanup authority.

## CONTEXT

Launchd currently appends standard output and standard error directly to
`ghostgc.out.log` and `ghostgc.err.log`. Those files have no rotation, so a
long-running or crash-looping service can consume unbounded disk space.
Periodic cleanup cannot establish a hard ceiling because output can exceed the
limit between cleanup passes.

The daemon therefore needs to own the only service-log writer. Launchd output
is redirected away from filesystem files, while the daemon's structured logger
writes to one exact managed file through an in-process bounded writer.

## REQUIREMENTS

- The combined size of current Ghostgc-managed background-service log files
  never exceeds 10 MB (10,000,000 bytes) after the service logger opens.
- Preserve the newest available diagnostic output when the bound is reached,
  aligning retained records at a newline when the retained tail contains one.
- Manage only `ghostgc.out.log` and the superseded `ghostgc.err.log` beneath the
  daemon's resolved private log directory. Never enumerate or delete other
  files.
- Validate the log directory and both managed paths before mutation. Refuse
  symlinks, non-regular files, multiple hard links, foreign ownership, and
  group- or world-writable paths.
- Empty an existing safe `ghostgc.err.log` when the service logger starts and
  stop launchd from writing either managed file. Differently named legacy logs
  remain untouched.
- Keep foreground daemon logging on standard error. Service registration opts
  into bounded logging with a boolean internal flag, not a caller-supplied log
  path.
- Preserve SQLite audit-log retention and `ghostgc logs` behavior; the service
  file contains only daemon diagnostic output.
- Tests cover opening an oversized file, rollover during writes, safe handling
  of the superseded error log, refusal of linked or unsafe paths, and launchd
  redirection without changing the user's installed service.

### Non-goals

- General-purpose cleanup of `~/Library/Logs`, legacy files, or SQLite audit
  records.
- User-configurable service-log paths, size limits, or rotated generations.
- Compressing, archiving, or retaining old service diagnostic output.

## ACCEPTED PLAN

1. Add one exact-path bounded writer for background-service diagnostics, with
   pre-mutation ownership, link, type, and permission validation.
2. Register service daemons with an internal boolean logging flag and redirect
   launchd streams to `/dev/null` so every filesystem writer is bounded.
3. Cover size boundaries, compaction, concurrency, legacy-error cleanup, and
   unsafe paths with isolated tests; do not touch the installed service.
4. Document the audit-log/diagnostic-log distinction and run every repository
   delivery gate.

## DECISIONS

### One current file establishes a combined bound

Rotated generations would require a second storage allowance and more cleanup
authority. Keeping only the newest output in `ghostgc.out.log` and emptying the
superseded error file makes the combined ceiling direct and observable.

### Service mode selects logging, configuration selects no arbitrary target

The internal flag derives the exact log directory from validated daemon paths.
Accepting a path from launchd arguments would make a registration mistake
capable of truncating an unrelated file.

### Unsafe paths stop startup

Ghostgc does not repair, replace, unlink, or follow an unsafe managed path.
Failing the daemon is preferable to turning log retention into broad
filesystem authority.

### Compaction leaves append headroom

When the file reaches its ceiling, keeping a full 10 MB tail would require
rewriting almost 10 MB for every subsequent log record. Compaction instead
retains up to the newest 75 percent before appending, amortizing the rewrite
while keeping useful recent diagnostics.

## DISCOVERIES

- Launchd's `StandardOutPath` and `StandardErrorPath` append without any
  rotation facility in the current service definition.
- Periodic truncation cannot enforce a hard maximum between cleanup intervals.
- In-place tail compaction keeps the newest evidence without a temporary copy
  that would itself exceed the combined storage bound.

## DESIGN

`internal/servicelog` owns an exact `ghostgc.out.log` file descriptor opened
without following symlinks. Its mutex-serialized writer retains a newline-
aligned newest tail of up to 75 percent in the same inode before appending
would exceed 10 MB.
In-place compaction avoids a temporary second copy that would violate the
combined-size ceiling. The writer re-stats the open descriptor on each write,
revalidates the exact path and directory, and refuses identity or link changes.
External truncation therefore cannot corrupt its size accounting, while a
moved or newly linked file receives no later append or compaction.

At startup the package validates both exact managed paths before changing
either. A safe existing error log is truncated to zero; no error log is
created. Launchd sends stdout and stderr to `/dev/null`, so early failures do
not bypass the cap. The registered daemon receives `--service-log`, derives the
log directory from its validated configuration, and sends structured logs to
the bounded writer. Failure to establish that writer stops daemon startup.

## VALIDATION

- Package tests with a small injected bound cover startup compaction, append
  headroom, combined output/error size, rollover, oversized and concurrent
  writes, and refusal of symlinked, hard-linked, writable, moved, or newly
  linked managed files.
- Focused service-log, CLI, and Darwin rendering tests pass under the race
  detector. Service registration includes the internal opt-in flag and both
  launchd streams use `/dev/null`.
- `make check`, `make race`, `make lint`, `make build`, `make size`,
  `git diff --check`, and `kit check 0016-bounded-service-logs` pass. Race
  builds emit the existing non-failing macOS malformed `LC_DYSYMTAB` linker
  warnings.
- Validation used only temporary test directories and faked service
  registration. It did not inspect, truncate, restart, or otherwise mutate the
  installed Ghostgc service or its real logs.

## OUTCOME

- Background-service structured diagnostics continuously retain their newest
  output in `ghostgc.out.log` within exactly 10,000,000 bytes.
- Service startup validates both managed paths, compacts oversized output in
  place, and empties the safe superseded `ghostgc.err.log`. Unsafe or changed
  paths stop writes without unlinking, replacing, or truncating their targets.
- Launchd no longer appends directly to filesystem logs, so service output
  cannot bypass the bounded writer. Foreground logging and the SQLite audit
  trail remain unchanged.

## REPOSITORY MEMORY

This spec owns the rationale and exact authority boundary for service-log
retention. `docs/references/operator-guide.md` owns observable operator
behavior, while `docs/references/architecture.md` owns the runtime wiring. The
existing project invariant that retained buffers are bounded already covers
this implementation, so no new Constitution rule is required.
