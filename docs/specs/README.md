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
- Tests are black-box ([ADR-0003](../adr/0003-black-box-tests-from-specs.md)): they start Parachute (binary or image) against a real Docker engine and a local `registry:2`, and assert only on containers and labels, volume contents, the Docker event stream, registry access logs, payloads received by a fake webhook, HTTP responses, logs, stdout, and exit codes. Fixture images (for example `localhost:5000/fixture/app`) are built and pushed locally; tests never use Docker Hub. The one exception is API-011's constant-time comparison, which a code-level unit test covers in addition to its black-box test (ADR-0015).
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
- `--allow-insecure-tcp` (`PARACHUTE_ALLOW_INSECURE_TCP`) is not implemented: M1 only warns about an insecure TCP host instead of refusing to start (DMN-003).
- The spec-hardening effort changed the meaning of these M1-covered requirements, so their M1 tests need updating in the next milestone: SEL-003, SEL-004, SEL-007, SEL-008, SEL-011, SEL-012, DET-004, DMN-003, STS-002.

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

## Requirement index

Every requirement ID with its title. IDs marked **new** were added by the spec-hardening effort; see [STATUS.md](STATUS.md) for changed and superseded ones.

### 01-selection

- [SEL-001](01-selection.md) Default selection
- [SEL-002](01-selection.md) Name filter
- [SEL-003](01-selection.md) Opt-out label
- [SEL-004](01-selection.md) Opt-in mode
- [SEL-005](01-selection.md) Exclude list
- [SEL-006](01-selection.md) Criteria combine with AND
- [SEL-007](01-selection.md) Monitor-only
- [SEL-008](01-selection.md) Label precedence
- [SEL-009](01-selection.md) Container states
- [SEL-010](01-selection.md) Stopped containers after update
- [SEL-011](01-selection.md) Scope
- [SEL-012](01-selection.md) Parachute's own containers are excluded
- [SEL-013](01-selection.md) Image-baked labels do not set behaviour (**new**)

### 02-scheduling

- [SCH-001](02-scheduling.md) Poll interval
- [SCH-002](02-scheduling.md) Cron schedule
- [SCH-003](02-scheduling.md) Interval and schedule are exclusive
- [SCH-004](02-scheduling.md) Time zone
- [SCH-005](02-scheduling.md) Run once
- [SCH-006](02-scheduling.md) No overlapping cycles
- [SCH-007](02-scheduling.md) Startup message
- [SCH-008](02-scheduling.md) API-triggered mode

### 03-update-detection

- [DET-001](03-update-detection.md) Image comparison
- [DET-002](03-update-detection.md) Digest check before pull
- [DET-003](03-update-detection.md) Digest request fallback and warnings
- [DET-004](03-update-detection.md) No-pull mode
- [DET-005](03-update-detection.md) Digest-pinned references are never moved
- [DET-006](03-update-detection.md) Images without a registry digest
- [DET-007](03-update-detection.md) Detection is reported even when not applied
- [DET-008](03-update-detection.md) The candidate digest is recorded and binds the update (**new**)

### 04-update-transaction

- [UPD-001](04-update-transaction.md) Phases and their order
- [UPD-002](04-update-transaction.md) Transaction ID
- [UPD-003](04-update-transaction.md) Recreate with the same configuration
- [UPD-004](04-update-transaction.md) Image defaults follow the new image
- [UPD-005](04-update-transaction.md) Bookkeeping labels
- [UPD-006](04-update-transaction.md) Graceful stop
- [UPD-007](04-update-transaction.md) Unreproducible configuration blocks the update
- [UPD-008](04-update-transaction.md) Previous container and image kept until commit
- [UPD-009](04-update-transaction.md) Commit
- [UPD-010](04-update-transaction.md) Image cleanup after commit
- [UPD-011](04-update-transaction.md) Anonymous volumes
- [UPD-012](04-update-transaction.md) No-restart
- [UPD-013](04-update-transaction.md) Rolling restart
- [UPD-014](04-update-transaction.md) Failure before the update phase changes nothing
- [UPD-015](04-update-transaction.md) Failure isolation
- [UPD-016](04-update-transaction.md) Interrupted transactions are resolved at startup
- [UPD-017](04-update-transaction.md) Never delete what Parachute did not create
- [UPD-018](04-update-transaction.md) Self-update
- [UPD-019](04-update-transaction.md) Transaction ceiling
- [UPD-020](04-update-transaction.md) Pull by the approved digest (**new**)

### 05-dependencies-and-stacks

