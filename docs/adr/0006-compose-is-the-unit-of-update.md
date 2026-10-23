# ADR-0006: The Compose stack is the unit of update

- **Status:** Accepted
- **Date:** 2026-10-22

## Context
Most self-hosters run apps as Docker Compose stacks (app plus database plus cache). Updating one container at a time ignores dependencies and can leave a stack half-updated. Restarting the whole stack for one changed service causes needless downtime.

## Decision
1. Parachute identifies a stack by Compose's container labels (`com.docker.compose.project`, `com.docker.compose.service`). We never need the original compose file to detect a stack.
2. For containers in a stack, the **service** is the unit of update and the **stack** is the unit of snapshot, verify, and rollback.
3. Updating one service restarts only that service and the services that depend on it (per `depends_on` as recorded in Compose's container labels), not the whole stack.
4. Containers not created by Compose are treated as a stack of one.
5. Parachute recreates containers using the **stored container configuration**, not by invoking `docker compose`, so it works without the compose file or CLI on the host. Where the compose file is mounted, it MAY be used to detect drift (DEP-011).

## Consequences
- Parachute must reproduce a container's full configuration faithfully when recreating it. Anything it cannot reproduce must be detected and must block the update (UPD-007).
- Stacks defined by `docker compose` stay editable by the user; Parachute only changes the image in use at runtime and records the change on the container (UPD-005, POL-005).

## Specs affected
DEP-*, UPD-*, BKP-002, VER-007.
