# 07 Backup before update

**Status:** Draft

Every transaction takes a **snapshot** before it changes anything, so that rollback has a restore point. Format and storage are proposed in [ADR-0009](../adr/0009-snapshot-format-and-storage.md).

### BKP-001 No snapshot, no update
Before the `update` phase, Parachute MUST create a valid snapshot (BKP-007) for the transaction. If it cannot, the transaction MUST end as in UPD-014 with reason `snapshot_failed` (or `insufficient_space` when the free-space check refuses it first, BKP-011). The only exception is a container whose effective backup mode is `none` (BKP-003).

- **Given** `--backup-volume` on a host path backed by a 2 GiB `tmpfs`, a container with a 10 MiB named volume (so BKP-011 passes), and `parachute.backup.dump="head -c 3221225472 /dev/zero"` (a 3 GiB dump that exits 0) **When** a cycle runs **Then** the container keeps its ID and image and the result is `failed` with reason `snapshot_failed`.
- **Given** the same and `parachute.backup=none` on the container **When** a cycle runs **Then** the container is updated and the replacement's `parachute.snapshot` label is `none`.

### BKP-002 Snapshot contents
A snapshot MUST contain: the full inspected configuration of every container in the stack; and the data of every named volume, anonymous volume, and writable bind mount mounted by an affected container. Read-only mounts, tmpfs mounts, and bind mounts whose source is not a directory or regular file (for example sockets) MUST be left out. A container label `parachute.backup.exclude` (comma-separated mount destinations) MUST leave those mounts out. System mounts are never copied (BKP-012).

- **Given** a container with named volume `data` at `/data`, writable bind mount at `/conf`, read-only bind at `/etc/ro`, and `/var/run/docker.sock` mounted **When** a snapshot is taken **Then** its manifest lists `/data` and `/conf` and not `/etc/ro` or the socket.
- **Given** `parachute.backup.exclude=/media` and a bind mount at `/media` **When** a snapshot is taken **Then** the manifest does not list `/media`.

### BKP-003 Backup modes
The effective backup mode of a container is its `parachute.backup` label, else `--backup`; values are `stop` (default), `live`, `none`. In `stop` mode the affected containers MUST be stopped before their data is copied, so the copy is consistent. In `live` mode data MUST be copied while containers run, and they are stopped only in the `update` phase. In `none` mode no data is copied; container configuration is still recorded so rollback can restore the container. A volume that is also mounted by a running container outside the transaction MUST be copied live, marked `"consistency": "live"` in the manifest, and logged at `warn`. Any other value is a startup error (exit code 2) for `--backup`, and a `failed` result with reason `invalid_label` for a label.

- **Given** mode `stop` and a container whose process writes a counter to its volume every 100 ms **When** a snapshot is taken **Then** the Docker event stream shows the container's `die` event before the snapshot's helper container `start` event.
- **Given** mode `live` **When** a snapshot is taken **Then** the container's `die` event comes after the helper container finished.
- **Given** `parachute.backup=none` and a failed verification **When** rollback runs **Then** the original container is restored and the report says `data not restored: backup=none`.

