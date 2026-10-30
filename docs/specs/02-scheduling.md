# 02 Scheduling

**Status:** Accepted

When cycles run. A **cycle** is one pass over all candidates on all managed hosts, ending with one report (NTF-002). Every cycle has a cycle ID matching `cyc-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}` (UTC start time plus random suffix), which appears in its logs, report, and porcelain output.

### SCH-001 Poll interval
Parachute MUST run cycles periodically. `--interval <seconds>` (a positive integer) sets the period; the default is `86400`. In interval mode the first cycle MUST start immediately after startup and each next cycle `interval` seconds after the previous one started. A value that is not a positive integer MUST be a startup error (exit code 2).

- **Given** no interval or schedule set **When** Parachute starts **Then** a cycle starts within 10 seconds, and the log announces the next cycle 86400 seconds after the first one's start.
- **Given** `--interval 60` **When** Parachute runs for 130 seconds **Then** exactly three cycles have started, about 60 seconds apart (±5 s).
- **Given** `--interval 0` or `--interval 1h` **When** Parachute starts **Then** it exits with code 2 and the message names `interval`.

### SCH-002 Cron schedule
`--schedule <expr>` MUST accept a 6-field cron expression (`second minute hour day-of-month month day-of-week`) as an alternative to the interval. In schedule mode no cycle runs at startup; the first runs at the next matching time. An invalid expression MUST be a startup error (exit code 2) whose message contains the expression.

- **Given** `--schedule "*/20 * * * * *"` **When** Parachute runs for 65 seconds **Then** the log shows three or four cycle starts, and every cycle start time has seconds `00`, `20`, or `40`.
- **Given** `--schedule "0 0 4 * * *"` **When** Parachute starts **Then** the log announces the next cycle at 04:00:00 in the configured time zone.
- **Given** `--schedule "not cron"` **When** Parachute starts **Then** it exits with code 2 and the message contains `not cron`.

### SCH-003 Interval and schedule are exclusive
Setting both `--interval` and `--schedule` MUST be a startup error (exit code 2) whose message names both settings.

- **Given** `--interval 60 --schedule "0 0 4 * * *"` **When** Parachute starts **Then** it exits with code 2 and the message contains `interval` and `schedule`.

### SCH-004 Time zone
The `TZ` environment variable MUST set the time zone used for log timestamps, cron schedules, and maintenance windows (POL-007). The default is UTC. An unknown zone name MUST be a startup error (exit code 2).

- **Given** `TZ=Europe/Berlin` and `--schedule "0 0 4 * * *"` **When** Parachute starts **Then** the log announces the next cycle at 04:00:00 with UTC offset `+01:00` or `+02:00` (whichever applies on that date).
- **Given** `TZ=Mars/Olympus` **When** Parachute starts **Then** it exits with code 2.

### SCH-005 Run once
With `--run-once`, Parachute MUST run exactly one cycle immediately, send its report, and exit with the code defined in CFG-009. Interval and schedule settings are ignored in run-once mode.

- **Given** `--run-once`, armed, and one container with a newer image that updates successfully **When** Parachute runs **Then** it exits with code 0 after the container is updated.
- **Given** `--run-once` and no updates available **When** Parachute runs **Then** it exits with code 0.
- **Given** `--run-once`, armed, and an update that fails verification and rolls back successfully **When** Parachute runs **Then** it exits with code 1.
- **Given** `--run-once`, armed, and an update whose rollback fails **When** Parachute runs **Then** it exits with code 3.

### SCH-006 No overlapping cycles
A cycle MUST NOT start while another cycle or a restore verification (RST-007) is running. A scheduled start that falls during a running cycle MUST be skipped, logged at `info` level with the running cycle's ID, and counted in `parachute_scans_skipped` (API-008).

- **Given** `--interval 5` and a registry proxy that delays every manifest response by 12 seconds **When** Parachute runs for 20 seconds **Then** no cycle starts before the previous cycle's end log line, and at least one `info` line records a skipped start and names the running cycle's ID.
- **Given** the same with `--http-api-metrics` **When** Parachute has run for 20 seconds **Then** `parachute_scans_skipped` is at least 1.

### SCH-007 Startup message
At startup Parachute MUST send one `info`-level notification summarising its configuration: version, managed hosts, schedule or interval, scope, armed state, and every relaxation of a safe default (POL-009). `--no-startup-message` MUST suppress it. The summary MUST also be logged.

- **Given** a notification URL pointing at a test webhook **When** Parachute starts **Then** the webhook receives one message containing the version, the schedule, and `armed: false`.
- **Given** `--no-startup-message` **When** Parachute starts **Then** the webhook receives no startup message.

### SCH-008 API-triggered mode
With `--http-api-update` on, Parachute MUST NOT run periodic cycles, and cycles run only when requested through the API (API-004), unless `--http-api-periodic-polls` is also on (API-006). Setting `--interval` or `--schedule` without `--http-api-periodic-polls` in this mode MUST log a `warn` line saying the schedule is ignored.

- **Given** `--http-api-update` with a token and `--interval 5` **When** Parachute runs for 20 seconds with no API calls **Then** no cycle runs and a `warn` line says the interval is ignored.
