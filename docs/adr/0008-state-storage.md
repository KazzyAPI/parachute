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
