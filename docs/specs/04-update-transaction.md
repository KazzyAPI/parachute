# 04 Update transaction

**Status:** Accepted

How an update is applied. Every update is a **transaction** ([ADR-0004](../adr/0004-update-is-a-transaction.md)). One transaction covers one stack ([05-dependencies-and-stacks.md](05-dependencies-and-stacks.md), DEP-003). Nothing in this file happens while Parachute is unarmed (CFG-006).

### UPD-001 Phases and their order
A stack's update MUST run these phases in this order, and MUST log the start of each phase it runs at `info` level with the phase name and, from `snapshot` on, the transaction ID. The `detect` and `policy` phases run before any transaction exists. A transaction, with its ID (UPD-002), MUST start at the `snapshot` phase, and only when at least one update in the stack passes the `policy` phase. A stack whose candidates are all `fresh`, whose every result is decided in `detect`, or whose every update is held or fails in the `policy` phase (for example `monitor_only`, `approval_required`, `previous_attempt_failed`, `delay`, `maintenance_window`, `invalid_label`) gets no transaction and no transaction ID: none of its log lines in that cycle carries one, nothing is recorded as a transaction in the state file, and its `last_transaction` keeps its previous value (STS-002).

| Phase | Steps |
|---|---|
| `detect` | DET-001 to DET-006 for every container in the stack. |
| `policy` | Runs only when `detect` found at least one update; otherwise no `policy` line is logged. Evaluate holds (POL-001), before any transaction exists. If every update in the stack is held, no transaction starts. |
| `snapshot` | Pull each new image by the digest that passed the `policy` phase, check the pulled digest, and re-tag it to the original reference (UPD-020); check that each affected container's configuration can be reproduced (UPD-007); check free space in the snapshot store (BKP-011); run `pre-update` hooks (HOK-007); run database dumps (BKP-004); stop affected containers when backup mode is `stop` (DEP-005); copy data and write the snapshot (BKP-001). |
| `update` | Stop affected containers not yet stopped; rename each one (UPD-008); create and start replacements (DEP-005); run `post-update` hooks. |
| `verify` | Health verification of every started replacement (VER-001); its deadline starts after the replacement's `post-update` hook (VER-006). |
| `commit` or `rollback` | On success, commit (UPD-009). On failure, roll back (VER-008). |

A self-update replaces the `update` and `verify` phases with one `handover` phase (UPD-018). The transaction's state follows the [transaction state machine](glossary.md#transaction-state-machine): each phase above is the open state of the same name, except that the `rollback` phase is state `rolling_back` and the `commit` phase line is logged when the transaction reaches the terminal state `committed`. The `detect` and `policy` phases are not transaction states. A transaction that ends in the `snapshot` phase (deferred by a hook, HOK-006, or a failure as in UPD-014) ends in state `aborted`.

The **affected containers** of a transaction are the containers whose image is updated plus their dependents (DEP-006).

- **Given** an armed Parachute and one container with a newer image **When** the update succeeds **Then** the log contains, in order, phase lines `detect`, `policy`, `snapshot`, `update`, `verify`, `commit`, the `detect` and `policy` lines without a transaction ID and the others all with the same transaction ID.
- **Given** an update whose verification fails **When** the cycle runs **Then** the phase lines end with `verify`, `rollback`.
- **Given** a committed update of one container and then a cycle in which its image is unchanged **When** that cycle ends **Then** the cycle logged a `detect` line and no `policy` line for it, no log line of that cycle carries a new `txn` value, and `parachute status --json` still shows the committed transaction's ID in `last_transaction`.
- **Given** a committed update of one container and then a newer image for it held with reason `delay` **When** that cycle ends **Then** its `policy` line carries no `txn` value, no log line of that cycle carries a new `txn` value, and `parachute status --json` still shows the committed transaction's ID and state in `last_transaction`.
- **Given** an approved self-update that succeeds **When** the cycle runs **Then** the phase lines for the own container are, in order, `detect`, `policy`, `snapshot`, `handover`, `commit`, and only the last three carry the transaction ID.

### UPD-002 Transaction ID
Every transaction MUST have an ID matching `txn-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}`. The ID MUST appear in every log line about the transaction (the `txn` field in JSON logs, CFG-008), in the report and notifications that mention it, in porcelain output, and in the `parachute.transaction` label of each replacement container.

