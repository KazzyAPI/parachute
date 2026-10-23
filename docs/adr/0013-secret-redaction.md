# ADR-0013: Secret redaction

- **Status:** Accepted
- **Date:** 2026-10-23

## Context
Parachute holds secrets that are root-equivalent or close to it: the HTTP API tokens, registry credentials, credentials inside notification URLs, and, through the containers it inspects, environment variable values such as database passwords. Its output goes to many places that are read by people and systems with less access than root: logs shipped to aggregators, chat notifications, porcelain output in CI logs, the status page given to dashboards, and HTTP error bodies. Error messages from libraries (HTTP clients, the Docker client, shoutrrr) often embed full URLs, including userinfo and query strings. Redacting at each call site misses these; one leak is enough.

## Decision
1. What is secret, and the placeholder `[REDACTED]`, are defined once in [glossary: Redaction](../specs/glossary.md#redaction). No other placeholder or partial masking (such as showing the last four characters) is used.
2. **Never collected.** Values that are only ever secret (container environment variable values, registry passwords and tokens, `Authorization` headers) are not copied into report entries, template fields, status documents, porcelain lines, metrics labels, or error details. Template and document fields carry only non-secret data.
3. **Filtered at every sink.** Every output sink (each log handler at levels other than `trace`, the notification sender after template rendering, porcelain output, the status view in JSON and HTML, and every HTTP response body) passes its final text through one shared redactor before writing. The redactor replaces:
   - every configured secret value (API token, status token, each notification URL's userinfo, password, and query values, each registry credential and its base64 `user:password` form), matched literally;
     a notification URL contributes its whole text, its userinfo and password (raw and percent-decoded), its host when it contains no `.` or `:` (a token in host position, as in `ifttt://KEY/`), its host plus path, each path segment, and each query value (raw and percent-decoded) except booleans, integers of 6 digits or fewer, and the values of the known non-secret keys `title`, `username`, `color`, `priority`, `avatar`, `contenttype`, `template`, `disabletls`, and `port`; values of other keys stay secret. This keeps settings such as `template=json` or `disabletls=true` from rewriting unrelated output;
   - the userinfo part of any URL;
   - the value of any `Authorization` header or `Bearer`/`Basic` credential.
4. The redactor is applied after rendering, so a custom notification template cannot print a secret even by accident. A secret shorter than 4 characters is still redacted when it appears as a whole URL component or header value, but is not matched as a free substring, to avoid destroying unrelated text.
5. Only `--log-level trace` logs unredacted values, and starting at `trace` logs a `warn` line saying so (REG-006). Notifications, porcelain output, the status view, and the HTTP API are redacted at every log level.
6. Secrets an exec inside a container needs are passed in its environment, never on its command line (ADR-0010).

## Consequences
- One redactor and one list of sinks are easy to test: a black-box test plants known secrets and searches every output for them.
- Text that happens to contain a configured secret value is altered; that is accepted.
- Library error messages are kept (redacted) instead of being replaced with generic text, so failures stay diagnosable.
- Debugging an authentication problem needs `trace`, which is loud on purpose.

## Specs affected
REG-006, NTF-001, NTF-014, API-002, API-012, STS-007, CFG-004, CFG-007, CFG-012, BKP-004.
