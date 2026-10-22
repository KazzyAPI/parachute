# 04 Update transaction

**Status:** Draft

How an update is applied. Every update is a **transaction** ([ADR-0004](../adr/0004-update-is-a-transaction.md)). One transaction covers one stack ([05-dependencies-and-stacks.md](05-dependencies-and-stacks.md), DEP-003). Nothing in this file happens while Parachute is unarmed (CFG-006).

### UPD-001 Phases and their order
A transaction MUST run these phases in this order, and MUST log the start of each phase at `info` level with the phase name and transaction ID:

| Phase | Steps |
|---|---|
| `detect` | DET-001 to DET-006 for every container in the stack. |
| `policy` | Evaluate holds (POL-001). If every update in the stack is held, the transaction ends here. |
| `snapshot` | Pull each new image by the digest that passed the `policy` phase (UPD-020); check that each affected container's configuration can be reproduced (UPD-007); check free space in the snapshot store (BKP-011); run `pre-update` hooks (HOK-007); run database dumps (BKP-004); stop affected containers when backup mode is `stop` (DEP-005); copy data and write the snapshot (BKP-001). |
| `update` | Stop affected containers not yet stopped; rename each one (UPD-008); create and start replacements (DEP-005); run `post-update` hooks. |
| `verify` | Health verification of every started replacement (VER-001); its deadline starts after the replacement's `post-update` hook (VER-006). |
| `commit` or `rollback` | On success, commit (UPD-009). On failure, roll back (VER-008). |

A self-update replaces the `update` and `verify` phases with one `handover` phase (UPD-018). The transaction's state follows the [transaction state machine](glossary.md#transaction-state-machine): each phase above is the open state of the same name, except that the `rollback` phase is state `rolling_back` and the `commit` phase line is logged when the transaction reaches the terminal state `committed`. A transaction that ends in the `detect`, `policy`, or `snapshot` phase (every update held, or a failure as in UPD-014) ends in state `aborted`.

The **affected containers** of a transaction are the containers whose image is updated plus their dependents (DEP-006).

- **Given** an armed Parachute and one container with a newer image **When** the update succeeds **Then** the log contains, in order, phase lines `detect`, `policy`, `snapshot`, `update`, `verify`, `commit`, all with the same transaction ID.
- **Given** an update whose verification fails **When** the cycle runs **Then** the phase lines end with `verify`, `rollback`.
- **Given** an approved self-update that succeeds **When** the cycle runs **Then** the phase lines for its transaction are, in order, `detect`, `policy`, `snapshot`, `handover`, `commit`.

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
On commit, Parachute MUST, in order: record the transaction as `committed` in the state file (CFG-010), which is the commit point; remove the renamed original containers (UPD-017); apply `--cleanup` (UPD-010) and `--remove-volumes` (UPD-011); run `post-check` hooks; and report the stack's updated services as `updated`. A committed transaction is never rolled back. If Parachute stops after the commit point and a renamed original of that transaction still exists at the next start, Parachute MUST log it at `warn` with the transaction ID and leave it in place. In a self-update, the renamed original is removed by the new instance (UPD-018 step 3).

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

- **Given** `--rolling-restart`, armed, and three replicas of one Compose service with a newer image **When** the cycle runs **Then** at no observed moment are two of the three stopped at the same time.

### UPD-014 Failure before the update phase changes nothing
A failure in the `detect`, `policy`, or `snapshot` phase MUST leave the stack's containers running (or in their original state) with their original IDs and images. Containers stopped for the snapshot MUST be started again. The result is `failed` with the failing step's reason (`pull_failed`, `config_unreproducible`, `insufficient_space`, `pre_update_hook_failed`, `dump_failed`, `snapshot_failed`, `transaction_timeout`).

- **Given** a newer tag whose blobs are deleted from the registry so the pull fails **When** a cycle runs **Then** the container keeps its ID and image and the result is `failed` with reason `pull_failed`.
- **Given** a snapshot that fails (the backup volume is read-only) **When** a cycle runs **Then** the container is running with its original ID and the result is `failed` with reason `snapshot_failed`.
- **Given** a snapshot store with less free space than BKP-011 requires **When** a cycle runs **Then** the container is running with its original ID, no container was stopped (no `die` event), and the result is `failed` with reason `insufficient_space`.

### UPD-015 Failure isolation
A failed transaction MUST NOT prevent transactions of unrelated stacks in the same cycle. Stacks are processed one at a time.

- **Given** unrelated containers `a` and `b` with newer images and `a`'s new image failing verification **When** a cycle runs **Then** `a` is `rolled_back` and `b` is `updated`.