- **Given** `--log-format json` and a successful update **When** the cycle runs **Then** every log line whose `container` field names the updated container in that cycle carries the same `txn` value, and the replacement's `parachute.transaction` label equals it.

### UPD-003 Recreate with the same configuration
The replacement MUST have the same name, configuration, and attachments as the original apart from the image: ports, environment, command, entrypoint, mounts (bind mounts, named volumes, anonymous volumes per UPD-011, tmpfs), networks with aliases and static addresses, restart policy, resource limits, capabilities, devices, user, working directory, hostname, labels, log driver, and health check. Exceptions are only those in UPD-004 and the bookkeeping labels in UPD-005.

- **Given** a container with ports `8080:3000`, env `A=1`, a bind mount, a named volume, restart policy `unless-stopped`, a memory limit of 64 MiB, a user-defined network with alias `web`, and label `x=y` **When** it is updated **Then** `docker inspect` of the replacement shows the same values for each of those fields and its image ID equals the new image's ID.
- **Given** the update **Then** the replacement has the original container name.

### UPD-004 Image defaults follow the new image
A setting that the original container inherited unchanged from its old image (environment variables, labels, command, entrypoint, working directory, user, exposed ports, health check, declared volumes) MUST take the new image's value. A setting the operator set explicitly (it differs from the old image's value) MUST be kept. For declared volumes, inheritance MUST NOT drop a mount: every anonymous volume mounted by the original is kept at its path (UPD-011), even when the new image no longer declares it; a path declared only by the new image gets a new, empty anonymous volume.

- **Given** image v1 with `ENV VERSION=1` and v2 with `ENV VERSION=2`, and a container created from v1 without setting `VERSION` **When** it is updated to v2 **Then** the replacement's environment contains `VERSION=2`.
- **Given** the same but the container created with `-e VERSION=custom` **When** it is updated **Then** the replacement's environment contains `VERSION=custom`.
- **Given** image v1 labelled `org.opencontainers.image.version=1` and v2 labelled `...=2` **When** a container from v1 is updated **Then** the replacement carries `org.opencontainers.image.version=2`.
- **Given** image v1 declaring `VOLUME /data`, v2 declaring `VOLUME /cache` and not `/data`, and a container from v1 whose anonymous volume at `/data` holds file `marker` **When** it is updated to v2 **Then** the replacement mounts the original's volume at `/data` (it contains `marker`) and a new, empty anonymous volume at `/cache`.

### UPD-005 Bookkeeping labels
Each replacement MUST carry these labels, in addition to the original's labels: `parachute.transaction` (UPD-002), `parachute.updated-at` (RFC 3339 UTC time of commit or of creation), `parachute.previous-image` (the previous image as `<reference>@<digest>`, or the image ID when no digest exists), and `parachute.snapshot` (the snapshot ID, or `none` under `parachute.backup=none`). Labels whose names start with `com.docker.compose.` MUST be copied unchanged.

- **Given** a Compose service container that is updated **When** the cycle commits **Then** the replacement has all four labels, and every `com.docker.compose.*` label equals the original's value.

### UPD-006 Graceful stop
Parachute MUST stop a container by sending the signal in its `parachute.stop-signal` label, else the container's configured stop signal, else `SIGTERM`, and MUST wait the stop timeout before killing it. The stop timeout is `--stop-timeout` (a duration such as `30s`; default `10s`), overridden per container by the container's own configured stop timeout when one is set.

- **Given** a container labelled `parachute.stop-signal=SIGHUP` whose process records the signal it receives **When** it is updated **Then** the recorded signal is `SIGHUP`.
- **Given** a container that ignores `SIGTERM` and `--stop-timeout 5s` **When** it is updated **Then** it is killed between 5 and 7 seconds after the stop began.
- **Given** a container that ignores `SIGTERM` and no stop-timeout settings **When** it is updated **Then** it is killed between 10 and 12 seconds after the stop began.

### UPD-007 Unreproducible configuration blocks the update
If Parachute cannot carry a container's configuration through the transaction, it MUST NOT stop or change that container (or the rest of its stack transaction), and the result MUST be `failed` with reason `config_unreproducible` naming the field. At minimum these cases MUST be detected: `AutoRemove` is on (stopping it would delete the container and the rollback target), the container is a Swarm task (has a `com.docker.swarm.service.id` label), and the container's network mode refers to a container that no longer exists. After creating each replacement and before starting it, Parachute MUST compare the replacement's configuration with the original (excluding UPD-004 and UPD-005 differences); any other difference MUST abort with rollback and the same reason.

