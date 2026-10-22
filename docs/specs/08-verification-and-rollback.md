# 08 Verification and rollback

**Status:** Draft

After an update, Parachute proves the new containers work, or puts the old ones back.

### VER-001 Verification is required
Every replacement that was started in the `update` phase MUST pass verification before the transaction commits. Verification passes when the base check (VER-002 or VER-003) and every configured extra check (VER-004, VER-005) pass within the deadline (VER-006). Checks run in that order.

- **Given** a new image whose process exits 2 seconds after start **When** it is updated **Then** the transaction rolls back and the result is `rolled_back` with reason `verify_failed`.

### VER-002 Health check base
When the replacement has a Docker health check (from its image or configuration), the base check MUST pass when its health status becomes `healthy` and MUST fail immediately when it becomes `unhealthy`, or when the container stops.

- **Given** a new image whose health check succeeds after 5 seconds **When** it is updated **Then** the transaction commits after the `healthy` event.
- **Given** a new image whose health check always fails **When** it is updated **Then** the transaction rolls back as soon as the status is `unhealthy`, before the deadline, with reason `verify_failed`.

### VER-003 Stability base
When the replacement has no health check, the base check MUST pass when the container has been `running` continuously, with an unchanged restart count, for `--verify-stable` (default `30s`; label `parachute.verify.stable`). It fails when the container stops or restarts.

- **Given** a new image without a health check that keeps running **When** it is updated **Then** the commit line is logged between 30 and 40 seconds after the replacement started.
- **Given** a new image without a health check that exits after 10 seconds, with restart policy `always` **When** it is updated **Then** the transaction rolls back.

### VER-004 Verify command
A container label `parachute.verify.cmd` MUST run as `sh -c "<value>"` inside the replacement after the base check passes, retried every 5 seconds until it exits 0 (pass) or the deadline expires (fail).

- **Given** `parachute.verify.cmd="test -f /ready"` and a new image that creates `/ready` 10 seconds after start **When** it is updated **Then** the transaction commits.
- **Given** `parachute.verify.cmd="exit 1"` **When** it is updated **Then** the transaction rolls back.

### VER-005 Verify URL
A container label `parachute.verify.url` MUST be requested with HTTP `GET` from a helper container (BKP-010) that shares the replacement's network namespace, so `localhost` refers to the replacement. A status from 200 to 399 passes; other statuses or connection errors are retried every 5 seconds until the deadline.

- **Given** `parachute.verify.url=http://localhost:8080/health` and a new image serving 200 there **When** it is updated **Then** the transaction commits.
- **Given** the same with a new image serving 500 **When** it is updated **Then** the transaction rolls back and the report includes status `500`.

### VER-006 Deadline
All checks of one replacement MUST complete within its **effective deadline**. The effective deadline is `--verify-timeout` (default `120s`; label `parachute.verify.timeout`), except for a replacement that has a Docker health check, where it is the larger of that value and `start_period + interval × (retries + 1)` of the replacement's health check (unset fields take Docker's defaults: `start_period` `0s`, `interval` `30s`, `retries` `3`), so a slow but correctly declared health check is not cut off. The deadline is counted from the replacement's start, or, when a `post-update` hook runs in it (HOK-002), from when that hook returns or times out (HOK-005). A check that has not passed when the deadline expires fails, and the transaction is rolled back with reason `verify_timeout` (a check that fails before the deadline gives `verify_failed`). The transaction ceiling (UPD-019) still applies.

