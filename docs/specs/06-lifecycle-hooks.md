# 06 Lifecycle hooks

**Status:** Draft

Operator-defined commands that run inside monitored containers around checks and updates.

### HOK-001 Hooks are off by default
Hooks MUST NOT run unless `--enable-lifecycle-hooks` is on. When on, Parachute MUST read these labels from each candidate: `parachute.lifecycle.pre-check`, `parachute.lifecycle.pre-update`, `parachute.lifecycle.post-update`, `parachute.lifecycle.post-check`.

- **Given** hooks disabled and a container with a `pre-update` hook that writes a file **When** the container is updated **Then** the file does not exist.
- **Given** hooks enabled and the same container **When** it is updated **Then** the file exists.

### HOK-002 Execution
Each hook MUST run inside the container as `sh -c "<label value>"`. `pre-check` and `pre-update` run in the original container; `post-update` runs in the replacement after it starts; `post-check` runs in whichever container exists after commit or rollback. A hook whose target container is not running MUST be skipped and logged at `debug`, and processing continues.

- **Given** a stopped candidate (with `--include-stopped`) and a `pre-check` hook **When** a cycle runs **Then** the hook does not run and the container is still checked.

### HOK-003 Order
For a container that is updated, the hooks MUST run in the order `pre-check`, `pre-update`, `post-update`, `post-check`. For a container whose transaction does not reach the `snapshot` phase (no update, or held by a policy), only `pre-check` and `post-check` run. For a container whose transaction ends in the `snapshot` phase (deferred by HOK-006 or failed before the `update` phase), `pre-update` may have run and `post-update` MUST NOT run.

- **Given** all four hooks appending their name to a file on a shared volume **When** the container is updated **Then** the file reads `pre-check pre-update post-update post-check`.
- **Given** the same and no update available **When** a cycle runs **Then** the file reads `pre-check post-check`.

### HOK-004 Held and monitor-only containers
For a container whose update is held (POL-001), including monitor-only and unarmed, `pre-check` and `post-check` MUST run and `pre-update` and `post-update` MUST NOT run.

- **Given** a monitor-only container with all four hooks and an update available **When** a cycle runs **Then** the file reads `pre-check post-check`.

### HOK-005 Timeout
Every hook MUST time out after 60 seconds by default. The labels `parachute.lifecycle.pre-update-timeout` and `parachute.lifecycle.post-update-timeout` (whole minutes) MUST override the timeout for those hooks; `0` disables the hook's own timeout, but the hook is still bounded by the transaction ceiling (UPD-019, reason `transaction_timeout`). A `post-update` hook's run time does not count against the verification deadline (VER-006). A timeout MUST be logged at `warn` with the hook name. A timed-out `pre-update` hook counts as a failure (HOK-007); any other timed-out hook lets processing continue.

- **Given** a `pre-check` hook that sleeps 90 seconds **When** a cycle runs **Then** the check continues about 60 seconds after the hook started and a `warn` line names `pre-check`.
- **Given** a `pre-update` hook that sleeps 90 seconds with `pre-update-timeout=2` **When** the container is updated **Then** the update succeeds.
- **Given** `--transaction-timeout 2m` and a `pre-update` hook that never exits with `pre-update-timeout=0` **When** a cycle runs **Then** the transaction ends within 2.5 minutes with result `failed` and reason `transaction_timeout`.

### HOK-006 Exit codes
Exit code `0` MUST count as success. Exit code `75` (`EX_TEMPFAIL`) from a `pre-update` hook MUST **defer** the transaction: no further `pre-update` hook or dump runs, no container is stopped, the transaction ends `aborted` ([glossary: Transaction state machine](glossary.md#transaction-state-machine)), and every affected container of the transaction gets result `held` with reason `hook_deferred` (the last reason in the POL-001 order). A deferral is not an error: the exit code and `failed` counts are unaffected, and `post-update` does not run while `post-check` does (HOK-003). Exit code `75` from `pre-check`, `post-update`, or `post-check` MUST count as success. Any other exit code is a failure and MUST be logged at `warn` with the hook name and code. A failing `pre-check`, `post-update`, or `post-check` hook MUST NOT change the outcome of the update. The `pre-update` hooks of a transaction run after the free-space check (BKP-011) and before any dump (BKP-004) or stop.

- **Given** a `post-update` hook that exits 1 **When** the container is updated **Then** the update commits and a `warn` line names `post-update` and code `1`.
- **Given** a `pre-update` hook that exits 75 **When** a cycle runs **Then** the container is still running with its original ID, its result is `held` with reason `hook_deferred`, and the run-once exit code is 0.
- **Given** the same hook that exits 0 on the next cycle **When** that cycle runs **Then** the update proceeds.
- **Given** a `post-check` hook that exits 75 **When** a cycle runs **Then** the outcome is unchanged and no `warn` line is logged.
- **Given** a stack `app` + `db` both updated in one transaction, a `pre-update` hook on `app` that exits 75, and all four hooks on both appending their name to a file **When** a cycle runs **Then** both containers are `held` with reason `hook_deferred`, no `die` event was emitted for either, and neither file contains `post-update`.

### HOK-007 Failing pre-update aborts that container's transaction
A `pre-update` hook that fails (any exit code other than `0` and `75`) or times out MUST abort the transaction it belongs to before any container is stopped (UPD-014), with result `failed` and reason `pre_update_hook_failed`.

- **Given** a `pre-update` hook that exits 1 **When** a cycle runs **Then** the container is still running with its original ID and the report shows reason `pre_update_hook_failed`.

### HOK-008 Hook output and secrets
Parachute MUST pass nothing to a hook beyond the container's own environment and the command `sh -c "<label value>"`. A hook's stdout and stderr MUST be logged only at `debug`, at most 4 KiB of each, redacted per [glossary: Redaction](glossary.md#redaction) (including the values of the container's environment variables). Notifications, the report, porcelain output, and the status view MUST name a hook failure only by hook name, exit code or timeout, and container, never with the hook's output.

- **Given** a container with `API_KEY=hunter2-secret`, `--log-level debug`, a webhook notification URL, and a `post-update` hook `echo "$API_KEY"; exit 1` **When** the container is updated **Then** `hunter2-secret` appears neither in the log, the report, nor the webhook payload, and the log contains `[REDACTED]`.
