> **TL;DR:** Detects the runtime platform (Docker, Kubernetes, ECS, Cloud Foundry, bare-metal, etc.) and the available container features (containerd socket, NVML, PodResources, …), producing a `FeatureMap` that drives autodiscovery and hostname resolution throughout the agent.

# pkg/config/env

**Import path:** `github.com/DataDog/datadog-agent/pkg/config/env`

## Purpose

`pkg/config/env` answers two related questions at runtime:

1. **What platform is the agent running on?** — Docker container, Kubernetes, ECS EC2, ECS Fargate, ECS Managed Instances, EKS Fargate, Cloud Foundry, bare-metal, etc.
2. **What container-related features are available?** — Docker socket, containerd, CRI-O, Podman, GPU (NVML), Kubernetes pod resources, device plugins, etc.

The answers drive decisions throughout the agent: which autodiscovery listeners to activate, which socket paths to use, whether to mount `/host/proc`, how to resolve hostnames, and so on. The package is intentionally lightweight — it has no heavy dependencies — because it is imported by nearly every agent binary.

## Key Elements

### Key types

### `Feature` and `FeatureMap`

```go
type Feature    string
type FeatureMap map[Feature]struct{}
```

A `Feature` is a named capability detected in the current environment. `FeatureMap` is the set of all features currently active. It implements `String()` for logging.

#### Predefined `Feature` constants

| Constant | String value | Detected when |
|---|---|---|
| `Docker` | `"docker"` | Docker socket is reachable or `DOCKER_HOST` is set |
| `Containerd` | `"containerd"` | containerd socket is reachable |
| `Cri` | `"cri"` | Any CRI socket is reachable and agent runs in Kubernetes |
| `Crio` | `"crio"` | CRI-O socket is reachable |
| `Kubernetes` | `"kubernetes"` | `KUBERNETES_SERVICE_PORT` or `KUBERNETES` env var is set |
| `ECSEC2` | `"ecsec2"` | ECS EC2 credentials/metadata env vars or `/etc/ecs/ecs.config` are present |
| `ECSFargate` | `"ecsfargate"` | `ECS_FARGATE` or `AWS_EXECUTION_ENV=AWS_ECS_FARGATE` |
| `ECSManagedInstances` | `"ecsmanagedinstances"` | `AWS_EXECUTION_ENV=AWS_ECS_MANAGED_INSTANCES` |
| `EKSFargate` | `"eksfargate"` | `eks_fargate` config key is true |
| `KubeOrchestratorExplorer` | `"kube_orchestratorexplorer"` | Kubernetes + `orchestrator_explorer.enabled` |
| `ECSOrchestratorExplorer` | `"ecs_orchestratorexplorer"` | ECS + orchestrator explorer + task collection enabled |
| `CloudFoundry` | `"cloudfoundry"` | `cloud_foundry` config key is true |
| `Podman` | `"podman"` | Podman containers storage path found (system-wide or in `/home`) |
| `PodResources` | `"podresources"` | Kubelet PodResources socket is reachable |
| `NVML` | `"nvml"` | NVIDIA NVML shared library found via `dlopen` |
| `KubernetesDevicePlugins` | `"kubernetes_deviceplugins"` | Kubelet device-plugins socket directory exists |
| `NonstandardCRIRuntime` | `"nonstandard-cri-runtime"` | A `cri_socket_path` was configured but is neither containerd nor CRI-O |
| `KubeletConfigOrchestratorCheck` | `"kubelet_config_orchestrator_check"` | Kubernetes + `orchestrator_explorer.kubelet_config_check.enabled` |

### Key functions

### Feature detection lifecycle

```go
func DetectFeatures(cfg model.Reader)
```

Runs detection once and stores the result. Called by `setup.LoadDatadog` (with a `defer`) so it always executes at config load time, even if the config file is missing. Panics if called before the config is loaded, and is intentionally a no-op when `autoconfig_from_environment` is false.

Detection reads sockets, environment variables, the filesystem, and config keys. On Linux/Windows it checks Docker, containerd, CRI-O, Kubernetes, all AWS environments, Cloud Foundry, Podman, PodResources, device plugins, and NVML. On other platforms (macOS), no container features are registered.

The set can be trimmed or augmented via:
- `autoconfig_exclude_features` (regex list) — remove detected features by name.
- `autoconfig_include_features` — force-add known features regardless of detection.

```go
func GetDetectedFeatures() FeatureMap
func IsFeaturePresent(feature Feature) bool
func IsAnyContainerFeaturePresent() bool
```

