package config

// exampleConfig is written by `ghostgc config init`. Audit mode remains the
// default; recommend or enforce must be explicitly selected at both levels.
const exampleConfig = `# ghostgc configuration
#
# Every generated configuration ships in audit mode. Recommend enables exact,
# manually approved SIGTERM. Enforce is intentionally omitted from this safe
# starter policy; see docs/references/dogfooding.md before enabling it.
version: 1
globalMode: audit

sampling:
  # Cheap system-wide process scan.
  processScan: 15s
  # Detailed per-process activity sample.
  activitySample: 60s
  # Classification evaluation.
  classification: 60s
  # Policy evaluation.
  policyEvaluation: 5m
  # Retention compaction.
  retention: 6h

retention:
  rawObservations: 24h
  aggregatedObservations: 720h
  actions: 2160h
  # Hard ceiling on the SQLite file. Exceeding it triggers aggressive
  # compaction and an audit-log entry.
  maxDatabaseBytes: 262144000

notifications:
  suspicious: true
  hung: true
  candidate: true
  actionTaken: true
  healthy: false

privacy:
  # Command lines are stored with credential-bearing arguments redacted.
  storeCommandLines: true
  # Environment values are redacted; only agent-relevant variable names are
  # retained at all.
  redactEnvironmentValues: true
  # Must remain false. ghostgc retains metadata, never source file contents.
  storeSourceContents: false
  # Must remain false. This build has no telemetry transport.
  networkTelemetry: false

agents:
  codex:
    enabled: true

# Cache observation is a separate, default-disabled authority lane. The only
# supported provider is Codex shell snapshots with exact session ownership.
cache:
  enabled: false
  globalMode: audit
  scanInterval: 30m
  minStable: 24h
  quarantineGrace: 168h
  maxEntriesPerScan: 10000
  maxEntriesPerAction: 1000
  maxBytesPerAction: 10GiB
  policies: []

worktrees:
  # Inventory is read-only. Removal always requires a fresh manual preview.
  enabled: true
  scanInterval: 5m
  # Values below seven days are rejected.
  staleAfter: 168h
  # Optional absolute canonical coding-agent workspace roots (maximum 32).
  # Agent-associated repository paths are discovered independently.
  roots: []

# Keep this example disabled until its exact executable scope fits your local
# workload. To dogfood manual cleanup, set globalMode and this policy mode to
# recommend, enable it, inspect ghostgc candidates, then request a preview.
policies:
  - id: completed-headless-browser
    description: audit an orphaned Codex headless browser
    enabled: false
    mode: disabled
    automatic: false
    states: [orphaned]
    agents: [codex]
    executables: [chrome-headless-shell]
    requireDetached: true
    requireSessionEnded: true
    minStable: 5m
    cooldown: 1h

# paths:
#   stateDir: ~/Library/Application Support/ghostgc
#   logDir: ~/Library/Logs/ghostgc
#   socket: ~/Library/Application Support/ghostgc/ghostgc.sock
#   database: ~/Library/Application Support/ghostgc/ghostgc.db
`
