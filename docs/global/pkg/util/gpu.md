# pkg/util/gpu

## Purpose

Provides shared, dependency-free utilities for identifying and naming GPU Kubernetes resources. It intentionally has no NVML or cgo dependency, making it safe to import anywhere in the codebase without pulling in GPU-specific build constraints. The heavier NVML wrappers live in `pkg/gpu/safenvml`.

## Key Elements

| Symbol | Description |
|--------|-------------|
| `ResourceGPU` | `string` type alias representing a Kubernetes extended-resource name for a GPU (e.g. `"nvidia.com/gpu"`). |
| `GpuNvidiaGeneric` | `"nvidia.com/gpu"` — the standard NVIDIA generic GPU resource name. |
| `GpuAMD` | `"amd.com/gpu"` |
| `GpuIntelXe` | `"gpu.intel.com/xe"` |
| `GpuInteli915` | `"gpu.intel.com/i915"` |
| `GpuNvidiaMigPrefix` | `"nvidia.com/mig"` — prefix for NVIDIA Multi-Instance GPU slices (e.g. `"nvidia.com/mig-3g.20gb"`). |
| `ExtractSimpleGPUName(ResourceGPU) (string, bool)` | Maps a full Kubernetes resource name to a short vendor string (`"nvidia"`, `"amd"`, `"intel"`). Returns `("", false)` for unrecognised resources. Handles MIG slice variants via prefix matching. |
| `IsNvidiaKubernetesResource(string) bool` | Returns `true` if the resource name is either `"nvidia.com/gpu"` or starts with `"nvidia.com/mig"`. |

No build tags are required — the package is pure Go and platform-independent.

## Usage

Consumers outside the main `pkg/gpu` package:

- `pkg/gpu/containers/containers.go` (`linux && nvml`) — uses `ExtractSimpleGPUName` to determine GPU vendor when matching devices to containers.
- `comp/core/workloadmeta/collectors/util/kubelet.go` and `comp/core/workloadmeta/collectors/internal/kubeapiserver/pod.go` — use `IsNvidiaKubernetesResource` and `ExtractSimpleGPUName` while parsing pod specs to populate GPU resource metadata in workloadmeta.
- `comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers/pod.go` — same purpose in the Kubernetes API-server collector path.

The package is the canonical source of GPU resource name constants; new GPU resource types should be added here so all collectors stay consistent.
