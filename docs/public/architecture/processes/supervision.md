# Process supervision

-----

Every platform starts and supervises the Agent's process family differently: systemd unit trees on Linux, Windows services with SCM dependencies, launchd on macOS, and per-process entrypoints or s6-overlay in containers. This page documents who starts whom, as which user, with which privilege boundaries — plus the `-exp` experiment units that Fleet Automation uses for remote upgrades with automatic rollback. The inventory of what these processes *are* is in [Binaries and flavors](binaries.md); how each boots internally is in [Startup and lifecycle](lifecycle.md).

## Key packages and files

| Path | Purpose |
|---|---|
| [`pkg/fleet/installer/packages/embedded/tmpl`](<<<SRC>>>/pkg/fleet/installer/packages/embedded/tmpl) | Go-templated systemd unit files; rendered units checked in under [`gen/debrpm`](<<<SRC>>>/pkg/fleet/installer/packages/embedded/tmpl/gen/debrpm) (plus `oci` and `-nocap` variants) |
| [`pkg/fleet/installer/packages/datadog_agent_linux.go`](<<<SRC>>>/pkg/fleet/installer/packages/datadog_agent_linux.go) | The real deb/rpm postinst logic: creates `dd-agent`, sets ownership, writes and enables units |
| [`omnibus/package-scripts/agent-deb/postinst`](<<<SRC>>>/omnibus/package-scripts/agent-deb/postinst) | deb maintainer script — a thin shim calling `installer postinst` |
| [`cmd/loader/main_nix.go`](<<<SRC>>>/cmd/loader/main_nix.go) | `trace-loader` socket-activation shim |
| [`tools/windows/DatadogAgentInstaller/CustomActions/Constants.cs`](<<<SRC>>>/tools/windows/DatadogAgentInstaller/CustomActions/Constants.cs) | Windows service names |
| [`cmd/agent/subcommands/run/dependent_services_windows.go`](<<<SRC>>>/cmd/agent/subcommands/run/dependent_services_windows.go) | Windows: core agent starts dependent services based on config (no-op on Unix: [`dependent_services_nix.go`](<<<SRC>>>/cmd/agent/subcommands/run/dependent_services_nix.go)) |
| [`pkg/util/winutil/servicemain/servicemain.go`](<<<SRC>>>/pkg/util/winutil/servicemain/servicemain.go) | Windows service wrapper with careful exit-code handling |
| [`omnibus/package-scripts/agent-dmg/postinst`](<<<SRC>>>/omnibus/package-scripts/agent-dmg/postinst) | macOS: installs launchd plists |
| [`packages/macos/app`](<<<SRC>>>/packages/macos/app) | launchd plist templates (agent, sysprobe, data-plane, GUI) |
| [`Dockerfiles/agent/entrypoint.sh`](<<<SRC>>>/Dockerfiles/agent/entrypoint.sh) + [`entrypoint.d`](<<<SRC>>>/Dockerfiles/agent/entrypoint.d) | Container entrypoint dispatch |
| [`Dockerfiles/agent/s6-services`](<<<SRC>>>/Dockerfiles/agent/s6-services) | s6-overlay supervision tree for the all-in-one container |
| [`pkg/procmgr/rust`](<<<SRC>>>/pkg/procmgr/rust) / [`pkg/procmgr/coat`](<<<SRC>>>/pkg/procmgr/coat) | `dd-procmgrd` process-manager daemon and its Go client |

## Linux: the systemd unit tree

The deb/rpm maintainer scripts are shims — the agent-deb postinst is essentially `${INSTALL_DIR}/embedded/bin/installer postinst datadog-agent deb`. The real logic is versioned Go code in [`datadog_agent_linux.go`](<<<SRC>>>/pkg/fleet/installer/packages/datadog_agent_linux.go): it creates the `dd-agent` user and group, chowns `/etc/datadog-agent`, writes units rendered from the templates in [`embedded/tmpl`](<<<SRC>>>/pkg/fleet/installer/packages/embedded/tmpl) into `/lib/systemd/system` (deb), `/usr/lib/systemd/system` (rpm), or `/etc/systemd/system` (OCI/fleet installs), and enables `datadog-agent.service`. Debugging a package install therefore means reading Go, not shell.

The resulting tree (rendered units in [`gen/debrpm`](<<<SRC>>>/pkg/fleet/installer/packages/embedded/tmpl/gen/debrpm)):

