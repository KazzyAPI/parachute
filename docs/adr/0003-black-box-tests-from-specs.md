# ADR-0003: Black-box tests derived from specs

- **Status:** Accepted (partly superseded by ADR-0015)
- **Date:** 2026-10-22

## Context
We want tests written from specs, not from code. Unit tests that mirror the implementation lock in structure and hide behavior bugs. The real risk here is in how Parachute behaves against Docker, registries, and volumes.

## Decision
1. The primary test suite is **black-box**: it starts Parachute (binary or image) against a real Docker engine and a local registry (`registry:2`), then asserts only on observable outcomes: container state and labels, volume contents, notification payloads received by a fake webhook, HTTP responses, logs, and exit codes.
2. Each acceptance criterion in a spec is one test. The test name begins with the requirement ID, for example `SEL-001_default_selects_all_running`.
3. Tests may not import application packages. They talk to the program from outside.
4. Unit tests are allowed for pure logic (cron parsing, version comparison) but never replace the black-box test for a requirement.
5. Test fixtures that need a "new image version" are built locally and pushed to the local registry. Tests never depend on Docker Hub.
6. A traceability check lists every requirement ID, its file's status, and the tests that name it. A `MUST` in an `Accepted` file with no test fails CI.

## Consequences
- Tests are slower and need Docker in CI.
- Refactors do not break tests; behavior changes do.
- Anyone can write a test from the spec without reading the code.

## Specs affected
All.
