# 01 Container selection

**Status:** Draft

Which containers Parachute considers in a cycle. A container that passes selection is a **candidate**. Settings are listed in [16-configuration.md](16-configuration.md#settings-reference); labels are read from the monitored container, never from Parachute's own container, and only as the operator set them when the container was created (SEL-013; [glossary: Trust boundaries](glossary.md#trust-boundaries)). Selection is evaluated per host (see [13-daemon-and-runtimes.md](13-daemon-and-runtimes.md)).

### SEL-001 Default selection
With no selection settings, Parachute MUST treat every running container on each managed host as a candidate, except the containers excluded by SEL-012.

- **Given** three running containers and no selection settings **When** a cycle runs **Then** all three appear in the cycle report as candidates.
- **Given** a running container and an exited container and no selection settings **When** a cycle runs **Then** only the running container is a candidate.

### SEL-002 Name filter
When container names are given as positional arguments or with the repeatable `--name <name>` flag, Parachute MUST consider only containers with those exact names. A leading `/` in a Docker container name is ignored for matching. Names that match no container MUST be logged at `warn` level once per cycle.

- **Given** running containers `a`, `b`, `c` and Parachute started with arguments `a b` **When** a cycle runs **Then** only `a` and `b` are candidates.
- **Given** the same containers and `--name a --name c` **When** a cycle runs **Then** only `a` and `c` are candidates.
- **Given** Parachute started with argument `zz` and no container named `zz` **When** a cycle runs **Then** a `warn` log line names `zz`.

### SEL-003 Opt-out label
A container labelled `parachute.enable=false` MUST NOT be a candidate.

- **Given** running container `a` labelled `parachute.enable=false` and container `b` unlabelled **When** a cycle runs **Then** the report has an entry for `b` and none for `a`, the registry access log shows no manifest request for `a`'s repository, and `a`'s container ID is unchanged.
- **Given** Parachute's own container labelled `parachute.enable=false` and monitored container `a` unlabelled **When** a cycle runs **Then** `a` is a candidate.

### SEL-004 Opt-in mode
With `--label-enable` on, Parachute MUST consider only containers labelled `parachute.enable=true`.

- **Given** `--label-enable`, container `a` labelled `parachute.enable=true`, container `b` unlabelled **When** a cycle runs **Then** only `a` is a candidate.

### SEL-005 Exclude list
Containers named in `--disable-containers` MUST NOT be candidates, even when labelled `parachute.enable=true`.

- **Given** `--disable-containers a` and `a` labelled `parachute.enable=true` **When** a cycle runs **Then** `a` is not a candidate.
- **Given** `PARACHUTE_DISABLE_CONTAINERS="a,b"` and running containers `a`, `b`, `c` **When** a cycle runs **Then** only `c` is a candidate.

### SEL-006 Criteria combine with AND
A container MUST be a candidate only if it satisfies every configured selection criterion (SEL-002 to SEL-011).

- **Given** argument `a` and `a` labelled `parachute.enable=false` **When** a cycle runs **Then** `a` is not a candidate.
- **Given** `--label-enable`, argument `a`, and container `b` labelled `parachute.enable=true` **When** a cycle runs **Then** `b` is not a candidate.

### SEL-007 Monitor-only
A container labelled `parachute.monitor-only=true`, or any container when `--monitor-only` is on, MUST be checked and reported but MUST NOT be changed. An available update is reported as `held` with reason `monitor_only` (POL-001).

- **Given** container `a` labelled `parachute.monitor-only=true` and a newer image in the registry **When** a cycle runs **Then** the report lists `a` as `held` with reason `monitor_only`, and `a`'s container ID and image ID are unchanged.
- **Given** `--monitor-only` and two containers with newer images **When** a cycle runs **Then** neither container changes and both are reported as `held`.

### SEL-008 Label precedence
With `--label-take-precedence` on, a container's own `parachute.monitor-only` and `parachute.no-pull` labels MUST override the corresponding global setting. With it off, a global setting that is on wins over a label that says `false`.

- **Given** `--monitor-only`, `--label-take-precedence`, armed, and container `a` labelled `parachute.monitor-only=false` with a newer image **When** a cycle runs **Then** `a` is updated.
- **Given** the same without `--label-take-precedence` **When** a cycle runs **Then** `a` is not updated and is reported `held` with reason `monitor_only`.

### SEL-009 Container states
By default only containers in state `running` MUST be candidates. `--include-restarting` MUST add containers in state `restarting`. `--include-stopped` MUST add containers in states `created` and `exited`. Containers in states `paused`, `removing`, and `dead` MUST never be candidates.

- **Given** a container in state `restarting` **When** a cycle runs without `--include-restarting` **Then** it is not a candidate; **When** run with it **Then** it is a candidate.
- **Given** containers in states `created` and `exited` and `--include-stopped` **When** a cycle runs **Then** both are candidates.
- **Given** a paused container and `--include-stopped --include-restarting` **When** a cycle runs **Then** it is not a candidate.

### SEL-010 Stopped containers after update
A candidate that was not running before its update MUST be left not running after a committed update, unless `--revive-stopped` is on, in which case it MUST be started. `--revive-stopped` without `--include-stopped` MUST be a startup error (exit code 2) whose message names both settings.

- **Given** an exited container with a newer image, `--include-stopped`, armed **When** a cycle runs **Then** the replacement uses the new image and is in state `created` or `exited`.
- **Given** the same with `--revive-stopped` **When** a cycle runs **Then** the replacement is `running`.
- **Given** `--revive-stopped` without `--include-stopped` **When** Parachute starts **Then** it exits with code 2 and the message contains `revive-stopped` and `include-stopped`.

### SEL-011 Scope
With `--scope <value>` set, Parachute MUST consider only containers whose `parachute.scope` label equals the value. The value `none` MUST select only containers without a `parachute.scope` label. Without `--scope`, the `parachute.scope` label is ignored. The scope applies to Parachute's own container (SEL-012) like any other container: outside the scope it is not a candidate and is never self-updated. Parachute MUST NOT stop, create, remove, or snapshot any container outside its scope, so that several Parachute instances can share a host.

- **Given** `--scope prod`, container `a` labelled `parachute.scope=prod`, `b` labelled `parachute.scope=dev`, `c` unlabelled **When** a cycle runs **Then** only `a` is a candidate.
- **Given** `--scope none` and the same containers **When** a cycle runs **Then** only `c` is a candidate.
- **Given** two Parachute instances with scopes `prod` and `dev`, armed, and newer images for `a` and `b` **When** both run a cycle **Then** each container is updated exactly once, by the instance with the matching scope.
- **Given** Parachute running with `--scope prod` as a container without a `parachute.scope` label, and a newer image for it **When** a cycle runs **Then** the report has no entry for Parachute's own container.

### SEL-012 Helper and other Parachute containers are excluded
Containers labelled `parachute.helper=true` (helpers, restore-verification containers) MUST never be candidates. Parachute's **own container**, the container the running instance runs in, MUST be identified by the engine as [ADR-0016](../adr/0016-identifying-own-container.md) says, never by a label (UPD-018), and is a candidate only for self-update; Parachute never needs a `parachute.self` label to find it. A `parachute.self=true` label, whether set at creation or inherited from the image (SEL-013), only ever makes Parachute more cautious: any container carrying it that is not the own container MUST NOT be a candidate, and the label never makes a container the own container, a candidate, or armed. (The official image carries `parachute.self=true` for information only.) Whether a container carries `parachute.helper=true` is read as SEL-013 says, so an image cannot exclude its own containers by baking this label in.

- **Given** a running container labelled `parachute.helper=true` at creation **When** a cycle runs **Then** the report has no entry for it.
- **Given** a running container created from an image whose configuration has `LABEL parachute.helper=true`, with no label set at creation, and a newer image **When** a cycle runs **Then** the report has an entry for it.
- **Given** Parachute running with `--scope ops` as a container from the official image labelled `parachute.scope=ops`, and a second container from the official image labelled `parachute.scope=ops` (another instance with the same scope), with a newer image **When** a cycle runs **Then** the report has an entry for Parachute's own container and none for the second container.
- **Given** Parachute running as a container from an image without any `parachute.*` label, and a newer image for it **When** a cycle runs **Then** the report has an entry for Parachute's own container, held with reason `approval_required` (UPD-018).
- **Given** Parachute running as a container created with `--hostname parachute-main`, and a newer image for it **When** a cycle runs **Then** the report has an entry for Parachute's own container, held with reason `approval_required`.
- **Given** a container `a` created with `--label parachute.self=true` that is not Parachute's own container, and a newer image **When** a cycle runs **Then** the report has no entry for `a` and its container ID is unchanged.

### SEL-013 Image-baked labels do not set behaviour
Parachute MUST use a `parachute.*` label only when the operator set it when creating the container. A label on the container whose key and value both equal a label in the configuration of the image the container was created from is **inherited** (the same rule as UPD-004) and MUST be treated as absent, with these exceptions, which can only make Parachute more cautious and are honoured when inherited: `parachute.self=true` (which only excludes a container, SEL-012), `parachute.monitor-only=true`, `parachute.approval=required`. Every other inherited `parachute.*` label (for example `parachute.enable`, `parachute.scope`, `parachute.armed`, `parachute.helper`, `parachute.backup`, `parachute.database`, `parachute.delay-days`, `parachute.tag-follow`, `parachute.lifecycle.*`, `parachute.backup.dump`, `parachute.verify.*`) is ignored, and Parachute MUST log a `warn` line once per cycle per container naming the container and the ignored label keys. A label whose value differs from the image's value is operator-set and is used. Parachute MUST first evaluate `parachute.enable`, `parachute.scope`, and `parachute.self` from the container's own labels alone. If they exclude the container whether or not each of them is inherited (it carries `parachute.self=true` and is not the own container, SEL-012; it lacks `parachute.enable=true` under `--label-enable`, SEL-004; or its `parachute.scope` value, or its lack of one, differs from a `--scope` value other than `none`, SEL-011), the container MUST be excluded silently: no report entry, no `warn` line, and no inspect request for its image. Otherwise, if the container's image cannot be inspected, the candidate's result is `failed` with reason `engine_error`.

- **Given** an image with `LABEL parachute.backup=none`, a container created from it without labels, and a newer image **When** a cycle runs **Then** a `warn` line names the container and `parachute.backup`, and no relaxation for that container is logged (POL-009).
- **Given** an image with `LABEL parachute.enable=false` and a container created from it without labels **When** a cycle runs **Then** the container is a candidate and a `warn` line names `parachute.enable`.
- **Given** an image with `LABEL parachute.enable=true`, `--label-enable`, and a container created from it without labels **When** a cycle runs **Then** the report has no entry for the container.
- **Given** an image with `LABEL parachute.scope=prod`, `--scope prod`, and a container created from it without labels **When** a cycle runs **Then** the report has no entry for the container.
- **Given** an image with `LABEL parachute.monitor-only=true`, a container created from it without labels, and a newer image **When** a cycle runs **Then** the result is `held` with reason `monitor_only`.
- **Given** an image with `LABEL parachute.enable=false` and a container created from it with `--label parachute.enable=true` **When** a cycle runs **Then** the container is a candidate and no `warn` line names `parachute.enable`.
- **Given** `--scope prod`, an engine proxy that answers `500` to every inspect request for container `a`'s image, and `a` labelled `parachute.scope=dev` at creation **When** a cycle runs **Then** the report has no entry for `a`, no `warn` line names `a`, and the proxy log shows no inspect request for `a`'s image.
- **Given** the same proxy, `--scope prod`, and `a` labelled `parachute.scope=prod` at creation **When** a cycle runs **Then** the result for `a` is `failed` with reason `engine_error`.
- **Given** `--include-stopped`, container `a` whose image was removed from the engine by ID, and a normal container `b` **When** a cycle runs **Then** `a` is `failed` with reason `engine_error` (its `detail` names the missing image), `b` gets its normal result, and the exit code is 1.
