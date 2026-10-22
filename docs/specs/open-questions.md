# Open questions

Decisions not yet made. Each has the options and a recommendation. The specs currently follow the recommendation; changing the answer means changing the named requirements.

### Q1 How strict should the defaults be?
**Decided 2026-10-22:** A, with a `convenient` profile so operators can opt into the relaxed defaults in one setting (CFG-011). Safe stays the default; every safety default is also individually settable.

Affects CFG-006, POL-002, POL-003, BKP-003, NTF-003.
- **A. Safe:** unarmed on start, 2-day delay, databases need approval, stop-then-copy snapshots, quiet digest.
- **B. Convenient:** armed on start, no delay, databases updated like anything else.
- **C. Safe, but armed by the official Compose example:** A as the binary default, with the documented example setting `PARACHUTE_ARMED=true`.

**Recommendation: A.** It is the reason to choose Parachute. The migration guide lists each default and how to relax it.

### Q2 Which images count as databases?
**Decided 2026-10-22:** A. Fixed list plus the `parachute.database` label.

Affects POL-003, BKP-004.
- **A.** A fixed list of repository names (current spec) plus the `parachute.database` label.
- **B.** A plus heuristics (image declares `VOLUME` and exposes a known database port).
- **C.** A list file the operator can extend.

**Recommendation: A for v1.** It is predictable and testable; add C if users ask for more names.

### Q3 Verification defaults
**Decided 2026-10-22:** Hybrid. Flat defaults stay `120s` / `30s`, but a container with a Docker health check gets at least `start_period + interval × (retries + 1)`; an expired deadline is reported as `verify_timeout` with a hint to set `parachute.verify.timeout` (VER-006, VER-011).

Affects VER-003, VER-006. Current: deadline `120s`, stability period `30s`.
- **A.** Stay at 120 s / 30 s.
- **B.** 300 s / 60 s, for slow-starting apps (Java, first-run migrations).

**Recommendation: A**, with the per-container labels documented for slow apps. A longer default lengthens every outage on a bad update.

### Q4 Where are snapshots stored?
**Decided 2026-10-22:** A. Snapshots live on each host.

Affects BKP-005, ADR-0009.
- **A. On each host**, in a Docker volume (default `parachute-snapshots`) or a host path (current spec).
- **B. Centrally**, in a directory on the Parachute host; remote data is streamed over the Engine API.
- **C. Both**, selectable.

**Recommendation: A.** Rollback stays local and fast and works if the Parachute host is down. A copy to off-host storage is a later feature.

### Q5 Anonymous volumes on recreate
**Decided 2026-10-22:** A. Existing anonymous mounts are always reattached, even if the new image no longer declares them; paths newly declared by the new image start empty (UPD-004, UPD-011).

Affects UPD-011. Recreating a container from its configuration alone gives it new, empty anonymous volumes and orphans the old ones.
- **A.** Re-attach the old anonymous volumes (current spec, same as `docker compose up` recreation); `--remove-volumes` gives fresh ones and removes the old ones at commit.
- **B.** Fresh anonymous volumes by default; `--remove-volumes` removes the old ones.

**Recommendation: A.** B silently loses data that the user may not know is in an anonymous volume.

### Q6 Exit code 75 from a `pre-update` hook
**Decided 2026-10-22:** B. Exit code 75 from `pre-update` defers the update (HOK-006).

Affects HOK-006, HOK-007. Exit code 75 (`EX_TEMPFAIL`) conventionally means "try again later".
- **A.** 75 is success and the update proceeds (current spec).
- **B.** 75 skips this container's update this cycle without reporting a failure (result `held`, reason `hook_deferred`).

**Recommendation: B.** It gives hooks a clean way to say "not now" that is not an error. 

### Q7 Is arming remembered?
**Decided 2026-10-22:** A. Arming comes only from settings read at every start, never from the state file (CFG-006, ADR-0007).

Affects CFG-006, ADR-0007.
- **A.** Arming is read from settings at every start (current spec).
- **B.** Arming once is recorded in the state file and persists until disarmed with a command.

**Recommendation: A.** The configuration stays the only source of truth, and a restored or copied state file cannot arm an instance by surprise.

### Q8 Self-update failure handling
**Decided 2026-10-22:** The old instance removes the failed replacement, keeps running, holds the digest (POL-010), and reports `rolled_back` at `warn`; `rollback_failed` at `error` only when it cannot remove the replacement or two instances are left running. The old instance runs verification and owns the state file until handover (UPD-018, ADR-0008).

Affects UPD-018. If the new Parachute instance fails verification, the old instance removes it and keeps running (current spec). Open: should the old instance also hold that digest (POL-010) and should the failure send an `error` notification instead of `warn`?

**Recommendation:** yes to both; a broken updater is worth a loud message.

### Q9 Browser access to the status page
**Decided 2026-10-22:** A, limited to `GET /status` and `GET /v1/status`, plus an optional read-only `--http-api-status-token` accepted only there; Basic credentials or the status token on any other endpoint get 401 (API-002).

Affects API-002, STS-003.
- **A.** Basic authentication with the token as password (current spec).
- **B.** Token in a query parameter.
- **C.** An unauthenticated read-only page on a separate port.

**Recommendation: A.** No token in URLs or proxy logs, and browsers support it natively.

### Q10 Delay: first seen, or image creation time?
**Decided 2026-10-22:** A. The delay counts from when Parachute first saw the digest; the image's `created` time is never used (POL-002).

Affects POL-002.
- **A.** Time Parachute first saw the digest (current spec). Reliable, but a new Parachute install waits the full delay for everything.
- **B.** The image's `created` time from its configuration. Available at once, but set by the image author and often wrong.

**Recommendation: A.**

### Q11 Updating only some images through the API
**Decided 2026-10-22:** The filter is deferred to a later release. Decided now: any query parameter on `/v1/update` returns 400 naming it and runs nothing, so adding a filter later cannot silently change what an old client's request does (API-004).

Affects API-004. Should `/v1/update` accept a filter (for example `?image=` or `?stack=`) so a CI pipeline can update one stack after pushing?

**Recommendation:** add `?stack=<name>&host=<name>` in a later release; not in v1.

### Q12 Disk space for snapshots
**Decided 2026-10-22:** Yes, as a v1 MUST: before anything stops, refuse with `insufficient_space` when free space is below the estimated snapshot size + 10 % + 1 GiB (BKP-011). Retention also runs after each rollback (BKP-008).

Affects BKP-001, BKP-005. Should Parachute estimate snapshot size and refuse early when the store lacks space, rather than failing during the copy?

**Recommendation:** yes, as a SHOULD in a later revision: refuse when free space is below the sum of mount sizes plus 10 %. The current spec already fails safe (BKP-009).
