# Spec hardening status

Generated from `git log` / `git diff` of `docs/spec-hardening..spec/<group>`. Owner is the group whose branch touches the file.

## Groups

| Group | Branch | State | Commits |
|---|---|---|---|
| selection | spec/selection | complete | 6 |
| transaction | spec/transaction | pending | 0 |
| backup | spec/backup | in progress | 2 |
| platform | spec/platform | in progress | 3 |
| migration | spec/migration | complete | 4 |

## Files

| File | Owner | State | Branch | Last commit | Added | Changed | Superseded |
|---|---|---|---|---|---|---|---|
| adr/0001-spec-driven-development.md | migration | complete | spec/migration | 23a80b7 | 0 | 0 | 0 |
| adr/0002-language-and-docker-client.md | migration | complete | spec/migration | 23a80b7 | 0 | 0 | 0 |
| adr/0003-black-box-tests-from-specs.md | migration | complete | spec/migration | 23a80b7 | 0 | 0 | 0 |
| adr/0004-update-is-a-transaction.md | unassigned | pending | - | - | 0 | 0 | 0 |
| adr/0005-label-and-configuration-namespace.md | unassigned | pending | - | - | 0 | 0 | 0 |
| adr/0006-compose-is-the-unit-of-update.md | unassigned | pending | - | - | 0 | 0 | 0 |
| adr/0007-safe-by-default.md | unassigned | pending | - | - | 0 | 0 | 0 |
| adr/0008-state-storage.md | unassigned | pending | - | - | 0 | 0 | 0 |
| adr/0009-snapshot-format-and-storage.md | unassigned | pending | - | - | 0 | 0 | 0 |
| adr/0010-docker-socket-access-and-security.md | platform | in progress | spec/platform | 6837b83 | 0 | 0 | 0 |
| migration.md | migration | complete | spec/migration | 451b72d | 0 | 0 | 0 |
| specs/01-selection.md | selection | complete | spec/selection | e08ff31 | 1 | 2 | 0 |
| specs/02-scheduling.md | selection | complete | spec/selection | 5c9bed9 | 0 | 3 | 0 |
| specs/03-update-detection.md | selection | complete | spec/selection | 0996cd8 | 1 | 3 | 0 |
| specs/04-update-transaction.md | unassigned | pending | - | - | 0 | 0 | 0 |
| specs/05-dependencies-and-stacks.md | unassigned | pending | - | - | 0 | 0 | 0 |
| specs/06-lifecycle-hooks.md | unassigned | pending | - | - | 0 | 0 | 0 |
| specs/07-backup.md | backup | in progress | spec/backup | 8d4a529 | 2 | 4 | 0 |
| specs/08-verification-and-rollback.md | unassigned | pending | - | - | 0 | 0 | 0 |
| specs/09-update-policies.md | unassigned | pending | - | - | 0 | 0 | 0 |
| specs/10-notifications.md | unassigned | pending | - | - | 0 | 0 | 0 |
| specs/11-http-api-and-metrics.md | unassigned | pending | - | - | 0 | 0 | 0 |
| specs/12-registry-auth.md | selection | complete | spec/selection | 8f66e14 | 2 | 3 | 0 |
| specs/13-daemon-and-runtimes.md | platform | in progress | spec/platform | 3a0d108 | 0 | 4 | 0 |
| specs/14-status-view.md | platform | in progress | spec/platform | 6837b83 | 0 | 1 | 0 |
| specs/15-restore-verification.md | unassigned | pending | - | - | 0 | 0 | 0 |
| specs/16-configuration.md | platform | in progress | spec/platform | f89670d | 0 | 3 | 0 |
| adr/0012-digest-pinned-updates.md | selection | complete | spec/selection | b2cffbb | 0 | 0 | 0 |
| adr/0011-untrusted-input-handling.md | migration | complete | spec/migration | be8e9e9 | 0 | 0 | 0 |

## Requirement IDs

- specs/01-selection.md: added SEL-013; changed SEL-003, SEL-012; superseded/withdrawn -
- specs/02-scheduling.md: added -; changed SCH-002, SCH-004, SCH-006; superseded/withdrawn -
- specs/03-update-detection.md: added DET-008; changed DET-001, DET-003, DET-007; superseded/withdrawn -
- specs/07-backup.md: added BKP-012, BKP-013; changed BKP-002, BKP-006, BKP-007, BKP-011; superseded/withdrawn -
- specs/12-registry-auth.md: added REG-007, REG-008; changed REG-001, REG-002, REG-006; superseded/withdrawn -
- specs/13-daemon-and-runtimes.md: added -; changed DMN-003, DMN-004, DMN-005, DMN-007; superseded/withdrawn -
- specs/14-status-view.md: added -; changed STS-001; superseded/withdrawn -
- specs/16-configuration.md: added -; changed CFG-007, CFG-009, CFG-011; superseded/withdrawn -

## Cross-group issues

- API-009 referenced in docs/adr/0010-docker-socket-access-and-security.md (platform) but not defined anywhere
- API-011 referenced in docs/adr/0010-docker-socket-access-and-security.md (platform) but not defined anywhere
