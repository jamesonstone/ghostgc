---
kit_metadata_version: 1
artifact: spec
workflow_version: 3
phase: complete
feature:
  id: 0008
  slug: product-messaging
  dir: 0008-product-messaging
relationships:
  - type: builds_on
    target: 0007-single-binary-runtime
references:
  - id: issue-21
    name: Product messaging issue
    type: issue
    target: https://github.com/jamesonstone/ghostgc/issues/21
    relation: guides
    read_policy: must
    used_for: scope and acceptance criteria
    status: active
skills:
  - name: github:yeet
    source: GitHub plugin
    path: github:yeet
    trigger: publish the validated messaging change as a ready pull request
    required: true
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Present ghostgc as a complete product whose messages describe current
capabilities and safety boundaries, not the development sequence used to build
them.

## CONTEXT

Delivery phases remain useful historical and planning documentation. Exposing
them in CLI banners, status responses, generated configuration, diagnostics,
audit entries and platform errors makes product output look provisional and
couples stable interfaces to an internal development milestone.

## REQUIREMENTS

- Product-facing text must not mention delivery phases or numbered phases.
- Machine-readable runtime responses must not expose delivery-phase metadata.
- Version output identifies the build only.
- Cleanup authority messages continue to state their exact current safety
  bounds without using roadmap language.
- Unsupported capabilities describe what is unavailable in the current build,
  not when a future phase might add them.
- Delivery-phase history remains in `README.md` and repository documentation.
- The current formatting-only README delta is included unchanged.

### Non-goals

- Removing delivery history from documentation.
- Changing cleanup authority, policy behavior or signal safety.
- Implementing unsupported adapters, Linux collection or Linux service
  management.

## ACCEPTED PLAN

1. Remove delivery metadata from version and status contracts and all CLI
   renderers.
2. Replace roadmap-oriented notes, diagnostics, audit summaries and generated
   configuration comments with current capability language.
3. Remove obsolete phase constants and phase-coupled tests; add assertions for
   the stable product messaging and retained safety authority text.
4. Rephrase non-documentation source and test comments so delivery-phase
   terminology remains confined to repository documentation.
5. Carry the existing README formatting delta, validate direct CLI output and
   run the complete required checks.

## DECISIONS

### Development history is not runtime metadata

The status API removes its `phase` field rather than retaining an invisible or
empty compatibility value. A development milestone is not a stable product
contract and should not require ongoing synchronization with releases.

### Capability language replaces roadmap language

Safety notes continue to state the one-candidate, full-revalidation and
SIGTERM-only boundaries. Unsupported features report their present state
without promising a future delivery schedule.

## DISCOVERIES

- Delivery language reached beyond help and version output into the status
  JSON schema, daemon startup audit, policy/explanation notes, generated
  configuration and unsupported-platform errors.
- The phase constant existed only to feed those product surfaces and can be
  removed once the messages are capability-based.

## VALIDATION

- `go test ./cmd/cli ./internal/api ./internal/classification ./internal/config
  ./internal/daemon ./internal/protection ./internal/sessions ./internal/storage
  ./internal/version` — passed the complete directly affected package set.
- `make check` — passed formatting, vet, the full native test suite and the
  repository-wide 300-line source-size audit.
- `make race` — passed every package. The macOS linker emitted its existing
  malformed `LC_DYSYMTAB` warnings for cgo test objects without failing.
- `make lint` — passed with zero issues.
- `GOOS=linux CGO_ENABLED=0 go test -exec=/usr/bin/true ./...` — compiled all
  packages and tests for Linux, including the revised unsupported-capability
  messages.
- `make build` plus direct `ghostgc`, `ghostgc version` and `ghostgc daemon
  --version` checks — passed; both version commands produce one build-only
  line, and root help renders the stable product-value banner.
- An isolated `ghostgc config init` generated a valid audit-mode configuration
  with capability-based guidance and no development-milestone language.
- A repository search found no phase references outside `README.md` and
  documentation; historical `tests/RUN_STATUS.md` evidence remains intact.
- The issue-branch `README.md` is byte-for-byte identical to the authorized
  formatting delta in the protected `main` checkout.
- `kit check 0008-product-messaging` — passed.
- `kit check --project` — the new spec passes, while project validation reports
  the same 20 pre-existing blocking findings on this branch and `main`: one
  legacy-front-matter warning, 19 legacy-spec or missing-summary errors.

## OUTCOME

ghostgc product output now describes its version, capabilities and safety
boundaries without exposing the development sequence. The CLI banner, version
commands, status output and JSON, daemon startup audit, policy and explanation
notes, doctor remedies, generated configuration, Linux errors and packaging
messages contain no delivery-phase metadata or roadmap promises.

The obsolete phase constants and status field are removed. Cleanup messaging
still states the singular-candidate, fresh-revalidation and SIGTERM-only bounds.
Delivery history remains available in `README.md`, specs, references and
historical validation evidence. The user-owned README formatting delta is
carried unchanged.

## REPOSITORY MEMORY

- This spec preserves why development phases remain documentation-only and why
  runtime authority messaging must describe capabilities directly.
- `docs/CONSTITUTION.md` records that product output describes current
  capabilities and safety boundaries while delivery history stays in
  documentation.
