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

`UPD-018` lives in [17-self-update.md](17-self-update.md), `RST` in [15-restore-verification.md](15-restore-verification.md), and `DMN-011`, `DMN-012` and `DMN-014` in [18-podman.md](18-podman.md); the IDs did not change when those moved.

## Keywords

MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are used as defined in RFC 2119. Names in `code` (flags, labels, states, reason codes, metric names) are exact and are part of the interface. Defaults stated in a requirement are normative. "Logged at `warn`" means a log line at that level whose message contains the named items.

## Tests

- Each acceptance criterion is one test, a Go test function in `e2e/*_test.go` named `Test`, the requirement ID with `_` for `-`, and a short description: `TestSEL_001_default_selects_all_running` ([ADR-0015](../adr/0015-traceability-and-test-practice.md)).
- Every `MUST` and `SHOULD` requirement has at least one test. `make trace` fails when a milestone lists an ID that is in no spec, or a `MUST` or `SHOULD` requirement listed for a milestone, or in a file with status `Accepted`, has no test.
- Tests are black-box ([ADR-0003](../adr/0003-black-box-tests-from-specs.md)): they start Parachute (binary or image) against a real Docker engine and a local `registry:2`, and assert only on containers and labels, volume contents, the Docker event stream, registry access logs, payloads received by a fake webhook, HTTP responses, logs, stdout, and exit codes. Fixture images (for example `localhost:5000/fixture/app`) are built and pushed locally. Official images used as test infrastructure (`registry:2`, `busybox`, `postgres`, `mysql`, `docker`) MAY be pulled from Docker Hub before the tests and re-pushed to the local registry; tests never depend on Docker Hub for the images under test. The one exception is API-011's constant-time comparison, which a code-level unit test covers in addition to its black-box test (ADR-0015).
- Unless a criterion says otherwise, it assumes Parachute is armed (CFG-006), lifecycle hooks are off, `--delay-days 0`, `--protect-databases` on, and one host named `local`.
- Times stated in criteria have a tolerance of ±5 seconds unless stated.

## Milestone coverage

Requirements that are built and tested. `make trace` fails when a `MUST` or `SHOULD` listed here has no test, and when a file with status `Accepted` has a `MUST` or `SHOULD` without one.

| Milestone | Implemented requirements |
|---|---|
| M1 report-only core | CFG-001, CFG-002, CFG-003, CFG-004, CFG-005, CFG-006, CFG-007, CFG-008, CFG-009, CFG-010, CFG-011, SEL-001, SEL-002, SEL-003, SEL-004, SEL-005, SEL-006, SEL-007, SEL-008, SEL-009, SEL-010, SEL-011, SEL-012, SCH-001, SCH-002, SCH-003, SCH-004, SCH-005, SCH-006, SCH-007, DET-001, DET-002, DET-003, DET-004, DET-005, DET-006, DET-007, REG-001, REG-002, REG-003, REG-004, DMN-001, DMN-002, DMN-003, DMN-004, DMN-005, DMN-006, NTF-001, NTF-002, NTF-003, NTF-004, NTF-005, NTF-006, NTF-008, NTF-010, NTF-013, STS-001, STS-002, STS-005, DEP-001, DEP-002 |
| M2 | SCH-008, API-001, API-002, API-003, API-004, API-005, API-006, API-007, API-008, API-009, API-010, API-011, API-012, STS-003, STS-006, STS-007, DEP-011, POL-001, POL-002, POL-003, POL-006, POL-007, POL-008, POL-009, POL-011, SEL-013, DMN-003, DMN-005, DMN-007, DMN-008, DMN-009, DET-008, REG-005, REG-007, REG-008, NTF-007, NTF-009, NTF-011, NTF-014, CFG-012, REG-006 |
| M3 | BKP-001, BKP-002, BKP-003, BKP-004, BKP-005, BKP-006, BKP-007, BKP-008, BKP-009, BKP-010, BKP-011, BKP-012, BKP-013, DMN-010, VER-001, VER-002, VER-003, VER-004, VER-005, VER-006, VER-007, VER-008, VER-009, VER-010, VER-011, VER-012, UPD-003, UPD-004, UPD-005, UPD-007, UPD-008, UPD-009, UPD-010, UPD-011, UPD-014, UPD-015, UPD-016, UPD-017, UPD-019, UPD-020, DEP-003, DEP-004, DEP-005, DEP-006, DEP-007, DEP-008, DEP-009, DEP-010, UPD-013, UPD-001, UPD-002, UPD-006, UPD-012 |
| M3 policy | POL-004, POL-005, POL-010, POL-011, API-007, NTF-012, STS-002, STS-004 |
| M4 hooks and onboarding | HOK-001, HOK-002, HOK-003, HOK-004, HOK-005, HOK-006, HOK-007, HOK-008, CFG-013, DMN-013 |
| M5 restore verification | RST-001, RST-002, RST-003, RST-004, RST-005 |
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
17. [Self-update](17-self-update.md)
18. [Podman](18-podman.md)

Also: [glossary](glossary.md), [open questions](open-questions.md), [ADRs](../adr/README.md).