- **Given** a container started with `--rm` and a newer image **When** a cycle runs **Then** the container is still running with its original ID and the result is `failed` with reason `config_unreproducible` mentioning `AutoRemove`.
- **Given** the same container in a stack with a second updatable service **When** a cycle runs **Then** neither service is changed.

### UPD-008 Previous container and image kept until commit
Until the transaction commits, Parachute MUST keep each original container (stopped, renamed to `<name>-parachute-<suffix>` where `<suffix>` is the last six characters of the transaction ID) and its image. Rollback restores them (VER-008). The one exception is a self-update: Parachute's own original container is renamed the same way but keeps running until the new instance removes it (UPD-018).

- **Given** an update during its `verify` phase (the new container's health check takes 60 seconds) **When** the test inspects containers **Then** a stopped container named `<name>-parachute-<suffix>` exists with the original container ID and its image is still present.

### UPD-009 Commit
On commit, Parachute MUST, in order: record the transaction as `committed` in the state file (CFG-010), which is the commit point; remove the renamed original containers (UPD-017); apply `--cleanup` (UPD-010) and `--remove-volumes` (UPD-011); run `post-check` hooks; and report the stack's updated services as `updated`. A committed transaction is never rolled back. If Parachute stops after the commit point and a renamed original of that transaction still exists at the next start, Parachute MUST log it at `warn` with the transaction ID and leave it in place. In a self-update, the renamed original is removed by the new instance (UPD-018 step 3, or step 6 when the old instance stopped after the commit point), and is not left in place.

- **Given** a successful update **When** the cycle ends **Then** no container named `<name>-parachute-<suffix>` for that transaction exists, the result is `updated`, and `parachute status --json` shows `last_transaction.state` `committed` with the transaction ID (STS-002).

### UPD-010 Image cleanup after commit
With `--cleanup` on, Parachute MUST remove each previous image after commit, unless another container still uses it. It MUST NOT remove a previous image before commit or after a rollback.

- **Given** `--cleanup` and a committed update **When** the cycle ends **Then** the previous image ID is no longer present locally.
- **Given** `--cleanup` and an update that rolled back **When** the cycle ends **Then** the previous image is present and the original container runs on it.
- **Given** `--cleanup` and a second container still using the previous image **When** the first is updated **Then** the previous image is present.

### UPD-011 Anonymous volumes
By default the replacement MUST mount the original's anonymous volumes at the same paths, so their data carries over, whether or not the new image still declares those paths (this wins over UPD-004). With `--remove-volumes` on, the replacement MUST get new anonymous volumes and the original's anonymous volumes MUST be removed at commit (after being included in the snapshot, BKP-002). Named volumes and bind mounts MUST NOT be removed in either case.

- **Given** a container with an anonymous volume at `/data` holding file `marker` **When** it is updated without `--remove-volumes` **Then** the replacement's `/data` contains `marker`.
- **Given** `--remove-volumes` **When** the update commits **Then** the replacement's `/data` volume has a different name, the original anonymous volume no longer exists, and named volumes are still present.

### UPD-012 No-restart
With `--no-restart` on, Parachute MUST pull and recreate but MUST NOT start replacements. Verification then follows VER-012.

- **Given** `--no-restart`, armed, and a running container with a newer image **When** the cycle ends **Then** the replacement uses the new image and is in state `created`.

### UPD-013 Rolling restart
With `--rolling-restart` on, Parachute MUST update the affected containers of a transaction one at a time, each fully stopped, recreated, started, and verified before the next is stopped. Containers linked by a dependency (DEP-004) are processed as one group within the rolling order, and Parachute MUST log that at `info`.

- **Given** `--rolling-restart`, armed, and three replicas of one Compose service with a newer image **When** the cycle runs **Then** in the Docker event stream, after each replica's `die` event the `start` event of its replacement comes before the next replica's `die` event.

### UPD-014 Failure before the update phase changes nothing
A failure in the `detect`, `policy`, or `snapshot` phase MUST leave the stack's containers running (or in their original state) with their original IDs and images. Containers stopped for the snapshot MUST be started again. A failure in `detect` or `policy` creates no transaction (UPD-001); a failure in `snapshot` ends the transaction in state `aborted`. The result is `failed` with the failing step's code from the [`failed` table](reasons.md#failed), for example `pull_failed` (including a pulled image that does not carry the approved digest, UPD-020), `config_unreproducible`, `insufficient_space`, `pre_update_hook_failed`, `dump_failed`, `snapshot_failed`, `transaction_timeout`, or, after a restart, `interrupted`.

- **Given** a newer tag whose blobs are deleted from the registry so the pull fails **When** a cycle runs **Then** the container keeps its ID and image and the result is `failed` with reason `pull_failed`.
- **Given** a snapshot that fails (the backup volume is read-only) **When** a cycle runs **Then** the container is running with its original ID and the result is `failed` with reason `snapshot_failed`.
- **Given** a snapshot store with less free space than BKP-011 requires **When** a cycle runs **Then** the container is running with its original ID, no container was stopped (no `die` event), and the result is `failed` with reason `insufficient_space`.

### UPD-015 Failure isolation
A failed transaction MUST NOT prevent transactions of unrelated stacks in the same cycle. Stacks are processed one at a time.

- **Given** unrelated containers `a` and `b` with newer images and `a`'s new image failing verification **When** a cycle runs **Then** `a` is `rolled_back` and `b` is `updated`.

### UPD-016 Interrupted transactions are resolved at startup
If Parachute stops (crash, kill, host reboot) while a transaction is in an open state, then at its next start, before running any cycle, it MUST end that transaction as the [transaction state machine](glossary.md#transaction-state-machine) prescribes, keep its transaction ID, and report the result:

- `snapshot`: start again every container stopped for the snapshot; state `aborted`, result `failed`, reason `interrupted`.
- `update`, `verify`, or `rolling_back` of an ordinary stack: roll back (VER-008); result `rolled_back` with reason `interrupted`, or `rollback_failed` with the VER-010 reason.
- `handover`, or `rolling_back` of a self-update: UPD-018 step 6.

An instance in handover mode (UPD-018 step 2) MUST NOT do this until it owns the state file. Recovery removes only what UPD-017 allows.

- **Given** an update in its `verify` phase and Parachute killed with `SIGKILL` **When** Parachute starts again **Then** the original container is running under its original name and ID, the replacement no longer exists, and the next report lists the transaction as `rolled_back` with reason `interrupted`.
- **Given** backup mode `stop`, a container whose data copy takes at least 30 seconds, and Parachute killed with `SIGKILL` while the snapshot helper container runs **When** Parachute starts again **Then** before the first cycle's `detect` line the container is running again with its original ID and image, and the next report lists the transaction as `failed` with reason `interrupted`.

### UPD-017 Never delete what Parachute did not create
Parachute MUST remove an object only when it can prove that it created the object, or replaced it, in the transaction being ended. A name, a name pattern (such as `<name>-parachute-<suffix>`), or a label value supplied by anyone other than Parachute in that transaction is never enough. The only objects it MAY remove, each with its proof:

| Object | Removed when | Proof |
|---|---|---|
| Replacement container | its transaction rolls back (VER-008, UPD-016) | it carries `parachute.transaction` equal to that transaction's ID (UPD-002) |
| Original container | its transaction commits (UPD-009) | its container ID is the one Parachute renamed in that transaction, in the same process; for a self-update, the checks of UPD-018 step 3 |
| Helper container or network (BKP-010) | its task ends | it carries `parachute.helper=true` and `parachute.transaction` equal to the ID of the transaction it was created for (BKP-010) |
| Restore-verification resource | its run ends (RST-002) | it carries `parachute.helper=true` and `parachute.restore-verify` naming a run of the removing process, or a run recorded open in `restore-verify.json` (BKP-010, RST-005) |
| Snapshot | retention (BKP-008) | the BKP-008 retention proof |
| Previous image | commit with `--cleanup` (UPD-010) | it is the image of an original removed in the same commit |
| Anonymous volume | commit with `--remove-volumes` (UPD-011) | it was mounted by an original removed in the same commit |

State recovered from the state file (UPD-016) MUST NOT by itself justify a removal ([glossary: Trust boundaries](glossary.md#trust-boundaries)); the proof in the table is still required. For a snapshot, the proof includes the manifest cross-check of [BKP-008](07-backup.md#bkp-008-retention).

- **Given** a stopped, unrelated container named `web-parachute-abc123`, a stopped container labelled `parachute.helper=true` and `parachute.transaction=txn-20260101T000000Z-abc123` (an ID of no transaction of this instance), and an unused named volume **When** cycles run with `--cleanup` and `--remove-volumes` on, one of them committing an update of container `web` and one rolling back an update of container `api` **Then** all three objects still exist.
- **Given** a rolled-back update **When** the cycle ends **Then** no container carrying that transaction's `parachute.transaction` label exists.

**UPD-018 Self-update** is specified in [17-self-update.md](17-self-update.md).

### UPD-019 Transaction ceiling
Every transaction MUST reach state `committed` or `rolling_back`, or end `aborted`, within `--transaction-timeout` (a duration; default `30m`; `0` or a negative value is a startup error, exit code 2), counted from the start of its `snapshot` phase, so that a hook without a timeout (HOK-005) or a long verification deadline (VER-006) cannot hold a stack indefinitely. The clock is paused while helper containers copy data into the snapshot (BKP-001, BKP-010), so the size of the data never causes a timeout; all other time counts, including the `handover` phase of a self-update (UPD-018). When the ceiling is reached, Parachute MUST stop waiting for any running hook (logged at `warn` with the hook name) and end the transaction with reason `transaction_timeout`: before the `update` or `handover` phase as in UPD-014 (state `aborted`, result `failed`), otherwise by rollback (result `rolled_back`, or `rollback_failed` per VER-010). The state `rolling_back` is never interrupted by the ceiling.

- **Given** hooks enabled, `--transaction-timeout 2m`, and a `pre-update` hook that sleeps 1 hour with `parachute.lifecycle.pre-update-timeout=0` **When** a cycle runs **Then** between 2 and 2.5 minutes after the `snapshot` phase started the result is `failed` with reason `transaction_timeout`, a `warn` line names `pre-update`, and the container still runs with its original ID.
- **Given** hooks enabled, `--transaction-timeout 2m`, and a `post-update` hook that sleeps 1 hour with `parachute.lifecycle.post-update-timeout=0` **When** the container is updated **Then** the transaction is rolled back with reason `transaction_timeout` and the original container runs with its original ID.
- **Given** `--transaction-timeout 0`, and separately `--transaction-timeout -1m` **When** Parachute starts **Then** it exits with code 2 and the message contains `transaction-timeout`.
- **Given** `--transaction-timeout 1m`, `--verify-stable 5s`, a container with a 200 MiB named volume, and the test running `docker pause` on the snapshot helper container at its `start` event and `docker unpause` 90 seconds later (the test fails as inconclusive when the pause fails because the helper has already exited) **When** a cycle runs **Then** the helper's `start` and `die` events are more than 1 minute apart and the result is `updated`.

### UPD-020 Pull by the approved digest
The **approved digest** of an update is the candidate digest that the `policy` phase evaluated: the digest whose age the delay counts (POL-002) and to which an approval applies (POL-006, API-007). Parachute MUST pull `<repository>@<approved digest>`, never the tag, so that a tag moved after detection cannot bypass the delay or an approval ([ADR-0012](../adr/0012-digest-pinned-updates.md)). After the pull it MUST check that the pulled image's repository digests contain the approved digest; if they do not, the transaction ends as in UPD-014 with result `failed`, reason `pull_failed`, and the detail names both digests. Parachute then points the original reference (the container's `Config.Image`, for example `repo:1`) at the pulled image locally and creates the replacement from that reference, so the replacement's `Config.Image` is unchanged (UPD-003) and it is not later skipped as `digest_pinned` (DET-005). The replacement's image ID MUST equal the pulled image's ID; on a difference Parachute MUST remove the replacement and roll back, with result `rolled_back` and reason `image_mismatch` naming both image IDs.

- **Given** a container on `localhost:5000/fixture/app:1`, a newer digest `D2` pushed to tag `1`, and the registry behind a recording proxy **When** the cycle updates it **Then** the proxy log shows the manifest fetched for the pull by path `/v2/fixture/app/manifests/D2`, no manifest request by tag `1` after detection, and the replacement's image has repository digest `D2`.
- **Given** the same, and the proxy moving tag `1` to a third digest `D3` right after it answers Parachute's detection request **When** the cycle runs **Then** the replacement's image has repository digest `D2`, not `D3`.
- **Given** a proxy that answers the manifest request for `D2` with a different manifest **When** the cycle runs **Then** the result is `failed` with reason `pull_failed`, and the container keeps its ID and image.
- **Given** a committed update **When** the test inspects the replacement **Then** its `Config.Image` equals the original's, and `docker image inspect localhost:5000/fixture/app:1` returns the replacement's image ID.
