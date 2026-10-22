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
      "services": [{
        "name": "web", "container": "shop-web-1",
        "image": "localhost:5000/fixture/app:1", "image_id": "sha256:...",
        "update_available": true, "candidate_digest": "sha256:...",
        "hold": {"reason": "delay", "until": "..."},
        "drift": null,
        "last_transaction": {"id": "txn-...", "result": "updated", "time": "..."}
      }]
    }]
  }]
}
```

Fields with no value MUST be `null`, never omitted. `update_available`, `candidate_digest`, and `hold` reflect the last completed cycle.

- **Given** a stack with one held service and one fresh service **When** `GET /v1/status` is called with the token **Then** the body validates against the shape above, the held service has a non-null `hold`, and the fresh one has `"hold": null` and `"update_available": false`.

### STS-003 HTML page
`GET /status` MUST return an HTML page showing the same information as STS-002, one row per service grouped by host and stack, with no external resources (scripts, styles, fonts), and readable without JavaScript.

- **Given** `--http-api-status` and a token **When** `GET /status` is called with Basic credentials (the token or the status token as password, API-002) **Then** the response is 200 `text/html`, contains every service name, and has no `<script src` or `<link` to another origin.

### STS-004 What is shown
For each service the view MUST show: current image reference and ID; whether an update is available and the candidate digest; the hold reason and its end where known; drift (DEP-011); and the last transaction's ID, result, and time. For each stack it MUST show the last snapshot's ID, time, and validity (BKP-007), and the last restore verification result and the age of the last passing one (RST-006).

- **Given** a stack whose last update rolled back **When** the status view is read **Then** the service shows `last_transaction.result` `rolled_back` and the stack's `last_backup` names the snapshot used.

### STS-005 Read-only
Producing the status view MUST NOT change containers, volumes, images, or the state file, and MUST NOT contact registries.

- **Given** the state file's checksum, the container list, and a recording registry proxy **When** the status view is read 10 times **Then** the checksum and containers are unchanged and the proxy saw no requests.
