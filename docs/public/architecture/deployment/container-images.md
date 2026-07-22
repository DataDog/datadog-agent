# Container images

-----

All Agent container images are assembled in [`Dockerfiles/`](<<<SRC>>>/Dockerfiles) from the same `.tar.xz` install tree that the OS packages use (see [Packaging](packaging.md)). The flagship `datadog/agent` image is deliberately schizophrenic: run with no arguments it supervises *all* agent processes under s6-overlay (the single-container Docker model), but an `ENTRYPOINT` environment variable turns the same image into a single-process container (the Kubernetes DaemonSet model, one container per product). Smaller purpose-built images exist for the Cluster Agent, standalone DogStatsD, the DDOT collector, and CWS instrumentation.

## Image inventory

| Image | Dockerfile | Contents and model |
|---|---|---|
| `datadog/agent` | [`Dockerfiles/agent/Dockerfile`](<<<SRC>>>/Dockerfiles/agent/Dockerfile) | Ubuntu 24.04 base; full agent tree; s6-overlay v2 supervision; optional JMX (`WITH_JMX` adds OpenJDK 11) and FIPS variants |
| `datadog/agent` (Windows) | [`Dockerfiles/agent/windows/amd64/Dockerfile`](<<<SRC>>>/Dockerfiles/agent/windows/amd64/Dockerfile) | Server Core; zip install via `install.ps1`; C++ `entrypoint.exe` supervisor ([`windows/entrypoint/`](<<<SRC>>>/Dockerfiles/agent/windows/entrypoint)) starting the Windows services |
| `datadog/cluster-agent` | [`Dockerfiles/cluster-agent/Dockerfile`](<<<SRC>>>/Dockerfiles/cluster-agent/Dockerfile) | Single `datadog-cluster-agent` binary, no s6; bundles compliance policies, `cws-instrumentation`, `secret-generic-connector`; supports unprivileged users (`dd-agent`, or OpenShift's random UID in the root group) but defaults to root — the `USER dd-agent` line is commented out because the external-metrics API binds port 443 |
| `datadog/dogstatsd` | [`Dockerfiles/dogstatsd/alpine/Dockerfile`](<<<SRC>>>/Dockerfiles/dogstatsd/alpine/Dockerfile) | Static `dogstatsd` binary on Alpine; `EXPOSE 8125/udp`; minimal attack surface |
| DDOT overlay | [`Dockerfiles/agent-ddot/Dockerfile`](<<<SRC>>>/Dockerfiles/agent-ddot/Dockerfile) | `FROM registry.datadoghq.com/agent:<version>`; adds `otel-agent`, `otel-config.yaml`, and an s6 `otel` service (see [DDOT](../otel/ddot.md)) |
| `datadog/cws-instrumentation` | [`Dockerfiles/cws-instrumentation/Dockerfile`](<<<SRC>>>/Dockerfiles/cws-instrumentation/Dockerfile) | `FROM scratch`, one binary, `USER 1000`; injected into user pods by the [admission controller](../containers/admission-controller.md) for [CWS](../ebpf/cws.md) |
| Example manifests | [`Dockerfiles/manifests/`](<<<SRC>>>/Dockerfiles/manifests) | Helm-template-generated Kubernetes manifests documenting the deployment shapes (`generate.sh`) |

## Anatomy of the Linux agent image

The multi-stage [`Dockerfile`](<<<SRC>>>/Dockerfiles/agent/Dockerfile) unpacks the Omnibus `datadog-agent*-<arch>.tar.xz` CI artifact into `/opt/datadog-agent`, then diverges from the host package in deliberate ways:

1. The bundled `installer` binary and the systemd units are **deleted** — containers upgrade by image replacement, so the [fleet](fleet.md) machinery has no place there.
1. s6-overlay v2 (sha256-pinned) is added as the in-container init/supervisor.
1. A seccomp-based shim, [`nosys.so`](<<<SRC>>>/Dockerfiles/agent/nosys-seccomp), is `LD_PRELOAD`ed so glibc does not fault on syscalls missing from old host kernels.
1. The `dd-agent` user is asserted to be **UID 100** — a build step fails otherwise, because Kubernetes `securityContext` blocks can only reference numeric UIDs and customer manifests hardcode `runAsUser: 100`.
1. Files are chowned `dd-agent:root` with group write so the image can run as an arbitrary non-root UID (OpenShift), though the default user is root because most host-inspection features need it.
1. `EXPOSE 8125/udp 8126/tcp`; the Docker `HEALTHCHECK` runs [`probe.sh`](<<<SRC>>>/Dockerfiles/agent/probe.sh), which calls `agent health` against the core agent API.

Environment markers are baked into the image: `DOCKER_DD_AGENT=true` — this is what [`env.IsContainerized()`](<<<SRC>>>/pkg/config/env/environment.go) keys on (see [Runtime environments](environments.md)) — plus s6 tuning (`S6_KEEP_ENV=1`, `S6_LOGGING=0`, `S6_BEHAVIOUR_IF_STAGE2_FAILS=2`, `S6_READ_ONLY_ROOT=1`).

The JMX flavor installs a JRE for [JMXFetch](../checks/jmx.md); the JMX-FIPS flavor additionally wires the BouncyCastle FIPS provider through `JAVA_TOOL_OPTIONS`; the FIPS agent flavor runs `fipsinstall.sh` at build time.

## Entrypoint dispatch

The image's default command (`CMD ["/bin/entrypoint.sh"]` — there is no Docker `ENTRYPOINT` instruction, so Kubernetes `command:` overrides it cleanly) is [`entrypoint.sh`](<<<SRC>>>/Dockerfiles/agent/entrypoint.sh), a tiny dispatcher: it execs `/opt/entrypoints/$ENTRYPOINT` (path-traversal guarded — any `/` in the value is rejected), falling back to `_default` when the `ENTRYPOINT` env var is unset. The scripts live in [`entrypoint.d/`](<<<SRC>>>/Dockerfiles/agent/entrypoint.d):

| `ENTRYPOINT` value | Behavior |
|---|---|
| *(unset)* → `_default` | `exec /init` — s6-overlay supervises all bundled processes (single-container model) |
| `agent`, `trace-agent`, `process-agent`, `security-agent`, `system-probe`, `otel-agent`, `agent-data-plane`, `privateactionrunner` | Run exactly one process (split-container model) |
| `clusterchecks-agent` | Wipes `conf.d/` then runs a bare agent — the [cluster-check runner](../containers/cluster-checks.md) personality |
| `init-config` | Init container: runs all `cont-init.d` scripts and exits |
| `init-volume` | Init container: copies `/etc/datadog-agent` into a shared volume |
| `seccomp-setup` | Init container: installs the system-probe seccomp profile into the host kubelet directory |
| `simple-all-in-one` | Bash supervisor running all agents in one container without s6; kills everything if any agent exits |

## Supervision inside the container

Under the default entrypoint, s6-overlay runs the service directories in [`s6-services/`](<<<SRC>>>/Dockerfiles/agent/s6-services) (`agent`, `trace`, `process`, `security`, `sysprobe`, `otel`, `data-plane`, `privateactionrunner`). The `agent` service is essential: its `finish` script calls `s6-svscanctl -t` to terminate the whole container when the core agent exits. Auxiliary services are restarted on crash but allowed to exit 0 to self-disable — a trace-agent that sees `DD_APM_ENABLED=false` simply turns itself off. The trace service starts through `trace-loader` ([`cmd/loader`](<<<SRC>>>/cmd/loader/main_nix.go)): with `apm_config.socket_activation.enabled`, the loader holds the TCP 8126/UDS listeners and only spawns the real trace-agent on the first client connection. The full semantics (and how they compare to systemd and the Windows SCM) are covered in [Process supervision](../processes/supervision.md) and documented in-repo in [`SUPERVISION.md`](<<<SRC>>>/Dockerfiles/agent/SUPERVISION.md).

## Init scripts and default configuration

At container start (under s6, or explicitly via the `init-config` entrypoint), the scripts in [`cont-init.d/`](<<<SRC>>>/Dockerfiles/agent/cont-init.d) select a default `datadog.yaml` for the environment — first match wins, and a user-mounted config is never overridden:

| Script | Trigger | Effect |
|---|---|---|
| [`50-ci.sh`](<<<SRC>>>/Dockerfiles/agent/cont-init.d/50-ci.sh) | `DD_INSIDE_CI` | `datadog-ci.yaml` CI defaults |
| [`50-ecs.sh`](<<<SRC>>>/Dockerfiles/agent/cont-init.d/50-ecs.sh) | `ECS_FARGATE` | `datadog-ecs.yaml`; deletes all non-autodiscovery default checks |
| [`50-ecs-managed.sh`](<<<SRC>>>/Dockerfiles/agent/cont-init.d/50-ecs-managed.sh) | `ECS_MANAGED_INSTANCES=true` and `DD_ECS_DEPLOYMENT_MODE=sidecar` | ECS Managed Instances sidecar defaults |
| [`50-eks.sh`](<<<SRC>>>/Dockerfiles/agent/cont-init.d/50-eks.sh) | `DD_EKS_FARGATE` | Keeps only the kubelet and `eks_fargate` checks |
| [`50-kubernetes.sh`](<<<SRC>>>/Dockerfiles/agent/cont-init.d/50-kubernetes.sh) | `KUBERNETES` | `datadog-kubernetes.yaml`; enables the apiserver check only when `DD_LEADER_ELECTION=true` |
| [`51-docker.sh`](<<<SRC>>>/Dockerfiles/agent/cont-init.d/51-docker.sh) | plain Docker | Docker defaults |
| [`59-defaults.sh`](<<<SRC>>>/Dockerfiles/agent/cont-init.d/59-defaults.sh) | fallback | Empty config |
| [`89-copy-customfiles.sh`](<<<SRC>>>/Dockerfiles/agent/cont-init.d/89-copy-customfiles.sh) | always | Copies `/conf.d` and `/checks.d` mounts into the config tree |

The selected YAML files are tiny (for example `apm_non_local_traffic: true`, `jmx_use_container_support: true`) — in containers, essentially all configuration arrives through `DD_*` environment variables (see [The configuration system](../configuration/overview.md)).

## Single-process versus all-in-one

The same image serves two topologies:

1. **Single container (Docker, ECS EC2)** — no `ENTRYPOINT` set; s6 supervises everything; one container is the whole agent installation.
1. **One container per process (Kubernetes DaemonSet)** — the Helm chart/Operator renders one container per product from the same image, each with an explicit entrypoint or `command:`. Containers share `/etc/datadog-agent` through an emptyDir populated by the `init-volume` and `init-config` init containers, and communicate over localhost and shared sockets ([IPC](../processes/ipc.md)). The [`Dockerfiles/manifests/all-containers/daemonset.yaml`](<<<SRC>>>/Dockerfiles/manifests/all-containers/daemonset.yaml) example shows the full shape: `hostPID: true` at the pod level, and a system-probe container with added capabilities, a Localhost seccomp profile, and host mounts (see [system-probe](../ebpf/system-probe.md)).

The Windows image only supports the supervisor model: `C:/entrypoint.exe` (a small C++ service supervisor) starts the `datadogagent` Windows service tree, with per-process PowerShell init scripts in [`entrypoint-ps1/`](<<<SRC>>>/Dockerfiles/agent/entrypoint-ps1).

## Gotchas

1. **Variants differ by removal, not by flags.** The base image deletes the `otel` and `data-plane` s6 services when their binaries are absent, and deletes the installer binary unconditionally. What an image *can* do is decided at build time; do not expect a runtime switch to bring a missing binary back.
1. **The s6 `agent` service exit kills the container.** To restart the core agent inside a running container for debugging, first remove `/var/run/s6/services/agent/finish`, or the whole container dies with it ([`SUPERVISION.md`](<<<SRC>>>/Dockerfiles/agent/SUPERVISION.md)).
1. **`ps` not showing trace-agent is normal.** With socket activation, `trace-loader` holds the sockets and the trace-agent process does not exist until the first tracer connects.
1. **UID 100 is load-bearing.** The build asserts `dd-agent` is UID 100; changing it breaks every customer manifest with `runAsUser: 100`.
1. **`DOCKER_DD_AGENT=true` has two jobs.** It drives runtime containerization detection ([Runtime environments](environments.md)) *and* short-circuits the deb postinst during image builds — unsetting it in a derived image changes agent behavior in subtle ways.
1. **Cluster Agent health is HTTP, not HEALTHCHECK.** The cluster-agent image has no Docker `HEALTHCHECK`; Kubernetes probes hit its port 5005 API instead (see [Cluster Agent](../containers/cluster-agent.md)).
