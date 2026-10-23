# 13 Docker daemons, multiple hosts, and runtimes

**Status:** Draft (Milestone 5 section: Deferred)

How Parachute connects to container engines. A **host** is one engine endpoint. The security model is proposed in [ADR-0010](../adr/0010-docker-socket-access-and-security.md).

## Connection

### DMN-001 Endpoint
With no host list (DMN-007), Parachute MUST manage one host named `local` at the endpoint in `--host` or `DOCKER_HOST`, default `unix:///var/run/docker.sock`. Supported schemes are `unix://` and `tcp://`; any other scheme (for example `ssh://`) MUST be a startup error (exit code 2) naming the scheme.

- **Given** the Docker socket mounted at the default path and no host settings **When** Parachute starts **Then** the startup message lists host `local` at `unix:///var/run/docker.sock`.
- **Given** `DOCKER_HOST=ssh://user@box` **When** Parachute starts **Then** it exits with code 2 and the message contains `ssh`.

### DMN-002 API version negotiation
Parachute MUST negotiate the Engine API version with each daemon and MUST NOT hard-code one: a fixed version stops working when a newer daemon raises its minimum supported version. `--api-version` (or `DOCKER_API_VERSION`) MAY force a version as an override.

- **Given** a daemon whose minimum API version is 1.40 or higher **When** Parachute starts without version settings **Then** it connects and a cycle completes.
- **Given** `--api-version 1.12` **When** Parachute starts against such a daemon **Then** it exits with code 4 and the message contains the daemon's error.

### DMN-003 TLS
With `--tlsverify` or `DOCKER_TLS_VERIFY` set, Parachute MUST connect to `tcp://` hosts over TLS, verifying the daemon certificate against `ca.pem` and presenting `cert.pem` and `key.pem` from `DOCKER_CERT_PATH` (default `/etc/ssl/docker`). It MUST NOT fall back to an unverified or plain connection.

