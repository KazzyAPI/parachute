# Spec hardening status

Generated from `git log` / `git diff` of `docs/spec-hardening..spec/<group>`. Owner is the group whose branch touches the file.

## Groups

| Group | Branch | State | Commits |
|---|---|---|---|
| selection | spec/selection | complete | 9 |
| transaction | spec/transaction | complete | 11 |
| backup | spec/backup | complete | 12 |
| platform | spec/platform | complete | 13 |
| migration | spec/migration | complete | 7 |
| integration | docs/spec-hardening | complete | - |

## Files

| File | Owner | State | Branch | Last commit | Added | Changed | Superseded |
|---|---|---|---|---|---|---|---|
| adr/0001-spec-driven-development.md | migration | complete | spec/migration | 23a80b7 | 0 | 0 | 0 |
| adr/0002-language-and-docker-client.md | migration | complete | spec/migration | 23a80b7 | 0 | 0 | 0 |
| adr/0003-black-box-tests-from-specs.md | migration | complete | spec/migration | 23a80b7 | 0 | 0 | 0 |
| adr/0004-update-is-a-transaction.md | transaction | complete | spec/transaction | e3a8715 | 0 | 0 | 0 |
| adr/0005-label-and-configuration-namespace.md | platform | complete | spec/platform | b8d07bb | 0 | 0 | 0 |
| adr/0006-compose-is-the-unit-of-update.md | transaction | complete | spec/transaction | e3a8715 | 0 | 0 | 0 |
| adr/0007-safe-by-default.md | integration | complete (accepted, Q19) | docs/spec-hardening | ae20953 | 0 | 0 | 0 |
| adr/0008-state-storage.md | platform | complete | spec/platform | b8d07bb | 0 | 0 | 0 |
| adr/0009-snapshot-format-and-storage.md | integration | complete (accepted, Q19) | docs/spec-hardening | ae20953 | 0 | 0 | 0 |
| adr/0010-docker-socket-access-and-security.md | platform | complete | spec/platform | 6837b83 | 0 | 0 | 0 |
| adr/0011-untrusted-input-handling.md | migration | complete | spec/migration | be8e9e9 | 0 | 0 | 0 |
| adr/0012-digest-pinned-updates.md | selection | complete | spec/selection | b2cffbb | 0 | 0 | 0 |
| adr/0013-secret-redaction.md | platform | complete | spec/platform | 581c310 | 0 | 0 | 0 |
| adr/0014-self-update-safety.md | transaction | complete | spec/transaction | 00c136f | 0 | 0 | 0 |
| adr/0015-traceability-and-test-practice.md | integration | complete (new, Q15, Q23) | docs/spec-hardening | 5d5a07f | 0 | 0 | 0 |
| migration.md | migration | complete | spec/migration | 451b72d | 0 | 0 | 0 |
| specs/01-selection.md | selection | complete | spec/selection | e08ff31 | 1 | 2 | 0 |
| specs/02-scheduling.md | selection | complete | spec/selection | 5c9bed9 | 0 | 3 | 0 |
| specs/03-update-detection.md | selection | complete | spec/selection | 0996cd8 | 1 | 3 | 0 |
| specs/04-update-transaction.md | transaction | complete | spec/transaction | b1d64bb | 1 | 9 | 0 |
| specs/05-dependencies-and-stacks.md | transaction | complete | spec/transaction | b1d64bb | 0 | 2 | 0 |
| specs/06-lifecycle-hooks.md | backup | complete | spec/backup | 4f924b1 | 1 | 3 | 0 |
| specs/07-backup.md | backup | complete | spec/backup | 4f924b1 | 2 | 9 | 0 |
| specs/08-verification-and-rollback.md | transaction | complete | spec/transaction | 9399d4b | 0 | 4 | 0 |
| specs/09-update-policies.md | backup | complete | spec/backup | b76218e | 1 | 2 | 0 |
| specs/10-notifications.md | platform | complete | spec/platform | 8fd70c3 | 1 | 3 | 0 |
| specs/11-http-api-and-metrics.md | platform | complete | spec/platform | 581c310 | 4 | 2 | 0 |
| specs/12-registry-auth.md | selection | complete | spec/selection | 8f66e14 | 2 | 3 | 0 |
| specs/13-daemon-and-runtimes.md | platform | complete | spec/platform | 3a0d108 | 0 | 4 | 0 |
| specs/14-status-view.md | platform | complete | spec/platform | 2a66de0 | 2 | 4 | 0 |
| specs/15-restore-verification.md | backup | complete | spec/backup | 20a8e30 | 0 | 4 | 0 |
| specs/16-configuration.md | platform | complete | spec/platform | b8d07bb | 1 | 7 | 0 |
| adr/README.md | migration | complete | spec/migration | cd81f45 | 0 | 0 | 0 |
| specs/glossary.md | migration | complete | spec/migration | 8734816 | 0 | 0 | 0 |
| specs/reasons.md | migration | complete | spec/migration | 8e147e2 | 0 | 0 | 0 |

