# Cleanup policies

**Nothing in this directory is loaded.** The policy engine is delivery phase 5,
recommendation mode is phase 6 and narrowly scoped enforcement is phase 7. This
build has no policy storage, no evaluation and no action path; `ghostgc policy
enable` refuses and says so.

The file here is a reference for the shape policies will take, kept alongside
the code so that phase 5 starts from a settled format rather than inventing one.

## What a policy will and will not be able to do

A policy will express **which processes a rule may consider** and **what
conditions must all hold**. It will not be able to:

- override a hard protection (specification section 12.4);
- act on a process whose attribution confidence is below 0.95;
- act while the global mode is anything other than `enforce`;
- send SIGKILL unless the policy sets `allowForce` *and* the operator has
  enabled it;
- act at all without passing the full pre-action revalidation sequence,
  including the process start-time check that defeats PID reuse.

A policy is a narrowing of what is permitted. It can never widen it.

## Why the first enforceable policies are so specific

Specification section 14 rules out enforcing against `node`, `python`, `go`,
`java`, `git`, `bash`, `zsh`, language servers, build systems, test runners and
container processes — the names are too broad to establish what the process is
doing. The first enforceable classes will be things whose disposability is
structurally evident: a headless browser created by a session that has since
completed, a detached shell wrapper with no descendants, an agent-owned helper
whose invocation identifier names a finished session.
