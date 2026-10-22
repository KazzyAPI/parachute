# Architecture Decision Records

ADRs record decisions the code must follow. Specs say **what** the system does; ADRs say **how we constrain building it**.

- One decision per file, numbered, never renumbered.
- Status: `Proposed` (not yet agreed), `Accepted`, `Superseded by ADR-NNNN`.
- A change of mind is a new ADR that supersedes the old one. Old ADRs are not edited except for status.
- Use [0000-template.md](0000-template.md).

| ADR | Title | Status |
|---|---|---|
| [0001](0001-spec-driven-development.md) | Spec-driven development | Accepted |
| [0002](0002-language-and-docker-client.md) | Go, using the official Docker client | Accepted |
| [0003](0003-black-box-tests-from-specs.md) | Black-box tests derived from specs | Accepted |
| [0004](0004-update-is-a-transaction.md) | An update is a transaction | Accepted |
| [0005](0005-label-and-configuration-namespace.md) | Label and configuration namespace | Accepted |
| [0006](0006-compose-is-the-unit-of-update.md) | The Compose stack is the unit of update | Accepted |
| [0007](0007-safe-by-default.md) | Safe by default | Proposed |
| [0008](0008-state-storage.md) | State storage layout | Accepted |
| [0009](0009-snapshot-format-and-storage.md) | Snapshot format and storage | Proposed |
| [0010](0010-docker-socket-access-and-security.md) | Docker socket access and security model | Accepted |
| [0011](0011-untrusted-input-handling.md) | Untrusted input handling | Accepted |
| [0012](0012-digest-pinned-updates.md) | Digest-pinned updates | Accepted |
| [0013](0013-secret-redaction.md) | Secret redaction | Accepted |
| [0014](0014-self-update-safety.md) | Self-update safety | Accepted |

0007 and 0009 stay Proposed pending a user decision (see [open questions](../specs/open-questions.md)).
