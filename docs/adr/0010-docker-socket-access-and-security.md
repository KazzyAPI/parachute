# ADR-0010: Docker socket access and security model

- **Status:** Accepted (amended 2026-10-23, see Amendment)
- **Date:** 2026-10-22

## Context
Access to the Docker socket or Engine API is equivalent to root on that host. Parachute needs that access to recreate containers and copy volumes. Anyone who can control Parachute, or read what it logs, inherits part of that power.

## Decision
1. Parachute needs full Engine API access; we do not pretend otherwise or support a read-only socket proxy in v1.
2. Parachute opens no network port unless an HTTP feature is enabled, and then every endpoint requires the token (API-001 to API-003). It refuses to start an HTTP server without a token. That token can trigger updates and approvals, so it is write-capable and root-equivalent. An optional status token (`--http-api-status-token`) is read-only: it is accepted only by the two status endpoints, which are also the only ones that accept HTTP Basic authentication (API-002).
3. Remote hosts are reached with TLS verification when it is enabled, never with an insecure fallback; a plain `tcp://` host is allowed but warned about at startup (DMN-003).
4. Secrets (registry credentials, tokens, notification URLs, database passwords) never appear in logs below `trace`, notifications, porcelain output, the status view, or the command line of an exec Parachute runs; secrets an exec needs are passed in its environment (REG-006, NTF-001, BKP-004).
5. Containers that Parachute creates (helpers, restore verification) are labelled, have no published ports, and are removed when done (BKP-010, RST-002, RST-005).

## Consequences
- Operators must treat the Parachute container and its main token like root credentials; the README must say so. The status token is the one to give to dashboards and browsers.
- A socket proxy that restricts API calls is not supported until we can list the exact calls Parachute needs.
- The image runs as root. The mounted Docker socket is owned by `root` and a `docker` group whose ID differs between hosts, so a fixed non-root `USER` cannot open it without per-host setup; since socket access is already root-equivalent, a non-root user inside the container would add no real isolation. Operators who want one can pass `--user <uid>:<docker gid>`.

## Specs affected
API-001, API-002, API-003, BKP-004, DMN-003, REG-006, NTF-001, BKP-010, RST-002, RST-005.

## Amendment (2026-10-23)
Decision 3 is narrowed: a plain or unverified `tcp://` host (loopback included) is a startup error (exit code 2) unless `--allow-insecure-tcp` / `PARACHUTE_ALLOW_INSECURE_TCP=true` is set; with it, Parachute warns at startup and at the start of every cycle and lists it as a relaxation (DMN-003). Decision 2 is extended by request limits, failed-authentication throttling, and constant-time token comparison (API-009 to API-011). Decision 4 is made precise by [ADR-0013](0013-secret-redaction.md).
