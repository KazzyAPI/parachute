# 18 Podman

**Status:** Deferred

Running against Podman instead of Docker.

## Podman

The requirements in this file are deferred.

### DMN-011 Podman's Docker-compatible API
Parachute MUST work against Podman's Docker-compatible API socket (rootful `unix:///run/podman/podman.sock`, rootless `unix://$XDG_RUNTIME_DIR/podman/podman.sock`) with the same observable behaviour as against Docker for every requirement in these specs, except where DMN-012 and DMN-014 say otherwise.

- **Given** the Milestone 1 black-box suite run with `DOCKER_HOST` set to a Podman socket **When** it runs **Then** every test passes except those listed as runtime-specific in DMN-012 and DMN-014.

### DMN-012 Podman pods
Containers in a Podman pod MUST form one stack named after the pod, unless they carry Compose project labels (DEP-001), which take precedence. Parachute MUST NOT update a pod's infra container and MUST refuse (UPD-007, reason `config_unreproducible`) a container whose recreation would require recreating the pod.

- **Given** pod `p` with containers `app` and `cache` and an update for `app` **When** a cycle runs **Then** the report lists stack `p`, and the infra container is unchanged.

### DMN-014 Rootless storage
Under rootless Podman, `--backup-volume` given as a host path MUST be inside a directory the rootless user can write; otherwise the snapshot fails with reason `snapshot_failed` and a message naming the path.

- **Given** rootless Podman and `--backup-volume /root/snaps` **When** an update runs **Then** the result is `failed` with reason `snapshot_failed` naming `/root/snaps`.
