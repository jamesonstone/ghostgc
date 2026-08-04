# Manual Worktree Cleanup Guide

ghostgc inventories local Git worktrees and can remove one stale checkout only
after explicit operator approval. Removal is never automatic, never deletes a
branch, never consults GitHub or another network service, and never uses force.

## Configure discovery authority

Agent-associated discovery is enabled by default. Each repository path recorded
for an observed coding-agent session grants read-only access to that repository's
registered worktree list. Optional roots add operator-declared coding-agent
workspace areas:

```yaml
worktrees:
  enabled: true
  scanInterval: 5m
  staleAfter: 168h
  roots:
    - /Users/example/worktrees/example-owner
```

`staleAfter` cannot be shorter than 168 hours. Roots must already be absolute,
physical, canonical directories. At most 32 are accepted. Filesystem roots,
the whole home directory and symlink roots are refused. Traversal stops after
four levels, never follows symlinks and is entry-bounded.

Restart the daemon after changing configuration:

```bash
ghostgc service install
ghostgc worktrees
```

## Understand inventory states

- `active` — a live agent session or same-user process is working inside it.
- `observing` — complete inactivity is accumulating but has not reached seven
  days.
- `stale` — seven uninterrupted days of complete, unchanged, safe evidence.
- `protected` — the checkout is present but a hard protection applies.
- `unknown` — a complete observation could not be made; inactivity was reset.
- `missing` — Git reports missing or prunable registration state; ghostgc does
  not prune it.
- `removed` — a bounded tombstone for a completed manual removal.

Review one exact identity or unambiguous prefix:

```bash
ghostgc worktrees --state protected
ghostgc worktree show '<id-or-prefix>'
```

The inactivity window starts only on a complete clean observation. A daemon
restart, scan gap, clock reversal, active session, same-user CWD, Git-state
change, dirty content, Git operation or incomplete evidence resets it. There is
no supported shortcut for changing timestamps or bypassing the seven-day
window.

## Hard protections

Only a present, registered secondary worktree can become stale. ghostgc refuses
primary, locked, prunable, missing, unreadable or symlinked registrations;
merge, rebase, cherry-pick, revert, bisect, lock and submodule state; staged,
tracked, conflicted, untracked or ignored material; local-only commits; and an
unreachable detached HEAD.

Targeted preview and apply additionally inspect every same-user process for a
working directory or open vnode inside the exact checkout and walk the
filesystem for nested mounts. Permission denial, process-table churn, bounds or
timeouts are refusals, never evidence of inactivity.

On macOS, a SIP-protected process may hide vnode paths. ghostgc falls back to
bounded scan-local device/inode comparison; if macOS denies that metadata too,
the preview refuses and reports the exact process key. Retry only after the
process or operating-system condition has changed.

The only ignored/untracked exception is a repository-root `.env` or `.envrc`
symlink resolving exactly to the matching file in the primary checkout. A
regular environment file, broken link, unexpected target or any other symlink
material remains protected. File contents are never retained.

## Preview and apply

When an entry is `stale`, request a preview by identity, never by raw path:

```bash
ghostgc worktree remove --dry-run --worktree '<id-or-prefix>'
```

The preview sends no filesystem mutation. It binds the registration, checkout
and Git-directory inodes, path, branch, ref, HEAD, aggregate status fingerprint,
inactivity timestamps, discovery authority, exact Git executable, approved
environment links, process usage and filesystem evidence. Its bearer approval
is memory-only, single-use and valid for two minutes.

Read the output and run only its generated apply command. Apply serializes with
daemon scans and other removals, consumes the approval once, and repeats every
check. A changed fact records `rejected` without unlinking anything.

After successful revalidation, ghostgc commits
`worktree.removal.attempting`, temporarily unlinks only verified environment
links, and invokes native `git worktree remove <canonical-path>` without force.
If Git refuses, those links are restored and the action becomes `failed`.
Success requires both the directory and Git registration to be absent before
the action becomes `removed`.

```bash
ghostgc worktree actions --worktree '<id-or-prefix>'
ghostgc logs --kind worktree.removal.removed
```

## Branch preservation and recovery

Removal leaves the local branch and commits intact. Every result includes a
quoted recreation command, for example:

```bash
git -C '/path/to/primary' worktree add '/path/to/lane' 'GH-123'
```

If final persistence fails after Git removed the checkout, the durable action
remains `attempting`; the error says that the branch remains and prints the same
recreation command. Investigate the action and audit log before recreating.
ghostgc never deletes a branch, fetches, prunes, resets, cleans, stashes, invokes
a shell or recursively deletes a path.
