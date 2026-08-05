# Manual Worktree Lifecycle Guide

ghostgc inventories local Git worktrees and can retire one stale checkout only
after explicit approval. Retirement is reversible. Later permanent removal is
separate, grace-gated, foreground-only, non-force, and branch-preserving.

## Configure discovery

Agent-associated repository discovery is enabled by default. Optional roots add
operator-declared workspace discovery:

```yaml
worktrees:
  enabled: true
  scanInterval: 5m
  staleAfter: 168h
  retirementGrace: 168h
  roots:
    - /Users/example/worktrees/example-owner
```

`staleAfter` cannot be shorter than seven days and `retirementGrace` cannot be
shorter than one day. Roots must be absolute canonical physical directories.
Filesystem roots, the whole home, symlinks, more than 32 roots, and traversal
beyond the bounded discovery depth are refused.

## Inventory states

- `active` — a live session or same-user process is working inside it.
- `observing` — complete inactivity is accumulating.
- `stale` — seven uninterrupted days of complete unchanged safe evidence.
- `protected` — a hard protection applies.
- `unknown` — observation was incomplete and inactivity reset.
- `missing` — Git reports missing or prunable registration state; ghostgc does
  not prune it.
- `retired` — the complete checkout remains registered at a restorable sibling.
- `removed` — a bounded tombstone after verified foreground finalization.

Only a present registered secondary can become stale. Primary, locked,
prunable, missing, dirty, unreadable, symlinked, active, operation-in-progress,
submodule, nested-mount, local-only-commit, and unsafe-detached worktrees are
protected. Incomplete process or vnode inspection is also a refusal.

The only ignored-material exception is a repository-root `.env` or `.envrc`
symlink resolving exactly to the corresponding primary-checkout file. Contents
are never read or retained.

## Retire

```bash
ghostgc worktrees --state stale
ghostgc worktree show '<id-or-prefix>'
ghostgc worktree remove --dry-run --worktree '<id-or-prefix>'
```

Preview binds the stable registration, checkout and Git-directory identities,
path, branch, ref, HEAD, status fingerprint, inactivity, discovery authority,
approved links, process use, filesystem evidence, configuration, and exact Git
executable. Its token is memory-only, single-use, and valid for two minutes.

Apply repeats every check and commits `attempting` before native non-force
`git worktree move`. The destination is a deterministic absent sibling on the
same filesystem. Success requires the original path to be absent, the directory
inode to be unchanged, and the same stable registration to point at the
retirement path. The action then becomes `retired`; no checkout content or
branch was deleted.

## Restore

```bash
ghostgc worktrees --state retired
ghostgc worktree show '<id-or-prefix>'
ghostgc worktree restore --dry-run --worktree '<id-or-prefix>'
```

Restore has its own approval and fresh no-use, filesystem, identity, and absent-
destination checks. Native non-force `git worktree move` returns the same
registration and directory inode to the original path. A restored checkout
must accumulate fresh observation before it can become stale again.

## Permanently finalize

Before the configured grace elapses, purge preview is refused. Afterward:

```bash
ghostgc worktree purge --dry-run --worktree '<id-or-prefix>'
```

Run only the generated apply command. It includes the complete worktree ID via
`--confirm`; a prefix is insufficient. The daemon repeats all clean, published,
no-use, filesystem, registration, configuration, and Git-executable checks,
commits `purging`, and returns one short-lived exact plan. It cannot invoke
native worktree removal.

The foreground CLI pins the same Git executable, removes only approved root
environment links, and invokes `git worktree remove <retired-path>` without
force. It reports the result through a single-use completion capability. The
daemon records `removed` only after it independently proves that both the path
and registration are absent. The branch remains.

```bash
ghostgc worktree actions --worktree '<id-or-prefix>'
ghostgc logs --kind worktree.retirement.retired
```

An ambiguous post-action state is recorded as partial and opens the filesystem
mutation circuit. Restart the daemon, allow a fresh scan, and investigate the
durable action. ghostgc never deletes a branch, uses force, fetches, prunes,
resets, cleans, stashes, invokes a shell, or calls recursive filesystem delete.
