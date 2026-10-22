# ADR-0001: Spec-driven development

- **Status:** Proposed
- **Date:** 2026-10-22

## Context
Parachute touches other people's running containers and volumes. A wrong update costs someone data. We also want contributors to be able to tell what the tool is supposed to do without reading the code.

## Decision
1. The files in `docs/specs/` are the source of truth for behavior. `docs/adr/` is the source of truth for constraints on how we build.
2. Behavior changes start as a spec change in a pull request. Code that changes behavior without a spec change is rejected.
3. Every requirement has a stable ID of the form `<DOMAIN>-NNN` (for example `SEL-001`). IDs are never reused.
4. Every `MUST` and `SHOULD` requirement has at least one test whose name starts with its ID.
5. A CI check fails when a `MUST` requirement in a spec file with status `Accepted` has no test.
6. A requirement whose behavior is not obvious states its reason in the spec.

## Consequences
- Slower to add features; much easier to review and to trust.
- Specs can be wrong. When a test and a spec disagree, the spec is fixed first, in its own commit.
- Contributors need to write prose before code.

## Specs affected
All.
