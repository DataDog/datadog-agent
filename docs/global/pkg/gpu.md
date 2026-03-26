> **TL;DR:** `pkg/gpu` implements GPU monitoring inside system-probe by intercepting CUDA runtime calls via eBPF uprobes and combining them with NVML device data to produce per-process GPU utilization and memory-allocation metrics.

# pkg/gpu

## Purpose

`pkg/gpu` implements GPU monitoring for the Datadog Agent. It intercepts CUDA
runtime calls from running processes via eBPF uprobes and combines that data
with device-level information retrieved through NVIDIA's NVML library to produce
per-process GPU utilization and memory-allocation metrics.

The package runs inside `system-probe` and is exposed as the
`GPUMonitoringModule` system-probe module (`cmd/system-probe/modules/gpu.go`).
The GPU core check (`pkg/collector/corechecks/gpu`) polls the module via
`Probe.GetAndFlush()` at every check interval.

**Build constraints:** The core logic requires `linux_bpf` and `nvml` build
tags. Most sub-packages that do not touch eBPF or NVML are usable under just
`linux` (or all platforms in the `cuda/` parser).

---

## Key elements

### Key types

#### Top-level data flow

```
libcudart / libcuda uprobe events
         │
         ▼
  cudaEventConsumer          (consumer.go)
  reads eBPF ringbuffer "cuda_events"
         │
         ▼
  streamCollection           (stream_collection.go)
  thread-safe map of StreamHandler, keyed by (pid, stream_id)
         │
         ▼
  StreamHandler              (stream.go)
  accumulates kernel launches; on sync → emits kernelSpan / memorySpan
         │
         ▼
  statsGenerator             (stats.go)
  collects spans, forwards to per-process Aggregator
         │
         ▼
  Aggregator                 (aggregator.go)
  merges intervals → process-level ActiveTimePct + memory stats
         │
         ▼
  Probe.GetAndFlush()        (probe.go)
  returns *model.GPUStats to the core check
```

#### Types

| Type | File | Description |
|------|------|-------------|
| `Probe` | `probe.go` | Top-level entry point. Owns the eBPF manager, uprobe attacher, consumer, and stats generator. Created via `NewProbe`. |
| `ProbeDependencies` | `probe.go` | Dependency injection struct: `Telemetry`, `ProcessMonitor`, `WorkloadMeta`. |
| `Config` / `StreamConfig` | `config/config.go` | All tunable parameters (ring-buffer sizes, stream timeouts, cgroup permissions, fatbin parsing flag). Loaded via `config.New()`. |
| `cudaEventConsumer` | `consumer.go` | Goroutine that drains the eBPF ring buffer and routes events to `StreamHandler`. |
| `streamCollection` | `stream_collection.go` | Thread-safe map (`sync.Map`) of `StreamHandler` instances; separate maps for regular and global CUDA streams. |
| `StreamHandler` | `stream.go` | Processes events for one CUDA stream. Accumulates `enrichedKernelLaunch` objects, emits `kernelSpan` and `memorySpan` on synchronisation events. |
| `statsGenerator` | `stats.go` | Called by `GetAndFlush`; iterates all stream handlers, collects past/current data, distributes to per-process aggregators. |
| `Aggregator` | `aggregator.go` | One per process. Merges kernel execution intervals to compute `ActiveTimePct`; tracks memory allocations. |
| `systemContext` | `context.go` | Holds NVML device cache, per-process visible-device cache, per-thread selected-device cache, and the `KernelCache`. |
| `KernelCache` | `cuda/kernel_cache.go` | Background-goroutine cache that resolves a kernel address to a `CubinKernel` by parsing the process's mapped fatbin files. |
| `SafeNVML` | `safenvml/lib.go` | Interface wrapping `github.com/NVIDIA/go-nvml`. Symbol-checks every NVML call at runtime; fails fast only on missing critical symbols. Singleton accessed via `GetSafeNvmlLib()`. |
| `SafeDevice` / `PhysicalDevice` / `MIGDevice` | `safenvml/device*.go` | Represent NVML GPU devices; `MIGDevice` wraps MIG slices. |

### Key interfaces

#### eBPF types (generated, `ebpf/`)

The `ebpf/` sub-package exposes C-struct mirrors generated from
`ebpf/c/types.h`:

| Go type | Description |
|---------|-------------|
| `CudaEventType` | Enum: `KernelLaunch`, `Memory`, `Sync`, `SetDevice`, `VisibleDevicesSet`, `SyncDevice`. |
| `CudaKernelLaunch` | Kernel launch parameters (grid dims, shared memory, stream pointer, kernel address). |
| `CudaMemEvent` | Memory allocation/free event (address, size, type). |
| `CudaSync` | Stream-synchronise event. |
| `CudaEventKey` / `CudaEventValue` | Keys/values for the `cuda_event_to_stream` BPF map. |

