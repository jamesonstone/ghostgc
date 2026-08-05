---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: complete
feature:
  id: 0012
  slug: simple-startup-modes
  dir: 0012-simple-startup-modes
relationships:
  - type: builds_on
    target: 0007-single-binary-runtime
  - type: builds_on
    target: 0011-filesystem-authority-hardening
references:
  - id: issue-29
    name: Simple startup modes issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/29
    relation: guides
    read_policy: must
    used_for: scope and acceptance criteria
    status: active
  - id: safety-model
    name: Safety model
    type: documentation
    target: docs/references/safety-model.md
    relation: constrains
    read_policy: must
    used_for: startup authority and filesystem safety
    status: active
  - id: testing-contract
    name: Testing and environment validation rule
    type: ruleset
    target: docs/references/rules/testing-and-environment-validation.md
    relation: constrains
    read_policy: must
    used_for: startup, configuration and adapter validation
    status: active
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Make ghostgc useful with one safe command while retaining an optional strict
configuration overlay for advanced users.

## CONTEXT

The daemon already loads safe built-in values when no configuration file
exists, but the documented flow still requires users to generate a config and
manage the service explicitly. Those steps obscure the product's two useful
operating states: observe what ghostgc would recommend, or permit exact manual
reconciliation after preview and confirmation.

Codex CLI and the macOS Codex app share the default `~/.codex` home. Their
standard shell snapshots and worktrees therefore have stable provider-owned
roots, and the app's agent backend has a distinct exact executable location.
These contracts are narrow enough to supply as built-in defaults without
granting broad home-directory access.

## REQUIREMENTS

- `ghostgc start` installs or refreshes the background service in audit mode.
- `ghostgc start --mode reconcile` installs or refreshes it in manual
  reconciliation mode.
- Neither command requires a generated configuration file.
- Audit is an authority ceiling: configuration may narrow it but cannot grant
  recommendation or automatic action.
- Reconcile is an authority ceiling: configuration may narrow it, and exact
  recommendations may be previewed and manually applied, but automatic
  enforcement remains unavailable.
- Built-in profiles enable the Codex adapter, one exact audit/recommend process
  policy, and exact existing `~/.codex/shell_snapshots` and
  `~/.codex/worktrees` roots.
- Missing, linked, non-directory or non-canonical default roots grant no
  authority. Custom roots remain opt-in configuration.
- An optional strict YAML file overlays the profile before its authority
  ceiling is applied. Unknown fields and unsafe settings remain errors.
- Codex sessions without an explicit `CODEX_HOME` use the same canonical
  `~/.codex` default as Codex CLI and the macOS app.
- The exact macOS app backend at
  `<Codex-or-ChatGPT>.app/Contents/Resources/codex` is observable; Electron
  framework helpers and `codex-code-mode-host` remain refused.
- Existing `service` and foreground `daemon` commands remain available for
  compatibility and advanced operation.
- README leads with install, the two startup commands, status, and primary
  inspection commands. Detailed configuration and lifecycle procedures remain
  in linked references.

### Non-goals

- Automatic process termination, automatic cache purge, automatic worktree
  removal, recursive deletion, broad cache discovery or whole-home authority.
- Inventing defaults for non-Codex agents or undocumented artifact layouts.
- Removing advanced configuration, foreground operation or service controls.

### Observable acceptance

- A clean user home can run `ghostgc start` without `config init`; no config is
  created as a side effect.
- Service arguments durably include the selected startup mode and optional
  config overlay path.
- Audit startup downgrades any configured recommendation or enforcement
  authority; reconcile startup downgrades enforcement to manual recommendation.
- Default Codex roots are enabled only when their exact physical directories
  exist, and explicit YAML can replace or disable them.
- Adapter fixtures distinguish the exact macOS Codex backend from existing app
  bundle false positives and attach the default Codex home when needed.
