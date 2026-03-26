> **TL;DR:** `comp/host-profiler` implements Linux-only eBPF-based continuous host profiling, wrapping an OpenTelemetry Collector pipeline to collect system-wide CPU profiles and upload them along with debug symbols to the Datadog profiling backend.

# comp/host-profiler

## Purpose

The `host-profiler` bundle implements eBPF-based continuous profiling of the Linux host. It wraps an OpenTelemetry Collector instance with custom receivers, processors, exporters, and extensions to collect system-wide CPU profiles and stack traces with minimal overhead, then ship them to the Datadog profiling backend.

This component is **Linux-only** (`//go:build linux` on all non-trivial files).

## Architecture overview

```
Linux Kernel (eBPF probes)
       ↓
hostprofiler receiver   — collects CPU samples, stack traces, process metadata
       ↓
Processors              — resource/infra attribute enrichment
       ↓
OTLP HTTP exporter      — sends profiles to Datadog intake
       ↓
Datadog profiling platform

Symbol uploader (side-channel)
       ↓
Datadog sourcemap intake (/api/v2/srcmap)
```

The bundle exposes two operational modes depending on whether a Datadog Agent core is co-located:

| Mode | Factories used | Notes |
|---|---|---|
| With Agent Core | `ExtraFactoriesWithAgentCore` | Uses Agent tagger for infra attributes, DD profiling extension, Agent flare integration, Go runtime metrics |
| Without Agent Core | `ExtraFactoriesWithoutAgentCore` | Uses K8s attributes processor, standalone config converters, no Agent dependencies |

## Key elements

### Key interfaces

```go
// Build tag: linux
// comp/host-profiler/collector/def
type Component interface {
    Run() error
}
```

`Run()` starts the OTel Collector main loop (blocking call). It is intended to be the entry point of the host-profiler binary.

### Key types

**`ExtraFactories` interface** — defined in `collector/impl/otel_col_factories.go`. Two concrete implementations:

| Mode | Implementation |
|---|---|
| With Agent Core | `ExtraFactoriesWithAgentCore` |
| Without Agent Core | `ExtraFactoriesWithoutAgentCore` |

**`DatadogSymbolUploader`** — top-level orchestrator for the symbol upload pipeline:

| Type | Description |
|---|---|
| `SymbolUploaderConfig` | Configuration: endpoints, dry-run mode, dynamic symbol upload, GoPCLnTab upload, HTTP/2, batch interval |
| `SymbolEndpoint` | `{Site, APIKey}` pair for a single Datadog org |
| `symbol.Elf` | Wrapper around an ELF file with helpers for build IDs, symbol sources, GoPCLnTab extraction |
| `symbol.Source` | Enum: `None`, `DynamicSymbolTable`, `DebugInfo`, `GoPCLnTab` |

### Key functions

**`agentprovider`** — a custom OTel `confmap.Provider` (`scheme: agent`) that translates Agent configuration (API keys, sites, endpoints) into the OTel collector YAML format so the host-profiler can inherit `datadog.yaml` settings.

**`oom` utilities** — `GetOOMScoreAdj` / `SetOOMScoreAdj` on `/proc/<pid>/oom_score_adj`. The collector resets its OOM score to `0` at startup to avoid preferential OOM kill.

### Configuration and build flags

| Setting | Default | Description |
|---|---|---|
| `host_profiler.*` | — | Master config namespace for the host profiler |
| `enabled` | `true` | Symbol uploader enabled |
| `upload_dynamic_symbols` | `false` | Upload `.dynsym` stripped library symbols |
| `upload_go_pcln_tab` | `true` | Upload Go `pclntab` symbols |
| `symbol_query_interval` | `5s` | Interval between backend symbol queries |
| `dry_run` | `false` | Disable actual symbol uploads |

Build tag: `linux` on all non-trivial files.

## Sub-packages

### `collector/impl` — Collector implementation

**`collector.go`** — `collectorImpl` wraps `otelcol.Collector`. On startup it:

