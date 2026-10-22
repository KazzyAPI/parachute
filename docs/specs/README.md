# Specs

Parachute updates Docker containers safely: it snapshots before an update, verifies the result, and rolls back automatically when the update fails. It understands Compose stacks and can manage several hosts.

These files describe Parachute's **observable behaviour**: what an operator sees in containers, labels, volumes, notifications, HTTP responses, logs, and exit codes. They do not describe code structure. Tests are written from these files ([ADR-0001](../adr/0001-spec-driven-development.md)); if a test and a spec disagree, the spec wins until the spec is changed. Ambiguity in a spec is a defect: report it, then fix the spec.

## Requirement format

```
### ABC-001 Example title
Parachute MUST ...

- **Given** ... **When** ... **Then** ...
```

- Each requirement has an ID, a title, one normative statement, and one or more acceptance criteria in Given/When/Then form.
- Each acceptance criterion is checkable from outside the program against a real engine and a local registry.
- Each file carries one status at the top: `Draft` (not yet reviewed), `Accepted`, or `Deferred` (a section whose tests may fail until a later milestone).
- Unresolved decisions do not go into requirements; they go into [open-questions.md](open-questions.md).

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

- Each acceptance criterion is one test. The test name starts with the requirement ID, then a short description: `SEL-001_default_selects_all_running`. Go test names cannot contain `-`, so a Go test writes the ID with `_` after the `Test` prefix (`TestSEL_001_default_selects_all_running`); the traceability check reads both forms.
- Every `MUST` and `SHOULD` requirement has at least one test. In a file with status `Accepted`, a `MUST` requirement without a test fails CI.
- Tests are black-box ([ADR-0003](../adr/0003-black-box-tests-from-specs.md)): they start Parachute (binary or image) against a real Docker engine and a local `registry:2`, and assert only on containers and labels, volume contents, the Docker event stream, registry access logs, payloads received by a fake webhook, HTTP responses, logs, stdout, and exit codes. Fixture images (for example `localhost:5000/fixture/app`) are built and pushed locally; tests never use Docker Hub.
- Unless a criterion says otherwise, it assumes Parachute is armed (CFG-006), lifecycle hooks are off, `--delay-days 0`, `--protect-databases` on, and one host named `local`.
- Times stated in criteria have a tolerance of ±5 seconds unless stated.

## Milestone coverage

Requirements implemented by each milestone. `make trace` fails when a `MUST` or `SHOULD` listed here has no test. Acceptance criteria that need update execution (an armed Parachute, UPD, BKP, VER, POL, HOK, API) are tested in the milestone that adds it.

| Milestone | Implemented requirements |
|---|---|
| M1 report-only core | CFG-001, CFG-002, CFG-003, CFG-004, CFG-005, CFG-006, CFG-007, CFG-008, CFG-009, CFG-010, CFG-011, SEL-001, SEL-002, SEL-003, SEL-004, SEL-005, SEL-006, SEL-007, SEL-008, SEL-009, SEL-010, SEL-011, SEL-012, SCH-001, SCH-002, SCH-003, SCH-004, SCH-005, SCH-006, SCH-007, DET-001, DET-002, DET-003, DET-004, DET-005, DET-006, DET-007, REG-001, REG-002, REG-003, REG-004, DMN-001, DMN-002, DMN-003, DMN-004, DMN-005, DMN-006, NTF-001, NTF-002, NTF-003, NTF-004, NTF-005, NTF-006, NTF-008, NTF-010, NTF-013, STS-001, STS-002, STS-005, DEP-001, DEP-002 |

### M1 deviations

M1 reports and never changes containers. Until update execution exists:

- Arming (`--armed`, `PARACHUTE_ARMED=true`, `parachute.armed=true` on Parachute's own container, or the `convenient` profile without `--armed=false`) is a startup error, exit code 2, with the message `updates are not implemented in this milestone` (CFG-006, CFG-011). Unarmed, every applicable update is reported `would_update`.
- Settings that turn on a feature M1 does not have fail at startup with `<setting> is not implemented in this milestone`: `--enable-lifecycle-hooks`, `--notification-report=false`, `--notification-log-stdout`, `--http-api-update`, `--http-api-metrics`, `--http-api-status`, `--hosts`, `--restore-verify-schedule`, and the `restore-verify` command.
- Settings that only tune updates or policies (`--stop-timeout`, `--cleanup`, `--backup*`, `--verify-*`, `--delay-days`, `--protect-databases`, `--tag-follow`, `--maintenance-window*`, `--approval-reminder-days`, ...) are validated but have no effect. Policies are not evaluated, so no update is `held` except by monitor-only (SEL-007), and `held` entries are reported every cycle (NTF-011 is not implemented).
- Criteria written for an armed Parachute are tested unarmed where the observable is the same: CFG-007, SEL-008 and DET-004 expect `would_update` instead of `updated`; NTF-003 uses `failed` instead of `rolled_back`.
- STS-002 is produced by `parachute status --json`; `GET /v1/status` arrives with the HTTP API.

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
