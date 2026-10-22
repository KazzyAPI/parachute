# 14 Status view

**Status:** Draft

A read-only view of every managed stack, built only from the engines and the state file (CFG-010). Host-level checks such as certificate expiry are out of scope.

### STS-001 Command line
`parachute status` MUST print a human-readable table of every stack and service in scope on every host, and `parachute status --json` MUST print the STS-002 document. It reads the state directory (`--state-dir`) and the hosts; it does not need a running Parachute daemon. Exit codes: 0 on success, 2 on invalid arguments, an [insecure TCP host](glossary.md) without `--allow-insecure-tcp` (DMN-003), or an unreadable or invalid state file (CFG-010), 4 when no host is reachable.

- **Given** no Parachute daemon running, a state file from earlier cycles, and the Docker socket **When** `parachute status --json` runs **Then** it exits 0 and prints a document listing the stacks.
- **Given** `DOCKER_HOST=tcp://127.0.0.1:1` and `--allow-insecure-tcp` **When** `parachute status` runs **Then** it exits 4.
- **Given** `DOCKER_HOST=tcp://127.0.0.1:1` without `--allow-insecure-tcp` or TLS settings **When** `parachute status` runs **Then** it exits 2 and the message contains `allow-insecure-tcp` (DMN-003).

### STS-002 JSON document
`GET /v1/status` (API-001) and `parachute status --json` MUST return the same document:

```json
{
  "format": "parachute.status/v1",
  "generated": "2026-10-22T10:00:00Z",
  "armed": true,
  "last_cycle": {"id": "cyc-...", "started": "...", "ended": "..."},
  "hosts": [{
    "name": "local", "reachable": true,
    "stacks": [{
      "name": "shop", "compose": true,
      "last_backup": {"snapshot": "snap-...", "time": "...", "valid": true},
      "last_restore_verification": {"time": "...", "result": "pass", "snapshot": "snap-..."},
      "last_good_restore_age_seconds": 86400,
      "frozen": null,
      "services": [{
        "name": "web", "container": "shop-web-1",
        "image": "localhost:5000/fixture/app:1", "image_id": "sha256:...",
        "update_available": true, "candidate_digest": "sha256:...",
        "hold": {"reason": "delay", "until": "..."},
        "drift": null,
        "last_transaction": {"id": "txn-...", "state": "committed", "result": "updated", "reason": null, "time": "..."}
      }]
    }]
  }]
}
```

Fields with no value MUST be `null`, never omitted. `update_available`, `candidate_digest`, and `hold` reflect the last completed cycle.

