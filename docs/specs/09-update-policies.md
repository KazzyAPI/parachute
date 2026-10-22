# 09 Update policies

**Status:** Draft

Rules that decide whether a detected update is applied now ([ADR-0007](../adr/0007-safe-by-default.md)). A policy that stops an update creates a **hold**. Holds are evaluated in the `policy` phase and never change containers.

### POL-001 Holds and reasons
A held update MUST get result `held` with exactly one reason code, the first that applies in this order: `monitor_only` (SEL-007), `approval_required` (POL-003, POL-006), `previous_attempt_failed` (POL-010), `delay` (POL-002), `maintenance_window` (POL-007), `hook_deferred` (HOK-006). `hook_deferred` is last because it is set in the `snapshot` phase by a `pre-update` hook, which runs only for an update that no policy holds; it is the one reason not evaluated in the `policy` phase. The report entry MUST state when the hold ends where that is known (`delay`: the time; `maintenance_window`: the next window start). A held update is not counted as failed.

- **Given** a database container with an update, inside the delay period, outside the maintenance window **When** a cycle runs **Then** the result is `held` with reason `approval_required`.
- **Given** an update held by delay **When** a cycle runs **Then** the report entry contains `delay` and an RFC 3339 end time.
- **Given** hooks enabled, a `pre-update` hook that exits 75, and an update inside the delay period **When** a cycle runs **Then** the result is `held` with reason `delay` and the hook did not run.

### POL-002 Delay after an image appears
An update MUST NOT be applied until the candidate digest has been seen by Parachute for at least `--delay-days` days (a non-negative decimal number; default `2`; label `parachute.delay-days`; `0` disables). The first-seen time per container and candidate digest is kept in the state file and survives restarts. The image's own `created` time MUST NOT be used, because the image author sets it. When the candidate digest changes, the delay restarts for the new digest.

