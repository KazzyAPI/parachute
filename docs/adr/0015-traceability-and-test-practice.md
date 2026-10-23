# ADR-0015: Traceability and test practice as built

- **Status:** Accepted
- **Date:** 2026-10-23

## Context
The specs and tooling drifted from three accepted ADRs, which are not edited except for their status:

- Test names: [ADR-0001](0001-spec-driven-development.md) rule 4 and [ADR-0003](0003-black-box-tests-from-specs.md) rule 2 say a test name starts with its requirement ID (`SEL-001_...`). Go test names cannot contain `-`, and `make trace` matches `TestABC_001_`.
- The tracing rule: ADR-0001 rule 5 and ADR-0003 rule 6 fail CI only on a `MUST` in an `Accepted` spec file. Every spec file is still `Draft`, so that rule checks nothing; `make trace` also fails on a `MUST` or `SHOULD` listed for a milestone.
- Notification delivery: [ADR-0002](0002-language-and-docker-client.md) rule 3 names shoutrrr; the code uses a maintained fork of it.

Separately, API-011 (constant-time token comparison) had a black-box timing criterion. Response times on shared CI are noisy, and no timing test can prove that a comparison is constant-time.

## Decision
1. **Test names** (supersedes ADR-0001 rule 4 and ADR-0003 rule 2 on naming). A test for a requirement is a Go test function in `e2e/*_test.go` named `Test<DOMAIN>_<NNN>_<description>`, for example `TestSEL_001_default_selects_all_running`. Only this form counts for traceability.
2. **Tracing rule** (supersedes ADR-0001 rule 5 and ADR-0003 rule 6). A requirement is **normative** when a line of its section other than an acceptance criterion contains `MUST` or `SHOULD`. `make trace` reports, per milestone row of [Milestone coverage](../specs/README.md#milestone-coverage), how many listed requirements have a test, and fails when:
   - a milestone row lists an ID that is in no spec file;
   - a normative requirement listed for a milestone has no test;
   - a normative requirement in a spec file with status `Accepted` has no test.
   Requirements not listed for any milestone are only counted. ADR-0001 rule 4 (every `MUST` and `SHOULD` has a test) still holds for every requirement once a milestone lists it.
3. **Notification library** (supersedes ADR-0002 rule 3). Notifications are delivered with the maintained fork of shoutrrr, `github.com/nicholas-fedor/shoutrrr`, used as a library. Operators keep the same service URL format.
4. **API-011 exception** (an exception to ADR-0003 rules 1 and 4). That the token comparison is constant-time (fixed-length SHA-256 digests compared in constant time, both configured tokens always compared) is tested by a code-level unit test of the comparison, next to the code. The black-box suite still has an `e2e` test for API-011's observable part (every wrong token gets 401, whatever its length or content), and that test is the one `make trace` counts. Any other exception to black-box testing needs its own ADR.

## Consequences
- The ADRs, [specs README](../specs/README.md#tests), and `make trace` say the same thing, so a contributor can learn the rule from any of them.
- Milestone coverage is enforced now, without waiting for spec files to reach `Accepted`.
- API-011 no longer depends on timing in CI; the unit test locks in the comparison's structure, which is the point of that requirement.
- Depending on a fork means following its releases; switching back is a new ADR.

## Specs affected
All (test names and traceability), NTF-001, API-011.