- `armed` is the armed state that the process producing the document computes from its own sources at its start, by the rules of CFG-006 (flags, environment, the `parachute.armed` label on its own container, the profile). For `GET /v1/status` that is the running daemon; for `parachute status` it is the `status` process itself, so `docker exec <parachute> parachute status` shows the daemon's setting. It is never read from the state file (STS-006).
- `last_transaction.state` is the transaction's state from the [transaction state machine](glossary.md#transaction-state-machine). While the transaction is open it is an open state (for example `verify`) and `result` and `reason` are `null`; once it is terminal, `result` is its result state and `reason` its reason code from [reasons.md](reasons.md).
- `frozen` is `null`, or, for a stack frozen after `rollback_failed` ([glossary](glossary.md#transaction-state-machine)), `{"transaction": "txn-...", "snapshot": "snap-..."}` naming the failed transaction and its snapshot.

- **Given** a stack with one held service and one fresh service **When** `GET /v1/status` is called with the token **Then** the body validates against the shape above, the held service has a non-null `hold`, and the fresh one has `"hold": null` and `"update_available": false`.
- **Given** an unarmed daemon with `--http-api-status` **When** `GET /v1/status` is called **Then** `"armed": false`; and with `PARACHUTE_ARMED=true` set only for the `parachute status --json` process, that process prints `"armed": true`.
- **Given** a stack whose last update rolled back with reason `verify_failed` **When** `parachute status --json` runs **Then** that service's `last_transaction` has `"state": "rolled_back"`, `"result": "rolled_back"`, and `"reason": "verify_failed"`.
- **Given** an update in its `verify` phase (a replacement whose health check takes 60 s) **When** `GET /v1/status` is called during it **Then** `last_transaction.state` is `verify` and `result` is `null`.
- **Given** a stack whose last update ended `rollback_failed` **When** `parachute status --json` runs **Then** the stack's `frozen` names that transaction and its snapshot.

### STS-003 HTML page
`GET /status` MUST return an HTML page showing the same information as STS-002, one row per service grouped by host and stack, with no external resources (scripts, styles, fonts), and readable without JavaScript.

- **Given** `--http-api-status` and a token **When** `GET /status` is called with Basic credentials (the token or the status token as password, API-002) **Then** the response is 200 `text/html`, contains every service name, and has no `<script src` or `<link` to another origin.

### STS-004 What is shown
For each service the view MUST show: current image reference and ID; whether an update is available and the candidate digest; the hold reason and its end where known; drift (DEP-011); and the last transaction's ID, state, result, reason, and time. For each stack it MUST show the last snapshot's ID, time, and validity (BKP-007), the last restore verification result and the age of the last passing one (RST-006), and whether it is frozen. The view MUST show the armed state.

- **Given** a stack whose last update rolled back **When** the status view is read **Then** the service shows `last_transaction.result` `rolled_back` and the stack's `last_backup` names the snapshot used.

### STS-005 Read-only
Producing the status view MUST NOT change containers, volumes, images, or the state file, and MUST NOT contact registries.

- **Given** the state file's checksum, the container list, and a recording registry proxy **When** the status view is read 10 times **Then** the checksum and containers are unchanged and the proxy saw no requests.

### STS-006 The state file is not trusted
The state file is untrusted input ([glossary: Trust boundaries](glossary.md#trust-boundaries)). The status view MUST NOT take the armed state from it, and MUST NOT show an update as approved, a hold as released, or a stack as no longer frozen on the strength of the state file alone: whether an approval applies is decided only by the daemon when it evaluates policies (POL-001). A state file that is not valid `parachute.state/v1` makes `parachute status` exit 2 (STS-001, CFG-010) and `GET /v1/status` uses the daemon's own validated state. Every string the view shows that comes from labels, engines, or the state file MUST be escaped for its output: JSON string encoding in STS-002, HTML escaping in STS-003.

- **Given** an unarmed setup and a `state.json` edited (validly) to claim the instance was armed and to record an approval for a held update **When** `parachute status --json` runs **Then** it prints `"armed": false` and the service's `hold` still has reason `approval_required`.
- **Given** a container labelled `com.docker.compose.project=shop` and `com.docker.compose.service=<script>alert(1)</script>` **When** `GET /status` is called **Then** the page contains `&lt;script&gt;alert(1)&lt;/script&gt;` and no `<script` element, and `GET /v1/status` returns valid JSON with that service name.

### STS-007 The status view is redacted
`parachute status`, `parachute status --json`, `GET /v1/status`, and `GET /status` MUST be redacted per [glossary: Redaction](glossary.md#redaction) ([ADR-0013](../adr/0013-secret-redaction.md)). The view shows no container environment values and no configured URLs or tokens.

- **Given** API token `T0ken-api`, status token `T0ken-ro`, a notification URL `generic://bob:hunter2@127.0.0.1:1/hook?token=q5ecret` that fails, registry credentials `user:s3cret-value` in `config.json`, and a monitored container with environment `DB_PASSWORD=pw-9z7` **When** `parachute status` and `parachute status --json` run **Then** neither output contains `T0ken-api`, `T0ken-ro`, `hunter2`, `q5ecret`, `s3cret-value`, its base64 form, or `pw-9z7`.
