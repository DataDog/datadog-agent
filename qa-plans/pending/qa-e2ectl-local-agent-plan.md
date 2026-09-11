# e2ectl: local host agent — go build, run in a container, wired to fakeintake

> **Category C — pending feature; not implemented.** A new environment type through the
> existing registration surface — no new CLI concepts. Revised design: the Agent runs in
> a Docker container, not as a host process (see §2 for why this is primary, not just a
> conflict-avoidance option). See the
> [plan status index](../qa-e2ectl-plans-index.md#5-category-c--pending-feature-designs-not-implemented).

**Status:** design only; no application code changes.
**Baseline:** current working tree (typed agent sections, failed-start recovery).

## 1. Goal

The fastest possible Agent dev loop: **no cluster, no VM, no Docker image build**. The
environment is the developer's machine; the Agent is built from the working tree with
the repo's sanctioned build task and run **in a local container** on a per-environment
Docker network, wired to a local fakeintake container. This answers the vision's
standing open question — "what does 'the user laptop as an environment' support" —
concretely.

```
e2ectl init   --base local --output local-dev.yaml
e2ectl start  --config local-dev.yaml --name dev      # docker network + fakeintake container
e2ectl install --env dev                              # dda inv agent.build -> agent container
e2ectl fakeintake metrics --env dev                   # what the Agent sends
# ... edit code ...
e2ectl update  --env dev                              # rebuild binary, restart container
e2ectl stop    --env dev
```

Config — `environment` has a fakeintake and nothing else; the installer section carries
the Agent knobs:

```yaml
schema: 1
environment:
  base: local
agent:
  install: binary
  binary:
    # runtime-image: gcr.io/datadoghq/agent:7.67.0   # optional; pinned default otherwise
    # config: |          # optional datadog.yaml overrides
    #   log_level: debug
    # integrations: ...  # optional conf.d folders, like agent.script
```

## 2. Why a container is the primary design (revision)

The first draft ran the binary as a host process. Checking the build path changed the
design: `agent.build` is `@run_on_devcontainer` — the sanctioned build produces a
**Linux** binary. A bare host process therefore only works on Linux hosts (a macOS
laptop would need an unsanctioned host-side `go build`); the Linux binary runs in the
container on any Docker host. On top of that portability fix, the container delivers
exactly the isolation asked for:

- **No conflicts with a host-installed Agent**: own network namespace (only the
  fakeintake publishes a port), own filesystem (`/etc/datadog-agent` is a mount of the
  env dir, never the host's), no systemd, no host sockets.
- **Multiple local environments** become safe: each gets its own Docker network and
  containers (`<name>-agent`, `<name>-fakeintake`), so ports and sockets cannot collide.
- **What is preserved**: the whole speed point — the binary is built once via
  `dda inv agent.build` and **mounted into an existing runtime image**; no
  `hacky-dev-image-build`, no cluster, no Helm.

Tradeoff, stated honestly: the Agent sees the container's view, not the host's
(reduced visibility for host-level checks in v1; privileged mounts/`--pid=host` are
deliberate future knobs, not defaults).

## 3. Architecture fit: two registrations, no CLI changes

| Piece | What it is |
|---|---|
| `local` driver (`internal/drivers/local`) | Pulumi-free, like kind. `Start` = create the per-env Docker network, run the fakeintake container on it (published port for the operator), write a snapshot containing only the `fakeIntake` resource, mark ready. `Stop` = remove the agent container (best-effort), the fakeintake container and the network, delete the entry. |
| `binary` installer | Implements the existing `Installer` contract (`AgentExample`, `Artifact`, `Validate`, `Install`) plus `Updatable`. Typed section schema in `cmd/internal/envconfig/binary`. |

Networking mirrors kind's two-address reality, but cleanly — proper Docker DNS instead
of the outbound-IP trick:

- **Operator** (inspection, `e2ectl fakeintake ...`): `127.0.0.1:<published-port>` —
  meta + snapshot, exactly like kind.
- **Agent** (its `dd_url`/`logs_dd_url`): `http://<name>-fakeintake:80` — both
  containers share the per-env network, so container DNS resolves the name.

`localinfra.RunFakeintake` gains a network-attached variant (kind keeps the plain one);
network/container names are deterministic (`<env>-net`, `<env>-agent`), so teardown needs
no bookkeeping — same principle as kind's cluster name.

Environment directory layout (`$E2ECTL_HOME/envs/<name>/`):

```
snapshot.json   fakeintake resource only
agent.yaml      0600: api key (runner profile) + fakeintake wiring + user config
conf.d/         integrations from the binary section
agent-binary    the built binary, pinned (copied) here
```

The pidfile/log file from the draft are gone: the **container is the process handle**
(`docker rm -f <name>-agent` stops it, `docker logs <name>-agent` is the first thing to
read when readiness fails).

## 4. The installer, step by step

1. **Build**: `dda inv agent.build` (repo rule: never raw `go build`) — produces
   `bin/agent/agent`, a Linux binary (dev-container build). The binary is **copied into
   the env dir**: the artifact is pinned for `update --skip-build`, restarts don't
   depend on the source tree, and a later `git clean` cannot break a running
   environment. Tradeoff: ~150 MB per local environment.
2. **Runtime image**: the default is a **pinned official Agent image** (policy resolved
   once, aligned with the fakeintake-image pinning precedent); the section's
   `runtime-image` overrides it. The Agent runs by **overriding the entrypoint to the
   mounted binary** at the image's own binary path — no image build, the image is
   pulled once.
3. **Generate `agent.yaml`**: api key from the existing runner profile secret store;
   fakeintake wiring (in-network URL, `logs_dd_url`, no-ssl/http) — **extracted from
   `installscript.buildAgentConfig` into one shared helper** so script and binary have
   a single wiring policy (the [receiver plan](qa-e2ectl-receiver-wiring-plan.md)
   principle applied now, not forked); then the section's `config` merged with the
   existing `yamlutil` merge.
4. **Write `conf.d/`**: the section's `integrations` map becomes folders — same rules
   (folder-name pattern, YAML validity) as the script section's `Rules`.
5. **Run the container**: `docker run -d --name <env>-agent --network <env>-net`
   with mounts for the binary (at the image's agent path), `agent.yaml` (at the image's
   config path) and `conf.d/`; entrypoint override; `agent run -c <config>`.
6. **Verify readiness**: bounded wait for the `datadog.agent.running` heartbeat in the
   fakeintake — the same readiness signal the kind loop was verified with. On failure:
   error naming `docker logs <env>-agent` and the container is removed — a locally
   built Agent that crashes on startup is exactly what this loop exists to debug.

**Update** (`Updatable`): rebuild (unless `--skip-build`, which reuses the pinned
binary) → `docker rm -f` → re-run steps 2–6. **Artifact()**: returns empty
version/image (the Agent is built from source); `list` shows `binary` for installed
local agents — one small display tweak in `cmdList`.

**Known runtime-compatibility risk, gated:** a freshly built binary mounted into an
older official image may mismatch the image's bundled `rtloader` (python checks). The
readiness gate catches a hard failure immediately; the escape hatch is the
`runtime-image` option pointing at a dev image built with
`dda inv agent.hacky-dev-image-build` (the kind loop's proven artifact) when
python-check fidelity matters. First implementation step must verify the
mount-and-override entrypoint works against the pinned default image before anything
else lands.

## 5. Generic changes (small)

1. **Non-image update builds**: `cmdUpdate`'s build gate is image-specific (it asks
   `Artifact` for an image). Add an optional installer capability:

   ```go
   // UpdateBuilder is implemented by installers whose update prepares a
   // non-image artifact (e.g. the local binary build).
   type UpdateBuilder interface {
       BuildForUpdate(cfg *config.File) error
   }
   ```

   `cmdUpdate`: when `--skip-build` is absent, call `BuildForUpdate` if implemented,
   otherwise keep the existing image gate (helm). A step toward follow-up plan §5.3
   ("artifact preparation owned by the update capability"), not that full refactor.
2. **Shared fakeintake wiring**: the `buildAgentConfig` extraction above.
3. **`localinfra`**: network-attached fakeintake variant + network create/remove
   helpers (deterministic names, best-effort cleanup).

## 6. Honest boundaries

- The Agent sees the container's view, not the host's (no host mounts/extra caps in
  v1); host-level features are deliberate future knobs.
- macOS and Linux both work (Linux binary + Docker); Windows later.
- `fakeintake: false` fails `start` with a pointer to the
  [receiver plan](qa-e2ectl-receiver-wiring-plan.md) — real-backend selection is that
  feature's job.
- The fakeintake is required in v1; `list` labels come from the `cmdList` tweak, not a
  fake version string.
- The build cost equals `dda inv agent.build` (minutes cold); the loop win is that no
  image is built, no cluster is created and no chart is installed — binary rebuild +
  container restart only.

## 7. Implementation steps

| Step | Files | Gate |
|---|---|---|
| 0. Mount-and-run spike | — | Verify `docker run --entrypoint <mounted binary> <pinned agent image>` starts and heartbeats with an image-built config (the rtloader/ABI question, answered before building anything) |
| 1. Section schema | `cmd/internal/envconfig/binary/config.go` (+tests) | Round-trip, rules (folder pattern, YAML), valid example — mirroring `script` |
| 2. Wiring extraction | shared helper in `testing/installers/...`; `installscript` delegates | Existing script tests stay green |
| 3. `localinfra` networking | network create/remove + `RunFakeintakeOnNetwork` | Hermetic: command construction tests with a stub docker; PATH-stripped failures honest |
| 4. `local` driver | `internal/drivers/local`, registry line, `UpdateBuilder` in `cmdUpdate`, `cmdList` tweak | Hermetic driver + command tests; failed-start recovery tests pass for `local` too |
| 5. `binary` installer | installer package, registry line | Hermetic: config generation (0600, wiring present, no secrets in *generated* configs), conf.d write, container command construction, readiness-timeout path |
| 6. Live loop | — | Local, Docker-only (no cloud): build → install → heartbeat in fakeintake → touch code (`rename-a-metric` parity with the kind verification) → `update` → renamed metric observed → `stop` cleans containers + network + entry |

## 8. Decisions to confirm

1. Base ID `local`, installer ID `binary` (the vision's `install: binary`).
2. Container-run primary (this revision) vs. keeping a bare-process variant flag for
   Linux hosts — recommendation: container only; one path, portable, isolated.
3. `dda inv agent.build` as the build entry (vs. asking for a lighter invoke task).
4. Pin the binary by copying into the env dir (vs. recording the `bin/agent/agent` path).
5. Default runtime image policy: a pinned official tag (which tag, and where the pin
   lives) vs. requiring `runtime-image` explicitly from day one.
6. Readiness wait bound (propose 3 minutes, excluding the build which happens first).
7. Whether `rtloader` fidelity needs the dev-image escape hatch documented in `init`
   comments (recommendation: yes, one comment line).