- [DEP-001](05-dependencies-and-stacks.md) Stack identification
- [DEP-002](05-dependencies-and-stacks.md) Containers outside Compose
- [DEP-003](05-dependencies-and-stacks.md) One transaction per stack
- [DEP-004](05-dependencies-and-stacks.md) Dependency sources
- [DEP-005](05-dependencies-and-stacks.md) Stop and start order
- [DEP-006](05-dependencies-and-stacks.md) Only updated services and their dependents restart
- [DEP-007](05-dependencies-and-stacks.md) Network-mode dependents are recreated
- [DEP-008](05-dependencies-and-stacks.md) Dependency cycles
- [DEP-009](05-dependencies-and-stacks.md) Replicas of a service
- [DEP-010](05-dependencies-and-stacks.md) Recreate without Compose
- [DEP-011](05-dependencies-and-stacks.md) Drift detection

### 06-lifecycle-hooks

- [HOK-001](06-lifecycle-hooks.md) Hooks are off by default
- [HOK-002](06-lifecycle-hooks.md) Execution
- [HOK-003](06-lifecycle-hooks.md) Order
- [HOK-004](06-lifecycle-hooks.md) Held and monitor-only containers
- [HOK-005](06-lifecycle-hooks.md) Timeout
- [HOK-006](06-lifecycle-hooks.md) Exit codes
- [HOK-007](06-lifecycle-hooks.md) Failing pre-update aborts that container's transaction
- [HOK-008](06-lifecycle-hooks.md) Hook output and secrets (**new**)

### 07-backup

- [BKP-001](07-backup.md) No snapshot, no update
- [BKP-002](07-backup.md) Snapshot contents
- [BKP-003](07-backup.md) Backup modes
- [BKP-004](07-backup.md) Database dumps
- [BKP-005](07-backup.md) Snapshot storage
- [BKP-006](07-backup.md) Snapshot ID and manifest
- [BKP-007](07-backup.md) Snapshot validity
- [BKP-008](07-backup.md) Retention
- [BKP-009](07-backup.md) Partial snapshots are removed
- [BKP-010](07-backup.md) Helper containers
- [BKP-011](07-backup.md) Free space is checked before anything stops
- [BKP-012](07-backup.md) System mounts are never copied (**new**)
- [BKP-013](07-backup.md) Restores stay inside their target (**new**)

### 08-verification-and-rollback

- [VER-001](08-verification-and-rollback.md) Verification is required
- [VER-002](08-verification-and-rollback.md) Health check base
- [VER-003](08-verification-and-rollback.md) Stability base
- [VER-004](08-verification-and-rollback.md) Verify command
- [VER-005](08-verification-and-rollback.md) Verify URL
- [VER-006](08-verification-and-rollback.md) Deadline
- [VER-007](08-verification-and-rollback.md) The stack passes or fails as a whole
- [VER-008](08-verification-and-rollback.md) Rollback procedure
- [VER-009](08-verification-and-rollback.md) Data restore
- [VER-010](08-verification-and-rollback.md) Rollback verification and rollback failure
- [VER-011](08-verification-and-rollback.md) Rollback report
- [VER-012](08-verification-and-rollback.md) Replacements that are not started

### 09-update-policies

- [POL-001](09-update-policies.md) Holds and reasons
- [POL-002](09-update-policies.md) Delay after an image appears
- [POL-003](09-update-policies.md) Databases are protected
- [POL-004](09-update-policies.md) Tags are not changed by default
- [POL-005](09-update-policies.md) Tag following
- [POL-006](09-update-policies.md) Manual approval
- [POL-007](09-update-policies.md) Maintenance windows
- [POL-008](09-update-policies.md) Per-container overrides
- [POL-009](09-update-policies.md) Relaxations are logged
- [POL-010](09-update-policies.md) A failed digest is not retried
- [POL-011](09-update-policies.md) The state file never weakens a policy (**new**)

### 10-notifications

- [NTF-001](10-notifications.md) Service URLs
- [NTF-002](10-notifications.md) One digest per cycle
- [NTF-003](10-notifications.md) Entry levels and the notification level
- [NTF-004](10-notifications.md) Title
- [NTF-005](10-notifications.md) Delay
- [NTF-006](10-notifications.md) Templates
- [NTF-007](10-notifications.md) Simple mode
- [NTF-008](10-notifications.md) Empty output sends nothing
- [NTF-009](10-notifications.md) Logger target
- [NTF-010](10-notifications.md) Dry runs use the same path
- [NTF-011](10-notifications.md) Approval reminders
- [NTF-012](10-notifications.md) Rollback failures are always sent
- [NTF-013](10-notifications.md) Per-provider notification options are not supported
- [NTF-014](10-notifications.md) Notifications are redacted (**new**)