```text
datadog-agent.service            (User=dd-agent, WantedBy=multi-user.target)
 |  Wants= (started with it, but independent)
 +-- datadog-agent-trace.service      dd-agent   ExecStart=trace-loader ... trace-agent
 +-- datadog-agent-process.service    dd-agent
 +-- datadog-agent-sysprobe.service   root       Before=datadog-agent.service
 |                                               Requires=sys-kernel-debug.mount
 +-- datadog-agent-security.service   root       ConditionPathExists=|security-agent.yaml
 +-- datadog-agent-installer.service  root       yields to standalone datadog-installer.service
 +-- datadog-agent-ddot.service       dd-agent   gated on DDOT payload + not procmgr-managed
 +-- datadog-agent-data-plane.service dd-agent   (ADP)
 +-- datadog-agent-action.service     dd-agent   (private action runner, CAP_NET_RAW)
 +-- datadog-agent-procmgr.service    dd-agent   gated on dd-procmgrd binary existing

every unit above also has an -exp "experiment" twin (see below)
```

Semantics worth knowing:

1. `datadog-agent.service` is the root: `User=dd-agent`, `AmbientCapabilities=CAP_NET_BIND_SERVICE`, `ExecStart=/opt/datadog-agent/bin/agent/agent run`, `Restart=on-failure`, and `Wants=` every satellite.
1. Satellites are `BindsTo=datadog-agent.service` — they die when the core agent unit stops. `systemctl stop datadog-agent` therefore stops the whole family.
1. Environment plumbing is uniform: every unit has `EnvironmentFile=-/etc/datadog-agent/environment` and `Environment="DD_FLEET_POLICIES_DIR=/etc/datadog-agent/managed/datadog-agent/stable"`.
1. The trace unit execs `trace-loader`, which loads `datadog.yaml`, exits 0 if APM is disabled, and with `apm_config.socket_activation.enabled` holds the receiver sockets itself and only spawns the real trace-agent on the first client connection (see [Binaries and flavors](binaries.md)).
1. **Conditions silently gate services.** The security-agent unit only starts if `security-agent.yaml` exists (in `/etc/datadog-agent/` or the fleet policies directory); the DDOT unit requires the DDOT payload *and* refuses to start once `dd-procmgrd` manages it (`ConditionPathExists=!/opt/datadog-agent/processes.d/datadog-agent-ddot.yaml`); the bundled installer unit yields to a standalone `datadog-installer.service`. A satellite showing `inactive (dead)` is very often "not configured", not broken — unconfigured satellites also simply exit 0 and stay dead until the next restart.

### Experiment (`-exp`) units

Every service has an experiment twin used by [Fleet automation](../deployment/fleet.md) for remote upgrades with automatic rollback. Taking `datadog-agent-exp.service` as the model:

1. `Conflicts=datadog-agent.service` — starting the experiment stops stable, and vice versa.
1. `OnFailure=datadog-agent.service` — if the experiment dies, systemd starts stable again automatically.
1. `Restart=no` and `ExecStart=/usr/bin/timeout --kill-after=15s $EXPERIMENT_TIMEOUT ...` — the experiment runs under a hard timeout (3000 s by default) and reads its config from `/etc/datadog-agent-exp`, so a hung upgrade cannot squat forever.
1. `ExecStopPost=/bin/false` marks any stop as a failure so the `OnFailure=` fallback fires.

During a fleet upgrade the installer daemon installs the new version under `/opt/datadog-packages/datadog-agent/<version>`, flips the `experiment` symlink, and starts the experiment tree. Seeing `datadog-agent-exp.service` running under `/usr/bin/timeout` is expected during an upgrade window; if the experiment is promoted, the symlinks flip to `stable` and the stable units restart on the new version.

## Windows: services and the SCM

The MSI ([`tools/windows/DatadogAgentInstaller`](<<<SRC>>>/tools/windows/DatadogAgentInstaller)) registers these services (names in [`Constants.cs`](<<<SRC>>>/tools/windows/DatadogAgentInstaller/CustomActions/Constants.cs)):