### Key functions

#### CUDA binary parsing (`cuda/`)

| Symbol | Description |
|--------|-------------|
| `GetSymbols(path, smVersionSet)` | Parses a process binary for fatbin sections; returns a `*Symbols` containing `Fatbin` and `SymbolTable` (offset → symbol name). |
| `Fatbin` | Parsed fatbin container; holds one or more `CubinPayload` objects. |
| `CubinKernel` | Per-kernel metadata extracted from a cubin: name, `KernelSize`, shared memory, register counts. |
| `KernelCache` | Asynchronous cache: `Get(pid, addr, smVersion)` returns a `*CubinKernel` immediately if cached, or `ErrKernelNotProcessedYet` while a background goroutine resolves it. |

#### Container / device mapping (`containers/`)

`MatchContainerDevices(container, devices)` maps a workloadmeta `Container` to
NVML `Device` objects using:
- Docker: reads `NVIDIA_VISIBLE_DEVICES` from the container's PID environment.
- Kubernetes: uses `ResolvedAllocatedResources` (NVIDIA device-plugin UUID or
  GKE `nvidiaX` index).
- ECS: uses `GPUDeviceIDs` (UUID format) stored on the container.

`HasGPUs(container)` is a lightweight pre-check before calling the full matcher.

#### Tag injection (`tags/`)

`GetTags()` returns host-level tags indicating GPU presence (e.g.,
`gpu:true`). The no-op stub (`tags_noop.go`) is used on non-Linux builds.

### Configuration and build flags

The core logic requires `linux_bpf` and `nvml` build tags. Sub-packages without eBPF or NVML dependencies compile under `linux` or all platforms. Configuration is loaded via `config.New()` from the `gpu_monitoring` section of `system-probe.yaml`.

#### Memory pools

Three object pools (`memoryPools` in `stream.go`) recycle `enrichedKernelLaunch`,
`kernelSpan`, and `memorySpan` to reduce GC pressure. Every `pool.Get()` must
be paired with a `pool.Put()`. The pools are initialised once via
`memPools.ensureInit(telemetry)` in `NewProbe`.

---

## Usage

### Enabling GPU monitoring

In `system-probe.yaml`:
```yaml
gpu_monitoring:
  enabled: true
  enable_fatbin_parsing: true   # resolves kernel names from binaries
  configure_cgroup_perms: true  # needed inside containers
```

The eBPF program requires CO-RE (preferred) or runtime compilation. Both must
be disabled to get a startup error rather than a silent no-op.

The CO-RE asset is loaded via `ebpf.LoadCOREAsset("gpu.o", ...)` following the
standard fallback chain (CO-RE → runtime compilation → prebuilt). The BTF source
selection and status code are surfaced on the agent status page via
`ebpf.GetCORETelemetry()`. See [`ebpf.md`](ebpf.md) for the `COREResult` status
codes and how to inspect which BTF source was used.

### Module registration

`cmd/system-probe/modules/gpu.go` registers `GPUMonitoring` via `init()`.  The
factory constructs a `Probe` using `gpu.NewProbe(cfg, deps)` and wires the HTTP
handler for `/gpu/check`.

### Core check integration

The GPU core check (`pkg/collector/corechecks/gpu`, `CheckName = "gpu"`) calls
`Probe.GetAndFlush()` each tick via the system-probe HTTP endpoint `/gpu/check`,
receives `*model.GPUStats`, and converts `ProcessMetrics` into Datadog metrics.
Metric names and expected tags are governed by the YAML spec under
`pkg/collector/corechecks/gpu/spec/`.

The check is not yet componentized with FX (similar to the service discovery check).
It relies on the global `system-probe` module handle rather than fx-injected
dependencies.

### Uprobe attachment and shared-library detection

`pkg/gpu/probe.go` creates a `UprobeAttacher` with rules targeting `libcudart.so`
and `libcuda.so`. The attacher is registered alongside the USM TLS probes in the
shared-libraries infrastructure:

1. `pkg/network/usm/sharedlibraries.GetEBPFProgram` hooks `do_sys_open_helper_exit`
   to detect when any process opens a library from the `LibsetGPU` set
   (containing `libcudart` and `libcuda` suffixes).
2. The `LibraryCallback` fires, and `UprobeAttacher` inspects the ELF to locate
   CUDA function offsets, then attaches the GPU eBPF probes via `ebpf-manager`.
3. On process exit, the attacher cleans up per-PID uprobe state automatically.

