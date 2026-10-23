# 10 Notifications

**Status:** Draft

How Parachute tells the operator what happened. Default behaviour is one quiet digest per cycle ([ADR-0007](../adr/0007-safe-by-default.md)). Delivery uses shoutrrr service URLs, through a maintained fork ([ADR-0002](../adr/0002-language-and-docker-client.md), [ADR-0015](../adr/0015-traceability-and-test-practice.md)). Tests use a `generic://` URL pointing at a fake webhook that records every request.

### NTF-001 Service URLs
Parachute MUST send each notification to every URL in `--notification-url` (a list, CFG-003). A delivery failure for one URL MUST NOT stop delivery to the others and MUST be logged at `error` with the URL's scheme and host, never the full URL.

- **Given** two webhook URLs **When** a cycle with an update ends **Then** each webhook receives the report once.
- **Given** an unreachable URL `generic://bob:hunter2@127.0.0.1:1/secret-path?token=q5ecret` and one working URL **When** a cycle ends **Then** the working webhook receives the report and one `error` line contains `127.0.0.1:1` and contains none of `hunter2`, `secret-path`, or `q5ecret`.

### NTF-002 One digest per cycle
With `--notification-report` on (default), Parachute MUST send at most one report notification per cycle, covering all hosts. Its default body MUST group entries by host, then by stack, then by service, and list each entry's result state, reason, image reference, and transaction ID when present. Results of restore verification (RST-006) are sent the same way, one notification per run.

- **Given** two stacks with two updated services each **When** the cycle ends **Then** the webhook receives exactly one notification, listing both stacks with their services under them.

### NTF-003 Entry levels and the notification level
Each report entry has a level by result state: `fresh` and `skipped` are `debug`; `updated`, `would_update`, and `held` are `info`; `failed` and `rolled_back` are `warn`; `rollback_failed` is `error`. Only entries at or above `--notifications-level` (default `info`; values `debug`, `info`, `warn`, `error`) MUST be included. Any other value is a startup error (exit code 2).

- **Given** the default level and a cycle with one `fresh` and one `updated` container **When** the cycle ends **Then** the report lists only the updated one.
- **Given** level `warn` and a cycle with one `updated` and one `rolled_back` **When** the cycle ends **Then** the report lists only the rolled-back one.

### NTF-004 Title
The notification title MUST be `Parachute on <hostname>`, where `<hostname>` is `--notifications-hostname` if set, else the operating system hostname of Parachute. `--notification-title-tag <tag>` MUST prefix the title with `[<tag>] `. `--notification-skip-title` MUST send no title.

- **Given** `--notifications-hostname nas --notification-title-tag prod` **When** a report is sent **Then** its title is `[prod] Parachute on nas`.
- **Given** `--notification-skip-title` **When** a report is sent **Then** it has no title.

### NTF-005 Delay
`--notifications-delay <seconds>` (default `0`) MUST delay sending each report by that many seconds after the cycle ends.

- **Given** `--notifications-delay 10` **When** a cycle ends **Then** the webhook receives the report between 10 and 13 seconds later.

