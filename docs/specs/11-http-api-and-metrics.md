# 11 HTTP API and metrics

**Status:** Draft

An optional HTTP server for triggering cycles, approving updates, metrics, and the status view ([14-status-view.md](14-status-view.md)). All responses with a body are JSON (`Content-Type: application/json`) except metrics and the HTML status page.

### API-001 Server
Parachute MUST listen for HTTP on `--http-api-port` (default `8080`, all interfaces) when any of `--http-api-update`, `--http-api-metrics`, or `--http-api-status` is on, and MUST NOT listen on any port otherwise. Endpoints whose setting is off MUST return 404.

| Endpoint | Methods | Enabled by |
|---|---|---|
| `/v1/update` | `GET`, `POST` | `--http-api-update` |
| `/v1/approve` | `POST` | `--http-api-update` |
| `/v1/metrics` | `GET` | `--http-api-metrics` |
| `/v1/status` | `GET` | `--http-api-status` |
| `/status` | `GET` | `--http-api-status` |

- **Given** no HTTP settings **When** Parachute runs **Then** no TCP port is open in its container.
- **Given** only `--http-api-metrics` and a token **When** `POST /v1/update` is called with the token **Then** the response is 404.
- **Given** `--http-api-update` and a token **When** `DELETE /v1/update` is called with the token **Then** the response is 405.