1. Enables the `service.profilesSupport` feature gate (disabled upstream by default).
2. Reads the OOM score adjustment and resets it to `0` if positive, so the profiler is not killed under memory pressure before less important processes.
3. Calls `collector.Run(ctx)`.

**`otel_col_factories.go`** — Defines the `ExtraFactories` interface and provides two concrete implementations used to register the correct set of OTel components for each operational mode.

**`agentprovider/`** — A custom OTel `confmap.Provider` (`scheme: agent`) that translates Agent configuration (API keys, sites, endpoints) into the OTel collector YAML format. This is how the host-profiler can inherit endpoint and authentication config from `datadog.yaml` without requiring the user to duplicate it.

**`converters/`** — OTel `confmap.Converter` implementations that normalise user-provided collector configs. Design principle: *user-set leaf values are never overwritten; missing required values are filled in; conflicting values emit a warning but are preserved; config that requires external information (API keys) errors out in standalone mode*.

**`receiver/`** — Factory for the `hostprofiler` receiver, which builds the eBPF-based profile collector. Config includes eBPF settings (e.g. maps sizes, sampling rate) and symbol uploader options.

**`extensions/hpflareextension/`** — An OTel extension that exposes an HTTP endpoint for flare bundle generation when running alongside the Agent.

### `oom` — OOM score utilities

```go
func GetOOMScoreAdj(pid int) (int, error)
func SetOOMScoreAdj(pid, score int) error
```

Thin wrappers around `/proc/<pid>/oom_score_adj`. Pass `pid = 0` for the current process. The collector calls this at startup to ensure it is not preferentially killed by the Linux OOM killer.

### `symboluploader` — Debug symbol upload

`DatadogSymbolUploader` implements a multi-stage pipeline that uploads ELF debug symbols to the Datadog sourcemap intake (`https://sourcemap-intake.<site>/api/v2/srcmap`). This powers native code symbolization in the Datadog UI.

#### Pipeline stages

```
ExecutableMetadata (from eBPF profiler reporter)
       ↓ retrieval workers (10)     — open ELF file, check symbol availability
       ↓ batching stage             — group binaries for batch backend queries
       ↓ query workers (10)         — ask backend which symbols are missing
       ↓ upload workers (10)        — extract with objcopy, compress, POST
```

#### Key types

| Type | Description |
|---|---|
| `DatadogSymbolUploader` | Top-level orchestrator; created by `NewDatadogSymbolUploader` |
| `SymbolUploaderConfig` | Configuration: endpoints, dry-run mode, dynamic symbol upload, GoPCLnTab upload, HTTP/2, batch interval |
| `SymbolEndpoint` | `{Site, APIKey}` pair for a single Datadog org |
| `symbol.Elf` | Wrapper around an ELF file with helpers for build IDs, symbol sources, GoPCLnTab extraction |
| `symbol.Source` | Enum: `None`, `DynamicSymbolTable`, `DebugInfo`, `GoPCLnTab` (higher = richer) |

#### Symbol sources supported

- **ELF `.debug_info`** / **DWARF** — full debug symbols
- **Dynamic symbol table** (`.dynsym`) — available in stripped shared libraries; opt-in via `upload_dynamic_symbols`
- **Go `pclntab`** — Go's native symbol table embedded in every Go binary; always tried for Go binaries (opt-out via `upload_go_pcln_tab: false`)

#### Upload deduplication

An LRU cache (`uploadCacheSize = 16 384` entries) keyed on `libpf.FileID` prevents re-uploading the same binary within a process lifetime.

#### Configuration defaults

| Setting | Default |
|---|---|
| `enabled` | `true` |
| `upload_dynamic_symbols` | `false` |
| `upload_go_pcln_tab` | `true` |
| `symbol_query_interval` | `5s` |
| `dry_run` | `false` |

### `symboluploader/pipeline` — Generic concurrent pipeline

A reusable package providing `Stage`, `BatchingStage`, `SinkStage`, and `BudgetedProcessingFunc` abstractions used by the symbol uploader to run its multi-worker pipeline with back-pressure and memory budget enforcement.

