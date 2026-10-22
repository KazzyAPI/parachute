# ADR-0014: Self-update safety

- **Status:** Accepted
- **Date:** 2026-10-23

## Context
Parachute can update its own container. That container holds the Docker socket, so a malicious or broken image of Parachute controls every managed host. A self-update is also the one case where two Parachute instances run side by side on one state directory (ADR-0008), and the instance that runs the transaction is the one being replaced: it cannot stop itself for a snapshot, and if it dies mid-transaction no one is left to roll back. The earlier UPD-018 text did not match the transaction state machine, trusted the `parachute.self` label (which an image can carry) to identify the own container, and trusted `handover.json` without checks.

## Decision
1. **Always approved.** A self-update is never automatic. It is held with `approval_required` until the operator approves its digest (API-007). No profile, flag, or label (`convenient`, `--protect-databases=false`, `parachute.approval=auto`) removes the hold.
2. **Digest pinned.** The replacement is pulled as `<repository>@<approved digest>` and the pulled digest is checked against it (UPD-020, ADR-0012). The tag is never pulled.
3. **Own container by identity.** The own container is the container the instance runs in, as reported by the engine. A `parachute.self=true` label never makes another container the instance's own.
4. **One state machine.** A self-update follows the glossary's transaction state machine: `snapshot -> handover -> committed | rolling_back | rollback_failed`. The `handover` phase replaces `update` and `verify`.
5. **Old instance owns everything until commit.** The old instance records `handover`, writes the marker, renames itself (still running), creates and verifies the replacement, records the outcome, and only then removes the marker. After a commit it never touches the state file or a container again; the new instance removes it.
6. **No data snapshot.** The own container is always snapshotted in backup mode `none`. The new instance writes nothing in the state directory before it owns the state file, so a rollback never needs a data restore.
7. **Marker is untrusted.** `handover.json` has a format version and is validated against the engine: it must name the starting instance's own container, or the starting instance's container must carry the marker's transaction ID and run the marker's digest. Anything else is a startup error (exit 2) and the file is kept for the operator.
8. **Removal needs proof.** The new instance removes the old container only when its ID, `parachute.self` label, and `<name>-parachute-<suffix>` name all match the committed transaction (UPD-017).
9. **Failure is loud and frozen.** If the old instance disappears during `handover`, the new instance records `rollback_failed` / `interrupted`, sends an `error` notification, changes no container, and the stack stays frozen until approval. A superseded instance that is started again exits with code 2.
10. **Side by side must be possible.** An own container that publishes host ports or uses a static IP address cannot run beside its replacement, so its self-update fails with `config_unreproducible` and nothing changes.

## Consequences
- Every Parachute upgrade needs one approval call; until the HTTP API exists, Parachute cannot update itself at all.
- A Parachute container that publishes ports (for example for the HTTP API) must be upgraded by the operator, or run without published ports behind another container.
- A crash of the old instance during `handover` needs an operator: two instances must never guess who owns the state file.
- The handover marker format is now part of the interface (`parachute.handover/v1`).

## Specs affected
UPD-001, UPD-008, UPD-009, UPD-016, UPD-017, UPD-018, UPD-019, UPD-020, DEP-003, VER-001, VER-010, SEL-012, API-007, CFG-010, CFG-011.
