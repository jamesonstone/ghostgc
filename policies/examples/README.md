# Cleanup policies

**Nothing in this directory is loaded directly.** Phase 5 loads policies from
the main strict YAML configuration. Copy a policy from this directory into that
file's top-level `policies` list, review every exact scope, then restart the
daemon. Runtime enable/disable remains intentionally unsupported.

Phase 5 evaluates and stores audit decisions but has no recommendation or
action path. Recommendation mode arrives in phase 6 and narrowly scoped
enforcement in phase 7.

## What a policy will and will not be able to do

A policy expresses **which processes a rule may consider** and **what conditions
must all hold**. It cannot:

- override a hard protection (specification section 12.4);
- act on a process whose attribution confidence is below 0.95;
- evaluate while the global mode is `disabled`;
- match `suspicious`, because that state means progress or live resources
  remain;
- recommend or signal anything in Phase 5.

A policy is a narrowing of what is permitted. It can never widen it.

## Why the first enforceable policies are so specific

Specification section 14 rules out policies against `node`, `python`, `go`,
`java`, `git`, `bash`, `zsh`, language servers, build systems, test runners and
container processes — the names are too broad to establish what the process is
doing. The first enforceable classes will be things whose disposability is
structurally evident: a headless browser created by a session that has since
completed or an agent-owned, purpose-specific helper whose finished lifecycle
is independently established.