## Requirement IDs

- specs/01-selection.md: added SEL-013; changed SEL-003, SEL-012; superseded/withdrawn -
- specs/02-scheduling.md: added -; changed SCH-002, SCH-004, SCH-006; superseded/withdrawn -
- specs/03-update-detection.md: added DET-008; changed DET-001, DET-003, DET-007; superseded/withdrawn -
- specs/04-update-transaction.md: added UPD-020; changed UPD-001, UPD-008, UPD-009, UPD-013, UPD-014, UPD-016, UPD-017, UPD-018, UPD-019; superseded/withdrawn -
- specs/05-dependencies-and-stacks.md: added -; changed DEP-003, DEP-008; superseded/withdrawn -
- specs/06-lifecycle-hooks.md: added HOK-008; changed HOK-003, HOK-006, HOK-007; superseded/withdrawn -
- specs/07-backup.md: added BKP-012, BKP-013; changed BKP-001, BKP-002, BKP-004, BKP-006, BKP-007, BKP-008, BKP-009, BKP-010, BKP-011; superseded/withdrawn -
- specs/08-verification-and-rollback.md: added -; changed VER-001, VER-008, VER-010, VER-011; superseded/withdrawn -
- specs/09-update-policies.md: added POL-011; changed POL-007, POL-010; superseded/withdrawn -
- specs/10-notifications.md: added NTF-014; changed NTF-001, NTF-007, NTF-013; superseded/withdrawn -
- specs/11-http-api-and-metrics.md: added API-009, API-010, API-011, API-012; changed API-002, API-007, API-008; superseded/withdrawn -
- specs/12-registry-auth.md: added REG-007, REG-008; changed REG-001, REG-002, REG-006; superseded/withdrawn -
- specs/13-daemon-and-runtimes.md: added -; changed DMN-003, DMN-004, DMN-005, DMN-007; superseded/withdrawn -
- specs/14-status-view.md: added STS-006, STS-007; changed STS-001, STS-002, STS-004, STS-005; superseded/withdrawn -
- specs/15-restore-verification.md: added -; changed RST-001, RST-002, RST-003, RST-005; superseded/withdrawn -
- specs/16-configuration.md: added CFG-012; changed CFG-001, CFG-006, CFG-007, CFG-008, CFG-009, CFG-010, CFG-011; superseded/withdrawn -

## Cross-group issues

Resolved in the integration pass on `docs/spec-hardening`:

- ADR index lists 0011 to 0014; statuses match each ADR's own Status line.
- [reasons.md](reasons.md) producers synced (UPD-007, UPD-020, HOK-008, DET-008, REG-007, REG-008, SEL-013); conflicts 3, 4, 7, 8, 10 closed (`image_mismatch` added, Q16).
- Cross-references aligned: UPD-001, UPD-014, UPD-018 to UPD-020; POL-002, POL-006, POL-010, API-007 to DET-008 and the `frozen` field; UPD-017 helper proof by label (BKP-010); BKP-001, BKP-003 own-container `none`; SEL-012 engine-identified own container; CFG label table and CFG-010 handover format; REG-006 to ADR-0013; glossary to STS-006, STS-007; DMN-006 alias removed.
- [README.md](README.md) M1 deviations list `--allow-insecure-tcp` and the M1-covered IDs whose meaning changed.

Decisions applied 2026-10-23 ([open questions](open-questions.md) Q14 to Q26, all resolved as recommended):

- Q14: own container identified only by the engine; `parachute.self` only excludes (SEL-012, SEL-013, UPD-018, CFG label reference, glossary, migration guide).
- Q15, Q23: [ADR-0015](../adr/0015-traceability-and-test-practice.md) supersedes the drifted parts of ADR-0001 to ADR-0003 and allows a unit test for API-011; README test conventions and NTF intro updated.
- Q16: `rolled_back`/`image_mismatch` (UPD-020, reasons.md).
- Q17: the state-file route to lift a freeze is removed from the glossary.
- Q18, Q21: ADR-0008 Amendment records the state-file replay limit; API-007 applies a stored approval only for the current candidate digest.
- Q19: ADR-0007 and ADR-0009 accepted.
- Q22: `--trusted-proxies` / `PARACHUTE_TRUSTED_PROXIES` (API-010, CFG settings reference).
- Q20, Q24, Q25, Q26: specs already matched; no change.

Still open: Q13 (URL lists with commas).

Checks: every referenced requirement ID is in the README index; every relative link and anchor resolves; the old tool's name appears only in `docs/migration.md`; `make trace` passes (M1: 61 requirements, 61 tested).
