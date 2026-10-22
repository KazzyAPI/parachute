# ADR-0011: Untrusted input handling

- **Status:** Accepted
- **Date:** 2026-10-23

## Context
Parachute has root-equivalent access to every host it manages ([ADR-0010](0010-docker-socket-access-and-security.md)). Much of what it reads is written by someone other than the operator: image authors, registries, whoever can reach the HTTP port, and whoever can write to the state directory or the snapshot store. If any of that input can switch off a safeguard, approve an update, choose a path, or name an object to delete, then the image author or the attacker has the operator's power. [Glossary: Trust boundaries](../specs/glossary.md#trust-boundaries) lists which inputs are trusted; this ADR makes that list a rule the code must follow.

## Decision
1. **Untrusted inputs** are exactly those in [glossary: Trust boundaries](../specs/glossary.md#trust-boundaries): image metadata from registries or pulled images (labels, configuration, `created` time, health check, entrypoint); registry responses (status, headers, bearer challenges and token realms, manifests, token bodies); the state file and `handover.json`; snapshot manifests and archives; HTTP API requests; `config.json` content beyond the expected `auths` fields. Anything not on the trusted list of that section is untrusted. Adding an input to the trusted list is a new ADR.
2. **Validate at the boundary.** Each untrusted input is parsed and validated once, in the adapter that reads it, into a typed value before any other package sees it. Parsing is strict: unknown formats, oversized bodies, out-of-range values, and invalid UTF-8 are rejected, not repaired. Every read has a size limit and every network read a timeout.
3. **Failure is local.** A bad value fails only the item it belongs to: one candidate (`failed` with a reason code from [reasons.md](../specs/reasons.md)), one request (a 4xx response), or one snapshot (`snapshot_invalid`). It never crashes Parachute, never affects other containers or hosts, and is never silently replaced by a default. The exception is the state file: an invalid state file is a startup error and is never overwritten (CFG-010).
4. **Untrusted input never grants privilege.** No untrusted input may:
   - arm Parachute, approve an update, lift a hold or a freeze, or relax a safe default (CFG-006, POL-009); image-baked `parachute.*` labels are ignored, and only labels on the container count;
   - choose a file path outside the snapshot or target mount (no absolute paths, no `..`, no symlink that leaves the root) or a command to run;
   - name an object for Parachute to stop, rename, or delete; Parachute acts only on objects it found through the engine or created itself, proven by its own transaction-ID labels;
   - widen where credentials are sent (REG-004) or appear unredacted in output ([glossary: Redaction](../specs/glossary.md#redaction)).
5. **Names are matched, not used.** Container, host, and stack names from an untrusted input are looked up among the known ones. They are never interpolated into paths, shell commands, or engine filters.
6. **Tests prove it.** For every untrusted input, the black-box suite ([ADR-0003](0003-black-box-tests-from-specs.md)) has at least one test that feeds a hostile value through the real boundary (a fixture image with `parachute.*` labels, a local registry or proxy returning bad responses, a crafted `state.json`, `handover.json`, or snapshot manifest, a malformed HTTP request) and asserts that the outcome is the documented local failure and that no other container, file, or result changed. Unit tests of a parser are allowed in addition, never instead.

## Consequences
- One place per input does the validation, so reviewers know where to look and other packages work with typed values only.
- Some convenience is lost: an image cannot ship its own Parachute settings, and a hand-edited state file cannot approve an update.
- Every new input source needs a trust decision and a hostile-input test before it is merged.

## Specs affected
CFG-006, CFG-010, POL-008, POL-009, DET-003, REG-001, REG-004, REG-006, BKP-007, UPD-005, UPD-018, API-002, API-004, API-007, RST-003.
