# 05 Dependencies and stacks

**Status:** Draft

How containers are grouped and ordered ([ADR-0006](../adr/0006-compose-is-the-unit-of-update.md)). The **service** is the unit of update; the **stack** is the unit of snapshot, verification, and rollback.

### DEP-001 Stack identification
Containers on the same host with the same `com.docker.compose.project` label MUST form one stack, named after that label. Within a stack, containers with the same `com.docker.compose.service` label form one service. Parachute MUST NOT need the Compose file or the Compose CLI to identify stacks.

- **Given** a Compose project `shop` with services `web` and `db`, started with `docker compose up -d`, and the Compose file then deleted **When** a cycle runs **Then** the report lists stack `shop` with services `web` and `db`.

### DEP-002 Containers outside Compose
A container without a `com.docker.compose.project` label MUST form a stack of one, named after the container, and its service is named after the container too.

- **Given** a container `solo` started with `docker run` **When** a cycle runs **Then** the report lists stack `solo` with one service `solo`.

### DEP-003 One transaction per stack
All updates detected in one stack in one cycle MUST be applied in a single transaction (UPD-001): one snapshot, one verification, and, on failure, one rollback of every container changed by that transaction. When a dependency (DEP-004) links containers in different stacks, those stacks MUST be joined into one transaction. The one exception is Parachute's own container, which is always updated in a transaction of its own (UPD-018).

- **Given** stack `shop` where both `web` and `worker` have newer images and `worker`'s new image fails verification **When** a cycle runs **Then** both `web` and `worker` are back on their previous images with their original container IDs, and the report shows one transaction ID for both with result `rolled_back`.
- **Given** container `app` in stack `a` with label `parachute.depends-on=cache` where `cache` is a stack of one, and an update for `cache` **When** a cycle runs **Then** `app` and `cache` share one transaction ID.

### DEP-004 Dependency sources
A container B **depends on** container A when any of these hold:
1. B has a Docker link to A.
2. B's `parachute.depends-on` label (comma-separated container names) names A.
3. B's network mode is `container:<A>`.
4. B is a Compose service whose `com.docker.compose.depends_on` label names A's service in the same stack.

When B has a `parachute.depends-on` label, sources 1 and 4 MUST be ignored for B (the label replaces them); source 3 always applies.

- **Given** container `a` labelled `parachute.depends-on=b,c` and updates for `b` and `c` **When** a cycle runs **Then** `a` is stopped before `b` and `c` are stopped, and started after both are running.
- **Given** Compose service `app` with `depends_on: [db]` and an update for `db` **When** a cycle runs **Then** `app` is stopped before `db` and started after `db`.
- **Given** `web` with a Docker link to `db` and label `parachute.depends-on=cache` **When** `db` is updated **Then** `web` is not stopped.

### DEP-005 Stop and start order
Within a transaction, Parachute MUST stop affected containers dependents-first and start them dependencies-first. For a Compose `depends_on` entry with condition `service_healthy`, the dependent MUST NOT be started until the dependency reports `healthy`, and with `service_completed_successfully` until it has exited with code 0.

- **Given** `web` linked to `db` and an update for `db` **When** a cycle runs **Then** the Docker event stream shows: `web` stop, `db` stop, new `db` start, `web` start.
- **Given** `app` with `depends_on: {db: {condition: service_healthy}}` and an update for `db` **When** a cycle runs **Then** `app`'s start event comes after `db`'s `health_status: healthy` event.

### DEP-006 Only updated services and their dependents restart
Parachute MUST stop and start only the containers being updated and their direct and transitive dependents. Other containers in the stack MUST keep running with unchanged IDs and no restart.

- **Given** stack `shop` with `web` depending on `db`, an unrelated `cache`, and an update only for `web` **When** a cycle runs **Then** only `web` is restarted; `db` and `cache` have the same container IDs, start times, and restart counts as before.

### DEP-007 Network-mode dependents are recreated
A dependent whose network mode is `container:<A>` MUST be recreated (not only restarted) so that it joins the replacement of A, keeping its own image and configuration (UPD-003).

- **Given** `sidecar` with network mode `container:app` and an update for `app` **When** the cycle commits **Then** `sidecar` has a new container ID, the same image, and its network mode refers to the new `app` container.

### DEP-008 Dependency cycles
A dependency cycle MUST NOT hang or crash Parachute. Every container on the cycle MUST get result `failed` with reason `dependency_cycle`, the report MUST name the containers on the cycle, and other stacks MUST still be processed.

- **Given** `a` labelled `parachute.depends-on=b`, `b` labelled `parachute.depends-on=a`, updates for both, and an unrelated `c` with an update **When** a cycle runs **Then** the cycle ends within 60 seconds of its start, `a` and `b` are `failed` with reason `dependency_cycle` and unchanged, and `c` is `updated`.

### DEP-009 Replicas of a service
All containers of a service (replicas, distinguished by `com.docker.compose.container-number`) MUST be updated in the same transaction and each MUST keep its container number and name.

- **Given** service `worker` scaled to 2 and a newer image **When** the cycle commits **Then** both replicas use the new image and keep their names `<project>-worker-1` and `<project>-worker-2`.

### DEP-010 Recreate without Compose
Parachute MUST recreate Compose containers from their stored container configuration (UPD-003), never by running `docker compose`. After an update the stack MUST still be listed by `docker compose ls` and its containers by `docker compose -p <project> ps`.

- **Given** a Compose stack, its Compose file removed, and an update for one service **When** the cycle commits **Then** `docker compose -p <project> ps` lists the updated container with the new image.

### DEP-011 Drift detection
When a stack's Compose file is readable by Parachute at the path recorded in the container label `com.docker.compose.project.config_files`, Parachute MUST compare each service's `image:` value with the reference the running container uses and report a difference as **drift** in the report and status view (STS-004). Parachute MUST NOT modify the file. An `image:` value containing `$` MUST be reported as drift `unknown`. No file or an unreadable file means no drift check, without error.

- **Given** a Compose file mounted into Parachute at its original path declaring `image: localhost:5000/fixture/app:2` while the container runs `...:1` **When** a cycle runs **Then** the status view shows drift for that service naming both references, and the file's checksum is unchanged.
- **Given** `image: localhost:5000/fixture/app:${TAG}` **When** a cycle runs **Then** the status view shows drift `unknown` for that service.
