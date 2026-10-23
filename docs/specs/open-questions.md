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

### Q13 List settings and URLs that contain commas
**Status:** Open.

Affects CFG-003, NTF-001. Lists are split on spaces and commas (CFG-003), so a `--notification-url` whose query string contains a comma (for example `...?to=a@example.com,b@example.com`) is split into two broken URLs. Should URL lists split only on whitespace, accept a repeated flag instead, or require commas inside URLs to be percent-encoded?

**Recommendation:** document percent-encoding (`%2C`) for now and accept repeated `--notification-url` flags; do not change the shared list syntax.

### Q14 How is Parachute's own container recognised?
**Status:** Resolved 2026-10-23: B. The own container is identified only by the engine; `parachute.self` is never needed, and set or inherited it only excludes a container that is not the own one (SEL-012, SEL-013, UPD-018, CFG label reference).

Affects SEL-012, SEL-013, UPD-018, CFG label reference. The official image bakes in `parachute.self=true`, while UPD-018 identifies the own container through the engine and SEL-013 honours the inherited label only because it is cautionary.
- **A.** The image-baked `parachute.self=true` label marks Parachute containers (SEL-012 today).
- **B.** The own container is identified only by the engine; the label is never needed and only ever makes Parachute more cautious.

**Recommendation: B**, keeping the label only as a cautionary marker.

### Q15 Superseding ADR for drift in ADR-0001 to ADR-0003
**Status:** Resolved 2026-10-23: New [ADR-0015](../adr/0015-traceability-and-test-practice.md) records the current test names, tracing rule, and shoutrrr fork; ADR-0001 to ADR-0003 are marked partly superseded.

Affects ADR-0001, ADR-0002, ADR-0003, `tools/trace`. The specs and tooling drifted from the accepted ADRs: test names (ADR-0001 says a test name starts with its ID; `make trace` matches `TestABC_001_`), the tracing rule (ADR-0001 and ADR-0003 fail only on a `MUST` in an `Accepted` file; `make trace` fails on a `MUST` or `SHOULD` listed for a milestone), and notification delivery (ADR-0002 names shoutrrr; a fork is used). Accepted ADRs are not edited, so the drift needs a new ADR.

**Recommendation:** one superseding ADR that records the current practice for all three.

### Q16 Rollback reason when the replacement's image ID differs from the checked one
**Status:** Resolved 2026-10-23: B. New `rolled_back` reason `image_mismatch` (UPD-020, [reasons.md](reasons.md) conflict 10).

Affects UPD-020, UPD-007, [reasons.md](reasons.md) conflict 10. UPD-020 calls a replacement whose image ID differs from the pulled, checked image a configuration mismatch (`rolled_back`/`config_unreproducible`).
- **A.** Keep `config_unreproducible`.
- **B.** A new `rolled_back` reason `image_mismatch`.

**Recommendation: B.** It points at the image, not the container's configuration.

### Q17 Lifting a freeze by editing the state file
**Status:** Resolved 2026-10-23: The state-file route is removed from the glossary; a freeze ends only by approval (API-007). POL-010, STS-006, and the migration guide already agree.

Affects glossary (Transaction state machine), POL-010, API-007, STS-006. The glossary says that until the HTTP API exists a freeze can be lifted only by editing the state file while Parachute is stopped, but the state file is untrusted (trust boundaries) and must not by itself lift a hold.

**Recommendation:** remove the state-file route from the glossary; the freeze ends only by approval (API-007).

### Q18 Replaying an old state file to undo a freeze or delay
**Status:** Resolved 2026-10-23: Out of scope for v1. ADR-0008's Amendment records the limit: write access to the state directory is trusted and an old state file can be replayed.

Affects CFG-010, POL-002, POL-010, ADR-0008. Someone who can write the state directory can restore an older valid `state.json` and so reset first-seen times or remove a freeze. An HMAC keyed outside the state directory would detect it.

**Recommendation:** out of scope for v1; document the limit (write access to the state directory is trusted) in ADR-0008's consequences.

### Q19 Accepting ADR-0007 and ADR-0009
**Status:** Resolved 2026-10-23: Both accepted as written.

Affects ADR-0007 (safe by default), ADR-0009 (snapshot format and storage). Both are still `Proposed` although the specs follow them (Q1, Q4).

