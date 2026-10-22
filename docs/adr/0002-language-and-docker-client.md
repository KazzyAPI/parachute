# ADR-0002: Go, using the official Docker client

- **Status:** Proposed
- **Date:** 2026-10-22

## Context
Parachute is a single long-running daemon that talks to the Docker Engine API, registries, and notification services. A client that hard-codes an Engine API version breaks as soon as a newer daemon raises its minimum supported version, so version handling must come from a well-maintained client.

## Decision
1. The implementation language is Go.
2. We use the official Docker Engine client library, and always **negotiate** the API version with the daemon. We never hard-code a version.
3. We use shoutrrr (as a library) for notification delivery, so operators configure every notification service with one widely used URL format.
4. Distribution is a single static binary and a container image. Nothing else is required at runtime.
5. No database. State that must survive a restart lives in Docker (labels on containers, volumes for snapshots) or in one small state file in a mounted directory.

## Consequences
- Go is the common language of the container tooling ecosystem, which makes contributing and shipping easy.
- Go is a bet on contributors, not on features.
- No database keeps install trivial but limits history queries; the status view must work from Docker plus the state file.

## Specs affected
DMN-001, DMN-002, NTF-001, BKP-005, CFG-010, STS-001.
