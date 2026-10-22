# Parachute

> **Pre-alpha. Milestone 1 is report-only: Parachute finds and reports available updates but never changes a container. Do not point this at real data.**

Safe Docker updates with automatic backup and rollback.

Parachute watches your containers for new images and updates them the way a careful operator would: it snapshots your data first, checks that the new version actually works, and puts the old one back if it doesn't. It understands Compose stacks, can manage several hosts, and starts in report-only mode until you tell it to act.

Coming from another updater? See the [migration guide](docs/migration.md).

## How this project works

Parachute is **spec-driven**. The specs and ADRs are the source of truth, and tests are written from the specs, not from the code.

| Where | What |
|---|---|
| [`docs/adr/`](docs/adr/) | Architecture decisions. The code must follow them. |
| [`docs/specs/`](docs/specs/) | Behavior requirements, one file per domain. |
| [`docs/specs/README.md`](docs/specs/README.md) | The spec format, requirement IDs, and how tests trace to them. |
| [`docs/migration.md`](docs/migration.md) | Moving from another updater. |

Start with [`docs/specs/README.md`](docs/specs/README.md), then [`docs/specs/open-questions.md`](docs/specs/open-questions.md).
