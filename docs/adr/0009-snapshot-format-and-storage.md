# ADR-0009: Snapshot format and storage

- **Status:** Accepted
- **Date:** 2026-10-22

## Context
Every update needs a restore point (ADR-0004, ADR-0007). Snapshots must be readable without Parachute, checkable for corruption, fast to restore on the host where the containers run, and possible on remote hosts reached only through the Engine API.

## Decision
1. Snapshots are stored on the same host as the containers they protect, in a Docker volume (default `parachute-snapshots`) or a host path.
2. One directory per snapshot, named by its ID. It holds `manifest.json` (format `parachute.snapshot/v1`), one gzip-compressed tar archive per mount, and one file per database dump.
3. The manifest records each archive's SHA-256 and size. A snapshot is valid only when every checksum matches.
4. Data is copied by short-lived helper containers on that host, labelled `parachute.helper=true`, so no data crosses the network and no agent is installed.
5. Parachute deletes only directories whose manifest it wrote.

## Consequences
- Restores are local and fast; a down Parachute host does not lose restore points.
- Snapshots use disk on each host, bounded by retention.
- An operator can restore by hand with `tar`, without Parachute.
- Off-host copies are not provided; that is a later decision (open question Q4).

## Specs affected
BKP-002, BKP-005, BKP-006, BKP-007, BKP-008, BKP-010, RST-002.
