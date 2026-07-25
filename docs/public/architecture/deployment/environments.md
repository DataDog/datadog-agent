# Runtime environments

-----

The same Agent binaries run on bare-metal hosts, in a single Docker container, as a Kubernetes DaemonSet, as an ECS or EKS Fargate sidecar, and inside serverless runtimes — and they adapt automatically. The adaptation mechanism is the *feature detection* system in [`pkg/config/env`](<<<SRC>>>/pkg/config/env): once, at configuration load time, the Agent probes its surroundings (sockets, environment variables, well-known files) and records a set of feature flags that nearly every containerized subsystem then keys off. This page explains the detection machinery and lays out the behavior matrix across deployment modes. Packaging per mode is covered in [Packaging](packaging.md) and [Container images](container-images.md); process topology per mode in [Process supervision](../processes/supervision.md).

## Key packages and files

| Path | Purpose |
|---|---|
| [`pkg/config/env/environment.go`](<<<SRC>>>/pkg/config/env/environment.go) | Coarse predicates: `IsContainerized`, `IsKubernetes`, `IsECS`, `IsECSFargate`, `IsECSSidecarMode`, `IsHostProcAvailable` |
| [`pkg/config/env/environment_detection.go`](<<<SRC>>>/pkg/config/env/environment_detection.go) | `DetectFeatures`, `IsFeaturePresent`, include/exclude handling |
| [`pkg/config/env/environment_containers.go`](<<<SRC>>>/pkg/config/env/environment_containers.go) | The per-feature probing logic (`detectContainerFeatures`) |
| [`pkg/config/env/environment_container_features.go`](<<<SRC>>>/pkg/config/env/environment_container_features.go) | The canonical feature list |
| [`pkg/config/autodiscovery/autodiscovery.go`](<<<SRC>>>/pkg/config/autodiscovery/autodiscovery.go) | Maps detected features to Autodiscovery providers and listeners |
| [`pkg/util/fargate/detection.go`](<<<SRC>>>/pkg/util/fargate/detection.go) | `IsSidecar()`, `GetOrchestrator()` — ECS vs EKS Fargate disambiguation |
| [`pkg/util/ecs/`](<<<SRC>>>/pkg/util/ecs) | ECS metadata endpoint clients and detection helpers |
| [`pkg/util/hostname/`](<<<SRC>>>/pkg/util/hostname) | Hostname provider chain, including container-aware strategies |
| [`pkg/config/setup/config.go`](<<<SRC>>>/pkg/config/setup/config.go) | Calls `DetectFeatures` at config load; `GetPlatformDefault`; `IsCLCRunner` |
| [`cmd/serverless-init/`](<<<SRC>>>/cmd/serverless-init) | Serverless PaaS binary (Cloud Run, Azure Container Apps, Azure App Service) |

## Feature detection

`DetectFeatures(cfg)` runs exactly once, from [`pkg/config/setup/config.go`](<<<SRC>>>/pkg/config/setup/config.go) at configuration load time — guaranteed to be after the YAML + environment variable merge, so probes can consult config values. Calling `IsFeaturePresent` before detection has run **panics** ("Trying to access features before detection has run"); any code path that executes before config load (early init, some CLI subcommands) must not touch features. The ordering guarantee is documented at the call site in [`pkg/config/autodiscovery/autodiscovery.go`](<<<SRC>>>/pkg/config/autodiscovery/autodiscovery.go).

Detection is gated by `autoconfig_from_environment` (default true). Turning it off disables most container components wholesale, so the supported way to trim behavior is selective: `autoconfig_exclude_features` (regex, optional `name:` prefix) removes detected features and `autoconfig_include_features` force-adds them.

### What is detected, and how

