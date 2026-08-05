# Dogfooding ghostgc

Start with observation. Move to manual reconciliation only after the reported
sessions, ownership evidence, and candidates match what you know about the
machine.

## 1. Start in audit mode

```bash
make install
ghostgc start
ghostgc doctor
ghostgc status
```

No configuration is required. The built-in profile recognizes Codex CLI and
the macOS Codex app, and uses existing physical `~/.codex/shell_snapshots` and
`~/.codex/worktrees` directories. Audit mode cannot signal a process,
quarantine a cache artifact, or retire a worktree.

Let the daemon run for at least two activity intervals, then inspect:

```bash
ghostgc sessions
ghostgc processes --all
ghostgc activity
ghostgc classifications
ghostgc candidates
ghostgc cache candidates
ghostgc worktrees
ghostgc logs
```

Use `ghostgc explain <pid>` and the relevant `show` or `explain` command for any
surprising conclusion. Unknown or incomplete evidence should appear as a
protection, never a candidate.

## 2. Enable manual reconciliation

When the audit output is credible, restart in reconciliation mode:

```bash
ghostgc start --mode reconcile
ghostgc status
```

This enables recommendations under the same exact defaults. It does not enable
automatic cleanup. Every action still needs a fresh dry-run preview, an
expiring single-use approval, and the generated apply command.

For process recommendations:

```bash
ghostgc candidates
ghostgc cleanup --dry-run --process '<pid:start_time_ns>' --policy '<policy-id>'
ghostgc actions
```

For session shell snapshots, cleanup is a reversible same-filesystem
quarantine. Restore is independently previewed; permanent purge is later,
grace-gated, foreground-only, and requires the complete artifact ID:

```bash
ghostgc cache candidates
ghostgc cache cleanup --dry-run --artifact '<artifact-id>' --policy codex-shell-snapshots
ghostgc cache quarantined
ghostgc cache restore --dry-run --artifact '<artifact-id>'
```

For worktrees, `remove` first retires the registered checkout intact. The
branch remains and the checkout can be restored during its grace period:

```bash
ghostgc worktrees --state stale
ghostgc worktree remove --dry-run --worktree '<id-or-prefix>'
ghostgc worktrees --state retired
ghostgc worktree restore --dry-run --worktree '<id-or-prefix>'
```

## 3. Extend defaults only when needed

Create a strict YAML overlay for custom Codex homes, additional workspace
roots, different exact executable policies, cadence, bounds, or storage paths:

```bash
ghostgc config init
${EDITOR:-vi} "$(ghostgc config path)"
ghostgc start
```

The selected startup mode remains the authority ceiling. An audit start
downgrades configured recommendation or enforcement; a reconciliation start
downgrades enforcement to manual recommendation. Explicit disabled settings
and narrower policies remain disabled or narrow.

Advanced automatic process enforcement is intentionally outside both startup
profiles. Validate it only with the bundled fixture and an explicitly reviewed
configuration while running `ghostgc daemon --config <path>` in the foreground;
see [manual-cleanup.md](manual-cleanup.md) and the
[safety model](safety-model.md). Filesystem lifecycle actions have no automatic
mode.
