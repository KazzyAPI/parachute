# Parachute

**Docker updates you can trust: back up first, verify after, roll back automatically.**

Keeping containers up to date matters, and blind auto-updates break things. Parachute updates your containers the way a careful operator would. It snapshots your data, checks that the new version really works, and puts the old one back if it doesn't. Built in the open: one small static binary, no database.

> **Status: pre-alpha.** Milestone 1 is report-only: Parachute finds and reports available updates but never changes a container. Do not point it at data you can't lose. Progress is tracked in [`docs/specs/README.md`](docs/specs/README.md).

## What Parachute is built to do

These are the design goals. Today only report-only detection ships; each row below arrives with the milestone in the roadmap.

| | |
|---|---|
| **Backup before every update** | Volumes and bind mounts are snapshotted first. Databases are dumped or stopped so the copy is consistent. |
| **Verified, or rolled back** | After the update Parachute waits for the container to be healthy. If it isn't, the previous image, container and data are restored. |
| **Safe by default** | It starts in report-only mode and updates nothing until you arm it. New releases wait two days, and databases need your approval. |
| **Compose-aware** | A stack updates as a unit, in dependency order, so a web app never meets a half-updated database. |
| **Multi-host** | One instance can look after several Docker hosts over TLS. Nothing is installed on the other hosts. |
| **Quiet notifications** | One digest per cycle through many services (Discord, Slack, Telegram, email, ntfy and more). Secrets are never logged or sent. |
| **Pinned by digest** | An update pulls exactly the digest you approved, never a tag that may have moved since. |

## How an update works

```
detect -> policy -> snapshot -> update -> verify -> commit
                                   \          \
                                    +-- roll back to the previous image and data
```

Every update is a transaction with an ID. The previous image is kept until the new version is verified. A failed rollback freezes the stack and tells you, instead of retrying blindly.

## Quick start

Parachute is pre-alpha and has no published image yet. Build it and look at what it would update:

```bash
git clone https://github.com/KazzyAPI/parachute.git
cd parachute
make image
docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v parachute-state:/var/lib/parachute \
  parachute:dev --run-once
```

You get a list of containers with a newer image available, and nothing is touched. To run it as a daemon, drop `--run-once` and `--rm` and add `-d`. It checks daily by default; set `PARACHUTE_SCHEDULE` for a cron schedule.

Opt a container out with a label, for example `parachute.enable=false`. Every setting and label is in the [configuration spec](docs/specs/16-configuration.md).

> Access to the Docker socket is root-equivalent on the host. [ADR-0010](docs/adr/0010-docker-socket-access-and-security.md) explains how Parachute limits what it does with it.

## Roadmap

| Stage | What it adds | State |
|---|---|---|
| Report-only core | Selection, scheduling, update detection, registry auth, notifications, status | Done; tests are catching up with later spec changes ([deviations](docs/specs/README.md)) |
| Safe updates | Backup, the update transaction, verification, rollback, policies and approvals, hooks | Next |
| Control plane | HTTP API and metrics, multi-host, restore verification | Planned |

## Project principles

- **Spec-driven.** The specs and ADRs are the source of truth, and the tests are written from the specs, not from the code. See [`docs/specs/`](docs/specs/) and [`docs/adr/`](docs/adr/).
- **Black-box tested.** End-to-end tests run the real binary against real Docker and a local registry.
- **Safe by default.** When in doubt, Parachute does nothing and tells you why.
- **Small.** One binary, one JSON state file, no database, no agent on your hosts.

## Coming from another updater?

See the [migration guide](docs/migration.md). It maps settings, labels and metrics, and explains what behaves differently on day one.

## Contributing

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) for the workflow and commit standard. A change in behaviour starts as a change to a spec.

## License

To be decided before the first release.
