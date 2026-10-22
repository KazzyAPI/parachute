# Glossary

- **Affected containers**: in a transaction, the containers whose image changes plus their dependents (UPD-001, DEP-006).
- **Armed**: the state in which Parachute is allowed to change anything. Unarmed, it only reports what it would do (CFG-006).
- **Candidate**: a container that passed selection in a cycle (SEL-001).
- **Cycle**: one pass over all candidates on all hosts, ending in one report. ID `cyc-...` (SCH-001).
- **Drift**: a difference between a service's `image:` in its Compose file and the image its container runs (DEP-011).
- **Helper container**: a short-lived container Parachute creates for copying data or probing URLs, labelled `parachute.helper=true` (BKP-010).
- **Host**: one container-engine endpoint that Parachute manages (DMN-001, DMN-007).
- **Policy hold**: a detected update that a policy stops for now, with one reason code such as `delay` or `approval_required` (POL-001).
- **Relaxation**: a setting or label that weakens a safe default, such as `parachute.backup=none`; always logged (POL-009).
- **Restore point**: what rollback returns to: the previous containers and images, kept until commit, plus the snapshot (UPD-008, BKP-001).
- **Restore verification**: a scheduled test restore of a snapshot into throwaway containers (RST-001).
- **Result state**: the outcome of a candidate in a cycle, for example `updated` or `held` (CFG-007).
- **Scope**: a label value that limits a Parachute instance to some containers, so several instances can share a host (SEL-011).
- **Service**: the containers of one Compose service in a stack; the unit of update (DEP-001).
- **Snapshot**: the stored copy of a stack's container configuration and data taken before an update. ID `snap-...` (BKP-006).
- **Stack**: the containers of one Compose project on one host, or a single non-Compose container; the unit of snapshot, verification, and rollback (DEP-001, DEP-002).
- **Transaction**: one update of one stack, through the phases detect, policy, snapshot, update, verify, and commit or rollback. ID `txn-...` (UPD-001).
- **Verify**: the check that replacements become healthy before commit (VER-001).
