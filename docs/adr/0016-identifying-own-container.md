# ADR-0016: Identifying Parachute's own container

- **Status:** Accepted
- **Date:** 2026-10-23

## Context
Several rules depend on Parachute's **own container**, the container the running instance runs in: it is the only container that can be self-updated (SEL-012, UPD-018), its image is the default helper image (BKP-010), and its `parachute.armed` label arms the instance (CFG-006). The specs say the own container is "identified by the engine", but not how the instance finds its container ID. A label cannot be used: the official image carries `parachute.self=true`, so every Parachute container (and any image that copies it) would claim to be the own container ([ADR-0011](0011-untrusted-input-handling.md), SEL-013). The container's hostname is its short ID only by default; an operator who sets `--hostname` must not lose self-update or the default helper image.

## Decision
1. **Candidate ID, in this order.** Parachute takes the first container ID it finds from:
   1. `/proc/self/mountinfo`: the root field (the fourth field, the path within the mounted file system, not the device) of the mount at `/etc/hostname`, `/etc/hosts`, or `/etc/resolv.conf`, when it ends with `/containers/<id>/(hostname|hosts|resolv.conf)` with `<id>` 64 lower-case hex characters. This does not depend on the engine's data-root path;
   2. `/proc/self/cgroup`: a 64-hex container ID in the cgroup path (cgroup v1, or v2 without a private cgroup namespace);
   3. the hostname, only when it is 12 or 64 lower-case hex characters and is a prefix of the ID of exactly one running container.
2. **Always confirmed by the engine.** The candidate ID becomes the own container only when `inspect` of that ID returns a running container; Parachute then uses the full ID the engine returns. A candidate the engine does not confirm is discarded, and the next source is tried. With one host, that host's engine confirms. With several hosts (DMN-007), the candidate is confirmed against each host's engine, and the own container belongs only to the host whose engine confirms it; source 3 looks only at the host whose endpoint is a local socket (`unix://`). A candidate confirmed by two engines is discarded.
3. **Not containerised.** When no source gives a confirmed ID, Parachute is **not containerised**: it has no own container, no container is self-updated, `parachute.armed` is not read from any container, and `--helper-image` has no default (BKP-010). This includes a container whose mountinfo gives nothing, on cgroup v2 with a private cgroup namespace, and with a custom hostname. When Parachute finds signs that it runs in a container (`/.dockerenv` or `/run/.containerenv` exists) but establishes no ID, it MUST log a `warn` line at startup saying it runs as not containerised.
4. **Labels never identify.** No `parachute.*` label, set or inherited, is a source, and a label never overrides the result (SEL-012, SEL-013).
5. **Custom hostnames are safe.** Sources 1 and 2 do not depend on the hostname, so `--hostname` does not change the result where they work; source 3 ignores any hostname that is not a hex prefix, and never picks between two containers.

## Consequences
- Self-update, the default helper image, and label arming work with a custom hostname on Docker.
- Source 3 is the weakest: an operator who sets a hostname equal to another container's short ID, in a runtime where sources 1 and 2 give nothing, makes that container the own container. The engine confirmation limits this to a running container on the local host, and self-update still needs approval (UPD-018).
- Other runtimes (Podman, DMN-011) keep the per-container files in another directory; supporting them may add a mount-source path to source 1 by amending this ADR.
- Identification needs read access to `/proc/self`, which every container has.

## Specs affected
SEL-011, SEL-012, SEL-013, UPD-018, BKP-010, CFG-006, DMN-007.