### API-002 Authentication
Every endpoint MUST require `Authorization: Bearer <token>` matching `--http-api-token`. This token can trigger updates and approvals and must be treated as root-equivalent ([ADR-0010](../adr/0010-docker-socket-access-and-security.md)). Only `GET /status` and `GET /v1/status` MUST also accept HTTP Basic authentication with any user name and the token as password, so browsers can open them, and MUST also accept the optional read-only `--http-api-status-token` (when set), as a Bearer token or as the Basic password. On every other endpoint, Basic credentials and the status token MUST be rejected with 401. A missing or wrong token MUST return 401 with header `WWW-Authenticate` and MUST NOT run anything. Requests are untrusted input ([glossary: Trust boundaries](glossary.md#trust-boundaries)): they are size-limited (API-009), failed authentication is throttled (API-010), and tokens are compared in constant time (API-011).

- **Given** a configured token **When** `POST /v1/update` is called without a token or with a wrong one **Then** the response is 401 and no cycle starts.
- **Given** a configured token **When** `GET /status` is called with Basic credentials `any:<token>` **Then** the response is 200.
- **Given** a configured token and `--http-api-update` **When** `POST /v1/update` is called with Basic credentials `any:<token>` **Then** the response is 401 and no cycle starts.
- **Given** `--http-api-status-token ro` **When** `GET /v1/status` is called with `Bearer ro`, and `GET /status` with Basic credentials `any:ro` **Then** both responses are 200.
- **Given** `--http-api-status-token ro`, `--http-api-update`, and `--http-api-metrics` **When** `POST /v1/update` and `GET /v1/metrics` are called with `Bearer ro` **Then** both responses are 401 and no cycle starts.

### API-003 A token is required
If the HTTP server would start (API-001) and no token is configured, Parachute MUST exit at startup with code 2 and a message naming `http-api-token`.

- **Given** `--http-api-update` and no token **When** Parachute starts **Then** it exits with code 2 and the message contains `http-api-token`.

### API-004 Trigger a cycle
`/v1/update` MUST run one cycle over all candidates on all hosts and respond when it ends with 200 and body `{"cycle": "<cycle-id>", "scanned": n, "updated": n, "failed": n, "held": n, "would_update": n}`. A cycle that ends with failed updates still returns 200. `/v1/update` takes no query parameters: a request with any query parameter MUST return 400 with body `{"error": "unknown parameter: <name>"}` naming the first unknown parameter, and MUST NOT run anything. (Filtering by stack or host is not part of v1.)

- **Given** an armed Parachute in API mode and a pending update **When** `POST /v1/update` is called with the token **Then** the container is updated and the 200 body has `"updated": 1`.
- **Given** an armed Parachute in API mode and a pending update **When** `POST /v1/update?stack=shop` is called with the token **Then** the response is 400, the body names `stack`, no cycle starts, and the container is unchanged.

### API-005 Overlapping requests
A trigger request that arrives while a cycle or restore verification is running MUST NOT start another one and MUST return 409 with body `{"running": "<cycle-id>"}` immediately. It is counted in `parachute_scans_skipped`.

- **Given** a cycle in progress (a pre-check hook sleeping 10 s) **When** a second `POST /v1/update` arrives **Then** it returns 409 within 1 second with the running cycle's ID.

### API-006 Periodic polling alongside the API
With `--http-api-periodic-polls` on, scheduled cycles (SCH-001, SCH-002) MUST also run in API mode.

- **Given** `--http-api-update --http-api-periodic-polls --interval 30` **When** Parachute runs for 40 seconds and one trigger request is made **Then** at least two cycles have run (one scheduled at startup, one on request) and none overlapped.

### API-007 Approve a pending update
`POST /v1/approve?container=<name>[&host=<host>]` MUST approve the update currently held for that container with reason `approval_required` or `previous_attempt_failed` (POL-006, POL-010), and return 202 with body `{"host": "...", "container": "...", "digest": "sha256:..."}`. With no such held update it MUST return 404. `host` defaults to `local` and is required (400 when missing) when more than one host is configured. The approval is stored in the state file and survives restarts.

- **Given** a held database update **When** `POST /v1/approve?container=db` is called with the token **Then** the response is 202 with the candidate digest, and after a restart of Parachute the next cycle updates `db`.
- **Given** no held update for `web` **When** `POST /v1/approve?container=web` is called **Then** the response is 404.

### API-008 Metrics
`GET /v1/metrics` MUST return the Prometheus text exposition format with these metrics. Gauges describe the last completed cycle; counters cover the process lifetime. Every metric except the two scan counters has a `host` label.

| Metric | Type | Meaning |
|---|---|---|
| `parachute_containers_scanned` | gauge | Candidates checked |
| `parachute_containers_updated` | gauge | Results `updated` |
| `parachute_containers_failed` | gauge | Results `failed`, `rolled_back`, or `rollback_failed` |
| `parachute_containers_held` | gauge | Results `held` |
| `parachute_scans_total` | counter | Cycles started |
| `parachute_scans_skipped` | counter | Cycle starts skipped (SCH-006, API-005) |
| `parachute_rollbacks_total` | counter | Transactions ended `rolled_back` |
| `parachute_rollback_failures_total` | counter | Transactions ended `rollback_failed` |
| `parachute_host_up` | gauge | 1 when the host was reachable in the last cycle, else 0 |
| `parachute_restore_verification_last_success_timestamp_seconds` | gauge | Unix time of the last passed restore verification, per `host` and `stack` |

- **Given** `--http-api-metrics`, a token, and a cycle that updated one of three containers **When** `GET /v1/metrics` is called **Then** the body contains `parachute_containers_scanned{host="local"} 3`, `parachute_containers_updated{host="local"} 1`, and `parachute_scans_total 1`.
- **Given** a second host that is unreachable **When** a cycle ends **Then** `parachute_host_up{host="<name>"} 0` is reported.

### API-009 Request limits
The server MUST bound what one request can cost. A request whose request line and headers together exceed 8 KiB MUST get 431. A request with a body larger than 1 KiB (by `Content-Length` or by bytes read) MUST get 413 after reading at most 1 KiB of the body; no endpoint uses a request body, and a smaller body is ignored. A connection that has not sent complete request headers within 10 seconds of opening, or that stays idle for 60 seconds between requests, MUST be closed. Limits are checked before authentication, and a rejected request MUST NOT run anything.

- **Given** `--http-api-update` and a token **When** `POST /v1/update` is sent with the token and a 9 KiB `X-Pad` header **Then** the response is 431 and no cycle starts.
- **Given** the same **When** `POST /v1/update` is sent with the token and a 2 KiB body **Then** the response is 413 and no cycle starts.
- **Given** the same **When** a client opens a connection and sends only `GET /v1/status HTTP/1.1\r\n` **Then** the server closes the connection 10 seconds later (tolerance ±5 s).

### API-010 Failed-authentication throttling
Parachute MUST count failed authentications (API-002) per source IP address over a sliding 60-second window. While an address has 10 or more failures in the window, every request from it MUST get 429 with a `Retry-After` header (whole seconds until the oldest counted failure leaves the window, 1 to 60) without checking its credentials or running anything; such throttled requests are not counted as failures. The first throttled request in a window MUST be logged at `warn` with the source address, never the presented credentials. Other source addresses are not affected.

- **Given** `--http-api-update` and a token **When** 10 `POST /v1/update` requests with a wrong token are sent from one address within 5 seconds, followed by one with the correct token **Then** the first 10 responses are 401, the 11th is 429 with `Retry-After` between 1 and 60, and no cycle starts.
- **Given** that state **When** a request with the correct token arrives from a second source address **Then** the response is 200.
- **Given** that state **When** a request with the correct token arrives from the first address 65 seconds after the last failure **Then** the response is 200.

### API-011 Constant-time token comparison
Parachute MUST compare a presented token with `--http-api-token` and `--http-api-status-token` in time that does not depend on the presented token's content or length, by comparing fixed-length digests (SHA-256) of both values with a constant-time comparison. Both configured tokens are always compared, whichever matches.

- **Given** a 64-character token **When** 100 requests with a wrong token equal to it except in the last character and 100 with a wrong token sharing no character with it are sent, interleaved, from source addresses chosen so that none is throttled (API-010) **Then** every response is 401 and the median response times of the two sets differ by less than 1 ms.