**Recommendation:** accept both as written.

### Q20 Free space that cannot be measured
**Status:** Resolved 2026-10-23: Fail with `failed`/`snapshot_failed` naming the measurement; BKP-011 already says so, no spec change.

Affects BKP-011. When a mount size or the store's free space cannot be measured, should the transaction fail, or proceed without the check?

**Recommendation:** fail, result `failed` with reason `snapshot_failed` naming the measurement (current spec).

### Q21 Approvals stored in an untrusted state file
**Status:** Resolved 2026-10-23: Persistence kept; a stored approval applies only while its digest equals the current candidate digest (POL-006 already said so; API-007 now says it explicitly). The replay limit is recorded with Q18 in ADR-0008.

Affects API-007, POL-006, STS-006, CFG-010. API-007 persists approvals in the state file so they survive restarts, but the state file is untrusted, so an edited file could approve an update. Options: trust persisted approvals (current), bind them to the digest and a per-instance secret, or not persist them.

**Recommendation:** keep persistence, apply an approval only when its digest equals the current candidate digest (DET-008), and document the limit with Q18.

### Q22 Failed-authentication throttling behind a reverse proxy
**Status:** Resolved 2026-10-23: Per-IP throttling by default; new setting `trusted-proxies` (`PARACHUTE_TRUSTED_PROXIES`, default empty) lists proxies whose `X-Forwarded-For` is honoured (API-010, CFG settings reference).

Affects API-010. Behind a reverse proxy every request has the proxy's source IP, so one client's failures throttle everyone.

**Recommendation:** per-IP throttling by default, plus a trusted-proxy setting that lists proxy addresses whose `X-Forwarded-For` is honoured.

### Q23 Testing constant-time token comparison
**Status:** Resolved 2026-10-23: A code-level unit test covers API-011 as a documented exception to black-box testing (ADR-0015); the black-box test keeps the observable 401 part.

Affects API-011, ADR-0003. The black-box timing criterion (median difference under 1 ms) is noisy on shared CI and cannot prove constant time.

**Recommendation:** allow a code-level unit test for API-011 as a documented exception to black-box testing.

### Q24 HTTP limit values
**Status:** Resolved 2026-10-23: Keep the values; API-009 and API-010 already state them, no spec change.

Affects API-009, API-010. Are 8 KiB for request line and headers, 1 KiB for a body, 10 s to complete headers (60 s idle), and 10 failures per 60 s the right numbers?

**Recommendation:** keep them (current spec); revisit if real clients hit them.

### Q25 Self-update when the own container publishes ports or has a static IP
**Status:** Resolved 2026-10-23: A. Refuse with `failed`/`config_unreproducible` naming the field; UPD-018 already says so, no spec change.

Affects UPD-018, UPD-007. The replacement must run beside the old instance, so a published host port or static IP would collide.
- **A.** Refuse with `failed`/`config_unreproducible` naming the field (current spec).
- **B.** Stop the old instance first, giving up side-by-side verification.

**Recommendation: A.**

### Q26 Originals left over after a crash past commit
**Status:** Resolved 2026-10-23: Warn at startup naming the leftover container and never delete it; UPD-009 and UPD-017 already say so, no spec change.

Affects UPD-009, UPD-016, UPD-017. If Parachute crashes after recording `committed` but before removing a renamed original, the original remains; UPD-017 forbids removal without in-process proof.

**Recommendation:** warn at startup naming the leftover container; never delete it.

### Q27 Reason code for a failed write in the update phase
**Status:** Resolved 2026-10-23: A. A write that fails from the `update` phase on rolls back with `rolled_back`/`engine_error`, and the digest is held (POL-010), as for every `rolled_back` reason (DMN-005, [reasons.md](reasons.md#rolled_back)).

Affects DMN-005, VER-008, reasons.md. DMN-005 says a failed engine write is never retried and fails its step; from the `update` phase on the transaction rolls back. The `rolled_back` table had no reason for a rollback caused by a failed write.
- **A.** Reuse `engine_error` in the `rolled_back` table (held for digest).
- **B.** Reuse `engine_error`, but re-evaluate the digest next cycle instead of holding it, because the image is not at fault.
- **C.** A new code, for example `update_step_failed`.

**Recommendation: A**, so that every `rolled_back` reason holds the digest.