### NTF-006 Templates
`--notification-template` MUST accept a Go `text/template`, given inline or as a file path (CFG-004). In report mode the template receives the report object (fields: `Title`, `Host` (Parachute's hostname), `CycleID`, `Armed`, `Hosts` (each with `Name`, `Reachable`, `Stacks`), stacks with `Name`, `Compose`, `Services`, services with `Name`, `Container`, `Image`, `CandidateDigest`, `State`, `Reason`, `HoldUntil`, `TransactionID`, `SnapshotID`; and counts `Scanned`, `Updated`, `Failed`, `Fresh`, `Skipped`, `Held`, `WouldUpdate`), already filtered by NTF-003. A template that fails to parse MUST be a startup error (exit code 2).

- **Given** template `{{range .Hosts}}{{range .Stacks}}{{.Name}};{{end}}{{end}}` and updates in stacks `a` and `b` **When** the cycle ends **Then** the body is `a;b;`.
- **Given** template `{{.Nope` **When** Parachute starts **Then** it exits with code 2.

### NTF-007 Simple mode
With `--notification-report=false`, Parachute MUST collect the log entries of a cycle at or above the notification level and send them as one notification at the end of the cycle. The template then receives a list of entries with fields `Time`, `Level`, `Message`, `Fields`; the default template is one message per line.

- **Given** `--notification-report=false`, `--log-format json`, and template `{{range .}}{{.Message}}|{{end}}` **When** a cycle with an update ends **Then** one notification arrives whose body is the `msg` values of the cycle's `info`-and-above stderr log lines, in order, each followed by `|`.

### NTF-008 Empty output sends nothing
When a template renders to an empty or whitespace-only string, no notification MUST be sent.

- **Given** the default template and a cycle where every container is `fresh` **When** the cycle ends **Then** the webhook receives nothing.

### NTF-009 Logger target
A `logger://` URL MUST write each notification to Parachute's log at `info`; with `--notification-log-stdout` it MUST write the notification body alone to stdout.

- **Given** `--notification-url logger:// --notification-log-stdout` and an update **When** the cycle ends **Then** stdout contains the report body.

### NTF-010 Dry runs use the same path
While unarmed (CFG-006), updates that would be applied MUST be reported as `would_update` through the same notification path and template as real updates.

- **Given** an unarmed Parachute and an update available **When** a cycle ends **Then** the webhook receives a report listing the container as `would_update`.

### NTF-011 Approval reminders
A `held` entry with reason `approval_required` MUST be included in the report the first time it is seen for a given container and candidate digest, and then, while it is still pending, again in the report of the first cycle that starts at least `--approval-reminder-days` days after the report that last included it (a positive decimal number; default `7`; `0` or a negative value is a startup error, exit code 2). Other `held` entries MUST be included only the first time they are seen for that container, digest, and reason. Every `held` entry is still shown in the status view (STS-004).

- **Given** a held database update, `--approval-reminder-days 0.001` (86.4 seconds), and `--interval 60` **When** cycles start at 0, 60, 120, 180, and 240 seconds **Then** the reports of the cycles at 0, 120, and 240 seconds include it and the others do not.
- **Given** `--approval-reminder-days 0` **When** Parachute starts **Then** it exits with code 2 and the message contains `approval-reminder-days`.
- **Given** an update held by `delay` **When** a second cycle runs with nothing new **Then** that cycle sends no notification.

### NTF-012 Rollback failures are always sent
An entry with state `rollback_failed` MUST be sent as its own `error` notification immediately, in addition to the cycle report, whatever the notification level, delay, or template.

- **Given** `--notifications-level error`, `--notifications-delay 60`, and a rollback failure **When** it happens **Then** the webhook receives an error notification containing the transaction ID within 10 seconds.

### NTF-013 Per-provider notification options are not supported
Per-provider notification settings (`--notifications`, and any setting whose name starts with `--notification-email-`, `--notification-slack-`, `--notification-msteams-`, or `--notification-gotify-`, plus their environment forms) MUST fail at startup with exit code 2 and a message that shows the matching service URL format (for example `slack://token@channel`), per CFG-005.

- **Given** `PARACHUTE_NOTIFICATION_SLACK_HOOK_URL=https://hooks.example/x` **When** Parachute starts **Then** it exits with code 2 and the message contains `--notification-url` and `slack://`.

### NTF-014 Notifications are redacted
Every notification (report, simple mode, restore verification, the NTF-012 error notification, and `logger://` output) MUST be redacted per [glossary: Redaction](glossary.md#redaction) after its template is rendered, whatever the template, so no secret appears in a title or body ([ADR-0013](../adr/0013-secret-redaction.md)). Template fields (NTF-006, NTF-007) MUST NOT carry secret values.

- **Given** API token `T0ken-api`, status token `T0ken-ro`, a notification URL `generic://bob:hunter2@127.0.0.1:1/hook?token=q5ecret` that fails, registry credentials `user:s3cret-value` in `config.json`, and a monitored container with environment `DB_PASSWORD=pw-9z7`, `--notification-report=false`, `--notifications-level debug`, and a working webhook **When** a cycle runs **Then** no notification received by the webhook contains `T0ken-api`, `T0ken-ro`, `hunter2`, `q5ecret`, `s3cret-value`, the base64 form of `user:s3cret-value`, or `pw-9z7`.
- **Given** a registry whose bearer challenge names the unreachable realm `https://u:realmpw@127.0.0.1:1/token`, `--notification-report=false`, and a working webhook **When** a cycle runs **Then** the notification contains `[REDACTED]` and not `realmpw`.