| Feature | Detection mechanism ([`environment_containers.go`](<<<SRC>>>/pkg/config/env/environment_containers.go)) |
|---|---|
| `docker` | `DOCKER_HOST`, or socket probe of `/var/run/docker.sock` (also `/host/var/run/docker.sock` when containerized — a successful probe sets `DOCKER_HOST` via a config override so downstream clients just work) |
| `containerd`, `cri` | `cri_socket_path`, or default socket probe (`containerd.sock`, `crio.sock`); deliberately **not** auto-probed when running inside Docker |
| `crio` | CRI socket identifying as CRI-O |
| `kubernetes` | `KUBERNETES_SERVICE_PORT` (injected by Kubernetes) or `KUBERNETES` set to any non-empty value (Datadog manifests conventionally use `KUBERNETES=yes`) |
| `ecsec2` | `AWS_EXECUTION_ENV=AWS_ECS_EC2`, ECS metadata URI env vars, or `/etc/ecs/ecs.config` |
| `ecsfargate` | `ECS_FARGATE` env or `AWS_EXECUTION_ENV=AWS_ECS_FARGATE` |
| `ecsmanagedinstances` | `AWS_EXECUTION_ENV=AWS_ECS_MANAGED_INSTANCES`; daemon vs sidecar chosen by `ecs_deployment_mode` |
| `eksfargate` | Purely the `eks_fargate` config setting (`DD_EKS_FARGATE=true`) — nothing is probed |
| `podman` | Podman storage directories, including a rootless scan of `/home/*/.local/share/containers/storage` |
| `cloudfoundry` | Purely the `cloud_foundry` config setting — nothing is probed |
| `podresources`, `kubernetes_deviceplugins` | Kubelet pod-resources / device-plugin sockets |
| `nvml` | `dlopen` probe of the NVIDIA management library (GPU support) |
| `process` | Linux hosts where service discovery can run |
| `kube_orchestratorexplorer`, `ecs_orchestratorexplorer` | Orchestrator explorer collection enabled for the platform |

Coarser predicates in [`environment.go`](<<<SRC>>>/pkg/config/env/environment.go) complement the feature set: `IsContainerized()` is simply "is `DOCKER_DD_AGENT` set" (baked into the [container images](container-images.md)), and `IsHostProcAvailable()` reports whether `/host/proc` is mounted, which gates host-level collection from inside a container.

### Who consumes features

Roughly 40 packages gate on `IsFeaturePresent`. The most important consumers:

1. **[Autodiscovery](../checks/autodiscovery.md)** — [`pkg/config/autodiscovery/autodiscovery.go`](<<<SRC>>>/pkg/config/autodiscovery/autodiscovery.go) picks listeners and providers: the kubelet listener iff `kubernetes`; the container listener iff `docker`/`containerd`/`podman`/ECS-sidecar and not Kubernetes; the `process` feature enables service discovery on Linux hosts.
1. **[Workloadmeta](../containers/workloadmeta.md)** collectors register per feature (docker, containerd, crio, podman, kubelet, ECS...).
1. **Container metrics providers** ([`pkg/util/containers/metrics`](<<<SRC>>>/pkg/util/containers/metrics)) register per runtime.
1. **Hostname resolution** ([`pkg/util/hostname`](<<<SRC>>>/pkg/util/hostname)) only tries container strategies when the relevant features are present.
1. The logs-agent's containers-or-pods decision, static tags, and the [tagger](../containers/tagger.md) collectors.

Per-platform configuration defaults use a parallel mechanism: `GetPlatformDefault` in [`pkg/config/setup/config.go`](<<<SRC>>>/pkg/config/setup/config.go) resolves defaults in the order fargate → container → GOOS → other, which is how, for example, system-probe defaults to disabled on Fargate.

## The behavior matrix

