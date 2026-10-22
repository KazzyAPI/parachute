# 03 Update detection

**Status:** Draft

How Parachute decides that a candidate has an update. Detection never changes containers. Results of detection feed policies ([09-update-policies.md](09-update-policies.md)) and the transaction ([04-update-transaction.md](04-update-transaction.md)).

Each candidate ends a cycle in exactly one **result state**: `fresh`, `updated`, `would_update`, `held`, `skipped`, `failed`, `rolled_back`, or `rollback_failed`. The states are defined in CFG-007 and used in reports, porcelain output, the status view, and metrics.

### DET-001 Image comparison
For each candidate, Parachute MUST determine whether the registry holds a different image for the candidate's image reference (repository and tag) than the image the container was created from. Equal images MUST give result `fresh`.

- **Given** a container created from `localhost:5000/fixture/app:1` and a different image later pushed to the same tag **When** a cycle runs **Then** an update is detected for that container.
- **Given** no change in the registry **When** a cycle runs **Then** the container's result is `fresh`.
- **Given** a container whose image reference has no tag **When** a cycle runs **Then** it is compared against tag `latest`.

### DET-002 Digest check before pull
Parachute MUST compare the registry's manifest digest (from a manifest `HEAD` request) with the digests recorded for the local image, and MUST NOT download image layers during detection. Registries on loopback addresses (`localhost`, `127.0.0.0/8`, `::1`) are contacted over plain HTTP, as the Docker engine does by default; all others over HTTPS. Layers are downloaded only when an update is applied (UPD-001).

- **Given** an unchanged image in the registry **When** a cycle runs **Then** the registry access log shows no blob `GET` requests for that repository.
- **Given** a changed image and an unarmed Parachute **When** a cycle runs **Then** the registry access log shows no blob `GET` requests and the local image list is unchanged.

### DET-003 Digest request fallback and warnings
If the manifest `HEAD` request fails, Parachute MUST fall back to a manifest `GET`; if that also fails, the candidate's result is `failed` with reason `registry_unreachable`. `--warn-on-head-failure` controls logging of `HEAD` failures and accepts `always`, `auto`, `never`; the default is `auto`. `always` logs every failure at `warn`; `never` logs at `debug`; `auto` logs at `warn` for `docker.io` (including `index.docker.io` and `registry-1.docker.io`) and `ghcr.io`, and at `debug` for all other registries. Any other value is a startup error (exit code 2).

- **Given** a registry proxy that rejects `HEAD` but serves `GET`, a changed image, and `--warn-on-head-failure never` **When** a cycle runs **Then** the update is still detected and no `warn` line mentions the failed request.
- **Given** the same with `always` **When** a cycle runs **Then** one `warn` line per failed `HEAD` request names the registry host.
- **Given** the same with `auto` and the registry at `localhost:5000` **When** a cycle runs **Then** no `warn` line mentions the failed request.
- **Given** a registry that is offline **When** a cycle runs **Then** the candidate's result is `failed` with reason `registry_unreachable`, and other candidates are still processed.

### DET-004 No-pull mode
With `--no-pull` on, or for a container labelled `parachute.no-pull=true` (precedence per SEL-008), Parachute MUST NOT contact registries for that container and MUST compare only against images already present locally under the same reference.

- **Given** `--no-pull`, armed, and a newer image tagged locally under the container's reference by another process **When** a cycle runs **Then** the container is updated to that local image.
- **Given** `--no-pull` and a newer image only in the registry **When** a cycle runs **Then** the result is `fresh` and the registry access log shows no requests from Parachute.

### DET-005 Digest-pinned references are never moved
A container whose image reference contains `@sha256:` MUST get result `skipped` with reason `digest_pinned` and MUST NOT be changed.

- **Given** a container created from `localhost:5000/fixture/app@sha256:<d>` and a newer image on tag `1` **When** a cycle runs **Then** the result is `skipped` with reason `digest_pinned` and the container is unchanged.

### DET-006 Images without a registry digest
A container whose image has no registry digest (built locally and never pushed or pulled), outside no-pull mode, MUST get result `skipped` with reason `local_image`. The same applies when the registry answers that it has no manifest for the reference (`404`): with the containerd image store the engine reports a digest for every local image, so an image that was never pushed is only visible as a reference the registry does not know.

- **Given** a container created from an image built locally as `<local registry>/local/app:dev` and never pushed **When** a cycle runs **Then** the result is `skipped` with reason `local_image`.

### DET-007 Detection is reported even when not applied
An update detected for a container that is not changed in this cycle (monitor-only, policy hold, unarmed) MUST still appear in the report with the candidate image digest.

- **Given** a monitor-only container with a newer image **When** a cycle runs **Then** the report entry contains the candidate digest `sha256:...` of the new image.
