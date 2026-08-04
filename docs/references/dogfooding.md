# Dogfooding ghostgc

Start with observation, graduate one exact policy to recommendation, and use
the bundled fixture before enabling automatic enforcement for real work. Audit
is useful immediately and is always the default.

## 1. Install and observe

`make install` installs one executable. `ghostgc service install` registers
that same executable with its persistent `daemon` subcommand.

```bash
make install
ghostgc config init
ghostgc service install
ghostgc doctor
ghostgc status
ghostgc sessions
ghostgc processes --all
ghostgc classifications --latest
ghostgc worktrees
```

Let the daemon run for at least two activity intervals (two minutes with the
default configuration), then inspect `ghostgc activity`, `ghostgc explain
<pid>`, `ghostgc metrics` and `ghostgc logs`. Nothing can be signalled while
`globalMode: audit` remains in place.

Worktree inventory is also safe to inspect immediately. Registered worktrees
from observed session repositories appear automatically; configured roots are
optional. A new record begins in `observing`, and there is no command or
configuration shortcut around seven continuous days of complete inactivity:

```bash
ghostgc worktrees --state observing
ghostgc worktree show '<id-or-prefix>'
ghostgc metrics
```

After a worktree becomes `stale`, follow
`manual-worktree-cleanup.md` to preview and manually remove exactly that ID.
Leave the branch in place until you independently decide its lifecycle.

The daemon reads configuration only at startup. After an edit, reinstall the
LaunchAgent in place to restart it without deleting configuration, SQLite
history or logs:

```bash
ghostgc service install
```

## 2. Audit one exact class

The generated configuration contains a disabled policy. Replace its executable
only with a basename you have observed and explained, keep the policy narrow,
then set `enabled: true` and `mode: audit`. After restart:

```bash
ghostgc policies
ghostgc candidates
ghostgc logs --kind policy.candidate
```

A candidate is evidence, not authority. Refusals and cooldowns remain visible.

## 3. Try manual cleanup

Set `globalMode: recommend` and the reviewed policy `mode: recommend`. Restart,
wait for a recommendation, and preview the exact process/policy pair:

```bash
ghostgc candidates
ghostgc cleanup --dry-run --process '<pid:start_time_ns>' --policy '<policy-id>'
```

Read the preview, then paste its generated apply command within two minutes.
The approval is single-use. Check `ghostgc actions` and the action audit log.
See `manual-cleanup.md` for the full contract.

## 4. Prove automatic enforcement on the fixture

Start the fixture, allow one scan to record its original parent, then end only
the fixture session root:

```bash
fixtures/fixture-agent.sh start
sleep 20
fixtures/fixture-agent.sh orphan
```

In the active configuration shown by `ghostgc config path`, temporarily add
this dedicated fixture policy and remove or disable every other enforce policy:

```yaml
globalMode: enforce
policies:
  - id: fixture-action-child
    description: enforce only the bundled orphaned native fixture target
    enabled: true
    mode: enforce
    automatic: true
    states: [orphaned]
    agents: [codex]
    executables: [fixture-helper]
    requireDetached: true
    requireSessionEnded: true
    minStable: 5m
    cooldown: 1h
```

Restart the daemon and watch with a portable shell loop (stop it with Ctrl-C):

```bash
while :; do
  clear
  date
  ghostgc candidates
  ghostgc actions
  sleep 5
done
```

After at least five continuous known-idle minutes, ghostgc may attempt one
automatic exact-key SIGTERM. Confirm the `automatic` action, target exit,
non-target survival and audit ordering, then always remove the fixture:

```bash
ghostgc logs --kind action.attempting
ghostgc logs --kind action.signalled
fixtures/fixture-agent.sh stop
```

## 5. Promote a real policy only after evidence

Keep a real policy in audit long enough to review every match and refusal.
Automatic enforcement is appropriate only for a single exact executable class
that is consistently attributed at confidence 0.95 or greater, has no terminal
or descendants, belongs to an ended session, is detached, and reaches orphaned
through complete continuous evidence. Preserve `minStable: 5m` or longer and
`cooldown: 1h` or longer. Never use a broad runtime such as `node`, `python`,
`go`, `java`, a shell, editor, language server, database or build tool.

To stop action immediately, set `globalMode: audit` and restart. Historical
decisions and actions remain available for review.