### BKP-004 Database dumps
For a container recognised as a database (POL-003) whose image is in the `postgres` or `mysql`/`mariadb` family, Parachute MUST, before stopping it, run a logical dump inside the container and store its output in the snapshot: `pg_dumpall -U <user>` (the container's `POSTGRES_USER`, `postgres` when unset) for the `postgres` family; `mysqldump --all-databases -uroot` (`mariadb-dump` for `mariadb`). Secrets for a dump MUST be passed as environment variables set on the exec, never as command arguments ([ADR-0010](../adr/0010-docker-socket-access-and-security.md) §4): `PGPASSWORD` from the container's `POSTGRES_PASSWORD`, and `MYSQL_PWD` from its `MYSQL_ROOT_PASSWORD` (or `MARIADB_ROOT_PASSWORD`), each only when set. A label `parachute.backup.dump` MUST override this: `off` disables the dump; any other value is run as `sh -c "<value>"` with its stdout stored as the dump. A dump that exits non-zero MUST fail the snapshot with reason `dump_failed`. The data copy (BKP-003) is taken in addition to the dump. A dump's stderr MUST be logged only at `debug`, redacted per [glossary: Redaction](glossary.md#redaction); the `dump_failed` detail names only the container and the exit code.

- **Given** an approved update for a `postgres:16` container with a table holding one row **When** the snapshot is taken **Then** the snapshot contains a dump file that, loaded into an empty `postgres:16`, recreates that row.
- **Given** an approved update for a `mysql:8` container with `MYSQL_ROOT_PASSWORD=s3cr3t-pw` and `--log-level debug` **When** the snapshot is taken **Then** the dump succeeds, and `s3cr3t-pw` appears neither in the command of any `exec_create` or `exec_start` event in the Docker event stream, nor in `docker inspect` of those execs (`ProcessConfig`), nor in Parachute's log.
- **Given** `parachute.backup.dump="exit 1"` **When** a cycle runs **Then** the result is `failed` with reason `dump_failed` and the container is unchanged.
- **Given** a container with `MYSQL_ROOT_PASSWORD=s3cr3t-pw`, `parachute.backup.dump='echo "$MYSQL_ROOT_PASSWORD" >&2; exit 1'`, `--log-level debug`, and a webhook notification URL **When** a cycle runs **Then** the result is `failed` with reason `dump_failed`, and `s3cr3t-pw` appears neither in the log, the report, nor the webhook payload, while the log contains `[REDACTED]`.
- **Given** `parachute.backup.dump=off` on a postgres container **When** a snapshot is taken **Then** the snapshot has no dump file.

### BKP-005 Snapshot storage
Snapshots MUST be stored on the same host as the containers they protect, in the location named by `--backup-volume`: a Docker volume name (default `parachute-snapshots`, created if missing) or an absolute host path (used as a bind mount). Each snapshot is one directory named after its snapshot ID. Data is copied by helper containers (BKP-010). Free space in this location is checked before each snapshot (BKP-011).

- **Given** default settings and a committed update **When** the test lists the `parachute-snapshots` volume **Then** it contains a directory whose name equals the replacement's `parachute.snapshot` label.
- **Given** `--backup-volume /srv/parachute` **When** a snapshot is taken **Then** `/srv/parachute/<snapshot-id>/manifest.json` exists on the host.

### BKP-006 Snapshot ID and manifest
Each snapshot MUST have an ID matching `snap-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}` and a `manifest.json` containing at least: `format` (`parachute.snapshot/v1`), `id`, `transaction` (the transaction ID), `host`, `stack`, `created` (RFC 3339 UTC), `containers` (inspected configuration per container), `mounts` (per mount: container, destination, source, archive file name, `sha256`, size in bytes, `consistency` of `stopped` or `live`), `dumps` (per dump: container, file name, `sha256`), and `skipped` (BKP-012). Data archives are gzip-compressed tar files.

- **Given** a committed update **When** the test reads the snapshot's `manifest.json` **Then** `transaction` equals the replacement's `parachute.transaction` label and every listed archive exists with the recorded `sha256`.

### BKP-007 Snapshot validity
Snapshot manifests and archives are untrusted ([glossary: Trust boundaries](glossary.md#trust-boundaries)). A snapshot is **valid** when all of these hold:

1. `manifest.json` is a regular file (not a symbolic link) of at most 16 MiB, parses as JSON, has `format` `parachute.snapshot/v1`, an `id` equal to the directory name, and, when used for a transaction, the `host`, `stack`, and `transaction` of that transaction.
2. Every archive and dump file name in the manifest is a single path component matching `[A-Za-z0-9._-]+`, is not `.` or `..`, and names a regular file (not a symbolic link) directly inside the snapshot directory.
3. Every listed archive and dump has the recorded size and `sha256`.

Parachute MUST check validity after writing a snapshot, and again immediately before restoring from it in rollback or restore verification, reading every archive in full. A snapshot that fails the check after writing counts as a snapshot failure (BKP-001); one that fails before rollback is reported in the rollback result (VER-010) and no data is restored from it.

- **Given** a snapshot whose archive is truncated by the test after the replacement's `start` event and before its verification fails **When** rollback runs **Then** the result is `rollback_failed` with reason `snapshot_invalid`, and the original container is still restored.
- **Given** a snapshot whose manifest the test changes after the replacement's `start` event so that an archive file name is `../../outside.tar.gz` (with a matching file and checksum at that path) **When** the update's verification fails **Then** the result is `rollback_failed` with reason `snapshot_invalid` and the volume's content is unchanged from just before rollback.
- **Given** a snapshot whose archive the test replaces with a symbolic link to a valid copy of itself **When** restore verification runs **Then** the result is `fail` with reason `snapshot_invalid`.

### BKP-008 Retention
Parachute MUST keep the snapshots of the newest `--backup-retention` transactions per stack (default `3`) and remove older ones after each commit and after each completed rollback (result `rolled_back`), so repeated rollbacks cannot fill the store. After a `rollback_failed` result nothing is removed (VER-010). A directory is counted or removed only when the state file records its snapshot ID, for that host and stack, as written by Parachute in a transaction whose ID equals the manifest's `transaction`, and the directory is valid per BKP-007 item 1. Removal MUST NOT follow symbolic links, and a directory that is itself a symbolic link is never counted or removed.

- **Given** default retention and four committed updates of one stack **When** the fourth commits **Then** exactly three snapshot directories for that stack remain, the newest three.
- **Given** default retention and five transactions of one stack that each rolled back (a newer failing image pushed before each cycle) **When** the fifth rollback finishes **Then** exactly three snapshot directories for that stack remain, the newest three.
- **Given** an unrelated directory `notes/` in the backup volume **When** retention runs **Then** `notes/` still exists.
- **Given** a directory the test created in the backup volume, named like a snapshot ID and holding a copy of a real snapshot's manifest with its `id` changed to that name **When** retention runs after four commits **Then** that directory still exists.
- **Given** a snapshot directory that the test replaced with a symbolic link to a host directory `/srv/keep` **When** retention would remove it **Then** `/srv/keep` and its content still exist.

### BKP-009 Partial snapshots are removed
When a snapshot fails, Parachute MUST remove its partial directory, which is the directory named by the snapshot ID it generated for this transaction and nothing else, and restart any container it stopped for the snapshot.

- **Given** a snapshot that fails while copying **When** the cycle ends **Then** no directory for that snapshot ID remains and the container is running.

### BKP-010 Helper containers
Helper containers created for snapshots, restores, or URL probes (VER-005) MUST use `--helper-image` (default: the image of Parachute's own container; required when Parachute does not run in a container, otherwise startup fails with exit code 2), and MUST be labelled `parachute.helper=true` and with the ID of the work they belong to: `parachute.transaction=<transaction ID>` for snapshots, rollback restores, and URL probes; `parachute.restore-verify=<snapshot ID>` for restore verification (RST-002). Each helper MUST be created:

- not privileged, with no devices, with `no-new-privileges`, all capabilities dropped and at most `CHOWN`, `DAC_OVERRIDE`, `FOWNER`, and `FSETID` added (URL probes: none added);
- with a read-only root file system, a `tmpfs` at `/tmp`, no published ports, restart policy `no`, and no environment variables copied from any other container;
- without the host's network, PID, IPC, or UTS namespace (a URL probe joins only the replacement's network namespace, VER-005);
- with only the mounts its task needs: for a snapshot, each source mount read-only and its own snapshot directory read-write; for a restore, the snapshot directory read-only and the target (BKP-013) read-write; for a URL probe, none. A Docker or Podman API socket is never mounted.

A helper MUST be removed when its task ends, whether it succeeded or failed. Parachute MUST remove a helper only when it carries `parachute.helper=true` and a `parachute.transaction` or `parachute.restore-verify` value naming work this instance started: in the running process, or recorded as open in the state file and recovered at startup (UPD-016, RST-005). Containers matched only by name, image, or `parachute.helper=true` are never removed.

- **Given** any completed cycle **When** the test lists containers labelled `parachute.helper=true` that carry a `parachute.transaction` label set by Parachute in that cycle **Then** none exist.
- **Given** a snapshot of a 200 MiB volume **When** the test inspects each helper container on its `create` event **Then** `Privileged` is false, `ReadonlyRootfs` is true, `CapDrop` contains `ALL`, `CapAdd` is a subset of `CHOWN`, `DAC_OVERRIDE`, `FOWNER`, `FSETID`, `SecurityOpt` contains `no-new-privileges`, `NetworkMode`, `PidMode`, `IpcMode`, and `UTSMode` are not `host`, `PortBindings` is empty, the source volume is mounted read-only, and no mount source is a Docker socket.
- **Given** a container the test created labelled `parachute.helper=true` with no `parachute.transaction` label, and another labelled `parachute.helper=true` and `parachute.transaction=txn-20260101T000000Z-abcdef` (an ID Parachute never generated) **When** Parachute starts and completes a cycle with an update **Then** both containers still exist.
- **Given** Parachute killed while a snapshot helper runs **When** it starts again **Then** after its startup no container carrying that transaction's ID in `parachute.transaction` and `parachute.helper=true` exists.

### BKP-011 Free space is checked before anything stops
Before running `pre-update` hooks or dumps, or stopping any container, Parachute MUST estimate the snapshot size as the sum of the current sizes of the mounts it will copy (BKP-002, after exclusions and BKP-012), counting only containers whose backup mode is not `none`, and MUST refuse the transaction when the free space in the snapshot store (BKP-005) is less than the estimate plus 10 % plus 1 GiB. Sizes and free space are measured on the managed host (by helper containers, BKP-010). Dumps (BKP-004) are not part of the estimate; a dump that fills the store fails the snapshot with reason `snapshot_failed`. A refusal ends the transaction as in UPD-014 with result `failed` and reason `insufficient_space`, changes nothing, and its report entry states the estimated and free bytes. When a size or the free space cannot be measured, the transaction ends the same way with reason `snapshot_failed` and a detail naming the measurement that failed. A transaction whose affected containers all have backup mode `none` skips the check.

- **Given** `--backup-volume` on a host path backed by a 2 GiB `tmpfs` and a container whose named volume holds 1.5 GiB **When** a cycle runs **Then** the result is `failed` with reason `insufficient_space`, the container keeps its ID and image, no `die` event was emitted for it, no `pre-update` hook ran, no snapshot directory was created, and the entry contains both byte counts.
- **Given** the same store and a volume holding 100 MiB **When** a cycle runs **Then** the update proceeds and commits.
- **Given** the same store, a container whose volume holds 1.5 GiB, and `parachute.backup=none` **When** a cycle runs **Then** the update proceeds and commits.

### BKP-012 System mounts are never copied
Parachute MUST NOT copy into a snapshot, or restore into, a **system mount**: a bind mount whose source, after symbolic links are resolved on the host, is `/`, `/etc`, `/usr`, `/boot`, `/var`, or `/var/lib`; is `/proc`, `/sys`, `/dev`, `/run`, `/var/run`, `/var/lib/docker`, or `/var/lib/containers` or a path under one of them; is a Docker or Podman API socket; or is the snapshot store (BKP-005) or a directory containing it. A volume whose `local`-driver `device` option names such a path is a system mount too. No label can include a system mount. Each system mount of an affected container MUST be logged at `warn` naming the container, destination, and source; listed in the manifest under `skipped` (per mount: container, destination, source); and named in the detail of the container's report entry. A skipped system mount does not fail the transaction. If the transaction rolls back, the rollback report names it as `data not restored: system mount <destination>`.

- **Given** a container with writable bind mounts of `/etc` at `/host-etc`, `/proc` at `/host-proc`, and `/srv/app` at `/data` **When** a snapshot is taken **Then** the manifest's `mounts` lists only `/data`, its `skipped` lists `/host-etc` and `/host-proc`, one `warn` line names each of them, and the result is `updated`.
- **Given** a host symbolic link `/srv/etc-link` pointing to `/etc`, bind-mounted writable at `/cfg` **When** a snapshot is taken **Then** `/cfg` is listed under `skipped` and not under `mounts`.
- **Given** `--backup-volume /srv/parachute` and a container with `/srv` bind-mounted writable at `/srv` **When** a snapshot is taken **Then** `/srv` is listed under `skipped`.
- **Given** a local volume created with `--opt type=none --opt o=bind --opt device=/etc` mounted at `/conf` **When** a snapshot is taken **Then** `/conf` is listed under `skipped`.
- **Given** a container with `/etc` bind-mounted writable at `/host-etc` whose update fails verification **When** rollback finishes **Then** the report entry contains `data not restored: system mount /host-etc`.

### BKP-013 Restores stay inside their target
When restoring data from a snapshot (rollback or restore verification), Parachute MUST write each archive only into its **target**: in rollback, the mount at the manifest's destination of the original container as the engine reports it now; in restore verification, the throwaway volume (RST-002). The manifest's `source` MUST NOT be used as a write target. Before writing anything to a target, Parachute MUST check every member of the archive and MUST reject the whole snapshot as `snapshot_invalid`, writing nothing from it, when any member has an absolute path or a `..` component, is a block or character device, or is a symbolic or hard link whose target resolves outside the target.

- **Given** a snapshot whose archive the test replaces after the replacement's `start` event with one containing the member `../escape` and updates the manifest's `sha256` and size to match **When** the update's verification fails **Then** the result is `rollback_failed` with reason `snapshot_invalid`, no file named `escape` exists in the volume's parent directory on the host, and the volume's content is unchanged from just before rollback.
- **Given** a snapshot whose archive, with matching manifest checksum, contains a symbolic link `link -> /etc` and a member `link/passwd` **When** restore verification runs **Then** the result is `fail` with reason `snapshot_invalid` and the host's `/etc/passwd` is unchanged.
- **Given** a snapshot whose manifest the test changes so that a mount's `source` is `/srv/victim` (a host directory with known content) **When** rollback restores that snapshot **Then** `/srv/victim` is unchanged and the original container's own mount receives the data.