Any new CUDA library hook must be registered in:
- `pkg/network/usm/sharedlibraries.LibsetToLibSuffixes` under `LibsetGPU`.
- `pkg/network/ebpf/c/shared-libraries/probes.h:do_sys_open_helper_exit` (kernel side).
- The `AttachRule.LibraryNameRegex` in `pkg/gpu/probe.go`.

See [`uprobes.md`](ebpf/uprobes.md) for the `AttachRule` pattern and probe naming
conventions, and [`usm.md`](network/usm.md) for the `sharedlibraries` sub-package API.

### Debug endpoint

`Probe.GetDebugStats()` returns a snapshot of connected GPUs, kernel cache
health, attacher active/blocked processes, and consumer health status. This is
exposed at `/gpu/debug/stats` on the system-probe Unix socket.

### Memory pool usage

Three `sync.Pool` instances (`memPools` in `stream.go`) recycle
`enrichedKernelLaunch`, `kernelSpan`, and `memorySpan` objects to reduce GC
pressure on the hot event-processing path. The pools are initialised once via
`memPools.ensureInit(telemetry)` inside `NewProbe`. Every `pool.Get()` call must
be paired with a `pool.Put()` to avoid leaks; missing `Put` calls will be caught
by tests but silently degrade performance in production.

### Discovery integration

`ServicesResponse.GPUPIDs` in `pkg/discovery/model` lists PIDs that have GPU
access, as detected by the discovery module's Rust core. These PIDs can be
correlated with the process-level GPU stats produced by `Probe.GetAndFlush()` to
associate GPU usage with service names discovered by `pkg/discovery`.

---

## Related packages

- [`pkg/ebpf`](ebpf.md) — shared eBPF infrastructure. `pkg/gpu` uses
  `ebpf.LoadCOREAsset` (with the CO-RE / runtime-compilation / prebuilt fallback chain),
  `ebpf.NewManagerWithDefault`, and `telemetry.ErrorsTelemetryModifier` to load and manage
  the CUDA uprobe eBPF program. The `PrintkPatcherModifier` is added by default via
  `NewManagerWithDefault`. The eBPF manager lifecycle (`InitWithOptions`, `Start`, `Stop`)
  and the `Modifier` hook points (`BeforeInit`, `AfterInit`, `BeforeStop`) are described in
  [`ebpf.md`](ebpf.md). For the BPF ringbuffer event loop used by `cudaEventConsumer`, see
  the `PerfHandler` and ring-buffer documentation in the same file.
- [`pkg/ebpf/uprobes`](ebpf/uprobes.md) — `Probe` creates a `UprobeAttacher` (visible in
  `probe.go`) to attach to `libcudart` and `libcuda` shared libraries. The attacher is
  registered as an existing caller in `uprobes.md` (`pkg/gpu/probe.go`). For adding new CUDA
  library hooks, follow the `AttachRule` pattern documented there and ensure the library regex
  is added to `do_sys_open_helper_exit` in `pkg/network/ebpf/c/shared-libraries/probes.h`.
  The `ExcludeInternal | ExcludeSelf` mode is recommended for the GPU attacher so that the
  agent does not attach GPU probes to its own processes or other Datadog agent binaries.
- [`pkg/util/gpu`](util/gpu.md) — provides the lightweight, cgo-free constants and helpers for
  GPU Kubernetes resource names (`GpuNvidiaGeneric`, `GpuNvidiaMigPrefix`,
  `ExtractSimpleGPUName`, `IsNvidiaKubernetesResource`). `pkg/gpu/containers/containers.go`
  uses `ExtractSimpleGPUName` to match container devices by vendor when processing Docker
  (`NVIDIA_VISIBLE_DEVICES` env var), Kubernetes (device-plugin UUID or GKE index), and ECS
  (`GPUDeviceIDs`) containers. These constants are also used by workloadmeta kubelet and
  kubeapiserver collectors. New GPU resource types (e.g. a new MIG profile) should be added
  to `pkg/util/gpu` so all collectors stay consistent.
- [`comp/core/workloadmeta`](../comp/core/workloadmeta.md) — GPU devices are stored in
  workloadmeta as `KindGPU` entities (populated by the `SourceNVML` collector in
  `comp/core/workloadmeta/collectors/internal/nvml`). `Probe` receives a `WorkloadMeta`
  dependency via `ProbeDependencies` and uses it in `containers/containers.go` to map
  containers to their visible GPU devices (via `MatchContainerDevices`). The `HasGPUs(container)`
  helper performs a lightweight pre-check before calling the full matcher, avoiding unnecessary
  NVML queries for non-GPU containers. Subscribe to `KindGPU` events to react to GPU device
  attach/detach dynamically.
