# Validation Run Status

Curated current-state map. Immutable high-level run evidence stays under
ignored `tmp/<UTC-date>/<stable-test-id>/<run-number>/`; feature rationale and
acceptance evidence live in the corresponding `SPEC.md`.

| Suite | Environment | Current status | Latest attempt | Latest pass | Source/deployment | Run ID | Evidence | Active finding |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| repository code gates | local macOS | PASS | 2026-08-03T17:08:27Z | 2026-08-03T17:08:27Z | `e4f1d76` | not applicable | `make check`; `make race`; `make lint`; `git diff --check` | none; all handwritten Go source/test files at or below 300 lines |
| Phase 3 live process activity | local macOS | PASS | 2026-08-03T15:43:39Z | 2026-08-03T15:43:39Z | `ghostgc` 6ab8520 | 20260803T154208Z-e90043 | `tmp/2026-08-03/fixture-agent.sh/1/` | none; fixture-owned PIDs removed and scratch state moved to Trash |
| pull-request CI | GitHub `macos-15` | PASS | 2026-08-03T16:58:26Z | 2026-08-03T16:58:26Z | `7743524` | not applicable | Actions run `30834492293` | none; Check, Lint and Race detector passed; later source-only fixes retain local gate coverage pending refreshed PR CI |
| Phase 4 deterministic classification | local macOS + GitHub `macos-15` | PASS | 2026-08-03T16:18:33Z | 2026-08-03T16:18:33Z | `51595a9` | 20260803T161558Z-p4-review | `tmp/2026-08-03/phase4-classification/3/`; Actions run `30831445158`; `make check`; `make race`; `make lint` | none; direct-launchd root stayed not-detached, later observed parent loss was detached with evidence, periodic/idle states remained correct |
| Phase 5 fail-closed policy audit | local macOS | PASS | 2026-08-03T17:07:23Z | 2026-08-03T17:07:23Z | exact source `e4f1d76`, deployed version `e4f1d76` | 20260803T170659Z-p5f1 | `tmp/2026-08-03/phase5-policy-live/1/result.json`; `output.txt`; `config.yaml`; SQLite evidence | none; fixture-owned crashed child was refused 17 times by active-session protection, 17 unique evaluation IDs, zero enforceable entries/actions, complete cleanup |