### 11-http-api-and-metrics

- [API-001](11-http-api-and-metrics.md) Server
- [API-002](11-http-api-and-metrics.md) Authentication
- [API-003](11-http-api-and-metrics.md) A token is required
- [API-004](11-http-api-and-metrics.md) Trigger a cycle
- [API-005](11-http-api-and-metrics.md) Overlapping requests
- [API-006](11-http-api-and-metrics.md) Periodic polling alongside the API
- [API-007](11-http-api-and-metrics.md) Approve a pending update
- [API-008](11-http-api-and-metrics.md) Metrics
- [API-009](11-http-api-and-metrics.md) Request limits (**new**)
- [API-010](11-http-api-and-metrics.md) Failed-authentication throttling (**new**)
- [API-011](11-http-api-and-metrics.md) Constant-time token comparison (**new**)
- [API-012](11-http-api-and-metrics.md) Responses are redacted (**new**)

### 12-registry-auth

- [REG-001](12-registry-auth.md) Credentials from a Docker config file
- [REG-002](12-registry-auth.md) The file is re-read every cycle
- [REG-003](12-registry-auth.md) Docker Hub keys
- [REG-004](12-registry-auth.md) Exact host matching
- [REG-005](12-registry-auth.md) Credential helpers are not supported
- [REG-006](12-registry-auth.md) Credentials are never logged
- [REG-007](12-registry-auth.md) Registry responses are size-limited and validated (**new**)
- [REG-008](12-registry-auth.md) Credentials go only where they belong (**new**)

### 13-daemon-and-runtimes

- [DMN-001](13-daemon-and-runtimes.md) Endpoint
- [DMN-002](13-daemon-and-runtimes.md) API version negotiation
- [DMN-003](13-daemon-and-runtimes.md) TLS
- [DMN-004](13-daemon-and-runtimes.md) Unreachable at startup
- [DMN-005](13-daemon-and-runtimes.md) Connection loss while running
- [DMN-006](13-daemon-and-runtimes.md) Health check command
- [DMN-007](13-daemon-and-runtimes.md) Host list
- [DMN-008](13-daemon-and-runtimes.md) Per-host TLS
- [DMN-009](13-daemon-and-runtimes.md) Hosts are isolated
- [DMN-010](13-daemon-and-runtimes.md) Per-host results
- [DMN-011](13-daemon-and-runtimes.md) Podman's Docker-compatible API
- [DMN-012](13-daemon-and-runtimes.md) Podman pods
- [DMN-013](13-daemon-and-runtimes.md) Podman auto-update labels
- [DMN-014](13-daemon-and-runtimes.md) Rootless storage

### 14-status-view

- [STS-001](14-status-view.md) Command line
- [STS-002](14-status-view.md) JSON document
- [STS-003](14-status-view.md) HTML page
- [STS-004](14-status-view.md) What is shown
- [STS-005](14-status-view.md) Read-only
- [STS-006](14-status-view.md) The state file is not trusted (**new**)
- [STS-007](14-status-view.md) The status view is redacted (**new**)

### 15-restore-verification

- [RST-001](15-restore-verification.md) Schedule
- [RST-002](15-restore-verification.md) Throwaway resources
- [RST-003](15-restore-verification.md) Procedure and result
- [RST-004](15-restore-verification.md) Live containers are never touched
- [RST-005](15-restore-verification.md) Cleanup
- [RST-006](15-restore-verification.md) Reporting
- [RST-007](15-restore-verification.md) No overlap with cycles

### 16-configuration

- [CFG-001](16-configuration.md) Names
- [CFG-002](16-configuration.md) Precedence
- [CFG-003](16-configuration.md) Value syntax
- [CFG-004](16-configuration.md) Secrets from files
- [CFG-005](16-configuration.md) Unknown and removed settings fail
- [CFG-006](16-configuration.md) Armed mode
- [CFG-007](16-configuration.md) Result states and porcelain output
- [CFG-008](16-configuration.md) Logging
- [CFG-009](16-configuration.md) Exit codes
- [CFG-010](16-configuration.md) State file
- [CFG-011](16-configuration.md) Profiles
- [CFG-012](16-configuration.md) Logs, errors, and porcelain output are redacted (**new**)

Also: [glossary](glossary.md), [open questions](open-questions.md), [ADRs](../adr/README.md).
