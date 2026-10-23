# Result states and reason codes

**Status:** Draft

The one table of result states and reason codes. Requirements name these codes; this file is the single list of them. A code not listed here MUST NOT be emitted. Reports, notifications, porcelain output (`reason` key), the status view, and metrics use exactly these strings.

Every candidate ends every cycle in exactly one result state (CFG-007), so every state is final **for the cycle**. The column **Next cycle** says what happens afterwards:

- **re-evaluated**: the next cycle checks the candidate again from scratch.
- **held for digest**: the next cycle holds the same candidate digest (POL-010); a new digest or an approval (API-007) clears it.
- **frozen**: every update is held in the `policy` phase, so no transaction starts for any container the freeze covers (for a self-update, the own container) until an operator releases it with `POST /v1/approve` (API-007; [glossary: Transaction state machine](glossary.md#transaction-state-machine), `rollback_failed`).

## Result states

| State | Reason | Level (NTF-003) | Exit code (CFG-009) |
|---|---|---|---|
| `fresh` | always `null` | `debug` | 0 |
| `would_update` | always `null` | `info` | 0 |
| `updated` | always `null` | `info` | 0 |
| `held` | exactly one code from [held](#held) | `info` | 0 |
| `skipped` | exactly one code from [skipped](#skipped) | `debug` | 0 |
| `failed` | exactly one code from [failed](#failed) | `warn` | 1 |
| `rolled_back` | exactly one code from [rolled_back](#rolled_back) | `warn` | 1 |
| `rollback_failed` | exactly one code from [rollback_failed](#rollback_failed) | `error` (NTF-012) | 3 |

## Reason codes

### held

Order of precedence is the order of the rows (POL-001): the first that applies is the one reported.

| Code | Meaning | Produced by | Next cycle |
|---|---|---|---|
| `monitor_only` | The container is monitor-only. | SEL-007, POL-001 | re-evaluated |
| `approval_required` | The update needs operator approval: database, `parachute.approval=required`, or Parachute's own container (self-update always needs approval). | POL-003, POL-006, UPD-018 | re-evaluated (stays held until approved) |
| `previous_attempt_failed` | A transaction to this candidate digest already ended `rolled_back` or `rollback_failed`, or a freeze covers the container (after `rollback_failed`, whatever the digest; [glossary](glossary.md#transaction-state-machine)). | POL-010, UPD-018 | held for digest (after `rollback_failed`: frozen) |
| `delay` | The candidate digest is younger than `--delay-days`. | POL-002 | re-evaluated |
| `maintenance_window` | The cycle is outside the maintenance window. | POL-007 | re-evaluated |
| `hook_deferred` | A `pre-update` hook exited 75; no container was stopped. | HOK-006 | re-evaluated |

### skipped

| Code | Meaning | Produced by | Next cycle |
|---|---|---|---|
| `digest_pinned` | The image reference contains `@sha256:`. | DET-005 | re-evaluated |
| `local_image` | The image has no registry digest, or the registry answered `404` for the manifest. | DET-006 | re-evaluated |
| `managed_by_runtime` | The container carries `io.containers.autoupdate`. | DMN-013 | re-evaluated |

### failed

Nothing was changed (UPD-014). The detail text names the failing step, field, label, or registry.

| Code | Meaning | Produced by | Next cycle |
|---|---|---|---|
| `registry_unreachable` | Transport error, timeout, TLS error, `5xx` or other unexpected status, malformed or oversized response, or invalid token realm, after the `HEAD` → `GET` fallback; an invalid or mismatched candidate digest, including a missing or invalid `Docker-Content-Digest` on a `200` `HEAD` response (no `GET` fallback); a forbidden redirect. | DET-003, DET-008, REG-007, REG-008 | re-evaluated |
| `registry_unauthorized` | The registry or its token endpoint answered `401` or `403`, or credentials were withheld from a non-HTTPS non-loopback token realm. | REG-001, REG-004 | re-evaluated |
| `engine_error` | A Docker engine request for this candidate failed before the `update` phase while the host stayed reachable. | DMN-005, SEL-013 | re-evaluated |
| `invalid_label` | A `parachute.*` label has an invalid value. | POL-008 | re-evaluated |
| `dependency_cycle` | The container is on a dependency cycle. | DEP-008 | re-evaluated |
| `pull_failed` | Pulling the new image failed, or the pulled image's repository digests do not contain the approved digest. | UPD-014, UPD-020 | re-evaluated |
| `config_unreproducible` | The container's configuration cannot be carried through (for example `AutoRemove`, Podman pod recreation). | UPD-007, DMN-012 | re-evaluated |
| `insufficient_space` | The snapshot store has too little free space. | BKP-011 | re-evaluated |
| `pre_update_hook_failed` | A `pre-update` hook failed or timed out. | HOK-007 | re-evaluated |
| `dump_failed` | The database dump failed. | BKP-004 | re-evaluated |
| `snapshot_failed` | Copying data or writing the snapshot failed. | BKP-001, DMN-014 | re-evaluated |
| `transaction_timeout` | The transaction ceiling was reached before the `update` phase. | UPD-019, HOK-005 | re-evaluated |
| `interrupted` | Parachute stopped during `snapshot` (state `aborted`); containers stopped for the snapshot were started again at the next start. | UPD-016 | re-evaluated |

### rolled_back

The previous state was restored and passed its base check (VER-010). The candidate digest is then held (POL-010).

| Code | Meaning | Produced by | Next cycle |
|---|---|---|---|
| `verify_failed` | A replacement exited, became `unhealthy`, or failed a check. | VER-001 to VER-005, UPD-018 | held for digest |
| `verify_timeout` | A replacement did not pass within its effective deadline. | VER-006 | held for digest |
| `transaction_timeout` | The transaction ceiling was reached in or after the `update` phase. | UPD-019 | held for digest |
| `config_unreproducible` | A created replacement's configuration differs from the original (beyond UPD-004 and UPD-005). | UPD-007 | held for digest |
| `image_mismatch` | A created replacement's image ID differs from the pulled, checked image's ID. | UPD-020 | held for digest |
| `engine_error` | A Docker engine request for this candidate failed in or after the `update` phase while the host stayed reachable (the engine answered with an error). | DMN-005 | held for digest |
| `interrupted` | Parachute stopped during `update`, `verify`, or `rolling_back` of an ordinary stack, or during `handover` or `rolling_back` of a self-update whose old instance then restarted (UPD-018 step 6); rolled back at the next start. | UPD-016, UPD-018 | held for digest |

### rollback_failed

The previous state could not be fully restored. Always an immediate `error` notification (NTF-012). Snapshots are kept (BKP-008).

| Code | Meaning | Produced by | Next cycle |
|---|---|---|---|
| `restore_failed` | An original could not be restored or did not pass its base check, data could not be restored, or (self-update) the replacement could not be removed or two Parachute containers for the scope are left running. | VER-010, UPD-018 | frozen |
| `snapshot_invalid` | The snapshot failed validation when rollback needed it; containers were restored without data. | BKP-007 | frozen |
| `interrupted` | During a self-update the old instance stopped while the handover marker existed; the new instance took over the state file and changed no container, and the transaction was still open. | UPD-018 | frozen |

### Restore verification

Restore verification (RST) has its own results, `pass` (reason `null`) and `fail`, and does not use the states above.

| Code | Meaning | Produced by |
|---|---|---|
| `snapshot_invalid` | The snapshot failed validation. | RST-003 |
| `verify_failed` | A throwaway container failed verification. | RST-003 |
| `verify_timeout` | A throwaway container did not pass within its deadline. | RST-003 |
| `restore_failed` | Data could not be restored into the throwaway containers. | RST-003 |

## Not reason codes

- **Host unreachable** is reported per host (`reachable: false` in the report and status view), not per candidate. A host that is unreachable at the start of the cycle, or becomes unreachable during it (DMN-005), contributes no entries for that cycle; in run-once mode it makes the exit code at least 1 (DMN-009).
- `not_found` is an internal registry-client outcome. It is never emitted; it maps to `skipped` / `local_image`.
- Startup errors (exit code 2 or 4) are not results; they have messages, not reason codes.

## Conflicts resolved

1. **DET-003 vs REG-001/REG-004 vs DET-006.** DET-003 says any failure of `HEAD` then `GET` gives `registry_unreachable`. Chosen mapping (matches M1): `401`/`403` from the registry or token endpoint → `failed`/`registry_unauthorized`; `404` for the manifest → `skipped`/`local_image`; everything else → `failed`/`registry_unreachable`. DET-003's `registry_unreachable` covers only the "everything else" case.
2. **`not_found`.** M1 code has a `not_found` reason in the registry client; it is never emitted (see above).
3. **`engine_error`.** Emitted by M1 for engine request errors on a reachable host but named in no spec. Adopted here; DMN-005 and SEL-013 now produce it.
4. **Host unreachable in CFG-007.** CFG-007 lists "host unreachable" under `failed`. M1 reports it per host with no per-candidate entries; that is the chosen behaviour. CFG-007 now says so.
5. **`rollback_failed` reasons.** VER-010 and UPD-018 step 5 name no reason code; `restore_failed` is added for both. UPD-018 step 6 keeps `interrupted`.
6. **`interrupted` before the `update` phase.** UPD-016 only covers rollback. An interruption before any replacement exists is reported `failed`/`interrupted`, since nothing changed.
7. **Retry after `rollback_failed`.** POL-010 lifts the hold when a new candidate digest appears; VER-010 forbids automatic retry. Chosen: after `rollback_failed` the stack is frozen until an operator releases it with `POST /v1/approve` (glossary, API-007). POL-010 now says so. Closed.
8. **Self-update and `convenient`.** CFG-011's `convenient` profile removes approval holds; it does not remove the self-update hold (`approval_required`, UPD-018). CFG-011 and UPD-018 now say so.
9. **Restore-verification reasons.** RST names only `snapshot_invalid`; `verify_failed`, `verify_timeout`, and `restore_failed` are reused for the other failures.
10. **Replacement image ID differs from the checked one.** UPD-020 required the replacement's image ID to equal the pulled image's ID and called a difference a configuration mismatch (UPD-007, `rolled_back`/`config_unreproducible`). Chosen ([Q16](open-questions.md#q16-rollback-reason-when-the-replacements-image-id-differs-from-the-checked-one)): a separate `rolled_back` reason `image_mismatch`, because it points at the image, not the container's configuration. UPD-020 now says so. Closed.