### UPD-016 Interrupted transactions are resolved at startup
If Parachute stops (crash, kill, host reboot) while a transaction is in an open state, then at its next start, before running any cycle, it MUST end that transaction as the [transaction state machine](glossary.md#transaction-state-machine) prescribes, keep its transaction ID, and report the result:

- `detect`, `policy`, `snapshot`: start again every container stopped for the snapshot; state `aborted`, result `failed`, reason `interrupted`.
- `update`, `verify`, `rolling_back`: roll back (VER-008); result `rolled_back` with reason `interrupted`, or `rollback_failed` with the VER-010 reason.
- `handover`: UPD-018 step 6.

An instance in handover mode (UPD-018 step 2) MUST NOT do this until it owns the state file. Recovery removes only what UPD-017 allows.

- **Given** an update in its `verify` phase and Parachute killed with `SIGKILL` **When** Parachute starts again **Then** the original container is running under its original name and ID, the replacement no longer exists, and the next report lists the transaction as `rolled_back` with reason `interrupted`.
- **Given** backup mode `stop`, a container whose data copy takes at least 30 seconds, and Parachute killed with `SIGKILL` while the snapshot helper container runs **When** Parachute starts again **Then** before the first cycle's `detect` line the container is running again with its original ID and image, and the next report lists the transaction as `failed` with reason `interrupted`.

### UPD-017 Never delete what Parachute did not create
Parachute MUST remove an object only when it can prove that it created the object, or replaced it, in the transaction being ended. A name, a name pattern (such as `<name>-parachute-<suffix>`), or a label value supplied by anyone other than Parachute in that transaction is never enough. The only objects it MAY remove, each with its proof:

| Object | Removed when | Proof |
|---|---|---|
| Replacement container | its transaction rolls back (VER-008, UPD-016) | it carries `parachute.transaction` equal to that transaction's ID (UPD-002) |
| Original container | its transaction commits (UPD-009) | its container ID is the one Parachute renamed in that transaction, in the same process; for a self-update, the checks of UPD-018 step 3 |
| Helper container or network (BKP-010) | its task ends | it carries `parachute.helper=true` and `parachute.transaction` equal to the ID of the transaction (or restore-verification run, RST-002) it was created for |
| Restore-verification resource | its run ends (RST-002) | as for helpers |
| Snapshot | retention (BKP-008) | its manifest has `format` `parachute.snapshot/v1` |
| Previous image | commit with `--cleanup` (UPD-010) | it is the image of an original removed in the same commit |
| Anonymous volume | commit with `--remove-volumes` (UPD-011) | it was mounted by an original removed in the same commit |

State recovered from the state file (UPD-016) MUST NOT by itself justify a removal ([glossary: Trust boundaries](glossary.md#trust-boundaries)); the proof in the table is still required.

- **Given** a stopped, unrelated container named `web-parachute-abc123`, a stopped container labelled `parachute.helper=true` and `parachute.transaction=txn-20260101T000000Z-abc123` (an ID of no transaction of this instance), and an unused named volume **When** cycles run with `--cleanup` and `--remove-volumes` on, one of them committing an update of container `web` and one rolling back an update of container `api` **Then** all three objects still exist.
- **Given** a rolled-back update **When** the cycle ends **Then** no container carrying that transaction's `parachute.transaction` label exists.

### UPD-018 Self-update
A **self-update** is an update of Parachute's **own container**: the container this instance runs in, as identified by the engine. A `parachute.self=true` label alone never makes a container this instance's own (SEL-012); another Parachute container is never changed by this instance. The design is [ADR-0014](../adr/0014-self-update-safety.md). Before its `handover` phase a self-update MUST follow these rules:

- **Approval.** It is always held with reason `approval_required` until its approved digest (UPD-020) is approved (API-007), as the glossary's **Self-update approval** says; no profile, flag, or label removes this hold.
- **Order and scope.** It runs after every other stack in the cycle, as a transaction of the own container alone, even when that container belongs to a Compose project; no other container is stopped or joined to it (DEP-003).
- **Image.** It pulls by the approved digest (UPD-020).
- **Snapshot.** The own container's effective backup mode is always `none`, whatever its labels or `--backup`: its configuration is recorded, no data is copied, and it is never stopped (BKP-003's `stop` mode does not apply). VER-009 never restores data for it, because the new instance writes nothing in the state directory before it owns the state file.
- **Configuration.** Besides UPD-007, an own container that publishes a host port or has a static IP address on any network fails with reason `config_unreproducible` naming the field, because the replacement must run beside it.

The `handover` phase (state `handover`, [glossary: Transaction state machine](glossary.md#transaction-state-machine)): the old instance owns the state file ([ADR-0008](../adr/0008-state-storage.md)) and runs the whole transaction.

1. The old instance records state `handover` in the state file, then writes the **handover marker** `<state-dir>/handover.json` (by replacing, never in place) with `format` `parachute.handover/v1`, the transaction ID, the old container's ID, and the approved digest. Then it renames its own container (UPD-008) but keeps it running, and creates and starts the replacement (with the UPD-005 labels).
2. An instance that starts while a marker exists MUST first validate it ([glossary: Trust boundaries](glossary.md#trust-boundaries)): it parses as `parachute.handover/v1` with a transaction ID matching UPD-002 and a 64-hex container ID, and either the marker's container ID is the starting instance's own container ID, or the starting instance's own container carries `parachute.transaction` equal to the marker's transaction ID and runs an image whose repository digests contain the marker's digest. Otherwise it MUST exit with code 2 naming `handover.json` and leave the file unchanged. An instance whose own container is not the one the marker names is in **handover mode**: it MUST NOT read or write `state.json`, resolve open transactions (UPD-016), run cycles, or change any container, and it MUST log at `info` that it is waiting for handover with the transaction ID. Its other startup checks complete normally, so `parachute healthcheck` (DMN-006) passes.
3. The old instance verifies the replacement (VER-001). On success it records the cycle's results and the transaction as `committed` in the state file, sends the cycle report, removes the marker, and from then on MUST NOT read or write the state file, run cycles, or change any container. When the marker is gone, the new instance reads the state file; if it records the marker's transaction as `committed`, the new instance owns the state file, removes the old container if, and only if, it has the container ID the marker named, carries `parachute.self=true`, and is named `<name>-parachute-<suffix>` for that transaction (UPD-017), applies `--cleanup` (UPD-010), and starts its cycles.
4. If the replacement fails verification, or the ceiling is reached (UPD-019), the old instance MUST roll back (state `rolling_back`): remove the replacement, rename its container back, record `rolled_back` with the VER or UPD-019 reason and the POL-010 hold for that digest, remove the marker, keep running, and report the result `rolled_back` at `warn`.
5. The result MUST be `rollback_failed` with reason `restore_failed` (an `error` notification, NTF-012) only when the old instance cannot remove the replacement or rename its container back, or two Parachute containers for that scope are left running. The old instance keeps the state file.
6. If, while the marker exists, the old container no longer exists or is neither running nor restarting, the new instance in handover mode MUST take ownership of the state file, record the transaction as `rollback_failed` with reason `interrupted`, remove the marker, send the `error` notification (NTF-012), and change no container; the stack is then frozen (glossary). If instead the marker names the starting instance's own container (the old instance restarted), that instance owns the state file and resolves the transaction by rollback as in step 4, with reason `interrupted`.

An instance whose own container ID the state file records as the old container of a self-update that ended `committed`, or `rollback_failed` with reason `interrupted`, MUST exit at startup with code 2, so a superseded instance never runs beside its successor. At the end of every self-update exactly one Parachute container for that scope is running, except after step 5.

- **Given** an armed Parachute with `--profile convenient`, `--protect-databases=false`, and `parachute.approval=auto` on its own container, and a newer Parachute image **When** a cycle runs **Then** the result for its own container is `held` with reason `approval_required` and no container is changed.
- **Given** an approved newer Parachute image and one other container with an update **When** a cycle runs **Then** the other container's `commit` line precedes the self-update's `snapshot` line, and afterwards exactly one container labelled `parachute.self=true` in that scope is running, with the original name, on an image whose repository digests contain the approved digest; no container named `<name>-parachute-<suffix>` for the transaction exists; `handover.json` does not exist; and `parachute status --json` shows `last_transaction.state` `committed`.
- **Given** an approved new Parachute image whose health check always fails **When** a cycle runs **Then** the original Parachute container is still running with its original ID and name, the replacement no longer exists, the result for it is `rolled_back` with reason `verify_failed` in a `warn` entry, no `error` notification is sent, and the next cycle holds that digest with reason `previous_attempt_failed`.
- **Given** an approved self-update in its `handover` phase, with old and new instance running side by side (the new image's health check passes after 30 seconds) **When** the test reads `state.json`, `handover.json`, and the new instance's log during verification **Then** `state.json` records the transaction with state `handover`, `handover.json` names the transaction ID and the old container's ID, the new instance has logged that it is waiting for handover and no phase line, and the transaction later ends `updated` with the marker removed.
- **Given** the same, and the test running `docker kill` on the old container during verification **When** 60 seconds have passed **Then** the new instance is running, `handover.json` does not exist, `parachute status --json` shows the transaction as `rollback_failed` with reason `interrupted`, the webhook received an `error` notification with the transaction ID, the killed container still exists, and the next cycle holds the self-update with reason `previous_attempt_failed`.
- **Given** a `handover.json` written by the test that names a transaction ID and container ID unrelated to the starting instance **When** Parachute starts **Then** it exits with code 2, the message names `handover.json`, and the file's checksum is unchanged.
- **Given** an own container that publishes port `8080:8080` and an approved newer Parachute image **When** a cycle runs **Then** the result is `failed` with reason `config_unreproducible` naming the port, and the container keeps its ID.
- **Given** an approved self-update, the test running `docker pause` on the new instance at its `health_status: healthy` event, and then, once `handover.json` is gone, `docker stop` on the old container **When** the test starts the old container again **Then** its Parachute process exits with code 2.

### UPD-019 Transaction ceiling
Every transaction MUST reach `commit` or `rollback` within `--transaction-timeout` (a duration; default `30m`; `0` is a startup error, exit code 2), counted from the start of its `snapshot` phase, so that a hook without a timeout (HOK-005) or a long verification deadline (VER-006) cannot hold a stack indefinitely. Time spent by helper containers copying data into the snapshot (BKP-003) is not counted. When the ceiling is reached, Parachute MUST stop waiting for any running hook (logged at `warn` with the hook name) and end the transaction with reason `transaction_timeout`: before the `update` phase as in UPD-014 (result `failed`), otherwise by rollback (result `rolled_back`). A rollback in progress is never interrupted by the ceiling.

- **Given** hooks enabled, `--transaction-timeout 2m`, and a `pre-update` hook that sleeps 1 hour with `parachute.lifecycle.pre-update-timeout=0` **When** a cycle runs **Then** between 2 and 2.5 minutes after the `snapshot` phase started the result is `failed` with reason `transaction_timeout`, a `warn` line names `pre-update`, and the container still runs with its original ID.
- **Given** hooks enabled, `--transaction-timeout 2m`, and a `post-update` hook that sleeps 1 hour with `parachute.lifecycle.post-update-timeout=0` **When** the container is updated **Then** the transaction is rolled back with reason `transaction_timeout` and the original container runs with its original ID.
- **Given** `--transaction-timeout 0` **When** Parachute starts **Then** it exits with code 2 and the message contains `transaction-timeout`.

### UPD-020 Pull by the approved digest
The **approved digest** of an update is the candidate digest that the `policy` phase evaluated: the digest whose age the delay counts (POL-002) and to which an approval applies (POL-006, API-007). Parachute MUST pull `<repository>@<approved digest>`, never the tag, so that a tag moved after detection cannot bypass the delay or an approval ([ADR-0012](../adr/README.md)). After the pull it MUST check that the pulled image's repository digests contain the approved digest; if they do not, the transaction ends as in UPD-014 with result `failed`, reason `pull_failed`, and the detail names both digests. Parachute then points the original reference (the container's `Config.Image`, for example `repo:1`) at the pulled image locally and creates the replacement from that reference, so the replacement's `Config.Image` is unchanged (UPD-003) and it is not later skipped as `digest_pinned` (DET-005). The replacement's image ID MUST equal the pulled image's ID; a difference is a configuration mismatch (UPD-007).

- **Given** a container on `localhost:5000/fixture/app:1`, a newer digest `D2` pushed to tag `1`, and the registry behind a recording proxy **When** the cycle updates it **Then** the proxy log shows the manifest fetched for the pull by path `/v2/fixture/app/manifests/D2`, no manifest request by tag `1` after detection, and the replacement's image has repository digest `D2`.
- **Given** the same, and the proxy moving tag `1` to a third digest `D3` right after it answers Parachute's detection request **When** the cycle runs **Then** the replacement's image has repository digest `D2`, not `D3`.
- **Given** a proxy that answers the manifest request for `D2` with a different manifest **When** the cycle runs **Then** the result is `failed` with reason `pull_failed`, and the container keeps its ID and image.
- **Given** a committed update **When** the test inspects the replacement **Then** its `Config.Image` equals the original's, and `docker image inspect localhost:5000/fixture/app:1` returns the replacement's image ID.
