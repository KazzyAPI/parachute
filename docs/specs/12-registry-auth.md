# 12 Registry authentication

**Status:** Draft

How Parachute authenticates to registries for detection and pulls. Tests use a local `registry:2` with htpasswd authentication. `config.json` beyond its `auths` fields and every registry response (status, headers, bearer challenges, token bodies, manifests) are untrusted ([glossary: Trust boundaries](glossary.md#trust-boundaries)). Credentials are secrets under [glossary: Redaction](glossary.md#redaction).

### REG-001 Credentials from a Docker config file
Parachute MUST read registry credentials from the `auths` section of `config.json` in the directory named by `DOCKER_CONFIG` (default `/`, so `/config.json`). Without a matching entry, requests are anonymous. Credentials MUST be sent only over HTTPS or to a loopback host (`localhost`, `127.0.0.0/8`, `::1`): when a registry's bearer challenge names a plain `http://` token realm on any other host, Parachute MUST NOT contact that realm with credentials, and the result is `failed` with reason `registry_unauthorized`.

- **Given** a registry requiring authentication and `config.json` with valid credentials for it **When** a cycle runs with a newer image there **Then** the result is `would_update` when unarmed (`updated` when armed) and the registry access log shows the authenticated user on the manifest request.
- **Given** no `config.json` **When** a cycle runs against that registry **Then** the result is `failed` with reason `registry_unauthorized`.
- **Given** credentials for a registry whose challenge names the token realm `http://auth.example.test/token` **When** a cycle runs **Then** no request reaches that realm and the result is `failed` with reason `registry_unauthorized`.

### REG-002 The file is re-read every cycle
Parachute MUST read `config.json` at the start of every cycle, so a replaced or edited file takes effect without a restart. Mounting the file itself (not its directory) MUST work.

- **Given** a cycle failing with wrong credentials, then `config.json` replaced with correct ones (a new file moved over the old) **When** the next cycle runs, without restarting Parachute, **Then** the candidate's result is no longer `failed`.

### REG-003 Docker Hub keys
For images on Docker Hub, entries keyed `https://index.docker.io/v1/`, `index.docker.io`, and `docker.io` MUST all be used.

- **Given** each of the three key forms in turn, a container from `docker.io/library/fixture:1`, and Parachute's HTTPS traffic sent to a recording proxy that impersonates Docker Hub (its CA trusted through `SSL_CERT_FILE`, so Docker Hub itself is never contacted) **When** a cycle runs **Then** the proxy records the configured credentials on the token request.

### REG-004 Exact host matching
For any other registry, the config key's host and port MUST match the image reference's registry host and port exactly (an optional `https://` or `http://` prefix and trailing path are ignored).

- **Given** image `127.0.0.1:5000/app` and entry `127.0.0.1:5000` **When** a cycle runs **Then** the credential is used.
- **Given** image `127.0.0.1:5000/app` and only entry `localhost:5000` **When** a cycle runs **Then** the credential is not used and the result is `failed` with reason `registry_unauthorized`.

### REG-005 Credential helpers are not supported
In this release, `credsStore` and `credHelpers` entries MUST NOT be executed. When a cycle needs credentials for a registry covered by a helper entry, Parachute MUST log a `warn` line naming the helper and registry once per cycle and proceed anonymously.

- **Given** `config.json` with `"credHelpers": {"127.0.0.1:5000": "ecr-login"}` **When** a cycle runs **Then** a `warn` line contains `ecr-login` and `127.0.0.1:5000`.

### REG-006 Credentials are never logged
Registry credentials, bearer tokens, and `Authorization` header values MUST be redacted as [glossary: Redaction](glossary.md#redaction) defines ([ADR-0013](../adr/0013-secret-redaction.md)): they MUST NOT appear in logs at any level other than `trace`, in notifications, in porcelain output, in the status view, in HTTP API responses, or in error messages and the detail text of `failed` results. The detail text of a registry failure MUST name the registry host and the HTTP status (or the transport error) and MUST NOT include response bodies or response headers. Starting with `--log-level trace` MUST log a `warn` line saying trace logs can contain credentials.

- **Given** credentials `user:s3cret-value` and `--log-level debug` **When** a cycle runs **Then** neither `s3cret-value` nor its base64 form appears in logs, notifications, porcelain output, or `parachute status --json`.
- **Given** credentials `user:s3cret-value` and a token endpoint that answers `500` with a body containing `s3cret-value` and `MARKER-BODY` **When** a cycle runs with `--log-level debug` **Then** the result is `failed` with reason `registry_unreachable`, its detail text names the registry host and `500`, and neither `s3cret-value` nor `MARKER-BODY` appears in logs, notifications, porcelain output, or `parachute status --json`.
- **Given** `--log-level trace` **When** Parachute starts **Then** a `warn` line mentions credentials.

### REG-007 Registry responses are size-limited and validated
Parachute MUST stop reading a registry response body that exceeds its limit: 4 MiB for a manifest, 1 MiB for a token response or any other body. An oversized body, a token response that is not a JSON object with a non-empty `token` or `access_token` string, or a bearer challenge whose `realm` is not an absolute `https://` or `http://` URL MUST give `failed` with reason `registry_unreachable` for the candidate (DET-003) and MUST NOT affect other candidates.

- **Given** a registry proxy that serves a 5 MiB manifest body for `GET` and rejects `HEAD` **When** a cycle runs **Then** the result is `failed` with reason `registry_unreachable`, Parachute keeps running, and other candidates are processed.
- **Given** a token endpoint that streams an endless response body **When** a cycle runs **Then** the result is `failed` with reason `registry_unreachable` before the cycle ends.
- **Given** a token endpoint that answers `200` with body `not json` **When** a cycle runs **Then** the result is `failed` with reason `registry_unreachable`.
- **Given** a registry whose bearer challenge has `realm="file:///etc/passwd"` **When** a cycle runs **Then** the result is `failed` with reason `registry_unreachable`.

### REG-008 Credentials go only where they belong
Parachute MUST send credentials or a bearer token only to the registry host of the image reference and to the token realm host named in that registry's bearer challenge, and only over HTTPS or to a loopback host (REG-001). When a registry response redirects to a different host, Parachute MUST follow it without the `Authorization` header; a redirect to a plain `http://` URL on a non-loopback host MUST NOT be followed and gives `failed` with reason `registry_unreachable`.

- **Given** credentials for a registry whose manifest `GET` redirects to a recording server on another host **When** a cycle runs **Then** the recording server receives no `Authorization` header.
- **Given** a registry whose manifest response redirects to `http://other.example.test/...` **When** a cycle runs **Then** no request reaches `other.example.test` and the result is `failed` with reason `registry_unreachable`.
