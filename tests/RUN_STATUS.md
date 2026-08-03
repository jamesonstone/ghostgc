# Validation Run Status

Curated current-state map. Immutable high-level run evidence stays under
ignored `tmp/<UTC-date>/<stable-test-id>/<run-number>/`; feature rationale and
acceptance evidence live in the corresponding `SPEC.md`.

| Suite | Environment | Current status | Latest attempt | Latest pass | Source/deployment | Run ID | Evidence | Active finding |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| repository code gates | local macOS | PASS | 2026-08-03T15:45:05Z | 2026-08-03T15:45:05Z | `6ab8520` plus evidence/docs curation | not applicable | `make check`; `make race`; `make lint`; Linux cross-build; Phase 3 spec validation | none |
| Phase 3 live process activity | local macOS | PASS | 2026-08-03T15:43:39Z | 2026-08-03T15:43:39Z | `ghostgc` 6ab8520 | 20260803T154208Z-e90043 | `tmp/2026-08-03/fixture-agent.sh/1/` | none; fixture-owned PIDs removed and scratch state moved to Trash |
| pull-request CI | GitHub `macos-15` | PASS | 2026-08-03T15:47:24Z | 2026-08-03T15:47:24Z | `de2d27e` | not applicable | Actions run `30829051577` | none; Check, Lint and Race detector passed |