| Aspect | Host (deb/rpm/MSI/dmg) | Docker single container | Kubernetes DaemonSet | ECS/EKS Fargate sidecar | serverless-init |
|---|---|---|---|---|---|
| Supervision | systemd / SCM / launchd ([supervision](../processes/supervision.md)) | s6-overlay, `agent` service fatal | kubelet; one container per product | task/pod runtime; agent is a plain sidecar | serverless-init wraps the user app (init mode) |
| Configuration | `/etc/datadog-agent/*.yaml` + fleet `managed/` layer | `DD_*` env vars + `cont-init.d` autoselected YAML | env vars from Helm/Operator; emptyDir copy of `/etc/datadog-agent` | env vars only | env vars, forced defaults (`DD_HOSTNAME=none`) |
| Hostname identity | host FQDN / cloud IMDS | underlying host, via mounted sockets | node name via kubelet | none — the task/pod is the identity unit | none |
| Upgrades | package manager or [fleet daemon](fleet.md) with experiments and auto-rollback | image replacement | image replacement (Helm/Operator) | task definition update | image/layer update |
| Installer daemon | runs (`remote_updates` defaults true) | absent (binary deleted from image) | absent | absent | absent |
| system-probe / eBPF | own unit, root | s6 `sysprobe` service (needs privileges) | dedicated container with capabilities, seccomp profile, host mounts | unavailable (no host access) | unavailable |
| Host filesystem | native | `/host/proc`, `/host/sys` mounts (`IsHostProcAvailable`) | hostPath mounts | none | none |

### Kubernetes DaemonSet

