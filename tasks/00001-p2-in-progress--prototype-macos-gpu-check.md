# Prototype macOS Apple Silicon support in the standard GPU check

## Goal

Make the existing `gpu` core check available on macOS Apple Silicon without changing its user-facing check name or disturbing the Linux/NVML implementation. The prototype should prove that an unprivileged Agent can collect useful device-level GPU telemetry on this local MBP and that the signal responds to an Ollama workload.

## Grounding and chosen approach

Today the check is compiled only with `linux && nvml`; every other target receives an empty factory. The Linux implementation also embeds NVML-specific device discovery, collectors, health reporting, workload/container attribution, and system-probe integration, so replacing NVML with a single interface is not a small change.

On the local Apple M5 Pro MBP, the unprivileged IOKit registry exposes an `AGXAccelerator` with:

- model (`Apple M5 Pro`)
- GPU core count (`20`)
- `PerformanceStatistics`, including `Device Utilization %`, renderer/tiler utilization, and memory counters

`powermetrics` exposes additional GPU power/frequency data but requires superuser access. IOReport and Apple GPU private frameworks have no public SDK contract. For this prototype, use the public IOKit/CoreFoundation APIs to read the AGX registry properties, while treating the property names themselves as best-effort, undocumented driver data.

Implement an independent Darwin check behind the same package, factory, configuration flag, and `gpu.*` metric namespace. Do not first refactor the mature Linux/NVML check into a large cross-platform framework.

## Implementation plan

1. **Add a Darwin-native AGX reader**
   - Add `darwin && cgo` files under `pkg/collector/corechecks/gpu` following the repository's existing battery/system-info pattern (`*_darwin.go` plus Objective-C/C helper files when useful).
   - Link only public `IOKit` and `CoreFoundation`/`Foundation` frameworks.
   - Enumerate `AGXAccelerator` services and read model, core count, and the `PerformanceStatistics` dictionary directly; do not invoke `ioreg`, `system_profiler`, or `powermetrics` as subprocesses.
   - Represent absent or differently typed properties as optional values. One missing property must not suppress the remaining metrics or panic the check.
   - Bound enumeration and release all CoreFoundation/IOKit objects correctly.

2. **Register the standard check on supported Darwin builds**
   - Adjust the unsupported factory build constraints so Darwin+cgo receives the real factory while Darwin without cgo and all other unsupported targets retain the no-op factory.
   - Add a small Darwin `Check` implementation using `core.CheckBase`, the existing `gpu.enabled` gate, collection interval override, sender lifecycle, and `Cancel` behavior.
   - Keep `pkg/collector/corechecks/gpu/gpu.go` and all NVIDIA collectors Linux/NVML-only and behaviorally unchanged.

3. **Emit a conservative canonical metric set**
   - Map AGX `Device Utilization %` to `gpu.gr_engine_active` (gauge, percent), the closest existing device-wide graphics-engine activity metric.
   - Emit `gpu.core.limit` from `gpu-core-count` and `gpu.device.total` for each discovered device.
   - Do not publish renderer/tiler counters or unified-memory values under misleading existing names, and do not add new public metrics in this prototype.
   - Attach deterministic, low-cardinality device tags conforming to the existing GPU tag specification: a synthetic host-scoped `gpu_uuid`, normalized `gpu_device`, `gpu_vendor:apple`, `gpu_architecture:apple_silicon`, an Apple model `gpu_type`, `gpu_virtualization_mode:none`, `gpu_slicing_mode:none`, and `gpu_host:true`.
   - Document how the synthetic ID is derived and keep it stable for the same model/device index across restarts. Avoid serial numbers or other sensitive identifiers.

4. **Graceful degradation and telemetry**
   - If no AGX accelerator exists (for example, an Intel Mac), return no device metrics with a rate-limited diagnostic rather than failing Agent startup.
   - If utilization is absent on a future macOS release, still emit inventory-derived metrics and record/log the unsupported property once at an appropriate level.
   - Add only the minimal internal telemetry needed to distinguish discovered devices, successful collections, and native-reader errors; do not report NVML health issues on Darwin.

5. **Tests**
   - Unit-test native-value conversion and metric mapping using injected snapshots so tests do not depend on current machine load or sleep.
   - Test partial/missing/malformed properties, zero devices, multiple snapshots/devices, stable tags/IDs, sender commit, and `gpu.enabled` gating.
   - Add a Darwin smoke test that calls the real IOKit reader and asserts coarse invariants only when an AGX device is present; skip cleanly on unsupported hardware.
   - Preserve Linux tests and ensure build-tag changes do not make NVML dependencies reachable on Darwin or the Darwin implementation reachable on Linux.

6. **Build and configuration integration**
   - Update Bazel/Gazelle metadata for cgo, Objective-C sources, public Apple framework link options, and Darwin-only dependencies.
   - Update `pkg/config/schema/yaml/gpu.yaml` wording so `gpu.enabled` describes both Linux GPU monitoring and the experimental Apple Silicon device-level path; retain the Linux-only system-probe requirement where applicable.
   - Add a release note marking macOS support as experimental and device-level only.

7. **Local validation on this MBP**
   - Run targeted tests with `dda inv test --targets=./pkg/collector/corechecks/gpu/...` and build the Agent with `dda inv agent.build` (never raw `go` commands).
   - Enable `gpu.enabled`, run the standard `gpu` check, and verify the canonical metrics and Apple device tags through the Agent sender/check output.
   - Use the installed Ollama (`0.32.5`) and an already-local model to create sustained Metal load. Capture several idle and loaded check samples and confirm `gpu.gr_engine_active` rises materially during generation and falls afterward. Do not pull a model from the network without asking first.
   - Compare the check's utilization qualitatively with repeated unprivileged `ioreg` observations. Use `powermetrics` only as an optional manually authorized reference, never as the implementation dependency.

## Acceptance criteria

- A Darwin arm64 Agent with cgo registers the existing `gpu` check when `gpu.enabled: true`.
- On this Apple M5 Pro, the check emits `gpu.gr_engine_active`, `gpu.core.limit`, and `gpu.device.total` with valid Apple GPU device tags.
- The check runs as the normal unprivileged Agent and does not spawn system tools or link private Apple frameworks.
- Missing AGX devices or properties degrade safely without preventing Agent startup or suppressing other available metrics.
- Linux/NVML behavior and tests remain unchanged.
- Unit tests cover the mapping/error cases without timing sleeps, and the Darwin native smoke test is hardware-tolerant.
- Local before/during/after Ollama evidence demonstrates that the utilization metric tracks GPU load.

## Deliberate non-goals

- GPU power, frequency, temperature, or energy metrics from root-only `powermetrics`.
- Direct use of undocumented IOReport functions or private AGX/GPU frameworks.
- Per-process or per-container GPU attribution, workloadmeta GPU inventory, eGPU/Intel/AMD Mac support, or system-probe integration.
- Refactoring all Linux/NVML collectors behind a universal backend abstraction.
- Shipping new renderer/tiler/unified-memory metric names before their semantics and backend contract are reviewed.