| Service | Binary | Account | Start type |
|---|---|---|---|
| `datadogagent` | `agent.exe` | `ddagentuser` | automatic (delayed) |
| `datadog-trace-agent` | `trace-agent.exe` | `ddagentuser` | demand, depends on `datadogagent` |
| `datadog-process-agent` | `process-agent.exe` | LocalSystem | demand, depends on `datadogagent` |
| `datadog-system-probe` | `system-probe.exe` | LocalSystem | demand, depends on `datadogagent` |
| `datadog-security-agent` | `security-agent.exe` | `ddagentuser` | demand, depends on `datadogagent` |
| `datadog-agent-action` | `privateactionrunner.exe` | `ddagentuser` | demand (absent in FIPS flavor) |
| `dd-procmgr-service` | dd-procmgr service exe | `ddagentuser` | demand |
| `ddnpm`, `ddprocmon` | kernel drivers | — | on demand |
| `Datadog Installer` | `datadog-installer.exe` | LocalSystem | demand, started when `remote_updates` is enabled |

Because the satellites are demand-start, **the core agent starts them itself**: [`dependent_services_windows.go`](<<<SRC>>>/cmd/agent/subcommands/run/dependent_services_windows.go) inspects the config at `agent run` (APM enabled → trace-agent; process/network keys → process-agent; NPM/CWS/crash-detection/software-inventory keys → system-probe; runtime security → security-agent) and asks the SCM to start what is needed. The Unix variant is a no-op — systemd owns that decision there.

Consequences of the SCM dependency edges:

1. `Restart-Service -Force datadogagent` restarts the entire tree, because the SCM stops and restarts dependents automatically.
1. Restart semantics are deliberately gated in [`servicemain.go`](<<<SRC>>>/pkg/util/winutil/servicemain/servicemain.go): config-change-triggered self-restarts exit with codes that do *not* trip the service failure actions, so recovery settings only fire on real crashes. `servicemain` also enforces a `HardStopTimeout` far shorter than the Fx stop timeout (see [Startup and lifecycle](lifecycle.md)).
1. What eBPF provides on Linux comes from kernel drivers on Windows: `ddnpm` (network) and `ddprocmon` (process monitoring), loaded on demand by system-probe.

The privilege split differs from Linux: process-agent and system-probe run as LocalSystem (privileged), while the core, trace, and security agents run as the low-privilege `ddagentuser`.

## macOS: launchd

The `.dmg` postinst ([`agent-dmg/postinst`](<<<SRC>>>/omnibus/package-scripts/agent-dmg/postinst)) installs LaunchDaemons from the templates in [`packages/macos/app`](<<<SRC>>>/packages/macos/app):

1. `com.datadoghq.agent` — `agent run` as user `_dd-agent`.
1. `com.datadoghq.sysprobe` — system-probe as root.
1. `com.datadoghq.data-plane` — ADP.

Per-user LaunchAgents provide the GUI (`com.datadoghq.gui`, systray) and the AI-usage desktop monitor. There are **no** separate trace-agent or process-agent launchd services on macOS.

## Containers: entrypoints and s6

One image ([`Dockerfiles/agent`](<<<SRC>>>/Dockerfiles/agent)) serves many personalities. [`entrypoint.sh`](<<<SRC>>>/Dockerfiles/agent/entrypoint.sh) execs `/opt/entrypoints/$ENTRYPOINT` from [`entrypoint.d`](<<<SRC>>>/Dockerfiles/agent/entrypoint.d): `agent`, `trace-agent`, `process-agent`, `security-agent`, `system-probe`, `otel-agent`, `agent-data-plane`, `privateactionrunner`, `clusterchecks-agent`, or `simple-all-in-one`. With no `ENTRYPOINT` variable, `_default` starts s6-overlay's `/init`, which supervises all services under [`s6-services`](<<<SRC>>>/Dockerfiles/agent/s6-services) (agent, trace, process, security, sysprobe, otel, data-plane, privateactionrunner) — the classic single-container Docker deployment.

The s6 `finish` scripts implement the disable-versus-restart policy: a service exiting 0 is disabled (`s6-svc -d`), anything else restarts after 2 seconds. This is how a disabled trace-agent turns itself off inside the all-in-one container instead of crash-looping. Init scripts in [`cont-init.d`](<<<SRC>>>/Dockerfiles/agent/cont-init.d) handle per-platform setup (ECS, EKS, Kubernetes detection, API-key checks) before services start.

Deployment-mode mapping (see [Container images](../deployment/container-images.md) and [Runtime environments](../deployment/environments.md)):

