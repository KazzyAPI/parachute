# Migrating from Watchtower to Parachute

Parachute is not a drop-in replacement: it uses its own names (`parachute.*` labels, `PARACHUTE_*` variables) and it is **safer by default**, so some things behave differently on day one. This guide lists every change. Most migrations are a find-and-replace plus reading the "behavior differences" section once.

> Status: draft, written against the specs in [`docs/specs/`](specs/). The current milestone only reports: it detects updates but cannot apply them, and arming it is a startup error until update execution ships ([M1 deviations](specs/README.md#m1-deviations)). Check names against the spec before relying on this.

## 1. The 5-minute path

1. Replace the image in your compose file.
2. Rename your environment variables and labels using the tables below.
3. Start it. Parachute always starts **unarmed** and will **not update anything** ([CFG-006](specs/16-configuration.md)).
4. Read the first notification: it lists what Parachute *would* update (`would_update`).
5. Arm it when you are happy (below).

```yaml
# before
services:
  watchtower:
    image: containrrr/watchtower
    environment:
      WATCHTOWER_SCHEDULE: "0 0 4 * * *"
      WATCHTOWER_CLEANUP: "true"
      WATCHTOWER_NOTIFICATION_URL: "discord://token@id"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock

# after
services:
  parachute:
    image: <parachute image>
    environment:
      PARACHUTE_SCHEDULE: "0 0 4 * * *"
      PARACHUTE_CLEANUP: "true"
      PARACHUTE_NOTIFICATION_URL: "discord://token@id"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - parachute-state:/var/lib/parachute
volumes:
  parachute-state:
```

Notification URLs are unchanged: both tools use the same URL format.

To arm it, add one of these and recreate the Parachute container. Arming is read at every start from these sources only, never from the state file, so removing the setting disarms it again:

```yaml
    environment:
      PARACHUTE_ARMED: "true"        # or the flag --armed
    # or, on Parachute's own container:
    labels:
      parachute.armed: "true"
```

If Docker is reached over `tcp://` without `DOCKER_TLS_VERIFY`, Parachute refuses to start (exit code 2); see section 6.

## 2. Environment variables and flags

For most settings the name is the same with `WATCHTOWER_` changed to `PARACHUTE_`. Exceptions are listed first.

| Watchtower | Parachute | Notes |
|---|---|---|
| `WATCHTOWER_POLL_INTERVAL` / `--interval` | `PARACHUTE_INTERVAL` / `--interval` | Seconds, default 86400 (same). |
| `WATCHTOWER_TIMEOUT` / `--stop-timeout` | `PARACHUTE_STOP_TIMEOUT` | Default `10s` (same). |
| `WATCHTOWER_LIFECYCLE_HOOKS` | `PARACHUTE_ENABLE_LIFECYCLE_HOOKS` | Off by default (same). |
| `--health-check` | `parachute healthcheck` | A command, not a flag. There is no `--health-check` alias: change your `HEALTHCHECK` / `healthcheck:` test. The official image already declares it. |
| `--porcelain v1` | `--porcelain v1` | **Format differs**: JSON lines, see [CFG-007](specs/16-configuration.md). |
| `WATCHTOWER_NOTIFICATION_REPORT` | `PARACHUTE_NOTIFICATION_REPORT` | **Default is now on**, not off. |

Same name with the prefix swapped: `DEBUG`, `TRACE`, `LOG_LEVEL`, `LOG_FORMAT`, `SCHEDULE`, `CLEANUP`, `REMOVE_VOLUMES`, `NO_PULL`, `NO_RESTART`, `NO_STARTUP_MESSAGE`, `RUN_ONCE`, `MONITOR_ONLY`, `LABEL_ENABLE`, `LABEL_TAKE_PRECEDENCE`, `DISABLE_CONTAINERS`, `INCLUDE_RESTARTING`, `INCLUDE_STOPPED`, `REVIVE_STOPPED`, `SCOPE`, `ROLLING_RESTART`, `WARN_ON_HEAD_FAILURE`, `HTTP_API_UPDATE`, `HTTP_API_METRICS`, `HTTP_API_TOKEN`, `HTTP_API_PERIODIC_POLLS`, `NOTIFICATION_URL`, `NOTIFICATIONS_LEVEL`, `NOTIFICATIONS_HOSTNAME`, `NOTIFICATION_TITLE_TAG`, `NOTIFICATION_SKIP_TITLE`, `NOTIFICATIONS_DELAY`, `NOTIFICATION_TEMPLATE`, `NOTIFICATION_LOG_STDOUT`.

Parachute does not read `WATCHTOWER_*` variables or accept legacy aliases: a variable you forget to rename is ignored, so its setting silently falls back to the Parachute default. Rename them all.

Unchanged Docker variables (and their flags `--host`, `--api-version`, `--tlsverify`): `DOCKER_HOST`, `DOCKER_API_VERSION` (no longer needed: the version is negotiated), `DOCKER_TLS_VERIFY`, `DOCKER_CERT_PATH`, `DOCKER_CONFIG`, `TZ`, `NO_COLOR`.

**Removed**, with the replacement. If you rename one of these to its `PARACHUTE_*` or flag form, Parachute exits at startup (code 2) naming the replacement ([CFG-005](specs/16-configuration.md)):

| Removed | Use instead |
|---|---|
| `--notifications` / `PARACHUTE_NOTIFICATIONS`, `--notification-email-*`, `--notification-slack-*`, `--notification-msteams-*`, `--notification-gotify-*` | `PARACHUTE_NOTIFICATION_URL` with a service URL |
| `notify-upgrade` | Write the URL by hand; see the service URL docs |

Unknown or misspelled `PARACHUTE_*` variables and flags are an error too (exit code 2), not silently ignored.

**New settings** with no Watchtower equivalent that you may want to know about: `PARACHUTE_ARMED` (section 1), `PARACHUTE_HTTP_API_STATUS_TOKEN` (read-only token for the status page, section 5), `PARACHUTE_HTTP_API_PORT` (default `8080`), `PARACHUTE_TRANSACTION_TIMEOUT` (default `30m`, the most one update may take, hooks included; section 6), and `PARACHUTE_ALLOW_INSECURE_TCP` (section 6). The full list is the [settings reference](specs/16-configuration.md#settings-reference).

## 3. Labels

| Watchtower | Parachute |
|---|---|
| `com.centurylinklabs.watchtower.enable` | `parachute.enable` |
| `com.centurylinklabs.watchtower.monitor-only` | `parachute.monitor-only` |
| `com.centurylinklabs.watchtower.no-pull` | `parachute.no-pull` |
| `com.centurylinklabs.watchtower.scope` | `parachute.scope` |
| `com.centurylinklabs.watchtower.depends-on` | `parachute.depends-on` |
| `com.centurylinklabs.watchtower.stop-signal` | `parachute.stop-signal` |
| `com.centurylinklabs.watchtower.lifecycle.pre-check` (and `pre-update`, `post-update`, `post-check`) | `parachute.lifecycle.pre-check` (and the same three) |
| `...lifecycle.pre-update-timeout`, `...post-update-timeout` (minutes) | `parachute.lifecycle.pre-update-timeout`, `...post-update-timeout` (minutes) |

Quick rename for compose files (review the diff before keeping it):

```sh
sed -i.bak 's/com\.centurylinklabs\.watchtower\./parachute./g' docker-compose.yml
```

Old labels are ignored, not translated. A container still carrying only old labels is treated as unlabelled, so check anything you had opted out with `enable=false`.

Set `parachute.*` labels on the container (in your compose file or `docker run --label`). Labels baked into an image are not read: an image cannot change how Parachute treats it. The only exceptions are `parachute.monitor-only=true`, `parachute.approval=required`, and `parachute.self=true`, which can only make Parachute more cautious (a container with `parachute.self=true` is never updated, unless it is Parachute's own container). Parachute finds its own container through Docker; it needs no label for that.

## 4. Metrics

| Watchtower | Parachute |
|---|---|
| `watchtower_containers_scanned` | `parachute_containers_scanned` |
| `watchtower_containers_updated` | `parachute_containers_updated` |
| `watchtower_containers_failed` | `parachute_containers_failed` |
| `watchtower_scans_total` | `parachute_scans_total` |
| `watchtower_scans_skipped` | `parachute_scans_skipped` |

Also: the endpoint is still `/v1/metrics`, but a token is now required and sent as `Authorization: Bearer <token>` (the read-only status token is refused there). Every metric except the two `scans` counters now carries a `host` label, for example `parachute_containers_updated{host="local"}`; adjust queries that expected no labels. `parachute_containers_failed` counts `failed`, `rolled_back`, and `rollback_failed` results. Parachute adds `parachute_containers_held`, `parachute_rollbacks_total`, `parachute_rollback_failures_total`, `parachute_host_up`, and `parachute_restore_verification_last_success_timestamp_seconds`. Update your dashboards and alert rules with find-and-replace on the metric names.

## 5. HTTP API

- `/v1/update` is unchanged in purpose. A token is now **mandatory** (Parachute will not start the API without one) and must be sent as `Authorization: Bearer <token>`.
- A request that arrives while a cycle runs returns **409** instead of queuing a second cycle.
- `/v1/update` accepts **no query parameters**: any parameter (for example `?image=`) returns **400** naming it and nothing runs. Remove parameters from scripts that sent them.
- New: `POST /v1/approve?container=<name>` approves an update that is held for approval (databases, `parachute.approval=required`, Parachute's own container) or held after a failed attempt.
- HTTP Basic authentication (any user name, token as password) works only on the status page (`/status`) and `/v1/status`. For dashboards and browsers, set `PARACHUTE_HTTP_API_STATUS_TOKEN` to a separate **read-only** token that works only there; the main token can trigger updates and is equivalent to root on the host.

## 6. Behavior differences you will notice

| Behavior | Watchtower | Parachute | Relax it with |
|---|---|---|---|
| First run | Updates at its first check | **Reports only** (`would_update`) until armed; every start is unarmed unless arming is set | `PARACHUTE_ARMED=true`, `--armed`, or `parachute.armed=true` on Parachute's container |
| Backup | None | Snapshot of volumes before every update; **no snapshot, no update** (`failed` / `snapshot_failed`) | `--backup none`, or `parachute.backup=none` per container |
| After an update | Nothing checked | Waits for the container to become healthy (120 s, or longer when its health check's start period needs it); **rolls back** if not, with reason `verify_timeout` when time ran out | `--verify-timeout`, `--verify-stable`, `parachute.verify.timeout` per container |
| Free disk space | Not checked | Update **refused** (`insufficient_space`, nothing stopped) when the snapshot store has less than the estimated snapshot size + 10 % + 1 GiB free | Free space, `--backup-volume`, `--backup-retention`, or `parachute.backup=none` |
| Databases | Updated like any container | **Held for approval** (`held` / `approval_required`) until `POST /v1/approve` | `--protect-databases=false` or `parachute.database=false` |
| Parachute's own container | Updates itself | **Always held for approval** (`approval_required`); no setting or profile removes this | n/a (approve with `POST /v1/approve`) |
| New releases | Updated at once | Waits `--delay-days` (default 2) after Parachute first sees the digest (`held` / `delay`) | `PARACHUTE_DELAY_DAYS=0` |
| Version tags (`app:1.2.0`) | Re-pulled on the same tag only | Same: a rebuilt image on the same tag is an ordinary update. Optionally moves to the highest `MAJOR.MINOR.PATCH` tag in the same minor (`patch`) or major (`minor`) line; tags of any other form, such as `1.2` or `latest`, are never moved | `--tag-follow patch\|minor` or `parachute.tag-follow` |
| A failed update | Retried at the next check | That image digest is **not retried** (`held` / `previous_attempt_failed`) until a newer image appears or it is approved; after `rollback_failed` the stack is frozen until approved | `POST /v1/approve` |
| Image cleanup | Removes old image during update | Only **after** the update is committed | n/a |
| `pre-update` hook fails | Update continues | **Update of the stack is aborted**, nothing stopped (`failed` / `pre_update_hook_failed`) | n/a (exit 0 to proceed; exit 75 to defer to the next cycle: `held` / `hook_deferred`, not an error) |
| Hook timeout `0` | Hook may run forever | Still ended after `--transaction-timeout` (default 30 min), with reason `transaction_timeout` | `--transaction-timeout` |
| Compose stacks | Containers treated individually | Stack is the unit of backup, verify, and rollback; only the service and its dependents restart | n/a |
| Notifications | Report mode is opt-in | **One digest per cycle** by default | `--notification-report=false` |
| Anonymous volumes | Removed only with its flag | Reattached to the new container by default | `--remove-volumes` |
| Plain `tcp://` Docker host | Allowed | **Startup error** (exit code 2) naming the host, unless TLS is verified (`DOCKER_TLS_VERIFY`) | `PARACHUTE_ALLOW_INSECURE_TCP=true` / `--allow-insecure-tcp` (logged at `warn` at startup and every cycle) |
| Exit codes (run-once) | Check its docs | 0 ok (including `held`, `skipped`, `would_update`), 1 a `failed` or `rolled_back` result or an unreachable host, 2 configuration or startup error, 3 a `rollback_failed` result, 4 no host reachable at startup; the highest applicable code wins | n/a |
| Results | Updated / failed / skipped | Every container ends each cycle in one [result state](specs/reasons.md) with one reason code, in notifications, `--porcelain v1`, status, and metrics | n/a |

If you want the old "update everything immediately" experience, set `PARACHUTE_PROFILE=convenient`. It arms Parachute, removes the update delay, and stops holding databases for approval. Updates of Parachute's own container are still held for approval. You still get backups and rollback; turn backups off separately with `--backup none` if you really want to.

## 7. Checklist

- [ ] Image replaced, state volume added
- [ ] Health checks call `parachute healthcheck`, not `--health-check`
- [ ] Docker over `tcp://` uses verified TLS (or `PARACHUTE_ALLOW_INSECURE_TCP=true` deliberately)
- [ ] Environment variables renamed (section 2)
- [ ] Removed settings deleted (section 2)
- [ ] Labels renamed on every container you had labelled (section 3)
- [ ] Dashboards and alerts use `parachute_*` metrics (section 4)
- [ ] API clients send a Bearer token (section 5)
- [ ] First report read, then Parachute armed (`PARACHUTE_ARMED=true`)
- [ ] Approval flow known for databases and Parachute itself (`POST /v1/approve`)
- [ ] Old Watchtower container stopped and removed (run only one updater against the same containers)
