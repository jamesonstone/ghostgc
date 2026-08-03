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
ownership, session lifecycle, activity, classification and every hard
protection. It records `attempting` before the system call and then records
`signalled` or `failed`. A stale or changed fact records `rejected` and sends no
signal.

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
Run it from a persistent non-TTY command runner for the positive action suite;
a normal interactive terminal deliberately makes the target hard-protected and
is useful as a refusal test:

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
