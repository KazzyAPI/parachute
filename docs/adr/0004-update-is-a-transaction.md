# ADR-0004: An update is a transaction

- **Status:** Accepted (amended 2026-10-23, see Amendment)
- **Date:** 2026-10-22

## Context
The simple way to update a container is to stop it, remove it, and start a new one. If the new one is broken, the old one is gone, its data may already be migrated to an incompatible format, and nothing tells the operator. This is the main reason operators distrust automatic updates.

## Decision
Every update of a container (or stack, see ADR-0006) is a transaction with these phases, in this order:

1. **Detect** a newer image.
2. **Check policy** (delay, pin, approval, protected data; see POL-*).
3. **Snapshot** the data and the container definition (BKP-*).
4. **Update**: pull, stop, recreate.
5. **Verify**: the new container must become healthy within a deadline (VER-*).
6. **Commit or roll back**: on success, clean up. On failure, restore the previous container and, if the data changed, the snapshot.

Rules:
1. The previous image and the previous container definition are kept until step 6 commits.
2. A failed phase before step 4 leaves everything untouched.
3. A failed rollback is reported loudly (error-level notification, non-zero exit in run-once mode) and never silently ignored.
4. Every transaction has an ID that appears in all logs and notifications for that update.

## Consequences
- Updates are slower and use more disk.
- Image cleanup (`--cleanup`) moves to after commit (UPD-010).
- "Roll back" is only as good as the snapshot. Snapshots of running databases must be consistent (BKP-003, BKP-004).
- A consistent snapshot of a stopped container needs the container stopped before the copy, and a failed pull must not cause downtime. UPD-001 therefore pulls the image and stops containers at the start of the snapshot phase; the phase names and their order are unchanged.

## Specs affected
UPD-*, BKP-*, VER-*, POL-*, DEP-003.

## Amendment (2026-10-23)
The phases keep their names and order, but steps 1 and 2 run before the transaction exists: a transaction, and its ID, start at the snapshot phase, and only when at least one update of the stack passes the policy check. A cycle in which every update is held or no update is detected creates no transaction and no ID, and its log lines carry none; rule 4 applies from the snapshot phase on. Specs affected by this amendment: UPD-001, UPD-014, UPD-016, POL-001, HOK-003, STS-002, CFG-008.
