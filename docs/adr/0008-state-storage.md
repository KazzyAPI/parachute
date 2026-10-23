# ADR-0008: State storage layout

- **Status:** Accepted (amended 2026-10-23, see Amendment)
- **Date:** 2026-10-22

## Context
ADR-0002 rules out a database. Some facts must still survive a restart: first-seen times for the delay policy, approvals, open transactions for crash recovery, last results for the status view, reminder times, and restore-verification results. The layout must survive crashes and be inspectable by an operator.

## Decision
1. Facts about a specific container that never change after it is created are stored as labels on that container (`parachute.transaction`, `parachute.previous-image`, `parachute.snapshot`, `parachute.updated-at`).
2. Everything else lives in one JSON file, `<state-dir>/state.json`, with a top-level `"format": "parachute.state/v1"`.
3. The file is replaced atomically (write a temporary file in the same directory, `fsync`, rename). It is never edited in place.
4. An open transaction is written to the file before its first change to a container and removed when it commits or finishes rolling back.
5. A file Parachute cannot parse is never overwritten; Parachute refuses to start.
6. Snapshot data is not in the state file; it lives in the snapshot store (ADR-0009).
7. The state file has exactly one owner at a time. During a self-update the old instance owns it until its transaction has committed or rolled back. It signals this with a handover marker, `<state-dir>/handover.json` (transaction ID and the old container's ID): the old instance writes it before starting the new instance and removes it after closing the transaction, and never touches the state file again after a successful handover. An instance that starts while the marker exists neither reads nor writes `state.json` nor recovers open transactions until the marker is gone (UPD-018).

## Consequences
- One file to back up and to inspect; no migrations beyond a `format` version.
- The file is a single point of failure for history; losing it loses delays, approvals, and recovery data, but never container or snapshot data.
- Only one Parachute instance may use a state directory at a time; the self-update handover is the one case where two run side by side, and the marker keeps the new one from acting on the old one's open transaction.

## Specs affected
CFG-010, UPD-016, UPD-018, POL-002, POL-006, NTF-011, STS-001, RST-006.

## Amendment (2026-10-23)
The state file and `handover.json` are untrusted input ([glossary: Trust boundaries](../specs/glossary.md#trust-boundaries)): they are parsed strictly, never arm Parachute (CFG-006), and the status view never treats an approval recorded in them as having released a hold (STS-006).

Accepted limit for v1 (2026-10-23): write access to the state directory is trusted. Parachute cannot tell a state file it wrote from an older valid `state.json` that someone with write access restored, so such a replay can reset first-seen times (POL-002), remove a freeze or a failed-digest hold (POL-010), or bring back a recorded approval, which still applies only to the digest it names while that digest is the current candidate digest (POL-006, API-007). Detecting a replay (for example an HMAC keyed outside the state directory) is out of scope for v1. Operators protect the state directory like the Docker socket ([ADR-0010](0010-docker-socket-access-and-security.md)).

Restore verification (2026-10-23): restore-verification runs are recorded only in `<state-dir>/restore-verify.json` (`parachute.restore-verify/v1`, replaced atomically), never in `state.json`, so `state.json` keeps one owner even when `parachute restore-verify` runs beside a daemon. `restore-verify.json` is written, by replacing, only by the process holding the restore-verification lock, an exclusive lock on the separate, never-replaced file `<state-dir>/restore-verify.lock` (one writer at a time: the command, a scheduled run, or the daemon's startup cleanup); the command runs standalone and exits 2 when another process holds the lock; the daemon reads the file for cleanup and the status view. Each run has its own ID `rv-<timestamp>-<rand>`, used as the `parachute.restore-verify` label value. Specs affected by this amendment: CFG-010, RST-001, RST-002, RST-005, RST-006, RST-007, BKP-010, UPD-017.

Terminal transactions (2026-10-23): Decision 4 removes only the open-state recovery data of a transaction when it commits or finishes rolling back. Its terminal state, result, and reason stay recorded until the next transaction of the same stack supersedes them, so the status view can show `last_transaction` (STS-002). The old container ID of a self-update that ended `committed`, or `rollback_failed` with reason `interrupted`, is kept in its own state-file list, `superseded_instances`, separate from `last_transaction`, so a later transaction of the same stack never removes it; an entry is pruned only when the engine no longer has that container. A superseded instance recognises itself from this list (UPD-018). Specs affected by this amendment: UPD-016, UPD-018, STS-002, CFG-010.
