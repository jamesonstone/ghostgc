# Manual Cleanup Guide

Phase 6 cleanup is local, explicit and single-use. The default configuration is
still audit-only and cannot signal anything.

## Enable one exact recommendation

Start from `ghostgc config init`. Set the global mode to `recommend`, then add
or change one policy whose agent, executable and strong state are exact:

```yaml
globalMode: recommend

policies:
  - id: completed-headless-browser
    description: manually clean up an orphaned Codex browser
    enabled: true
    mode: recommend
    states: [orphaned]
    agents: [codex]
    executables: [chrome-headless-shell]
    requireDetached: true
    requireSessionEnded: true
    minStable: 5m
    cooldown: 1h
```

Replace the executable with the exact basename you intend to manage. Broad
runtimes and infrastructure classes are rejected during configuration load.
Restart the daemon, then verify the loaded authority boundary:

```bash
ghostgc doctor
ghostgc policies
ghostgc status
```

## Preview and approve

`ghostgc candidates` lists a preview command for each current recommendation:

```bash
ghostgc candidates
ghostgc cleanup --dry-run --process '<pid:start_time_ns>' --policy '<policy-id>'
```

The dry run sends no signal. It returns an exact command containing an opaque
approval and `--yes`. Read the target, policy, signal and revalidation list,
then run that exact command within two minutes if they are correct.

The daemon consumes the approval once, serializes with process scans, and
freshly verifies the committed decision, canonical policy, PID plus start time,
executable path and kernel name, ownership, session lifecycle, activity,
classification and every hard protection. The platform verifies the key and
image once more immediately beside the system call. It records `attempting`
before that call and then records `signalled` or `failed`. A stale or changed
fact records `rejected` and sends no signal.

```bash
ghostgc actions
ghostgc logs --kind action.rejected
ghostgc logs --kind action.signalled
ghostgc processes --all
```

`signalled` means the operating system accepted SIGTERM for the exact identity;
the next observation proves whether the process actually exited. There is no
SIGKILL or escalation path.

## Exercise the bundled fixture

The fixture creates a dedicated direct `action-child` using the exact basename
`fixture-helper`. It is the only process intended for live action validation.
The fixture root creates its own POSIX session, so its processes do not inherit
the operator's terminal even when started from an interactive shell. Production
processes with a terminal remain hard-protected. Fixture teardown records and
verifies each process start time before signalling, so an exited target's
recycled PID is refused:

```bash
fixtures/fixture-agent.sh start
fixtures/fixture-agent.sh orphan
```

Use a temporary policy scoped to `codex`, `fixture-helper`, `orphaned`, required
detachment and an ended session. After five continuous minutes of complete idle
evidence, preview and approve only the `action-child` exact identity. Confirm it
exited and every other recorded fixture process remains, then always clean up:

```bash
fixtures/fixture-agent.sh stop
```
