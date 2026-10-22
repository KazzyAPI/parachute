# 15 Restore verification

**Status:** Draft

An optional, scheduled proof that the latest snapshot of each stack can actually be restored and booted. It never touches live containers or volumes.

### RST-001 Schedule
With `--restore-verify-schedule <cron>` (6-field, SCH-002 syntax) set, Parachute MUST run a restore verification at each match. Without it (default), no restore verification runs. `parachute restore-verify [--stack <name>] [--host <name>]` MUST run one immediately, exit 0 when every verified stack passes, 1 when any fails, and 2 on invalid arguments.

- **Given** no restore-verify schedule, `--interval 10`, and a valid snapshot for stack `shop` **When** Parachute runs three cycles **Then** the Docker event stream shows no `create` event for a container labelled `parachute.restore-verify`.
- **Given** a valid snapshot for stack `shop` **When** `parachute restore-verify --stack shop` runs **Then** it exits 0.

### RST-002 Throwaway resources
For each stack, Parachute MUST restore the stack's newest snapshot, among those it counts for retention (BKP-008), into new volumes and containers named `parachute-rv-<suffix>-<original-name>` (suffix: last six characters of the snapshot ID), attached only to a new internal network `parachute-rv-<suffix>` on which each container has its service name as alias. Every throwaway container, volume, and network MUST be labelled `parachute.helper=true` and `parachute.restore-verify=<snapshot-id>`. The snapshot's recorded configuration is untrusted (BKP-007): a throwaway container MUST take from it only the image, command, entrypoint, environment, user, working directory, health check, labels (with all `com.docker.compose.*` labels removed), and mount destinations, each mount being a throwaway volume; it MUST have no published ports, restart policy `no`, no bind mounts, no devices, no added capabilities, not be privileged, and not share any host namespace. Containers use the images recorded in the snapshot, pulled by digest when missing.

- **Given** a restore verification in progress **When** the test inspects its containers **Then** each is named `parachute-rv-*`, has no published ports, is attached only to an internal network, and has no `com.docker.compose.project` label.
- **Given** a snapshot whose manifest the test changed so that a container's configuration has `Privileged: true`, `NetworkMode: host`, and a bind mount of `/` at `/host` (checksums of archives unchanged) **When** restore verification runs **Then** each throwaway container has `Privileged` false, a `NetworkMode` other than `host`, and no bind mount.

### RST-003 Procedure and result
Before restoring, Parachute MUST check the snapshot (BKP-007) and restore only as BKP-013 allows. After restoring data, Parachute MUST start the throwaway containers dependencies first and apply the verification of VER-001 to VER-006, using each original container's labels and defaults. The stack **passes** when all of them pass, and **fails** otherwise, or when the snapshot is invalid, or when restoring fails.

- **Given** a snapshot whose app container needs `db` reachable as `db` and becomes healthy only then **When** restore verification runs **Then** the result is `pass`.
- **Given** a snapshot whose archive the test corrupted **When** restore verification runs **Then** the result is `fail` with reason `snapshot_invalid`.

### RST-004 Live containers are never touched
Restore verification MUST NOT stop, restart, rename, or modify any container outside its throwaway set, nor write to any volume or bind mount other than its own throwaway volumes and the snapshot store (read-only access).

- **Given** a live stack with recorded container IDs, start times, and volume checksums **When** restore verification runs and passes, and again when it fails **Then** all of those are unchanged.

### RST-005 Cleanup
Every throwaway container, volume, and network MUST be removed when the verification of that stack ends, whatever the result, and leftovers from an interrupted run MUST be removed at the next start. The state file records each run as open before its first throwaway resource is created. Parachute MUST remove only resources that carry `parachute.helper=true` and a `parachute.restore-verify` value naming a run this instance started in the running process or recorded as open; a name starting with `parachute-rv-` alone never makes a resource removable.

- **Given** a completed restore verification **When** the test lists containers, volumes, and networks labelled `parachute.restore-verify` **Then** none exist.
- **Given** Parachute killed during a restore verification **When** it starts again **Then** within its startup no resource labelled with that run's `parachute.restore-verify` value remains.
- **Given** a container, a volume, and a network the test created named `parachute-rv-abcdef-keep` without Parachute labels **When** a restore verification runs and Parachute restarts **Then** all three still exist.

### RST-006 Reporting
Each run MUST send one notification (NTF-002) listing each stack's result (`pass` or `fail`, with reason) and the age of its last passing restore; record results in the state file for the status view (STS-004); and update `parachute_restore_verification_last_success_timestamp_seconds` (API-008). A failure is a `warn` entry; a pass is `info`.

- **Given** a passing restore verification **When** it ends **Then** the webhook receives one notification naming the stack and `pass`, and the metric for that stack equals the run's end time within 5 seconds.

### RST-007 No overlap with cycles
A restore verification and a cycle MUST NOT run at the same time. Whichever is scheduled while the other runs is skipped and logged at `info`.

- **Given** a restore verification in progress **When** a scheduled cycle start occurs **Then** the cycle is skipped and `parachute_scans_skipped` increases by 1.
