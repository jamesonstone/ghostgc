package config

// exampleConfig is written by `ghostgc config init`. Audit mode remains the
// default; recommend must be explicitly selected at both levels.
const exampleConfig = `# ghostgc configuration
#
# Every generated configuration ships in audit mode. Phase 6 also accepts
# recommend for exact, manually approved SIGTERM. Enforcement remains rejected
# until delivery phase 7.
version: 1
globalMode: audit

sampling:
  # Cheap system-wide process scan.
  processScan: 15s
  # Detailed per-process activity sample. Delivery phase 3.
  activitySample: 60s
  # Classification evaluation. Delivery phase 4.
  classification: 60s
  # Policy evaluation. Delivery phase 5.
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
  # Must remain false. ghostgc records paths and metadata, never file contents.
  storeSourceContents: false
  # Must remain false. This build has no telemetry transport.
  networkTelemetry: false

agents:
  codex:
    enabled: true

# Keep this example disabled until its exact executable scope fits your local
# workload. To dogfood manual cleanup, set globalMode and this policy mode to
# recommend, enable it, inspect ghostgc candidates, then request a preview.
policies:
  - id: completed-headless-browser
    description: audit an orphaned Codex headless browser
    enabled: false
    mode: disabled
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
