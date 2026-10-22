# ADR-0005: Label and configuration namespace

- **Status:** Accepted
- **Date:** 2026-10-22

## Context
Parachute is configured by flags, environment variables, and labels on the containers it manages. Each setting needs one predictable name in each form, and mistakes in configuration must be visible, because a silently ignored safety setting can cost data.

## Decision
1. Parachute's labels use the prefix `parachute.` (for example `parachute.enable`).
2. Parachute's environment variables use the prefix `PARACHUTE_`, derived from the flag name (`--stop-timeout` is `PARACHUTE_STOP_TIMEOUT`). Standard Docker client variables (`DOCKER_HOST` and friends), `TZ`, and `NO_COLOR` keep their usual names.
3. Parachute reads no other label or environment namespace.
4. Unknown and removed settings fail at startup with a clear message, naming the replacement for a removed setting. Nothing is silently ignored. Unknown `parachute.*` labels are logged as warnings.
5. Moving from another tool is covered by a separate migration guide (`docs/migration.md`), not by aliases in the product.

## Consequences
- One name per setting keeps the test matrix and the documentation small.
- Operators moving from another tool must rename labels and variables; the migration guide must make that a mechanical step.
- The `parachute.` prefix is short and easy to type, at the cost of not following the reverse-DNS convention.

## Specs affected
CFG-001, CFG-002, CFG-005, NTF-013, and every requirement that names a label.
