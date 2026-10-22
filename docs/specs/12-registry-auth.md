# 12 Registry authentication

**Status:** Draft

How Parachute authenticates to registries for detection and pulls. Tests use a local `registry:2` with htpasswd authentication.

### REG-001 Credentials from a Docker config file
Parachute MUST read registry credentials from the `auths` section of `config.json` in the directory named by `DOCKER_CONFIG` (default `/`, so `/config.json`). Without a matching entry, requests are anonymous.

- **Given** a registry requiring authentication and `config.json` with valid credentials for it **When** a cycle runs with a newer image there **Then** the update is detected and pulled.
- **Given** no `config.json` **When** a cycle runs against that registry **Then** the result is `failed` with reason `registry_unauthorized`.

### REG-002 The file is re-read every cycle
Parachute MUST read `config.json` at the start of every cycle, so a replaced or edited file takes effect without a restart. Mounting the file itself (not its directory) MUST work.

- **Given** a cycle failing with wrong credentials, then `config.json` replaced with correct ones (a new file moved over the old) **When** the next cycle runs **Then** it succeeds without restarting Parachute.

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
Registry credentials, tokens, and `Authorization` header values MUST NOT appear in logs at any level other than `trace`, in notifications, in porcelain output, or in the status view. Starting with `--log-level trace` MUST log a `warn` line saying trace logs can contain credentials.

- **Given** credentials `user:s3cret-value` and `--log-level debug` **When** a cycle runs **Then** neither `s3cret-value` nor its base64 form appears in logs, notifications, or `/v1/status`.
- **Given** `--log-level trace` **When** Parachute starts **Then** a `warn` line mentions credentials.