1. **Docker single container**: s6 supervises everything in one container; the image's files are chowned `dd-agent:root` with group-write so it can run as an arbitrary non-root UID, but the default is root (needed for host-mounted checks).
1. **Kubernetes DaemonSet** (Helm/Operator): one container per process from the same image, each with an explicit `ENTRYPOINT`; the kubelet is the supervisor and per-container liveness/readiness probes (`agent health`, `health_port` 5555) replace s6. The [Cluster Agent](../containers/cluster-agent.md) is its own Deployment; [cluster-check runners](../containers/cluster-checks.md) are agent containers with the `clusterchecks-agent` entrypoint.
1. **ECS/Fargate**: the agent runs as a sidecar container in the task definition, typically `agent` plus `trace-agent` entries; no system-probe.
1. **PaaS runtimes**: `serverless-init` is itself the supervisor — in init mode it is the container entrypoint and wraps the user's command as a child process.

## dd-procmgrd: the emerging in-house supervisor

`dd-procmgrd` ([`pkg/procmgr/rust`](<<<SRC>>>/pkg/procmgr/rust)) is a Rust process-manager daemon that supervises processes declared in `/opt/datadog-agent/processes.d/*.yaml` (Windows: `processes.d` under the install root, `%ProgramFiles%\Datadog\Datadog Agent`; overridable via `DD_PM_CONFIG_DIR`), with a gRPC control plane over UDS/named pipe and a Go client in [`pkg/procmgr/coat`](<<<SRC>>>/pkg/procmgr/coat) used by `agent status`. It runs as `dd-agent` under systemd (`datadog-agent-procmgr.service`, condition-gated on the binary existing) or as the Windows service `dd-procmgr-service`. It is the intended replacement for the one-systemd-unit-per-subagent model; DDOT is the first migrated service, which is why the DDOT systemd unit self-disables when a procmgr process declaration for it exists.

## Privilege boundaries

| Process | Linux | Windows | Why privileged (when it is) |
|---|---|---|---|
| core agent | `dd-agent` + `CAP_NET_BIND_SERVICE` | `ddagentuser` | capability only for low ports (syslog, SNMP traps) |
| trace-agent | `dd-agent` | `ddagentuser` | — |
| process-agent | `dd-agent` | LocalSystem | Windows: process enumeration APIs |
| system-probe | root | LocalSystem + kernel drivers | eBPF/kernel access |
| security-agent | root | `ddagentuser` | Linux: compliance file access; Windows work is delegated to system-probe |
| installer daemon | root | separate service | writes packages, systemd units |
| otel-agent (DDOT) | `dd-agent` | — | — |
| ADP / dd-procmgrd / action runner | `dd-agent` (runner adds `CAP_NET_RAW`) | `ddagentuser` | raw sockets for the runner's network actions |

The cross-process trust model that rides on top of these users (auth token file ownership, socket permissions) is described in [Inter-process communication](ipc.md).

## Gotchas

1. **`inactive (dead)` is usually intentional.** systemd conditions and exit-0-on-unconfigured mean most "missing" satellites are simply not enabled. Check the unit's `ConditionPathExists` lines before assuming failure.
1. **`ps` shows `trace-loader`, not `trace-agent`,** on Linux package installs until the first APM client connects. Heroku builds ship a noop loader instead.
1. **An `-exp` unit running under `timeout` is not an incident** — it is a fleet upgrade in progress. Killing it manually triggers the `OnFailure=` rollback to stable.
1. **deb/rpm maintainer scripts are shims**; the actual install/enable logic is Go code in [`pkg/fleet/installer/packages`](<<<SRC>>>/pkg/fleet/installer/packages). Strace-ing the postinst shell script tells you almost nothing.
1. **On Windows the core agent is the supervisor** for its satellites; if a satellite service does not start, the first place to look is the config keys that `dependent_services_windows.go` evaluates, not the SCM start type.
1. **Fx stop timeouts (5 min) exceed every supervisor's kill timeout** — slow shutdown paths get SIGKILLed by systemd/SCM/s6 before Fx would ever report a stop-hook timeout; see [Startup and lifecycle](lifecycle.md).
1. **Unit files differ per install channel**: deb/rpm installs use the `debrpm` renders, fleet (OCI) installs use the `oci` renders in `/etc/systemd/system`, and `-nocap` variants exist for environments without ambient-capability support. Do not hand-edit installed units; they are overwritten by the installer.
