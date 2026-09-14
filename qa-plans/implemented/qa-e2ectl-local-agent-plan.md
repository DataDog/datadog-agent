# e2ectl: local host agent — Agent in a Docker container, wired to fakeintake

> **Category A — implemented, live-verified.** Both the Agent and the fakeintake run in
> Docker, on one per-environment network; the full loop (two concurrent environments with
> different agent code, different metrics, clean teardown) was verified live. See the
> implementation notes below and the
> [plan status index](../qa-e2ectl-plans-index.md#3-category-a--implemented-foundations).

**Status:** implemented in this branch; live verification recorded below.
**Baseline:** current working tree (typed agent sections, failed-start recovery).

### Implementation notes (what landed, and what the plan got wrong)

- Landed: `local` driver (`internal/drivers/local`), `binary` installer
  (`internal/installer/binary.go`), `agent.binary` section schema
  (`cmd/internal/envconfig/binary`), `UpdateBuilder` on `cmdUpdate`, `agentconfig`
  wiring extraction (`testing/installers/agentconfig`), `localinfra` network helpers,
  `cmdList` `binary` label, registry line, 12 hermetic test targets green.
- **Readiness now waits for any flushed metric**, not the heartbeat specifically:
  the live loop renamed `datadog.agent.running` — the readiness signal — and the
  install misreported a healthy agent as failed. Waiting for a specific metric is
  wrong in an environment whose purpose is changing that metric.
- **The container is replaced before the binary is pinned**: the pinned binary is
  bind-mounted, and Linux refuses to overwrite a bind-mounted file ("text file
  busy") — install and update both remove the container first, and update never
  rebuilds (BuildForUpdate only builds).
- **Default core checks are seeded into conf.d**: the first live loop sent only
  the heartbeat because the mounted (empty) conf.d replaced the image's defaults
  with nothing. The Go core checks (cpu/memory/disk/network/uptime/load/io/
  file_handle) now get default configs written alongside user integrations;
  ~161 metric names including system.* flow from a fresh install.
- The agent needs its **rtloader shared libraries** from `dev/lib` — discovered by the
  mount-and-run spike, not by the plan. They are pinned alongside the binary and
  mounted with `LD_LIBRARY_PATH`.
- The agent also needs an **explicit hostname** — it exits when it cannot determine
  one. The generated config sets `hostname: <env>-agent`, which also makes metrics
  attributable per environment.
- Live verification: dev1 (original code) showed `datadog.agent.running`; after
  renaming the metric in `pkg/aggregator/aggregator.go`, dev2 showed
  `datadog.agent.running_modified` — both concurrently, on isolated networks; `update`
  (rebuild path) and `update --skip-build` (restart path) both verified; `stop`
  cleaned containers, networks and entries with nothing left behind. The source
  change was reverted.

## 1. Goal

The fastest possible Agent dev loop. The environment is the developer's machine; the
Agent is built from the working tree with the repo's sanctioned build task and run in a
local container, wired to a fakeintake container on the same Docker network — no image
build, no cluster, no chart.

```
e2ectl init   --base local --output local-dev.yaml
e2ectl start  --config local-dev.yaml --name dev      # docker network + fakeintake container
e2ectl install --env dev                              # dda inv agent.build -> agent container
e2ectl fakeintake metrics --env dev                   # what the Agent sends
# ... edit code ...
e2ectl update  --env dev                              # rebuild binary, restart container
e2ectl stop    --env dev
```

Config — nothing to provision beyond the common fakeintake toggle; the installer section
carries the Agent knobs:

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

## 2. Why containers on both sides (the converged decision)

- **Handling**: the container *is* the process handle — `docker rm -f <env>-agent`
  stops it, `docker logs <env>-agent` is the first thing to read when readiness fails.
  No pidfiles, no detached-process management, no orphan risk.
- **No conflicts between several agents on one host** — the stated motivation. Each
  environment gets its own Docker network and deterministic container names
  (`<env>-agent`, `<env>-fakeintake`): ports, sockets and filesystems cannot collide,
  and nothing ever touches `/etc/datadog-agent`. Multiple simultaneous local
  environments are safe, not "verify later".
- **Portability**: `dda inv agent.build` runs in the dev container and produces a Linux
  binary; it runs in the Linux container on any Docker host — macOS laptops included.
  The native draft's platform gap and exec guard disappear with it.
- **The tradeoff, stated honestly**: the Agent sees the container's view, not the
  host's (host mounts/`--pid=host` are deliberate future knobs, not v1 defaults), and
  the direct `delve`/`strace` attach of a native process is traded for `docker exec`/
  `docker logs` (a debug-port knob can come later).

## 3. The design

### Driver `local` (Pulumi-free, like kind)

- Section schema: an **empty data-only struct** — the environment is the host; the
  common `environment.fakeintake` toggle (default true) is honored exactly like kind,
  and `false` fails `start` with a pointer to the
  [receiver plan](../pending/qa-e2ectl-receiver-wiring-plan.md) (real-backend selection is that
  feature's job).
- `Start`: create the network `<env>-net`, run the fakeintake container on it
  (`localinfra.RunFakeintake` gains a network-attached variant; kind's plain one is
  untouched) with a published port for the operator, write the snapshot containing
  only the `fakeIntake` resource (operator URL `127.0.0.1:<port>`), set meta, mark
  ready. Failed starts are marked `error` and recoverable by the existing lifecycle.
- `Stop`: `docker rm -f <env>-agent` (best-effort — install may never have run), remove
  the fakeintake container and the network, delete the entry. Deterministic names, no
  bookkeeping — mirroring kind's name-based teardown; `stop --force` semantics apply
  unchanged.

### Installer `binary` (typed section, existing contract)

1. **Build**: `dda inv agent.build` (repo rule: never raw `go build`) → `bin/agent/agent`,
   a Linux binary. **Pin by copy** into the env dir (`agent-binary`) so
   `update --skip-build` reuses it and a later `git clean` cannot break a running
   environment (~150 MB tradeoff noted).
2. **Runtime image**: a **pinned official Agent image** by default (pull once), the
   section's `runtime-image` overrides it. The Agent runs by **overriding the
   entrypoint to the mounted binary** at the image's own agent path — no image build.
3. **Generate `agent.yaml`** (0600): api key from the runner profile; fakeintake
   wiring (`dd_url: http://<env>-fakeintake:80`, `logs_dd_url`, no-ssl/http)
   **extracted from `installscript.buildAgentConfig` into one shared helper** — one
   wiring policy across script and binary (the receiver-plan principle applied now);
   then the section's `config` merged with `yamlutil`. The operator's URL
   (`127.0.0.1:<port>`, from the snapshot via `provisioner.ReadSnapshotResource`)
   drives the readiness check — two addresses, both derived deterministically, none
   recorded twice.
4. **Write `conf.d/`**: the section's `integrations` map becomes folders mounted into
   the container — same rules (folder-name pattern, YAML validity) as the script
   section's `Rules`.
5. **Run the container**: `docker run -d --name <env>-agent --network <env>-net` with
   mounts for the binary, `agent.yaml` and `conf.d` at the image's expected paths;
   entrypoint override; `agent run -c <config>`.
6. **Verify readiness**: bounded wait for the `datadog.agent.running` heartbeat in the
   fakeintake (the fakeintake Go client is already Pulumi-free). On failure: error
   naming `docker logs <env>-agent`, container removed — a locally built Agent that
   crashes on startup is what this loop exists to debug.

**Update** (`Updatable`): rebuild unless `--skip-build` (reuses the pinned binary) →
`docker rm -f` → re-run 3–6. **Artifact()**: empty version/image (built from source);
`list` shows `binary` (small `cmdList` tweak — no fake version strings).

**Known runtime-compatibility risk, gated:** a freshly built binary mounted into an
older official image may mismatch the image's bundled `rtloader` (python checks). The
readiness gate catches a hard failure immediately; the escape hatch is `runtime-image`
pointing at a dev image built with `dda inv agent.hacky-dev-image-build` (the kind
loop's proven artifact) when python-check fidelity matters. Implementation step 0
verifies mount-and-override against the pinned default image before anything else lands.

Environment directory layout (`$E2ECTL_HOME/envs/<name>/`):

```
snapshot.json   fakeintake resource only (operator URL)
agent.yaml      0600: api key + container-network wiring + user config
conf.d/         integrations from the binary section
agent-binary    the pinned build
```

## 4. Generic changes (small)

1. **Non-image update builds** — `cmdUpdate`'s build gate is image-specific; add:

   ```go
   // UpdateBuilder is implemented by installers whose update prepares a
   // non-image artifact (e.g. the local binary build).
   type UpdateBuilder interface {
       BuildForUpdate(cfg *config.File) error
   }
   ```

   `cmdUpdate` calls it when `--skip-build` is absent, otherwise keeps the existing
   image gate (helm). A step toward follow-up plan §5.3, not that full refactor.
2. **Shared fakeintake wiring** — the `buildAgentConfig` extraction above (the helper
   takes scheme/host/port, so script and binary feed it different addresses through
   one policy).
3. **`localinfra` networking** — network create/remove helpers and a network-attached
   `RunFakeintake` variant, deterministic names, best-effort cleanup.

## 5. Honest boundaries

- Container view, not host view (no host mounts/extra caps in v1); host-level features
  are deliberate future knobs.
- macOS and Linux both work (Linux binary + Docker); Windows later.
- Core Agent only in v1 — no system-probe, no privileged collectors.
- The fakeintake is required in v1; `fakeintake: false` fails `start` with a pointer to
  the receiver plan.
- Debugging is `docker exec`/`docker logs`, not native-process `delve`; a debug-port
  knob can come later.
- Build cost equals `dda inv agent.build` (minutes cold); the loop win is that no
  image is built, no cluster is created and no chart is installed — binary rebuild +
  container restart only.

## 6. Implementation steps

| Step | Files | Gate |
|---|---|---|
| 0. Mount-and-run spike | — | On the dev host: `dda inv agent.build` → mount the binary into the pinned official image with an image-built config → heartbeat observed. Answers the rtloader/ABI question before anything lands |
| 1. Section schema | `cmd/internal/envconfig/binary` (+tests); empty `local` driver schema | Round-trip, rules (folder pattern, YAML), valid example — mirroring `script` |
| 2. Wiring extraction | shared helper; `installscript` delegates | Existing script tests stay green |
| 3. `localinfra` networking | network create/remove + `RunFakeintake` variant | Hermetic: command construction with a stub docker; PATH-stripped failures honest |
| 4. `local` driver | `internal/drivers/local`, registry line | Hermetic driver tests; failed-start recovery applies unchanged |
| 5. `binary` installer | installer package, registry line, `UpdateBuilder` in `cmdUpdate`, `cmdList` tweak | Hermetic: config generation (0600, container-network wiring, no secrets in *generated* configs), conf.d write, `docker run` command construction, readiness-timeout path |
| 6. Live loop | — | Local, Docker-only (no cloud): build → install → heartbeat in fakeintake → touch code (`rename-a-metric` parity with the kind verification) → `update` → renamed metric observed → `stop` cleans containers, network and entry |

## 7. Decisions to confirm

1. Base ID `local`, installer ID `binary`; no mode knob — the Agent always runs in a
   container.
2. `dda inv agent.build` as the build entry (vs. a lighter invoke task).
3. Pin the binary by copy into the env dir (vs. recording the `bin/agent/agent` path).
4. Default runtime image policy: a pinned official tag (which tag, and where the pin
   lives) vs. requiring `runtime-image` explicitly from day one.
5. Readiness wait bound (propose 3 minutes, excluding the build which happens first).
6. Whether the rtloader escape hatch (`runtime-image` → dev image) is documented in
   `init` comments (recommendation: yes, one line).
