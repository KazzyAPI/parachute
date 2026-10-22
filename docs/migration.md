# Migrating from Watchtower to Parachute

Parachute is not a drop-in replacement: it uses its own names (`parachute.*` labels, `PARACHUTE_*` variables) and it is **safer by default**, so some things behave differently on day one. This guide lists every change. Most migrations are a find-and-replace plus reading the "behavior differences" section once.

> Status: draft, written against the specs in [`docs/specs/`](specs/). Parachute has no released code yet; check names against the spec before relying on this.

## 1. The 5-minute path

1. Replace the image in your compose file.
2. Rename your environment variables and labels using the tables below.
3. Start it. It will **not update anything** yet (see "Armed mode").
4. Read the first notification: it lists what Parachute *would* update.
5. Set `PARACHUTE_ARMED=true` when you are happy.

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
      PARACHUTE_ARMED: "true"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - parachute-state:/var/lib/parachute
volumes:
  parachute-state:
```

Notification URLs are unchanged: both tools use the same URL format.

## 2. Environment variables and flags

For most settings the name is the same with `WATCHTOWER_` changed to `PARACHUTE_`. Exceptions are listed first.

| Watchtower | Parachute | Notes |
|---|---|---|
| `WATCHTOWER_POLL_INTERVAL` / `--interval` | `PARACHUTE_INTERVAL` / `--interval` | Seconds, default 86400 (same). |
| `WATCHTOWER_TIMEOUT` / `--stop-timeout` | `PARACHUTE_STOP_TIMEOUT` | Default `10s` (same). |
| `WATCHTOWER_LIFECYCLE_HOOKS` | `PARACHUTE_ENABLE_LIFECYCLE_HOOKS` | Off by default (same). |
| `--health-check` | `parachute healthcheck` | A command, not a flag. |
| `--porcelain v1` | `--porcelain v1` | **Format differs**: JSON lines, see [CFG-007](specs/16-configuration.md). |
| `WATCHTOWER_NOTIFICATION_REPORT` | `PARACHUTE_NOTIFICATION_REPORT` | **Default is now on**, not off. |

Same name with the prefix swapped: `SCHEDULE`, `CLEANUP`, `REMOVE_VOLUMES`, `NO_PULL`, `NO_RESTART`, `NO_STARTUP_MESSAGE`, `RUN_ONCE`, `MONITOR_ONLY`, `LABEL_ENABLE`, `LABEL_TAKE_PRECEDENCE`, `DISABLE_CONTAINERS`, `INCLUDE_RESTARTING`, `INCLUDE_STOPPED`, `REVIVE_STOPPED`, `SCOPE`, `ROLLING_RESTART`, `WARN_ON_HEAD_FAILURE`, `HTTP_API_UPDATE`, `HTTP_API_METRICS`, `HTTP_API_TOKEN`, `HTTP_API_PERIODIC_POLLS`, `NOTIFICATION_URL`, `NOTIFICATIONS_LEVEL`, `NOTIFICATIONS_HOSTNAME`, `NOTIFICATION_TITLE_TAG`, `NOTIFICATION_SKIP_TITLE`, `NOTIFICATIONS_DELAY`, `NOTIFICATION_TEMPLATE`, `NOTIFICATION_LOG_STDOUT`.

Unchanged Docker variables: `DOCKER_HOST`, `DOCKER_API_VERSION` (no longer needed: the version is negotiated), `DOCKER_TLS_VERIFY`, `DOCKER_CERT_PATH`, `DOCKER_CONFIG`, `TZ`, `NO_COLOR`.

**Removed**, with the replacement. Parachute exits at startup with a message if you leave one set:

| Removed | Use instead |
|---|---|
| `WATCHTOWER_NOTIFICATIONS`, `--notification-email-*`, `--notification-slack-*`, `--notification-msteams-*`, `--notification-gotify-*` | `PARACHUTE_NOTIFICATION_URL` with a service URL |
| `notify-upgrade` | Write the URL by hand; see the service URL docs |

Unknown or misspelled settings are an error too, not silently ignored.

**New settings** with no Watchtower equivalent that you may want to know about: `PARACHUTE_HTTP_API_STATUS_TOKEN` (read-only token for the status page, section 5) and `PARACHUTE_TRANSACTION_TIMEOUT` (default `30m`, the most one update may take, hooks included; section 6).

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

## 4. Metrics

| Watchtower | Parachute |
|---|---|
| `watchtower_containers_scanned` | `parachute_containers_scanned` |
| `watchtower_containers_updated` | `parachute_containers_updated` |
| `watchtower_containers_failed` | `parachute_containers_failed` |
| `watchtower_scans_total` | `parachute_scans_total` |
| `watchtower_scans_skipped` | `parachute_scans_skipped` |

Also: the endpoint is still `/v1/metrics`, but a token is now required and sent as `Authorization: Bearer <token>`. Parachute adds `parachute_containers_held`, `parachute_rollbacks_total`, `parachute_rollback_failures_total`, `parachute_host_up`, and `parachute_restore_verification_last_success_timestamp_seconds`. Update your dashboards and alert rules with find-and-replace on the metric names.

## 5. HTTP API

- `/v1/update` is unchanged in purpose. A token is now **mandatory** (Parachute will not start the API without one) and must be sent as `Authorization: Bearer <token>`.
- A request that arrives while a cycle runs returns **409** instead of queuing a second cycle.
- `/v1/update` accepts **no query parameters**: any parameter (for example `?image=`) returns **400** naming it and nothing runs. Remove parameters from scripts that sent them.
- HTTP Basic authentication (any user name, token as password) works only on the status page (`/status`) and `/v1/status`. For dashboards and browsers, set `PARACHUTE_HTTP_API_STATUS_TOKEN` to a separate **read-only** token that works only there; the main token can trigger updates and is equivalent to root on the host.

## 6. Behavior differences you will notice

| Behavior | Watchtower | Parachute | Relax it with |
|---|---|---|---|
| First run | Updates at its first check | **Reports only** until armed | `PARACHUTE_ARMED=true` |
| Backup | None | Snapshot of volumes before every update; **no snapshot, no update** | `--backup`, or `parachute.backup=none` per container |
| After an update | Nothing checked | Waits for the container to become healthy (120 s, or longer when its health check's start period needs it); **rolls back** if not, with reason `verify_timeout` when time ran out | `--verify-timeout`, `--verify-stable`, `parachute.verify.timeout` per container |
| Free disk space | Not checked | Update **refused** (`insufficient_space`, nothing stopped) when the snapshot store has less than the estimated snapshot size + 10 % + 1 GiB free | Free space, `--backup-volume`, `--backup-retention`, or `parachute.backup=none` |
| Databases | Updated like any container | **Held for approval** | `--protect-databases=false` or `parachute.database=false` |
| New releases | Updated at once | Waits `--delay-days` (default 2) | `PARACHUTE_DELAY_DAYS=0` |
| Version-pinned images (`app:1.2`) | Re-pulled on same tag | Not moved to a different tag unless opted in | `--tag-follow patch\|minor` |
| Image cleanup | Removes old image during update | Only **after** the update is committed | n/a |
| `pre-update` hook fails | Update continues | **Update of the stack is aborted** | n/a (exit 0 to proceed; exit 75 to defer to the next cycle) |
| Hook timeout `0` | Hook may run forever | Still ended after `--transaction-timeout` (default 30 min) | `--transaction-timeout` |
| Compose stacks | Containers treated individually | Stack is the unit of backup, verify, and rollback; only the service and its dependents restart | n/a |
| Notifications | Report mode is opt-in | **One digest per cycle** by default | `--notification-report=false` |
| Anonymous volumes | Removed only with its flag | Reattached to the new container by default | `--remove-volumes` |
| Exit codes (run-once) | Check its docs | 0 ok, 1 failed or rolled back, 2 config error, 3 rollback failed, 4 no host reachable | n/a |

If you want the old "update everything immediately" experience, set `PARACHUTE_PROFILE=convenient`. It arms Parachute, removes the update delay, and stops holding databases for approval. You still get backups and rollback; turn those off separately with `--backup none` if you really want to.

## 7. Checklist

- [ ] Image replaced, state volume added
- [ ] Environment variables renamed (section 2)
- [ ] Removed settings deleted (section 2)
- [ ] Labels renamed on every container you had labelled (section 3)
- [ ] Dashboards and alerts use `parachute_*` metrics (section 4)
- [ ] API clients send a Bearer token (section 5)
- [ ] First report read, then `PARACHUTE_ARMED=true` set
- [ ] Old Watchtower container stopped and removed (run only one updater against the same containers)