Both `GetDetectedFeatures` and `IsFeaturePresent` panic if called before `DetectFeatures` has run — this is intentional to catch incorrect call order.

### Simple environment predicates

These do not require detection to have run first and can be called at any time:

```go
func IsContainerized() bool        // DOCKER_DD_AGENT env var is set
func IsDockerRuntime() bool        // /.dockerenv file exists
func IsKubernetes() bool           // KUBERNETES_SERVICE_PORT or KUBERNETES env var
func IsECS() bool                  // ECS-specific env vars or /etc/ecs/ecs.config
func IsECSFargate() bool           // ECS_FARGATE or AWS_EXECUTION_ENV=AWS_ECS_FARGATE
func IsECSManagedInstances() bool  // AWS_EXECUTION_ENV=AWS_ECS_MANAGED_INSTANCES
func IsHostProcAvailable() bool    // /host/proc present (when containerized)
func IsHostSysAvailable() bool     // /host/sys present (when containerized)
```

### ECS deployment mode helpers

```go
func IsECSSidecarMode(cfg model.Reader) bool
func IsECSManagedInstancesDaemonMode(cfg model.Reader) bool
```

These helpers combine environment detection with the `ecs_deployment_mode` config key to distinguish sidecar vs. daemon ECS deployments. Fargate is always sidecar; Managed Instances default to daemon unless explicitly set to sidecar.

### Autoconfig

```go
func IsAutoconfigEnabled(cfg model.Reader) bool
```

Returns true if `autoconfig_from_environment` is enabled (default: true). Handles the deprecated `AUTOCONFIG_FROM_ENVIRONMENT` / `AUTCONFIG_FROM_ENVIRONMENT` env vars with a warning.

### Configuration and build flags

`environment_containers.go` (which registers and detects all container features) is gated on `//go:build linux || windows`. On other platforms, `environment_nocontainers.go` is compiled instead and provides stub implementations that detect nothing. This means `IsFeaturePresent(Docker)` etc. always return false on macOS in production builds.

### Test helpers (build tag `test`)

`environment_testing.go` (gated on `//go:build test`) changes the behaviour:
- `init()` disables auto-detection and initialises `detectedFeatures` to an empty map, so tests don't accidentally inherit the runner's environment.
- `SetFeatures(t, features...)` — set a specific feature set for the duration of a test; `t.Cleanup` resets it automatically.
- `SetFeaturesNoCleanup(features...)` — for integration tests that lack a `testing.T`.
- `ClearFeatures()` — remove all features (used by the cleanup registered by `SetFeatures`).

## Usage

### Checking whether a feature is active

```go
import pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"

if pkgconfigenv.IsFeaturePresent(pkgconfigenv.Docker) {
    // start Docker listener
}

if pkgconfigenv.IsAnyContainerFeaturePresent() {
    // enable container-aware hostname resolution
}
```

### Checking the raw environment

```go
if pkgconfigenv.IsContainerized() {
    procRoot = "/host/proc"
}

if pkgconfigenv.IsKubernetes() {
    // configure kubelet endpoint
}
```

### Writing tests

```go
import pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"

func TestMyFeature(t *testing.T) {
    pkgconfigenv.SetFeatures(t, pkgconfigenv.Docker, pkgconfigenv.Kubernetes)
    // t.Cleanup(ClearFeatures) is registered automatically

    result := myFunctionThatChecksFeatures()
    assert.True(t, result)
}
```

### Real-world patterns

- **Autodiscovery** (`cmd/agent/common/autodiscovery.go`) calls `pkgconfigenv.IsFeaturePresent(pkgconfigenv.Kubernetes)` to decide whether to enable the kubelet listener.
- **DogStatsD** (`comp/dogstatsd/listeners/`) uses `IsFeaturePresent(Docker)` to conditionally start the Docker listener.
- **Process agent** (`cmd/process-agent/command/main_common.go`) calls `IsECSFargate()` before the config is fully loaded to gate ECS-specific startup logic.
- **System-probe GPU module** (`cmd/system-probe/modules/gpu.go`) checks `IsFeaturePresent(NVML)` to decide whether to register the GPU module.
- **Misconfig checks** (`cmd/agent/common/misconfig/mounts.go`) combine `env.IsContainerized()` with a config value to build the right `/proc/mounts` path.
- **`pkg/config/setup`** calls `DetectFeatures(config)` at the end of `LoadDatadog` so that by the time any component reads the config, feature flags are already populated.
