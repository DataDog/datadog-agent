> **TL;DR:** `pkg/ebpf/telemetry` exposes runtime observability for eBPF programs in the agent, surfacing map operation errors, BPF helper call errors, and perf/ring-buffer usage as Prometheus metrics via a bytecode-patching mechanism that adds zero overhead when disabled.

# pkg/ebpf/telemetry

## Purpose

Exposes runtime observability for eBPF programs running in the agent. It covers three distinct signal types:

1. **Map operation errors** — errno counts for failed `bpf_map_*` calls, tracked per map and module.
2. **Helper errors** — errno counts for failed BPF helper calls (`bpf_probe_read`, `bpf_perf_event_output`, etc.), tracked per probe and module.
3. **Perf/ring-buffer usage** — byte usage, percentage full, lost-sample counts, and Go channel lengths for `PerfEventArray` and `RingBuf` maps.

All metrics are exposed as Prometheus counters/gauges and collected by the system-probe's telemetry pipeline.

## Key elements

### Configuration and build flags

| Build tag | Files affected |
|-----------|---------------|
| `linux_bpf` | All core logic: `errors_telemetry.go`, `modifier.go`, `errors_collector_linux.go`, `debugfs.go`, `types_linux.go`. |
| `linux` | `perf_metrics.go` (no BPF dependency, uses `prometheus` only). |
| Non-Linux stubs | `*_nonlinux.go` / `*_noop.go` provide empty implementations so the package compiles on macOS/Windows. |

### Key interfaces

| Type | Description |
|------|-------------|
| `ErrorsTelemetryModifier` | `ebpf-manager` modifier (implements `ModifierBeforeInit`, `ModifierAfterInit`, `ModifierBeforeStop`). Automatically wires telemetry into any manager it is attached to—no per-module boilerplate needed. |

### Key types

| Type | Description |
|------|-------------|
| `EBPFErrorsCollector` | Prometheus `Collector`. Reads map-error and helper-error telemetry maps and emits them as counters with labels `{map_name, error, module}` and `{helper, probe_name, error, module}`. Created by `NewEBPFErrorsCollector()`. |
| `ebpfTelemetry` (unexported) | Singleton holding maps keyed by `(resourceName, moduleName)`. Shared across all modules via the package-level `errorsTelemetry` variable. |
| `KprobeStats` | `{Hits uint64, Misses uint64}` read from `tracefs/kprobe_profile`. |

**Constants / map names:**

| Constant | Value | Description |
|----------|-------|-------------|
| `MapErrTelemetryMapName` | `"map_err_telemetry_map"` | eBPF hash map storing per-map error counts. One entry per map per module. |
| `HelperErrTelemetryMapName` | `"helper_err_telemetry_map"` | eBPF hash map storing per-probe helper error counts. |

### Key functions

| Function | Description |
|----------|-------------|
| `NewEBPFErrorsCollector() prometheus.Collector` | Initializes the singleton `errorsTelemetry` and returns the collector. Must be called before any manager using `ErrorsTelemetryModifier` is started. Returns `nil` if the kernel is older than 4.14. |
| `EBPFTelemetrySupported() (bool, error)` | Returns true on kernel >= 4.14. Cached result. |
| `PatchEBPFTelemetry(programSpecs, enable, module)` | Patches eBPF bytecode: replaces stub `call -1` instructions with `stxadd` (atomic add) when enabled, or `mov r1, r1` (noop) when not. Also patches the `telemetry_program_id_key` constant so the program knows its own hash-key. |
| `PatchConstant(symbol, programSpec, key)` | Replaces a `lddw` immediate value in the bytecode at the reference site of `symbol`. Used to inject map/helper keys at load time. |
| `GetProbeStats() map[string]uint64` | Reads `tracefs/kprobe_profile` and returns `{name}_hits` and `{name}_misses` for probes owned by the agent's PID. |
| `GetProbeTotals() KprobeStats` | Aggregates all kprobe hits/misses across the agent. |
| `NewPerfUsageCollector() prometheus.Collector` | Creates a Prometheus collector for perf-buffer and ring-buffer buffer-usage metrics. |
| `ReportPerfMapTelemetry(pm *manager.PerfMap)` | Registers a perf map for collection. Must be called after the manager is started. |
| `ReportRingBufferTelemetry(rb *manager.RingBuffer)` | Same for ring buffers. |
| `UnregisterTelemetry(m *manager.Manager)` | Removes a manager's maps from the perf collector on shutdown. |

### Bytecode patching mechanism

The C eBPF programs contain stub calls (`call -1`) around helper invocations that may fail. `PatchEBPFTelemetry` rewrites these stubs at load time:

- When telemetry is **enabled**: the stub becomes `stxadd mem[key], r0` — an atomic increment into the helper-error map keyed by the patched `telemetry_program_id_key` constant.
- When telemetry is **disabled**: the stub becomes `mov r1, r1` (a no-op), adding no overhead.

### Linux-specific requirements

- Requires kernel >= 4.14 for telemetry support (atomic operations in BPF).
- Requires `tracefs` mounted (typically at `/sys/kernel/tracing` or `/sys/kernel/debug/tracing`) for `GetProbeStats`/`GetProbeTotals`.
- The `perf_metrics.go` file reads from the `system_probe_config.telemetry_perf_buffer_emit_per_cpu` configuration key.

## Usage

**Wiring telemetry into a new module:**

```go
// 1. At process startup (once):
telemetry.NewEBPFErrorsCollector()  // registers the Prometheus collector

// 2. When constructing the manager:
mgr := &manager.Manager{...}
mgr.Modifiers = append(mgr.Modifiers, &telemetry.ErrorsTelemetryModifier{})

// 3. After manager.Init(), register perf/ring-buffer maps:
telemetry.ReportPerfMapTelemetry(myPerfMap)
telemetry.ReportRingBufferTelemetry(myRingBuffer)

// 4. On shutdown:
telemetry.UnregisterTelemetry(mgr)
```

**Reading kprobe stats:**

```go
stats := telemetry.GetProbeStats()
// stats["p_tcp_connect_hits"] = 1234
// stats["p_tcp_connect_misses"] = 0
```

Example callers: `cmd/system-probe/subcommands/run/command.go`, `pkg/network/tracer/connection/ebpf_tracer.go`, `pkg/network/tracer/tracer.go`, `pkg/security/probe/probe_ebpf.go`, `pkg/gpu/probe.go`.
