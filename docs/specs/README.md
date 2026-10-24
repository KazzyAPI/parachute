# Specs

Parachute updates Docker containers safely: it snapshots before an update, verifies the result, and rolls back automatically when the update fails. It understands Compose stacks and can manage several hosts.

These files describe Parachute's **observable behaviour**: what an operator sees in containers, labels, volumes, notifications, HTTP responses, logs, and exit codes. They do not describe code structure. Tests are written from these files ([ADR-0001](../adr/0001-spec-driven-development.md), [ADR-0015](../adr/0015-traceability-and-test-practice.md)); if a test and a spec disagree, the spec wins until the spec is changed. Ambiguity in a spec is a defect: report it, then fix the spec.

## Requirement format

```
### ABC-001 Example title
Parachute MUST ...

- **Given** ... **When** ... **Then** ...
```

- Each requirement has an ID, a title, one normative statement, and one or more acceptance criteria in Given/When/Then form.
- Each acceptance criterion is checkable from outside the program against a real engine and a local registry, except where an ADR names an exception (API-011, [ADR-0015](../adr/0015-traceability-and-test-practice.md)).
- Each file carries one status at the top: `Draft` (not yet reviewed), `Accepted`, or `Deferred` (a section whose tests may fail until a later milestone).
- Unresolved decisions do not go into requirements; they go into [open-questions.md](open-questions.md).
- Result states and reason codes are listed only in [reasons.md](reasons.md). A requirement that produces a new code adds it there in the same commit.
- Transaction states, trust boundaries, and redaction are defined once in the glossary: [Transaction state machine](glossary.md#transaction-state-machine), [Trust boundaries](glossary.md#trust-boundaries), [Redaction](glossary.md#redaction). Requirements reference these sections instead of restating them.

## IDs

IDs are `<DOMAIN>-NNN`. An ID is never changed and never reused; a removed requirement keeps its heading with the text `Withdrawn` and the reason.

| Prefix | Domain | File |
|---|---|---|
| `SEL` | Container selection | [01-selection.md](01-selection.md) |
| `SCH` | Scheduling | [02-scheduling.md](02-scheduling.md) |
| `DET` | Update detection | [03-update-detection.md](03-update-detection.md) |
| `UPD` | Update transaction | [04-update-transaction.md](04-update-transaction.md) |
| `DEP` | Dependencies and stacks | [05-dependencies-and-stacks.md](05-dependencies-and-stacks.md) |
| `HOK` | Lifecycle hooks | [06-lifecycle-hooks.md](06-lifecycle-hooks.md) |
| `BKP` | Backup before update | [07-backup.md](07-backup.md) |
| `VER` | Verification and rollback | [08-verification-and-rollback.md](08-verification-and-rollback.md) |
| `POL` | Update policies | [09-update-policies.md](09-update-policies.md) |
| `NTF` | Notifications | [10-notifications.md](10-notifications.md) |
| `API` | HTTP API and metrics | [11-http-api-and-metrics.md](11-http-api-and-metrics.md) |
| `REG` | Registry authentication | [12-registry-auth.md](12-registry-auth.md) |
| `DMN` | Daemons, multiple hosts, Podman | [13-daemon-and-runtimes.md](13-daemon-and-runtimes.md) |
| `STS` | Status view | [14-status-view.md](14-status-view.md) |
| `RST` | Restore verification | [15-restore-verification.md](15-restore-verification.md) |
| `CFG` | Configuration, output, exit codes, state | [16-configuration.md](16-configuration.md) |

## Keywords

MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are used as defined in RFC 2119. Names in `code` (flags, labels, states, reason codes, metric names) are exact and are part of the interface. Defaults stated in a requirement are normative. "Logged at `warn`" means a log line at that level whose message contains the named items.

## Tests

- Each acceptance criterion is one test, a Go test function in `e2e/*_test.go` named `Test`, the requirement ID with `_` for `-`, and a short description: `TestSEL_001_default_selects_all_running` ([ADR-0015](../adr/0015-traceability-and-test-practice.md)).
- Every `MUST` and `SHOULD` requirement has at least one test. `make trace` fails when a milestone lists an ID that is in no spec, or a `MUST` or `SHOULD` requirement listed for a milestone, or in a file with status `Accepted`, has no test.
- Tests are black-box ([ADR-0003](../adr/0003-black-box-tests-from-specs.md)): they start Parachute (binary or image) against a real Docker engine and a local `registry:2`, and assert only on containers and labels, volume contents, the Docker event stream, registry access logs, payloads received by a fake webhook, HTTP responses, logs, stdout, and exit codes. Fixture images (for example `localhost:5000/fixture/app`) are built and pushed locally. Official images used as test infrastructure (`registry:2`, `busybox`, `postgres`, `mysql`, `docker`) MAY be pulled from Docker Hub before the tests and re-pushed to the local registry; tests never depend on Docker Hub for the images under test. The one exception is API-011's constant-time comparison, which a code-level unit test covers in addition to its black-box test (ADR-0015).
- Unless a criterion says otherwise, it assumes Parachute is armed (CFG-006), lifecycle hooks are off, `--delay-days 0`, `--protect-databases` on, and one host named `local`.
- Times stated in criteria have a tolerance of ±5 seconds unless stated.

## Milestone coverage

Requirements implemented by each milestone. `make trace` fails when a `MUST` or `SHOULD` listed here has no test. Acceptance criteria that need update execution (an armed Parachute, UPD, BKP, VER, POL, HOK, API) are tested in the milestone that adds it.

| Milestone | Implemented requirements |
|---|---|
| M1 report-only core | CFG-001, CFG-002, CFG-003, CFG-004, CFG-005, CFG-006, CFG-007, CFG-008, CFG-009, CFG-010, CFG-011, SEL-001, SEL-002, SEL-003, SEL-004, SEL-005, SEL-006, SEL-007, SEL-008, SEL-009, SEL-010, SEL-011, SEL-012, SCH-001, SCH-002, SCH-003, SCH-004, SCH-005, SCH-006, SCH-007, DET-001, DET-002, DET-003, DET-004, DET-005, DET-006, DET-007, REG-001, REG-002, REG-003, REG-004, DMN-001, DMN-002, DMN-003, DMN-004, DMN-005, DMN-006, NTF-001, NTF-002, NTF-003, NTF-004, NTF-005, NTF-006, NTF-008, NTF-010, NTF-013, STS-001, STS-002, STS-005, DEP-001, DEP-002 |
| M2 | SCH-008, API-001, API-002, API-003, API-004, API-005, API-006, API-007, API-008, API-009, API-010, API-011, API-012, STS-003, STS-006, STS-007, DEP-011, POL-001, POL-002, POL-003, POL-006, POL-007, POL-008, POL-009, POL-011, SEL-013, DMN-003, DMN-005, DMN-007, DMN-008, DMN-009, DET-008, REG-005, REG-007, REG-008, NTF-007, NTF-009, NTF-011, NTF-014, CFG-012, REG-006 |
| M3 recreate | UPD-003, UPD-004, UPD-005, UPD-007, UPD-008, UPD-011, UPD-020 |

### M1 deviations

M1 reports and never changes containers. Until update execution exists:

- Arming (`--armed`, `PARACHUTE_ARMED=true`, `parachute.armed=true` on Parachute's own container, or the `convenient` profile without `--armed=false`) is a startup error, exit code 2, with the message `updates are not implemented in this milestone` (CFG-006, CFG-011). Unarmed, every applicable update is reported `would_update`.
- Settings that turn on a feature M1 does not have fail at startup with `<setting> is not implemented in this milestone`: `--enable-lifecycle-hooks`, `--notification-report=false`, `--notification-log-stdout`, `--http-api-update`, `--http-api-metrics`, `--http-api-status`, `--hosts`, `--restore-verify-schedule`, and the `restore-verify` command.
- Settings that only tune updates or policies (`--stop-timeout`, `--cleanup`, `--backup*`, `--verify-*`, `--delay-days`, `--protect-databases`, `--tag-follow`, `--maintenance-window*`, `--approval-reminder-days`, ...) are validated but have no effect. Policies are not evaluated, so no update is `held` except by monitor-only (SEL-007), and `held` entries are reported every cycle (NTF-011 is not implemented).
- Criteria written for an armed Parachute are tested unarmed where the observable is the same: CFG-007, SEL-008 and DET-004 expect `would_update` instead of `updated`; NTF-003 uses `failed` instead of `rolled_back`.
- STS-002 is produced by `parachute status --json`; `GET /v1/status` arrives with the HTTP API.
- `--allow-insecure-tcp` (`PARACHUTE_ALLOW_INSECURE_TCP`) is not implemented: M1 only warns about an insecure TCP host instead of refusing to start (DMN-003).
- The spec-hardening effort changed the meaning of these M1-covered requirements, so their M1 tests need updating in the next milestone: DMN-003, STS-002 (M2 updated SEL-003, SEL-004, SEL-007, SEL-008, SEL-011, SEL-012 and DET-004).

### M3 armed transactions (integration branch `milestone/3`)

Spine S0 lays shared foundations; parallel work lands in these packages:

| Package | Requirements |
|---|---|
| R recreate | UPD-003, UPD-004, UPD-005, UPD-007, UPD-008, UPD-011, UPD-020 |
| B snapshot | BKP-001–BKP-013 |
| V verify | VER-001–VER-007, VER-012 |
| G graph | DEP-003–DEP-010, UPD-013 |
| K rollback + recovery | VER-008–VER-011, UPD-009, UPD-010, UPD-014–UPD-017, UPD-019 |
| F failure policy | POL-010, POL-011 (release), API-007 (release), NTF-012, STS-004, STS-002 (`frozen`, `last_transaction`), POL-004, POL-005 |
| H second engine | DMN-010 |
| M1 rewrites | CFG-001, CFG-006, CFG-007, CFG-008, CFG-009, CFG-010, CFG-011, SCH-005, SEL-008, SEL-010, SEL-011, DET-001, DET-004, REG-001, NTF-003 |

`internal/state` depends on `internal/txn` (ID and state validation), never the reverse.

### Later milestones

M4 hooks and self-update; M5 restore verification and Podman. Each gets its row in the coverage table when its tests exist, because `make trace` fails for a listed requirement without a test.

### M2 deviations

- `parachute_restore_verification_last_success_timestamp_seconds` is emitted only once restore verification exists (M5), so its absence is not an M2 gap.

M2 is unarmed, like M1. Per requirement, its criteria are tested as follows:

- Criteria written for an armed Parachute whose observable is the same unarmed: tested in M2, asserting `would_update` where the criterion expects `updated`.
- API-005: tested in M2 with a registry that delays its responses instead of a pre-check hook.
- API-007, POL-011: the approval half in M2; the freeze-release half in M3.
- DET-008: the digest criteria in M2; the pull-by-digest criterion in M3.
- POL-001: its hooks criterion in M4; the others in M2.
- STS-002 (listed in M1): its document-shape, `hold`, and unarmed `armed` criteria are retested in M2; its `PARACHUTE_ARMED=true`, `last_transaction`, and `frozen` criteria in M3.
- Criteria meaningless unarmed are tested in M3: CFG-006's `PARACHUTE_ARMED=true` criterion (expects `updated`), DMN-005's stop-request and replacement-start criteria, UPD-019's paused-helper criterion, and BKP-010's armed startup criterion.

## Index

1. [Container selection](01-selection.md)
2. [Scheduling](02-scheduling.md)
3. [Update detection](03-update-detection.md)
4. [Update transaction](04-update-transaction.md)
5. [Dependencies and stacks](05-dependencies-and-stacks.md)
6. [Lifecycle hooks](06-lifecycle-hooks.md)
7. [Backup before update](07-backup.md)
8. [Verification and rollback](08-verification-and-rollback.md)
9. [Update policies](09-update-policies.md)
10. [Notifications](10-notifications.md)
11. [HTTP API and metrics](11-http-api-and-metrics.md)
12. [Registry authentication](12-registry-auth.md)
13. [Daemons, multiple hosts, and runtimes](13-daemon-and-runtimes.md)
14. [Status view](14-status-view.md)
15. [Restore verification](15-restore-verification.md)
16. [Configuration, output, and safety switches](16-configuration.md)

Also: [glossary](glossary.md), [open questions](open-questions.md), [ADRs](../adr/README.md).
