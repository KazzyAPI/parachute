# Glossary

- **Affected containers**: in a transaction, the containers whose image changes plus their dependents (UPD-001, DEP-006).
- **Armed**: the state in which Parachute is allowed to change anything. Unarmed, it only reports what it would do (CFG-006).
- **Candidate**: a container that passed selection in a cycle (SEL-001).
- **Cycle**: one pass over all candidates on all hosts, ending in one report. ID `cyc-...` (SCH-001).
- **Drift**: a difference between a service's `image:` in its Compose file and the image its container runs (DEP-011).
- **Helper container**: a short-lived container Parachute creates for copying data or probing URLs, labelled `parachute.helper=true` (BKP-010).
- **Host**: one container-engine endpoint that Parachute manages (DMN-001, DMN-007).
- **Insecure TCP host**: a `tcp://` host used without `--tlsverify` / `DOCKER_TLS_VERIFY` (DMN-003), including loopback addresses. Configuring one is a startup error (exit code 2) naming the host, unless `--allow-insecure-tcp` / `PARACHUTE_ALLOW_INSECURE_TCP=true` is set. With it set, Parachute logs a `warn` line naming each insecure host at startup and at the start of every cycle, and lists it as a relaxation (POL-009).
- **Policy hold**: a detected update that a policy stops for now, with one reason code such as `delay` or `approval_required` (POL-001).
- **Reason code**: the one code that explains a `held`, `skipped`, `failed`, `rolled_back`, or `rollback_failed` result. The full list is [reasons.md](reasons.md).
- **Relaxation**: a setting or label that weakens a safe default, such as `parachute.backup=none`; always logged (POL-009).
- **Restore point**: what rollback returns to: the previous containers and images, kept until commit, plus the snapshot (UPD-008, BKP-001).
- **Restore verification**: a scheduled test restore of a snapshot into throwaway containers (RST-001).
- **Result state**: the outcome of a candidate in a cycle, for example `updated` or `held` (CFG-007).
- **Scope**: a label value that limits a Parachute instance to some containers, so several instances can share a host (SEL-011).
- **Self-update approval**: an update of Parachute's own container (the container the instance runs in, identified by the engine, not by a label; SEL-012, UPD-018) is never applied automatically. It is always held with reason `approval_required` until approved (API-007). Neither the `convenient` profile (CFG-011), `--protect-databases=false`, nor `parachute.approval=auto` removes this hold.
- **Service**: the containers of one Compose service in a stack; the unit of update (DEP-001).
- **Snapshot**: the stored copy of a stack's container configuration and data taken before an update. ID `snap-...` (BKP-006).
- **Stack**: the containers of one Compose project on one host, or a single non-Compose container; the unit of snapshot, verification, and rollback (DEP-001, DEP-002).
- **Transaction**: one update of one stack, through the phases detect, policy, snapshot, update, verify, and commit or rollback. ID `txn-...` (UPD-001).
- **Verify**: the check that replacements become healthy before commit (VER-001).

## Transaction state machine

A transaction's state is recorded in the state file (CFG-010) and shown by `parachute status` (STS-002, field `last_transaction.state`). Open states are the phase names of UPD-001; a transaction ends in exactly one terminal state.

| State | Kind | Meaning |
|---|---|---|
| `detect` | open | DET checks for the stack. |
| `policy` | open | Holds are evaluated (POL-001). |
| `snapshot` | open | Pull, configuration check, free-space check, `pre-update` hooks, dumps, data copy. No replacement exists. |
| `update` | open | Originals stopped and renamed, replacements created and started. |
| `verify` | open | Replacements are verified (VER-001). |
| `handover` | open | Self-update only: the old instance has written `handover.json` and started the replacement, and is verifying it (UPD-018 steps 1 to 3). The new instance waits in handover mode. |
| `rolling_back` | open | Rollback in progress (VER-008). Never interrupted by the transaction ceiling (UPD-019). |
| `aborted` | terminal | Ended before the `update` phase; nothing changed. Result `held` or `failed` (UPD-014). |
| `committed` | terminal | Replacements verified and committed (UPD-009). Result `updated`. |
| `rolled_back` | terminal | Previous state restored and passed its base check. Result `rolled_back`. |
| `rollback_failed` | terminal | Previous state could not be fully restored. Result `rollback_failed`. |

