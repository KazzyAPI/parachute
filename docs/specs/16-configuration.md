# 16 Configuration, output, and safety switches

**Status:** Draft

The configuration surface, the arming switch, machine-readable output, logging, exit codes, and the state file. Naming is set by [ADR-0005](../adr/0005-label-and-configuration-namespace.md).

### CFG-001 Names
Every setting MUST be available as a flag `--<name>` and as an environment variable `PARACHUTE_<NAME>` (upper case, `-` replaced by `_`), except the Docker client variables `DOCKER_HOST`, `DOCKER_API_VERSION`, `DOCKER_TLS_VERIFY`, `DOCKER_CERT_PATH`, `DOCKER_CONFIG`, and the variables `TZ` and `NO_COLOR`, which keep their standard names. Per-container settings are labels `parachute.<name>`. The full list is in [Settings reference](#settings-reference) and [Label reference](#label-reference).

- **Given** `PARACHUTE_INTERVAL=60` **When** Parachute starts **Then** the startup message announces the next cycle 60 seconds after the first, as with `--interval 60`.
- **Given** `PARACHUTE_STOP_TIMEOUT=5s` and a container that ignores `SIGTERM` **When** it is updated **Then** the Docker event stream shows its `kill` with `SIGKILL` 5 seconds after the stop signal, as with `--stop-timeout 5s`.

### CFG-002 Precedence
For each global setting, a flag MUST win over its environment variable, which wins over the default. A per-container label wins over the global setting where the label reference says so; `parachute.monitor-only` and `parachute.no-pull` follow SEL-008 instead.

- **Given** `PARACHUTE_INTERVAL=60` and `--interval 120` **When** Parachute starts **Then** the announced next cycle is 120 seconds after the first.

### CFG-003 Value syntax
Booleans MUST accept `true`, `false`, `1`, `0` (case-insensitive); a boolean flag given without a value means `true`. Lists MUST accept items separated by spaces, commas, or both, with or without surrounding single or double quotes on the whole value. Durations use Go syntax (`90s`, `5m`, `2h`). Integers and decimals are base 10. An invalid value MUST be a startup error (exit code 2) naming the setting and the value.

- **Given** `PARACHUTE_DISABLE_CONTAINERS='"a, b c"'` **When** a cycle runs **Then** `a`, `b`, and `c` are excluded.
- **Given** `PARACHUTE_CLEANUP=yes` **When** Parachute starts **Then** it exits with code 2 and the message contains `cleanup` and `yes`.

### CFG-004 Secrets from files
For `--notification-url`, `--http-api-token`, `--http-api-status-token`, and `--notification-template`, when the value is an absolute path to an existing file, Parachute MUST use the file's contents, with one trailing newline removed. A path that exists but cannot be read MUST be a startup error (exit code 2).

- **Given** `PARACHUTE_HTTP_API_TOKEN=/run/secrets/token` containing `abc\n` **When** a request carries `Bearer abc` **Then** it is accepted.
- **Given** the same file with mode `000` for Parachute's user **When** Parachute starts **Then** it exits with code 2.

### CFG-005 Unknown and removed settings fail
An unknown flag, an unknown `PARACHUTE_*` environment variable, or a removed setting MUST be a startup error (exit code 2). For an unknown name the message MUST name it and suggest the closest known setting when one is within edit distance 3. For a removed setting the message MUST name its replacement from this table. Unknown `parachute.*` labels on containers MUST be logged at `warn` once per container and otherwise ignored.

| Removed setting | Replacement |
|---|---|
| `--notifications`, `--notification-email-*`, `--notification-slack-*`, `--notification-msteams-*`, `--notification-gotify-*` | `--notification-url` with a service URL (NTF-013) |
| `notify-upgrade` command | `--notification-url` with a service URL |

- **Given** `--intervall 60` **When** Parachute starts **Then** it exits with code 2 and the message contains `--intervall` and `--interval`.
- **Given** `PARACHUTE_FOO=1` **When** Parachute starts **Then** it exits with code 2 and the message contains `PARACHUTE_FOO`.
- **Given** a container labelled `parachute.enabel=false` **When** a cycle runs **Then** one `warn` line names the container and `parachute.enabel`, and the container is still a candidate.

### CFG-006 Armed mode
Parachute MUST start **unarmed** unless `--armed` (or `PARACHUTE_ARMED=true`) is set, its own container carries the label `parachute.armed=true`, or the `convenient` profile is active (CFG-011). Arming is read at every start from these sources only; it is never remembered and never read from the state file, so a copied or restored `state.json` cannot arm an instance. While unarmed, Parachute MUST run cycles, detection, and policies, report every update it would apply as `would_update` (NTF-010), and MUST NOT pull images, stop, create, rename, or remove containers, create helpers or snapshots, run `pre-update` or `post-update` hooks, or run restore verification. The startup message and status view MUST show the armed state.

- **Given** a fresh install, unarmed, and a container with a newer image **When** a cycle runs **Then** the webhook receives a report listing it as `would_update`, and the container's ID and image, the local image list, and the volume list are unchanged.
- **Given** the same with `PARACHUTE_ARMED=true` **When** a cycle runs **Then** the container is updated.
- **Given** a Parachute that was armed and then restarted without the setting, and a container with a newer image **When** a cycle runs **Then** the container is reported `would_update` and its ID and image are unchanged.
- **Given** an unarmed Parachute whose `state.json` was copied from an armed instance, and a container with a newer image **When** a cycle runs **Then** the container is reported `would_update` and its ID and image, the local image list, and the volume list are unchanged.

### CFG-007 Result states and porcelain output
Every candidate on a reachable host MUST end each cycle in exactly one of these states. The reason code for each state and its notification level and exit code are listed only in [reasons.md](reasons.md).

| State | Meaning |
|---|---|
| `fresh` | No update available. |
| `updated` | Update applied and committed. |
| `would_update` | Update would be applied, but Parachute is unarmed. |
| `held` | Update available but not applied this cycle: held by a policy (POL-001) or deferred by a `pre-update` hook (HOK-006, reason `hook_deferred`). |
| `skipped` | Not checkable for a non-error reason (`digest_pinned`, `local_image`, `managed_by_runtime`). |
| `failed` | Not updated because of an error; nothing changed (UPD-014, BKP-011, UPD-019, DET-003, DEP-008, POL-008, DMN-005). An unreachable host is not a result: it is reported per host and contributes no entries (DMN-005). |
| `rolled_back` | Update applied, verification failed, previous state restored (reason `verify_failed`, `verify_timeout`, `transaction_timeout`, or `interrupted`; VER-011). |
| `rollback_failed` | Update applied, and the previous state could not be fully restored (VER-010). |

With `--porcelain v1`, Parachute MUST write exactly one JSON object per line to stdout and nothing else (logs go to stderr): one `entry` line per candidate per cycle, then one `summary` line. Every key is always present, `null` when it has no value:

```json
{"v":1,"type":"entry","cycle":"cyc-...","host":"local","stack":"shop","service":"web","container":"shop-web-1","image":"localhost:5000/fixture/app:1","image_id":"sha256:...","candidate_digest":"sha256:...","state":"updated","reason":null,"txn":"txn-...","snapshot":"snap-..."}
{"v":1,"type":"summary","cycle":"cyc-...","armed":true,"scanned":3,"updated":1,"failed":0,"fresh":2,"skipped":0,"held":0,"would_update":0}
```

Porcelain mode does not change notification settings. Any value other than `v1` MUST be a startup error (exit code 2).

- **Given** `--porcelain v1 --run-once`, armed, three containers, one with a newer image **When** Parachute exits **Then** stdout has exactly four lines, each valid JSON with `"v":1`: three `entry` lines (one `updated`, two `fresh`) and a final `summary` with `"scanned":3,"updated":1`.
- **Given** `--porcelain v2` **When** Parachute starts **Then** it exits with code 2.

### CFG-008 Logging
`--log-level` MUST accept `error`, `warn`, `info` (default), `debug`, `trace`; `--debug` and `--trace` are shorthands for the last two. `--log-format` MUST accept `auto` (default: `pretty` on a terminal, else `logfmt`), `logfmt`, `pretty`, `json`. With `json`, each line MUST have keys `time` (RFC 3339), `level`, `msg`, and, when they apply, `cycle`, `txn`, `host`, `stack`, `service`, `container`, `phase`. `--no-color` or a non-empty `NO_COLOR` MUST remove colour codes.

- **Given** `--log-format json` and an update **When** the cycle runs **Then** every log line parses as JSON, at least one line has `"phase": "verify"`, and every line with a `phase` key also has a `txn` key.
- **Given** `NO_COLOR=1 --log-format pretty` **When** Parachute logs **Then** the output contains no `ESC[` sequences.

### CFG-009 Exit codes
The Parachute process (daemon and run-once mode) MUST use these exit codes; they match the per-state exit codes in [reasons.md](reasons.md). In run-once mode, when several apply, the highest wins. After startup a daemon exits only on a signal, with 0. `parachute status` (STS-001) uses 0, 2, and 4 with the same meanings; `parachute healthcheck` (DMN-006) uses only 0 and 1.

| Code | Meaning |
|---|---|
| 0 | Success: every candidate is `fresh`, `updated`, `would_update`, `held`, or `skipped`. In daemon mode, a clean shutdown on `SIGTERM` or `SIGINT`. |
| 1 | At least one candidate `failed` or `rolled_back`, or a host was unreachable (run-once). |
| 2 | Invalid configuration or startup error: bad or unknown setting, removed setting, missing token (API-003), [insecure TCP host](glossary.md#insecure-tcp-host) without `--allow-insecure-tcp` (DMN-003), unreadable or invalid state file (CFG-010). |
| 3 | At least one `rollback_failed` (run-once). |
| 4 | No managed host reachable at startup (DMN-004), including a failed TLS handshake (DMN-003) or a rejected forced API version (DMN-002). |

- **Given** `--run-once` with one `rolled_back` and one `rollback_failed` **When** Parachute exits **Then** the code is 3.
- **Given** `--run-once` with one `failed` and one `fresh` candidate **When** Parachute exits **Then** the code is 1.
- **Given** a running daemon **When** it receives `SIGTERM` while idle **Then** it exits with code 0 within 5 seconds.

### CFG-010 State file
Parachute MUST keep its state in one JSON file, `<state-dir>/state.json` (`--state-dir`, default `/var/lib/parachute`), whose top-level `format` is `parachute.state/v1` ([ADR-0008](../adr/0008-state-storage.md)). A missing file MUST be created. The file MUST always be complete, valid JSON (written by replacing, never in place). The file is untrusted input ([glossary: Trust boundaries](glossary.md#trust-boundaries)): it is parsed strictly, and it never arms Parachute (CFG-006). A state directory that cannot be written, or a file that is not valid `parachute.state/v1` JSON, MUST be a startup error (exit code 2) naming the file; Parachute MUST NOT overwrite it. The state directory holds only two other files: during a self-update, the handover marker `<state-dir>/handover.json`, format `parachute.handover/v1` (UPD-018); and `<state-dir>/restore-verify.json`, format `parachute.restore-verify/v1`, written only by restore verification and untrusted like `state.json` (RST-005). Shutting down on `SIGTERM` during a transaction MUST finish or roll back that transaction first, within the stop grace period, else UPD-016 applies at the next start.

- **Given** an empty state directory **When** Parachute starts **Then** `state.json` exists and has `"format": "parachute.state/v1"`.
- **Given** a `state.json` containing `{broken` **When** Parachute starts **Then** it exits with code 2, the message names the file, and the file still contains `{broken`.
- **Given** a Parachute killed with `SIGKILL` at random points during 20 cycles with updates **When** the test reads `state.json` after each kill **Then** it always parses.

### CFG-011 Profiles
Parachute MUST accept `--profile` with the values `safe` (default) and `convenient`. A profile only changes the **defaults** of the settings below; any setting given explicitly (flag, environment variable, or label) wins over the profile. The startup message MUST name the active profile, and, for `convenient`, list each safety default it relaxed (POL-009).

| Setting | `safe` | `convenient` |
|---|---|---|
| `--armed` | `false` | `true` |
| `--delay-days` | `2` | `0` |
| `--protect-databases` | `true` | `false` |

The profile MUST NOT change `--backup`, `--verify-timeout`, or `--verify-stable`: snapshots and rollback stay on in both profiles, and are turned off only by their own settings. No profile MUST remove the [self-update approval](glossary.md): an update of Parachute's own container is held with reason `approval_required` under `convenient` as under `safe` (UPD-018). An unknown profile value MUST be a startup error (exit code 2).

- **Given** `--profile convenient` and a newer image for a database container **When** a cycle runs **Then** the update is applied, with no approval hold.
- **Given** `--profile convenient --delay-days 3` **When** Parachute starts **Then** the delay is 3 days and the other two defaults are the `convenient` values.
- **Given** `--profile convenient` **When** Parachute starts **Then** the startup message names `convenient` and lists `armed`, `delay-days`, and `protect-databases`.
- **Given** `--profile convenient` and a failing update **When** the cycle runs **Then** the container is rolled back.
- **Given** `--profile convenient` and a newer image for Parachute's own container (as identified by the engine) **When** a cycle runs **Then** the result for it is `held` with reason `approval_required` and its container ID is unchanged.
- **Given** `--profile fast` **When** Parachute starts **Then** it exits with code 2 and the message contains `profile` and `fast`.

### CFG-012 Logs, errors, and porcelain output are redacted
Log lines at every level except `trace`, startup and error messages (including those of startup errors with exit code 2 or 4), and porcelain output MUST be redacted per [glossary: Redaction](glossary.md#redaction) ([ADR-0013](../adr/0013-secret-redaction.md)). A setting whose value is a secret (CFG-004) MUST be named in messages but its value MUST NOT be shown, also when the value is invalid.

- **Given** API token `T0ken-api`, status token `T0ken-ro`, a notification URL `generic://bob:hunter2@127.0.0.1:1/hook?token=q5ecret` that fails, registry credentials `user:s3cret-value` in `config.json`, and a monitored container with environment `DB_PASSWORD=pw-9z7`, `--log-level debug`, `--log-format json`, and `--porcelain v1 --run-once` **When** Parachute runs **Then** neither stdout nor stderr contains `T0ken-api`, `T0ken-ro`, `hunter2`, `q5ecret`, `s3cret-value`, its base64 form, or `pw-9z7`.
- **Given** `--notification-url 'generic://bob:hunter2@exa mple'` (invalid) **When** Parachute starts **Then** it exits with code 2, the message names `notification-url`, and stderr does not contain `hunter2`.

## Settings reference

| Flag | Default | Spec |
|---|---|---|
| positional names, `--name` | all containers | SEL-002 |
| `--label-enable` | `false` | SEL-004 |
| `--disable-containers` | empty | SEL-005 |
| `--monitor-only` | `false` | SEL-007 |
| `--label-take-precedence` | `false` | SEL-008 |
| `--include-restarting` | `false` | SEL-009 |
| `--include-stopped` | `false` | SEL-009 |
| `--revive-stopped` | `false` | SEL-010 |
| `--scope` | unset | SEL-011 |
| `--interval` | `86400` | SCH-001 |
| `--schedule` | unset | SCH-002 |
| `TZ` | `UTC` | SCH-004 |
| `--run-once` | `false` | SCH-005 |
| `--no-startup-message` | `false` | SCH-007 |
| `--warn-on-head-failure` | `auto` | DET-003 |
| `--no-pull` | `false` | DET-004 |
| `--stop-timeout` | `10s` | UPD-006 |
| `--cleanup` | `false` | UPD-010 |
| `--remove-volumes` | `false` | UPD-011 |
| `--no-restart` | `false` | UPD-012 |
| `--rolling-restart` | `false` | UPD-013 |
| `--transaction-timeout` | `30m` | UPD-019 |
| `--enable-lifecycle-hooks` | `false` | HOK-001 |
| `--backup` | `stop` | BKP-003 |
| `--backup-volume` | `parachute-snapshots` | BKP-005 |
| `--backup-retention` | `3` | BKP-008 |
| `--helper-image` | image of Parachute's own container | BKP-010 |
| `--verify-timeout` | `120s` | VER-006 |
| `--verify-stable` | `30s` | VER-003 |
| `--delay-days` | `2` | POL-002 |
| `--protect-databases` | `true` | POL-003 |
| `--tag-follow` | `none` | POL-005 |
| `--maintenance-window` | unset (always open) | POL-007 |
| `--maintenance-window-duration` | `2h` | POL-007 |
| `--notification-url` | empty | NTF-001 |
| `--notification-report` | `true` | NTF-002 |
| `--notifications-level` | `info` | NTF-003 |
| `--notifications-hostname` | OS hostname | NTF-004 |
| `--notification-title-tag` | empty | NTF-004 |
| `--notification-skip-title` | `false` | NTF-004 |
| `--notifications-delay` | `0` | NTF-005 |
| `--notification-template` | built-in | NTF-006 |
| `--notification-log-stdout` | `false` | NTF-009 |
| `--approval-reminder-days` | `7` | NTF-011 |
| `--http-api-port` | `8080` | API-001 |
| `--http-api-update` | `false` | API-001 |
| `--http-api-metrics` | `false` | API-001 |
| `--http-api-status` | `false` | API-001 |
| `--http-api-token` | unset | API-002 |
| `--http-api-status-token` | unset | API-002 |
| `--http-api-periodic-polls` | `false` | API-006 |
| `--trusted-proxies` | empty (list of IP addresses and CIDR ranges) | API-010 |
| `DOCKER_CONFIG` | `/` | REG-001 |
| `--host` / `DOCKER_HOST` | `unix:///var/run/docker.sock` | DMN-001 |
| `--api-version` / `DOCKER_API_VERSION` | negotiated | DMN-002 |
| `--tlsverify` / `DOCKER_TLS_VERIFY` | `false` | DMN-003 |
| `--allow-insecure-tcp` | `false` | DMN-003 |
| `DOCKER_CERT_PATH` | `/etc/ssl/docker` | DMN-003 |
| `--hosts` | unset | DMN-007 |
| `--restore-verify-schedule` | unset (off) | RST-001 |
| `--armed` | `false` (`true` under `convenient`) | CFG-006 |
| `--profile` | `safe` | CFG-011 |
| `--porcelain` | unset | CFG-007 |
| `--log-level`, `--debug`, `--trace` | `info` | CFG-008 |
| `--log-format` | `auto` | CFG-008 |
| `--no-color` / `NO_COLOR` | `false` | CFG-008 |
| `--state-dir` | `/var/lib/parachute` | CFG-010 |

Commands: `parachute` (daemon), `parachute healthcheck` (DMN-006), `parachute status` (STS-001), `parachute restore-verify` (RST-001).

## Label reference

Labels are read from monitored containers unless noted, and only when set at creation: a label inherited from the image is ignored unless it is cautionary (SEL-013). "Overrides" names the global setting the label overrides (CFG-002).

| Label | Values | Overrides | Spec |
|---|---|---|---|
| `parachute.enable` | `true`, `false` | | SEL-003, SEL-004 |
| `parachute.monitor-only` | boolean | `--monitor-only` (SEL-008) | SEL-007 |
| `parachute.no-pull` | boolean | `--no-pull` (SEL-008) | DET-004 |
| `parachute.scope` | text | | SEL-011 |
| `parachute.self` | `true`; only excludes a container that is not Parachute's own (the official image carries it for information) | | SEL-012, SEL-013 |
| `parachute.armed` | boolean, on Parachute's own container | `--armed` | CFG-006 |
| `parachute.helper`, `parachute.transaction` | set by Parachute on helpers | | BKP-010 |
| `parachute.stop-signal` | signal name | | UPD-006 |
| `parachute.depends-on` | container names | | DEP-004 |
| `parachute.lifecycle.pre-check`, `.pre-update`, `.post-update`, `.post-check` | shell command | | HOK-001 |
| `parachute.lifecycle.pre-update-timeout`, `.post-update-timeout` | minutes | | HOK-005 |
| `parachute.backup` | `stop`, `live`, `none` | `--backup` | BKP-003 |
| `parachute.backup.exclude` | mount destinations | | BKP-002 |
| `parachute.backup.dump` | `auto`, `off`, shell command | | BKP-004 |
| `parachute.verify.timeout` | duration | `--verify-timeout` | VER-006 |
| `parachute.verify.stable` | duration | `--verify-stable` | VER-003 |
| `parachute.verify.cmd` | shell command | | VER-004 |
| `parachute.verify.url` | URL | | VER-005 |
| `parachute.delay-days` | decimal | `--delay-days` | POL-002 |
| `parachute.database` | boolean | recognition list | POL-003 |
| `parachute.approval` | `required`, `auto` | | POL-006 |
| `parachute.tag-follow` | `none`, `patch`, `minor` | `--tag-follow` | POL-005 |
| `parachute.maintenance-window`, `parachute.maintenance-window-duration` | cron, duration | `--maintenance-window*` | POL-007 |
| `parachute.transaction`, `parachute.updated-at`, `parachute.previous-image`, `parachute.snapshot` | set by Parachute on replacements | | UPD-005 |
| `parachute.helper`, `parachute.restore-verify` | set by Parachute on restore-verification resources | | RST-002, BKP-010 |