- **Given** `--delay-days 0.0005` (43.2 seconds), `--interval 30`, and a newer image pushed just before startup **When** Parachute runs for 70 seconds **Then** the first cycle's result is `held` with reason `delay` and the cycle at 60 seconds updates the container.
- **Given** the same, but a second newer image pushed between the two cycles **When** the second cycle runs **Then** the result is again `held` with reason `delay`.
- **Given** `parachute.delay-days=0` on a container **When** a newer image appears **Then** it is updated in the same cycle.
- **Given** `--delay-days 0.0005` and a newer image whose configuration `created` time is `1970-01-01T00:00:00Z`, pushed just before startup **When** the first cycle runs **Then** the result is `held` with reason `delay` and an end time 43.2 seconds after Parachute first saw the digest (the image's `created` time is not used).

### POL-003 Databases are protected
With `--protect-databases` on (default), a container recognised as a database MUST NOT be updated without approval (POL-006); its update is held with reason `approval_required`. A container is recognised when the last path component of its image repository is one of: `postgres`, `postgresql`, `postgis`, `timescaledb`, `mysql`, `mariadb`, `percona-server`, `mongo`, `mongodb`, `redis`, `valkey`, `influxdb`, `elasticsearch`, `opensearch`, `cassandra`, `couchdb`, `clickhouse-server`, `neo4j`. A label `parachute.database=true` or `=false` MUST override recognition.

- **Given** a `localhost:5000/library/postgres:16` container with a newer image **When** a cycle runs **Then** it is `held` with reason `approval_required` and unchanged.
- **Given** the same with `parachute.database=false` **When** a cycle runs **Then** it is updated.
- **Given** a container from `localhost:5000/fixture/app` labelled `parachute.database=true` **When** a newer image appears **Then** it is held with reason `approval_required`.

### POL-004 Tags are not changed by default
Parachute MUST compare each container only against its own tag (DET-001) and MUST NOT change a container's image reference, unless tag following is on (POL-005). A rebuilt image on the same tag is an ordinary update.

- **Given** a container on `fixture/app:1.2.0`, a newer image on that same tag, and tag `1.3.0` in the registry **When** a cycle runs **Then** the replacement's reference is `fixture/app:1.2.0` and its image is the rebuilt one.

### POL-005 Tag following
With `parachute.tag-follow` (or `--tag-follow`) set to `patch` or `minor`, for a container whose tag is a version `[v]MAJOR.MINOR.PATCH` (all numeric), Parachute MUST move the container to the highest tag in the registry with the same major (`minor`) or the same major and minor (`patch`). Tags with any other form are ignored. Default is `none`. A tag move is an update like any other (policies, snapshot, verification apply) and the replacement's `parachute.previous-image` records the old reference.

- **Given** `parachute.tag-follow=patch`, container on `app:1.2.0`, registry tags `1.2.3`, `1.3.0`, `2.0.0`, `1.2.4-rc1` **When** a cycle runs **Then** the replacement uses `app:1.2.3`.
- **Given** `parachute.tag-follow=minor` and the same tags **When** a cycle runs **Then** the replacement uses `app:1.3.0`.
- **Given** a container on `app:latest` with `parachute.tag-follow=minor` **When** a cycle runs **Then** only same-tag updates are applied.

### POL-006 Manual approval
A container labelled `parachute.approval=required` MUST be held with reason `approval_required` like a database; `parachute.approval=auto` MUST waive approval, including for databases. An operator approves the pending update with `POST /v1/approve` (API-007). An approval applies to the candidate digest pending at the time; if the candidate digest changes before the update is applied, the approval lapses and the update is held again. An approval clears `approval_required` and `previous_attempt_failed` for that digest; the other holds (`delay`, `maintenance_window`) still apply.

- **Given** a held database update and a successful approval call **When** the next cycle runs **Then** the container is updated.
- **Given** an approval and then a newer image pushed before the next cycle **When** the next cycle runs **Then** the result is `held` with reason `approval_required`.

### POL-007 Maintenance windows
With `--maintenance-window <cron>` (6-field, SCH-002 syntax) and `--maintenance-window-duration` (default `2h`), an update MUST start its `snapshot` phase only while a window is open: from a cron match until the duration has passed. Labels `parachute.maintenance-window` and `parachute.maintenance-window-duration` override per container. Without a window, updates may start at any time. A transaction that has started finishes even if the window closes.

- **Given** a window of `1h` whose cron expression matches only the time 12 hours after the test starts **When** a cycle runs with an update available **Then** it is `held` with reason `maintenance_window` and the report states that next window start.
- **Given** a window of `1h` whose cron expression matches only the time 10 minutes before the test starts **When** a cycle runs **Then** the update is applied.

### POL-008 Per-container overrides
Every policy setting that has a label MUST be overridable per container by that label, and the label MUST win over the global setting. A label with an invalid value MUST make that container's result `failed` with reason `invalid_label` naming the label, and other containers are unaffected.

- **Given** `--delay-days 7` and a container labelled `parachute.delay-days=0` **When** a newer image appears **Then** that container is updated in the same cycle.
- **Given** `parachute.delay-days=soon` **When** a cycle runs **Then** that container is `failed` with reason `invalid_label` mentioning `parachute.delay-days`.

### POL-009 Relaxations are logged
Each relaxation of a safe default MUST be logged at `warn` once at startup (global settings) and once when a container carrying it is first seen (labels), and listed in the startup message (SCH-007). Relaxations are: `--backup none` or `parachute.backup=none`; `parachute.database=false` on a recognised image; `parachute.approval=auto`; `--delay-days 0` or `parachute.delay-days=0`; `--tag-follow` or `parachute.tag-follow` other than `none`; `--protect-databases=false`.

- **Given** `PARACHUTE_BACKUP=none` **When** Parachute starts **Then** a `warn` line names `backup` and `none`, and the startup message lists it.
- **Given** a container labelled `parachute.approval=auto` **When** it is first seen **Then** one `warn` line names the container and the label; a second cycle logs no new line for it.

### POL-010 A failed digest is not retried
After a transaction for a candidate digest ends `rolled_back`, Parachute MUST hold updates of that container to that digest with reason `previous_attempt_failed` until a different candidate digest appears or the update is approved (API-007). After a transaction ends `rollback_failed`, the stack is **frozen** ([glossary: Transaction state machine](glossary.md#transaction-state-machine)): every update of every container in that stack MUST be held with reason `previous_attempt_failed`, whatever the candidate digest, until an operator approves the failed container's update (API-007); a new candidate digest does not end the freeze.

- **Given** an update that rolled back **When** the next cycle runs with the same registry state **Then** the container is `held` with reason `previous_attempt_failed` and unchanged.
- **Given** the same and then a newer image pushed **When** the next cycle runs **Then** the update to the newer image is attempted.
- **Given** a stack `app` + `web` whose transaction for `app` ended `rollback_failed`, and then newer images pushed for both **When** the next cycle runs **Then** both are `held` with reason `previous_attempt_failed`, no container in the stack changed, and `parachute status` shows the stack as frozen with the `rollback_failed` transaction ID.
- **Given** the same and then `POST /v1/approve?container=app` **When** the next cycle runs **Then** the updates of the stack are attempted.

### POL-011 The state file never weakens a policy
The state file is untrusted ([glossary: Trust boundaries](glossary.md#trust-boundaries)). Each approval MUST be stored with the host, container ID, and candidate digest it was given for, and MUST be applied only when all three equal the held update; otherwise it is discarded and logged at `info`. Nothing read from the state file MUST arm Parachute (CFG-006), approve an update, or end a hold except an approval stored this way.

- **Given** an approval for `db` and then Parachute stopped, `db` removed and created again with the same name, image, and labels (a new container ID), and Parachute started **When** the next cycle runs **Then** `db` is `held` with reason `approval_required` and unchanged.