- **Given** `parachute.verify.timeout=20s` and a new image with health check `--interval 2s --retries 3` that stays `starting` **When** it is updated **Then** the transaction rolls back between 20 and 25 seconds after the replacement started, with reason `verify_timeout`.
- **Given** default verification settings and a new image whose health check has `start_period` `200s` and turns `healthy` 180 seconds after start **When** it is updated **Then** the transaction commits.
- **Given** default verification settings and a new image whose health check never completes (its command sleeps longer than the deadline, with health check `--timeout 1h` and default interval and retries, so the status stays `starting`) **When** it is updated **Then** the transaction rolls back between 120 and 125 seconds after the replacement started, the result is `rolled_back` with reason `verify_timeout`, and the report entry names `parachute.verify.timeout`.
- **Given** hooks enabled, `parachute.lifecycle.post-update="sleep 30 && touch /ready"`, `parachute.verify.cmd="test -f /ready"`, `parachute.verify.timeout=20s`, `parachute.verify.stable=5s`, and a new image without a health check that keeps running **When** it is updated **Then** the transaction commits (the deadline started when the hook returned, not at the replacement's start).

### VER-007 The stack passes or fails as a whole
A transaction MUST commit only if every started replacement in it passes verification. If any fails, all containers changed by the transaction MUST be rolled back.

- **Given** a stack transaction updating `web` and `api` where only `api` fails verification **When** the cycle runs **Then** both `web` and `api` run their previous images with their original container IDs.

### VER-008 Rollback procedure
Rollback (transaction state `rolling_back`, [glossary: Transaction state machine](glossary.md#transaction-state-machine)) MUST: stop and remove every replacement created by the transaction, identified only as UPD-017 allows; restore data (VER-009); rename each original container back to its original name; start the originals that were running before, dependencies first; and verify them (VER-010).

- **Given** a failed verification **When** rollback finishes **Then** each original container has its original name, ID, and image and is `running`, and no replacement container from that transaction exists.

### VER-009 Data restore
When at least one replacement in the transaction was started (and so may have written or migrated data), rollback MUST restore every mount in the snapshot to the snapshot's content, replacing what is there. When no replacement was started, data MUST NOT be restored.

- **Given** a new image that writes `migrated` into `/data/version` on start and then fails verification **When** rollback finishes **Then** `/data/version` has its pre-update content.
- **Given** a replacement that could not be created **When** rollback finishes **Then** the volume's content was not rewritten (its files keep their modification times).

### VER-010 Rollback verification and rollback failure
After rollback, the restored originals MUST pass the base check (VER-002 or VER-003) within the deadline. If any original cannot be restored, does not pass, or data cannot be restored, the result MUST be `rollback_failed`, Parachute MUST send an `error` notification regardless of the notification level (NTF-012), log at `error`, and exit with code 3 in run-once mode. The transaction is left as it is for the operator; Parachute MUST NOT retry it automatically.

- **Given** a failed verification and the original image removed by the test before rollback **When** rollback runs **Then** the result is `rollback_failed`, the webhook receives an error notification with the transaction ID, and with `--run-once` the exit code is 3.

### VER-011 Rollback report
The report entry for a rolled-back transaction MUST include: the transaction ID, the reason (`verify_failed`, `verify_timeout`, `transaction_timeout`, or `interrupted`), the failed check and its last output (health check log, command output, or HTTP status; truncated to 1000 characters), whether data was restored, and the snapshot ID. For reason `verify_timeout`, the entry and the notification MUST also state the effective deadline that expired and suggest setting the label `parachute.verify.timeout` for a slow-starting container.

- **Given** a new image whose health check prints `db connection refused` and fails **When** the cycle reports **Then** the entry contains `rolled_back`, `verify_failed`, the transaction ID, `db connection refused`, and `data restored: yes`.
- **Given** a rollback with reason `verify_timeout` **When** the cycle reports **Then** the webhook's notification contains `verify_timeout`, the expired deadline (for example `120s`), and `parachute.verify.timeout`.

### VER-012 Replacements that are not started
A replacement that is intentionally not started (UPD-012 no-restart, or SEL-010 stopped container without revive) MUST pass verification when it has been created and its configuration check (UPD-007) passed.

- **Given** `--no-restart` and an update **When** the cycle runs **Then** the transaction commits without waiting for a health check.