A `tcp://` host used without TLS verification is an [insecure TCP host](glossary.md#insecure-tcp-host), loopback addresses included, because anyone who can reach that port controls the host. Configuring one MUST be a startup error (exit code 2) whose message names the host's endpoint and `allow-insecure-tcp`, and Parachute MUST NOT open a connection to any host first. With `--allow-insecure-tcp` (or `PARACHUTE_ALLOW_INSECURE_TCP=true`) Parachute MUST instead log a `warn` line naming each insecure host's endpoint at startup and at the start of every cycle, and MUST list `allow-insecure-tcp` as a relaxation in the startup message (POL-009). `parachute status` (STS-001) applies the same rule.

- **Given** TLS verify on and a daemon presenting a certificate not signed by `ca.pem` **When** Parachute starts **Then** it exits with code 4, and the daemon's log shows no completed plain-HTTP request.
- **Given** `DOCKER_HOST=tcp://127.0.0.1:2375` (a reachable plain daemon) and no TLS settings **When** Parachute starts **Then** it exits with code 2, the message contains `tcp://127.0.0.1:2375` and `allow-insecure-tcp`, and the daemon received no request.
- **Given** the same with `--allow-insecure-tcp --interval 30` **When** Parachute runs for 40 seconds **Then** at least three `warn` lines name `tcp://127.0.0.1:2375` (one at startup, one at the start of each of the two cycles) and the startup message lists `allow-insecure-tcp` as a relaxation.
- **Given** `PARACHUTE_HOSTS="a=unix:///var/run/docker.sock,b=tcp://127.0.0.1:2375"` and no TLS settings **When** Parachute starts **Then** it exits with code 2 and the message names `b`'s endpoint.

### DMN-004 Unreachable at startup
If no managed host is reachable at startup, Parachute MUST exit with code 4 and a message naming each endpoint. If at least one is reachable, the unreachable ones are reported (DMN-009) and Parachute keeps running.

- **Given** `DOCKER_HOST=tcp://127.0.0.1:1` and `--allow-insecure-tcp` **When** Parachute starts **Then** it exits with code 4 and the message contains `tcp://127.0.0.1:1`.

### DMN-005 Connection loss while running
If a host becomes unreachable while Parachute runs, Parachute MUST NOT exit. **Read requests** are container and network inspect and list, image inspect and list, system information, events, and registry requests (DET-002). Parachute MUST retry each failed read request with exponential backoff (1 s, 2 s, 4 s, ... capped at 30 s) for up to 5 minutes, then give up on that host for the rest of the cycle and report it as unreachable (`reachable: false`); an unreachable host contributes no result entries for that cycle ([reasons.md](reasons.md#not-reason-codes)). Every other request (for example create, rename, remove, start, stop, pull, tag) is a **write request** and MUST NOT be retried, whether the host became unreachable or answered with an error: a failed write fails its step. An open transaction on a host that became unreachable is rolled back as in UPD-016 when the host is reachable again. A Docker request for one candidate that the engine answers with an error while the host stays reachable MUST NOT affect other candidates, and MUST end that candidate with reason `engine_error`: before the `update` phase as in UPD-014 (result `failed`), from the `update` phase on by rollback (VER-008; result `rolled_back`, and the digest is held, POL-010; or `rollback_failed` per VER-010).

- **Given** a running Parachute and the daemon stopped for 20 seconds during an idle period **When** the daemon returns and a cycle runs **Then** the Parachute process is still running and the cycle completes.
- **Given** a daemon paused for 6 minutes during a cycle **When** the cycle ends **Then** the report lists that host as unreachable and Parachute is still running.
- **Given** an engine proxy in front of a reachable daemon that answers `500` to every image inspect request for container `a`'s image, and containers `a` and `b` **When** a cycle runs **Then** `a` is `failed` with reason `engine_error`, `b` gets its normal result, and the host is reported reachable.
- **Given** backup mode `stop`, container `a` with a newer image, and an engine proxy that answers the first stop request for `a` with `500` without forwarding it **When** a cycle runs **Then** the proxy log shows exactly one stop request for `a` in that cycle, `a` is running with its original ID, and the result is `failed` with reason `engine_error`.
- **Given** container `a` with a newer image and an engine proxy that answers the first start request for `a`'s replacement with `500` without forwarding it **When** a cycle runs **Then** the proxy log shows exactly one start request for the replacement, `a` is running with its original name and ID, the result is `rolled_back` with reason `engine_error`, and the next cycle holds that digest with reason `previous_attempt_failed`.

### DMN-006 Health check command
`parachute healthcheck` MUST exit 0 when a Parachute daemon process is running in the same container and has completed its startup checks, and 1 otherwise. The official image MUST declare it as its Docker `HEALTHCHECK`.

- **Given** a started Parachute container **When** `docker exec <c> parachute healthcheck` runs **Then** it exits 0, and the container's health status becomes `healthy`.
- **Given** a container from the Parachute image whose command is `sleep infinity` **When** `parachute healthcheck` runs inside it **Then** it exits 1.

## Multiple hosts

### DMN-007 Host list
`--hosts` (or `PARACHUTE_HOSTS`, a list per CFG-003) MUST define the managed hosts as `name=endpoint` entries. Names MUST match `[a-z0-9][a-z0-9-]{0,31}` and be unique; violations are a startup error (exit code 2). Two reachable hosts whose engines report the same engine ID (`ID` in the engine's system information) are one engine under two names, and MUST be a startup error (exit code 2) whose message names both hosts. When `--hosts` is set, `DOCKER_HOST` and `--host` are ignored and a `warn` line says so. Each host is reached directly over its Engine API (Parachute pulls state from it; no agent is installed on hosts).

- **Given** `PARACHUTE_HOSTS="a=unix:///var/run/docker.sock,b=tcp://10.0.0.5:2376"` and `--allow-insecure-tcp` **When** Parachute starts **Then** the startup message lists hosts `a` and `b`.
- **Given** two entries both named `a` **When** Parachute starts **Then** it exits with code 2.
- **Given** `PARACHUTE_HOSTS="a=unix:///var/run/docker.sock,b=unix:///var/run/docker.sock"` **When** Parachute starts **Then** it exits with code 2 and the message names `a` and `b`.

### DMN-008 Per-host TLS
For a `tcp://` host named `<name>`, when the directory `DOCKER_CERT_PATH/<name>/` exists, Parachute MUST use the certificates in it instead of those in `DOCKER_CERT_PATH`. TLS verification (DMN-003) applies to every `tcp://` host when enabled.

- **Given** two TLS daemons with different CAs and their certificates in `DOCKER_CERT_PATH/a/` and `DOCKER_CERT_PATH/b/` **When** a cycle runs **Then** both hosts are reachable.

### DMN-009 Hosts are isolated
A host that is unreachable, or whose transactions fail, MUST NOT stop or delay processing of other hosts beyond the retry window of DMN-005. In run-once mode an unreachable host makes the exit code at least 1.

- **Given** hosts `a` (reachable, one update) and `b` (unreachable) and `--run-once` **When** Parachute runs **Then** the update on `a` is applied, the report shows `b` unreachable, and the exit code is 1.

### DMN-010 Per-host results
Every report entry, porcelain line, status entry, and metric MUST name its host. Stacks, scopes, snapshots (BKP-005), and dependencies are per host; containers on different hosts are never in one stack or one transaction.

- **Given** hosts `a` and `b` on two distinct engines, and stack `shop` on each, both with updates **When** a cycle runs **Then** the report has two `shop` entries, one under each host, with different transaction IDs.

## Milestone 5: Podman

The requirements in this section are deferred. Tests for them are written but not required to pass until Milestone 5.

### DMN-011 Podman's Docker-compatible API
Parachute MUST work against Podman's Docker-compatible API socket (rootful `unix:///run/podman/podman.sock`, rootless `unix://$XDG_RUNTIME_DIR/podman/podman.sock`) with the same observable behaviour as against Docker for every requirement in these specs, except where DMN-012 to DMN-014 say otherwise.

- **Given** the Milestone 1 black-box suite run with `DOCKER_HOST` set to a Podman socket **When** it runs **Then** every test passes except those listed as runtime-specific in DMN-012 to DMN-014.

### DMN-012 Podman pods
Containers in a Podman pod MUST form one stack named after the pod, unless they carry Compose project labels (DEP-001), which take precedence. Parachute MUST NOT update a pod's infra container and MUST refuse (UPD-007, reason `config_unreproducible`) a container whose recreation would require recreating the pod.

- **Given** pod `p` with containers `app` and `cache` and an update for `app` **When** a cycle runs **Then** the report lists stack `p`, and the infra container is unchanged.

### DMN-013 Podman auto-update labels
A container with Podman's `io.containers.autoupdate` label MUST be reported `skipped` with reason `managed_by_runtime`, so two updaters never act on one container.

- **Given** a container labelled `io.containers.autoupdate=registry` **When** a cycle runs **Then** its result is `skipped` with reason `managed_by_runtime`.

### DMN-014 Rootless storage
Under rootless Podman, `--backup-volume` given as a host path MUST be inside a directory the rootless user can write; otherwise the snapshot fails with reason `snapshot_failed` and a message naming the path.

- **Given** rootless Podman and `--backup-volume /root/snaps` **When** an update runs **Then** the result is `failed` with reason `snapshot_failed` naming `/root/snaps`.