### `symboluploader/cgroup` — cgroup memory limit

`GetMemoryBudget()` reads the cgroup v1/v2 memory limit and returns it as an `int64` byte count. The symbol uploader uses this to avoid allocating more memory for in-flight symbol files than the container allows.

### `flare` — Flare integration

Provides a `hpflare` implementation that serialises OTel Collector internal state (pipeline configs, component status) into the flare bundle when running alongside the Agent.

### `version` — Version constants

Exports `ProfilerName` and `ProfilerVersion` used as `origin` / `origin_version` metadata in symbol upload requests and OTel build info.

## Usage

The bundle is registered in the host-profiler binary's fx app:

```go
// comp/host-profiler/bundle.go
func Bundle(params collectorimpl.Params) fxutil.BundleOptions {
    return fxutil.Bundle(collectorfx.Module(params))
}
```

`Params` carries the OTel config URI (`uri`) and whether to enable Go runtime metrics (`GoRuntimeMetrics`). The collector then resolves the config using the registered providers (Agent provider, env provider, file provider).

The component is **not** included in the main `datadog-agent` binary; it runs as a dedicated `host-profiler` binary.

### Modes of operation

**With Agent core** — wire `ExtraFactoriesWithAgentCore` as the `ExtraFactories`
implementation. This activates:
- The `ddflare` OTel extension (backed by `comp/host-profiler/flare`) so that
  `datadog-agent flare` automatically includes profiler diagnostics.
- The Datadog tagger for infrastructure attribute enrichment.
- Go runtime metrics via the `ddprofiling` extension.

**Without Agent core (standalone)** — wire `ExtraFactoriesWithoutAgentCore`.
The profiler reads API keys and endpoints directly from its own config; the
`agentprovider` confmap provider translates `datadog.yaml`-style keys into OTel
YAML. K8s attribute enrichment comes from the standard `k8sattributes`
processor instead of the Datadog tagger.

### Flare integration

When running alongside the Agent, `comp/host-profiler/flare` contributes to
every flare archive. It serialises the OTel Collector's internal pipeline state
(component configs, extension data) using the same `flaretypes.Provider`
pattern as other agent components. See
[comp/core/flare](../comp/core/flare.md) for the provider registration API.

### Relationship to agent-process profiling

`comp/host-profiler` and `pkg/util/profiling` serve complementary but distinct
purposes:

| | `comp/host-profiler` | `pkg/util/profiling` |
|---|---|---|
| What is profiled | All processes on the Linux host (eBPF, system-wide) | The agent process itself (dd-trace-go profiler) |
| Mechanism | eBPF kernel probes + OTel pipeline | dd-trace-go SDK → Datadog profiling backend |
| Config key | `host_profiler.*` | `internal_profiling.enabled` |
| Platform | Linux only | All platforms |

See [pkg/util/profiling](../../pkg/util/profiling.md) for agent-process profiling.

---

## Related packages and components

| Package / Component | Doc | Relationship |
|---|---|---|
| `pkg/util/profiling` | [../../pkg/util/profiling.md](../../pkg/util/profiling.md) | Manages continuous profiling of the agent process itself using the dd-trace-go profiler. Complementary to `comp/host-profiler`, which profiles the Linux host system-wide via eBPF. |
| `comp/otelcol/collector` | [../otelcol/collector.md](../otelcol/collector.md) | The main-agent OTel Collector component. Both `comp/host-profiler` and `comp/otelcol/collector` embed `otelcol.Collector`, but for different purposes: `host-profiler` uses a fixed eBPF-profile pipeline; `comp/otelcol/collector` accepts a user-supplied OTel config and is used by the OTel Agent. They do not share state. |
| `comp/core/flare` | [../core/flare.md](../core/flare.md) | The flare component. When running with Agent core, `comp/host-profiler/extensions/hpflareextension` registers an OTel extension that hooks into the flare provider group (`group:"flare"`) to include profiler diagnostics in every `datadog-agent flare` bundle. |