Allowed transitions (any other is a defect):

```
detect -> policy -> snapshot -> update -> verify -> committed
detect | policy | snapshot -> aborted
update | verify -> rolling_back -> rolled_back | rollback_failed
snapshot -> handover                      (self-update; replaces update and verify)
handover -> committed                     (replacement verified; old instance records commit, removes marker)
handover -> rolling_back                  (replacement failed verification, UPD-018 step 4)
handover -> rollback_failed               (old instance stopped while the marker exists, step 6; new instance records it)
```

Interruption (crash, kill, reboot) in an open state is resolved at the next start, before any cycle (UPD-016): `detect`, `policy`, `snapshot` go to `aborted` with reason `interrupted` after starting any container stopped for the snapshot; `update`, `verify`, `rolling_back` go to `rolling_back` and then `rolled_back` or `rollback_failed` with reason `interrupted`; `handover` follows UPD-018 step 6.

A terminal state is never left. A new attempt is a new transaction with a new ID.

**`rollback_failed` is never retried automatically.** After it, the stack is **frozen**: Parachute starts no transaction for any container in that stack, whatever the candidate digest, and holds each such update with reason `previous_attempt_failed`. Snapshots of the stack are not removed (BKP-008). The freeze ends only by operator acknowledgement: approving the failed container's update with `POST /v1/approve` (API-007) after repairing the stack. Until the HTTP API exists the freeze cannot be lifted except by editing the state file while Parachute is stopped. `parachute status` shows the frozen stack, its `rollback_failed` transaction ID, and the snapshot ID (field `frozen`, STS-002). This overrides POL-010's "until a different candidate digest appears" for `rollback_failed`.

## Trust boundaries

**Untrusted** (validated before use; a bad value fails only the item it belongs to, never crashes Parachute or affects other containers; [ADR-0011](../adr/0011-untrusted-input-handling.md)):

- Image metadata from registries or pulled images: labels, configuration, `created` time, health check, entrypoint. Image labels never set Parachute behaviour (`parachute.*` labels are read only from the container, not inherited from the image config), except the cautionary ones of SEL-013, which can only make Parachute more cautious.
- Registry responses: status codes, headers, bearer challenges and token realms, manifests (size-limited), token bodies.
- The state file and `handover.json`: parsed strictly; an invalid file is a startup error (CFG-010), never overwritten; the status view never takes the armed state or an approval from it (STS-006).
- Snapshot manifests and archives: validated (BKP-007) before rollback or restore verification; paths in them never escape the snapshot or target mount.
- HTTP API requests: authenticated by token, parameters validated; container and host names are matched against known ones, never used as paths or commands.
- `config.json` content (REG-001) beyond the expected `auths` fields.

**Trusted** (the operator's own configuration):

- Container labels set by the operator when the container is created, including `parachute.lifecycle.*` hook commands and `parachute.backup.dump`. Anyone who can create containers on the engine already controls the host.
- Flags, environment variables, and files named by them (CFG-001 to CFG-004).
- The Docker or Podman daemon on a managed host, reached over `unix://` or verified TLS (see Insecure TCP host).

## Redaction

These MUST NOT appear in logs at any level other than `trace` (REG-006), notifications, porcelain output, the status view (STS-007), the HTTP API, or error messages:

- API tokens (`--http-api-token`) and notification URLs' credentials, tokens, and query secrets.
- Registry credentials, bearer tokens, and `Authorization` header values, including their base64 forms.
- Userinfo in any URL (`https://user:pass@host`).
- Container environment variable values (names may be shown).

Each redacted value is replaced with the single placeholder `[REDACTED]`, keeping the surrounding text (for example `https://[REDACTED]@hooks.example.test/...`). [ADR-0013](../adr/0013-secret-redaction.md).