The Helm chart ([DataDog/helm-charts](https://github.com/DataDog/helm-charts)) and the [Datadog Operator](https://github.com/DataDog/datadog-operator) live in external repositories; the generated examples in [`Dockerfiles/manifests/`](<<<SRC>>>/Dockerfiles/manifests) document the shapes. The DaemonSet pod runs one container per product from the same image (`agent`, `trace-agent` via `trace-loader`, `process-agent`, `system-probe`, `security-agent`) with `hostPID: true`. Cross-container and app-to-agent IPC ride shared volumes: the DogStatsD UDS `/var/run/datadog/dsd.socket` and APM UDS `/var/run/datadog/apm.socket` are hostPath mounts shared into application pods; the kubelet is reached via `DD_KUBERNETES_KUBELET_HOST` = `status.hostIP`; the [Cluster Agent](../containers/cluster-agent.md) is reached through a Service with the shared `DD_CLUSTER_AGENT_AUTH_TOKEN`. [Cluster-check runners](../containers/cluster-checks.md) are ordinary agent containers started via the `clusterchecks-agent` entrypoint and classified at runtime by `IsCLCRunner` ([`pkg/config/setup/config.go`](<<<SRC>>>/pkg/config/setup/config.go)).

### ECS and Fargate

1. **ECS on EC2** — the agent runs as a per-host daemon container; feature `ecsec2`; task metadata comes from the ECS metadata endpoints ([`pkg/util/ecs`](<<<SRC>>>/pkg/util/ecs)).
1. **ECS Fargate** — no host access exists, so the agent runs as a sidecar *inside each task* (feature `ecsfargate`). [`IsSidecar()`](<<<SRC>>>/pkg/util/fargate/detection.go) is true, and the Agent reports **no hostname** — the task is the identity unit. The `50-ecs.sh` init script strips all non-autodiscovery default checks.
1. **EKS Fargate** — same sidecar idea per pod, opted in with `DD_EKS_FARGATE=true`. The node name must be fed through the downward API as `DD_KUBERNETES_KUBELET_NODENAME`, otherwise kubelet-based features fail with an explicit error.
1. **ECS Managed Instances** — feature `ecsmanagedinstances`; runs as a daemon by default (hostname from EC2 IMDS) or as a sidecar with `ecs_deployment_mode: sidecar`.

### Serverless

[`cmd/serverless-init`](<<<SRC>>>/cmd/serverless-init) targets Cloud Run (and Jobs), Azure Container Apps, and Azure App Service, with one detector per platform in [`cloudservice/`](<<<SRC>>>/cmd/serverless-init/cloudservice). It has two modes decided by argv ([`mode/mode.go`](<<<SRC>>>/cmd/serverless-init/mode/mode.go)): **init mode** wraps the user's process as a child (`serverless-init /path/to/app args...`), while **sidecar mode** (no args) runs as a separate container and force-enables cross-container traffic (`DD_APM_NON_LOCAL_TRAFFIC`, `DD_DOGSTATSD_NON_LOCAL_TRAFFIC`). Both force `DD_HOSTNAME=none` and disable remote config. Shutdown is engineered against Cloud Run's 10-second SIGTERM grace window: the flush-phase budgets sum to roughly 9 seconds and are documented exhaustively at the top of [`main.go`](<<<SRC>>>/cmd/serverless-init/main.go). The AWS Lambda extension is **not** in this repository — it builds from [DataDog/datadog-lambda-extension](https://github.com/DataDog/datadog-lambda-extension) against the shared libraries in [`pkg/serverless/`](<<<SRC>>>/pkg/serverless).

### Reduced flavors

The Heroku buildpack flavor and the IoT agent are host-style deployments with reduced binary sets (see [Binaries and flavors](../processes/binaries.md) and [Packaging](packaging.md)): IoT is a single lean Go binary without Python; Heroku is the full agent minus packaging niceties like the trace-loader shim, running inside dynos where only environment variables configure it.

## Hostname resolution differences

Hostname is the identity key for most Datadog products, and its resolution is environment-dependent ([`pkg/util/hostname/providers.go`](<<<SRC>>>/pkg/util/hostname/providers.go) runs a provider chain: config, hostname file, fargate, GCE, Azure, FQDN, container, OS, EC2):

1. **Host installs** resolve `hostname` config, then cloud-provider metadata (IMDS), then OS FQDN.
1. **Docker/Kubernetes** must report the *underlying host's* identity, not the container's: container strategies query the runtime or kubelet through the mounted sockets, which is why the DaemonSet mounts them. Feature detection gates these strategies — an agent that fails Docker detection silently falls back to reporting its container hostname, which fragments host identity.
1. **Fargate sidecars** intentionally resolve to no hostname (`IsSidecar()` short-circuits the chain): there is no host to bill or aggregate against, and container/task tags carry identity instead.
1. **serverless-init** forces `DD_HOSTNAME=none` for the same reason.

## Configuration

| Setting / env | Effect |
|---|---|
| `autoconfig_from_environment` (`DD_AUTOCONFIG_FROM_ENVIRONMENT`) | Master switch for feature detection (default true) |
| `autoconfig_exclude_features`, `autoconfig_include_features` | Selectively remove/force features (exclude entries are regexes with an optional `name:` prefix; include entries are exact feature names) |
| `cri_socket_path`, `containerd_namespace(s)`, `podman_db_path` | Runtime probe inputs |
| `KUBERNETES=yes`, `KUBERNETES_SERVICE_PORT` | Kubernetes detection |
| `ECS_FARGATE`, `AWS_EXECUTION_ENV` | ECS-family detection |
| `eks_fargate` (`DD_EKS_FARGATE`) | EKS Fargate mode (explicit opt-in) |
| `ecs_deployment_mode` | ECS Managed Instances daemon vs sidecar |
| `DOCKER_DD_AGENT` | Baked into images; drives `IsContainerized()` |
| `DD_KUBERNETES_KUBELET_HOST`, `DD_KUBERNETES_KUBELET_NODENAME` | Kubelet reachability and node identity in Kubernetes/EKS Fargate |
| `hostname`, `hostname_fqdn` | Hostname resolution overrides |

## Gotchas

1. **`IsFeaturePresent` panics before config load.** Feature access is only legal after `DetectFeatures` has run from config setup; early-init code and some CLI subcommands must avoid it.
1. **CRI autodetection is suppressed inside Docker** to avoid discovering Docker's own internal containerd. Running the agent in a Docker container on a containerd host requires setting `cri_socket_path` explicitly.
1. **EKS Fargate is config-driven, not probed.** Forgetting `DD_EKS_FARGATE=true` leaves the agent thinking it is a normal Kubernetes pod; forgetting `DD_KUBERNETES_KUBELET_NODENAME` breaks kubelet features.
1. **Disabling `autoconfig_from_environment` is a sledgehammer.** It logs a warning and disables most container components; use the exclude/include lists instead.
1. **Hostname misdetection is silent.** If runtime sockets are not mounted, the container hostname provider cannot resolve the host identity and each agent container may report itself as a distinct host.
1. **The Docker probe mutates config.** Detecting Docker at `/host/var/run/docker.sock` sets `DOCKER_HOST` through a config override — later code that reads `DOCKER_HOST` sees the probe's result, not the process environment.