- Focused tests, `make check`, `make race`, `make lint`, `make size`,
  `git diff --check`, a one-cycle audit fixture, and the feature Kit check pass.

## ACCEPTED PLAN

1. Add typed audit and reconcile startup profiles with conservative Codex
   defaults, YAML overlay semantics, and hard post-overlay authority ceilings.
2. Add `ghostgc start`, route service installation through the selected mode,
   and pass that mode to the persistent daemon without writing configuration.
3. Recognize the exact macOS Codex backend and supply the canonical Codex home
   when the process does not explicitly export one.
4. Cover mode parsing, overlay precedence, service registration, root safety,
   app detection and help output with focused tests.
5. Rewrite the README around the two startup commands and move operational
   detail into the operator and dogfooding references.
6. Run complete validation, curate durable documentation, and publish a ready
   issue-linked pull request.

## DECISIONS

### Startup mode is a ceiling, not an authority override

The command selects the maximum authority available to that daemon instance.
Configuration remains useful for narrowing selectors, roots, bounds and
cadence, but a stale or surprising file cannot turn audit startup into action
or reconciliation startup into automation.

### Standard Codex roots are pinned product defaults

An exact existing physical `~/.codex/shell_snapshots` or `~/.codex/worktrees`
directory is an application-owned default allowlist entry, not runtime path
discovery. Session metadata must still match the cache root, and every existing
identity, ownership, stability, quarantine, retirement and foreground
confirmation safeguard remains in force.

### Reconciliation remains manual

"Live" means recommendations and exact user-approved reconciliation are
available. It does not mean unattended deletion. This preserves the capability
split introduced by filesystem authority hardening and keeps the persistent
daemon unable to perform irreversible filesystem actions.

## DISCOVERIES

- Local Codex CLI 0.146.0 and the macOS app both use
  `/Users/jamesonstone/.codex`; the app bundle identifier is `com.openai.codex`
  and its agent backend is `ChatGPT.app/Contents/Resources/codex`.
- The existing loader already supports safe missing-file fallback and strict
  partial YAML overlays, so startup profiles can extend it without adding a
  second configuration format.
- Keeping the legacy `service install` path configuration-authoritative
  preserves advanced foreground/background operation, while the new `start`
  path always persists its explicit audit or reconcile ceiling.

## VALIDATION

- Focused configuration, service, help, daemon and Codex adapter tests pass.
- `make check` passes complete formatting, vet, source-size and package tests.
- `make race` passes all packages. The macOS linker emits its existing
  non-failing malformed `LC_DYSYMTAB` warnings.
- `make lint` reports zero issues; `make size` and `git diff --check` pass.
- A built binary completes `ghostgc daemon --mode audit --once` in a clean
  temporary home without a configuration file.
- Root and command help show `start` first and document audit as its default.
- `kit check simple-startup-modes` passes.

## OUTCOME

- `ghostgc start` now starts or refreshes the background service in safe audit
  mode; `ghostgc start --mode reconcile` enables exact manual reconciliation.
- Both modes supply narrow existing Codex roots and policies, accept strict
  optional overrides, and enforce a post-overlay authority ceiling that cannot
  become automatic.
- Codex CLI and the exact macOS app backend share a physical default Codex home;
  application-bundle helpers remain refused.
- Configuration generation, diagnostics, help, README, dogfooding, operator,
  architecture and safety documentation all lead with the no-config workflow.

## REPOSITORY MEMORY

- This spec owns startup-mode authority, built-in Codex defaults, and the
  rationale for treating reconciliation as manual.
- User commands and advanced overrides belong in the README and operator guide;
  demonstrated safety invariants belong in the constitution and safety model.
- `docs/CONSTITUTION.md` records the project-wide startup ceiling and pinned
  standard-root invariants. `docs/references/architecture.md`,
  `operator-guide.md`, `dogfooding.md`, `manual-cleanup.md`, and
  `safety-model.md` preserve reusable operating detail.
