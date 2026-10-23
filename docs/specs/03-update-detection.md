# 03 Update detection

**Status:** Draft

How Parachute decides that a candidate has an update. Detection never changes containers. Results of detection feed policies ([09-update-policies.md](09-update-policies.md)) and the transaction ([04-update-transaction.md](04-update-transaction.md)).

Each candidate ends a cycle in exactly one **result state**: `fresh`, `updated`, `would_update`, `held`, `skipped`, `failed`, `rolled_back`, or `rollback_failed`. The states are defined in CFG-007 and used in reports, porcelain output, the status view, and metrics; the states and reason codes are listed in [reasons.md](reasons.md). Registry responses and image metadata are untrusted ([glossary: Trust boundaries](glossary.md#trust-boundaries)).

### DET-001 Image comparison
For each candidate, Parachute MUST determine whether the registry holds a different image for the candidate's image reference (repository and tag) than the image the container was created from. Equal images MUST give result `fresh`.

- **Given** a container created from `localhost:5000/fixture/app:1` and a different image later pushed to the same tag **When** a cycle runs **Then** the container's result is `would_update` when unarmed (`updated` when armed) and its report entry has a candidate digest different from the running image's digest.
- **Given** no change in the registry **When** a cycle runs **Then** the container's result is `fresh`.
- **Given** a container whose image reference has no tag **When** a cycle runs **Then** it is compared against tag `latest`.

### DET-002 Digest check before pull
Parachute MUST compare the registry's manifest digest (from a manifest `HEAD` request) with the digests recorded for the local image, and MUST NOT download image layers during detection. Registries on loopback addresses (`localhost`, `127.0.0.0/8`, `::1`) are contacted over plain HTTP, as the Docker engine does by default; all others over HTTPS. Layers are downloaded only when an update is applied (UPD-001).

- **Given** an unchanged image in the registry **When** a cycle runs **Then** the registry access log shows no blob `GET` requests for that repository.
- **Given** a changed image and an unarmed Parachute **When** a cycle runs **Then** the registry access log shows no blob `GET` requests and the local image list is unchanged.

### DET-003 Digest request fallback and warnings
A manifest `HEAD` request **fails** when, after authentication (REG-001, REG-004) and redirects (REG-008) are handled, it ends in a transport error, a timeout, or a status other than `200`. Then Parachute MUST fall back to a manifest `GET`. A `200` `HEAD` response is not a failure: when its `Docker-Content-Digest` header is missing or invalid (DET-008), the result is `failed` with reason `registry_unreachable`, no `GET` is sent, and the response is logged like a `HEAD` failure under `--warn-on-head-failure`. If the `GET` also fails, the result MUST be: `skipped` with reason `local_image` when the registry answered `404` for the manifest (DET-006); `failed` with reason `registry_unauthorized` when the registry or its token endpoint answered `401` or `403` (REG-001, REG-004); otherwise `failed` with reason `registry_unreachable` (transport error, timeout, TLS error, `5xx` or other unexpected status, malformed or oversized response (REG-007), invalid token realm, or a digest that fails DET-008's checks). `--warn-on-head-failure` controls logging of `HEAD` failures and accepts `always`, `auto`, `never`; the default is `auto`. `always` logs every failure at `warn`; `never` logs at `debug`; `auto` logs at `warn` for `docker.io` (including `index.docker.io` and `registry-1.docker.io`) and `ghcr.io`, and at `debug` for all other registries. Any other value is a startup error (exit code 2).

- **Given** a registry proxy that rejects `HEAD` but serves `GET`, a changed image, and `--warn-on-head-failure never` **When** a cycle runs **Then** the update is still detected and no `warn` line mentions the failed request.
- **Given** the same with `always` **When** a cycle runs **Then** one `warn` line per failed `HEAD` request names the registry host.
- **Given** the same with `auto` and the registry at `localhost:5000` **When** a cycle runs **Then** no `warn` line mentions the failed request.
- **Given** a registry that is offline **When** a cycle runs **Then** the candidate's result is `failed` with reason `registry_unreachable`, and other candidates are still processed.
- **Given** a registry that answers `500` to both `HEAD` and `GET` for the manifest **When** a cycle runs **Then** the result is `failed` with reason `registry_unreachable`.
- **Given** a registry that answers `403` to both `HEAD` and `GET` for the manifest **When** a cycle runs **Then** the result is `failed` with reason `registry_unauthorized`.

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

### DET-008 The candidate digest is recorded and binds the update
For each detected update Parachute MUST record one **candidate digest**: the manifest digest that the tag resolved to in this cycle's detection. From a `HEAD` response it is the `Docker-Content-Digest` header; a `HEAD` response without that header, or whose header does not match `sha256:[0-9a-f]{64}`, gives `failed` with reason `registry_unreachable`, with no fallback to `GET` (DET-003); from a `GET` response it is the `sha256` of the response body, and when the `GET` response also carries `Docker-Content-Digest` the two MUST be equal. A digest that does not match `sha256:[0-9a-f]{64}`, or a mismatch, gives `failed` with reason `registry_unreachable`. The candidate digest is the one reported (DET-007), the one the delay (POL-002), approval (POL-006, API-007), and failed-digest hold (POL-010) are keyed on, and the only one an update may install: the pull MUST be by `<repository>@<candidate digest>`, never by tag ([ADR-0012](../adr/0012-digest-pinned-updates.md), UPD-001). A tag that moves after detection affects only the next cycle's detection.

- **Given** a registry proxy whose `HEAD` response carries `Docker-Content-Digest: sha256:abc` **When** a cycle runs **Then** the result is `failed` with reason `registry_unreachable` and the proxy log shows no manifest `GET` for that repository.
- **Given** a registry proxy whose `200` `HEAD` response has no `Docker-Content-Digest` header **When** a cycle runs **Then** the result is `failed` with reason `registry_unreachable` and the proxy log shows no manifest `GET` for that repository.
- **Given** a registry proxy that rejects `HEAD` and serves a `GET` manifest body whose `Docker-Content-Digest` header is a different valid digest than the body's `sha256` **When** a cycle runs **Then** the result is `failed` with reason `registry_unreachable`.
- **Given** a registry proxy that rejects `HEAD` and serves the manifest by `GET` without `Docker-Content-Digest` **When** a cycle runs **Then** the report's candidate digest equals `sha256` of the served manifest body.
- **Given** an armed Parachute, a newer image `X` on tag `1`, and a registry proxy that starts serving a different image `Y` on tag `1` after the first manifest request **When** the cycle runs **Then** the registry access log shows the manifest fetched for the pull by digest `X`, never by tag `1`, and the replacement's image digest is `X`.
