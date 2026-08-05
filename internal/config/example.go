package config

// exampleConfig is written by `ghostgc config init`. Every omitted field keeps
// the selected startup profile's built-in value, so this file is an overlay
// rather than a second required setup step.
const exampleConfig = `# Optional ghostgc overrides
#
# ghostgc start supplies safe Codex defaults. Add only the fields you
# need to change. Audit and reconcile startup modes remain authority ceilings.
version: 1

# sampling:
#   processScan: 15s
#   activitySample: 60s
#   classification: 60s
#   policyEvaluation: 5m
#   retention: 6h

# Add a custom Codex home by replacing the built-in cache root.
# Paths must be absolute, canonical, physical directories.
# cache:
#   enabled: true
#   roots:
#     - /Users/example/custom-codex/shell_snapshots
#   scanInterval: 30m
#   minStable: 24h
#   quarantineGrace: 168h
#   maxEntriesPerScan: 10000
#   maxEntriesPerAction: 1000
#   maxBytesPerAction: 10GiB

# Add custom workspace discovery roots. The whole home and filesystem root are
# always refused; worktree lifecycle actions remain manually approved.
# worktrees:
#   roots:
#     - /Users/example/worktrees

# Replace the built-in process policy only with an exact reviewed selector.
# policies:
#   - id: custom-codex-helper
#     description: custom orphaned Codex helper
#     enabled: true
#     mode: audit
#     automatic: false
#     states: [orphaned]
#     agents: [codex]
#     executables: [exact-helper-basename]
#     requireDetached: true
#     requireSessionEnded: true
#     minStable: 5m
#     cooldown: 1h

# paths:
#   stateDir: /absolute/private/state/path
#   logDir: /absolute/private/log/path
#   socket: /absolute/private/state/path/ghostgc.sock
#   database: /absolute/private/state/path/ghostgc.db
`
