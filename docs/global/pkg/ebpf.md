> **TL;DR:** `pkg/ebpf` is the shared eBPF infrastructure for the Datadog Agent, providing unified program loading (CO-RE, runtime compilation, prebuilt), a `Manager` with lifecycle modifiers, type-safe map helpers, uprobe attachment, and telemetry for every eBPF-based feature.

# pkg/ebpf

## Purpose

`pkg/ebpf` is the shared eBPF infrastructure used by every eBPF-based feature in the agent
(network monitoring, security, system probes, USM, etc.). It sits between the raw
[cilium/ebpf](https://github.com/cilium/ebpf) and
[DataDog/ebpf-manager](https://github.com/DataDog/ebpf-manager) libraries and the individual
feature packages, providing:

- A unified `Config` type read from `system_probe_config.*` settings.
- Three eBPF program loading strategies (CO-RE, runtime compilation, prebuilt), with automatic
  fallback and telemetry for each.
- A `Manager` wrapper that drives lifecycle hooks (*modifiers*) around program init/start/stop.
- Generic, telemetry-aware helpers for maps, perf buffers, map cleaning, uprobes, and lock
  contention.

The package is **Linux-only**. Almost every file carries the `//go:build linux_bpf` build tag.
On other platforms the package exports only `ErrNotImplemented` and the platform-agnostic
`Config` constructor.

---

## Key elements

### Key types

#### `Config` (`config.go`)

Central configuration struct for all eBPF features. Constructed with `NewConfig()`, which
reads keys from the `system_probe_config` namespace:

| Field | Config key | Description |
|---|---|---|
| `BPFDir` | `bpf_dir` | Directory containing compiled `.o` object files |
| `EnableCORE` | `enable_co_re` | Use CO-RE (Compile Once, Run Everywhere) |
| `BTFPath` | `btf_path` | Path to a user-supplied BTF file |
| `EnableRuntimeCompiler` | `enable_runtime_compiler` | Compile eBPF programs on-host |
| `EnableKernelHeaderDownload` | `enable_kernel_header_download` | Auto-download kernel headers |
| `AllowPrebuiltFallback` | `allow_prebuilt_fallback` | Fall back to prebuilt objects if RC fails |
| `AllowRuntimeCompiledFallback` | `allow_runtime_compiled_fallback` | Fall back to RC if CO-RE fails |
| `EnableTracepoints` | `enable_tracepoints` | Prefer tracepoints over kprobes when available |
| `BPFDebug` | `bpf_debug` | Enable BPF debug logs |
| `RemoteConfigBTFEnabled` | `remote_config_btf_enabled` | Fetch BTF via Remote Configuration |

`Config.ChooseSyscallProbe(tracepoint, indirectProbe, fallback)` picks the best hook
attachment mechanism (tracepoint > arch-specific kprobe > plain kprobe) at runtime.

### Key functions

#### Program loading: CO-RE (`co_re.go`, `ebpf.go`, `btf.go`)

CO-RE is the preferred strategy. `Setup(cfg, rcclient)` initialises the singleton loader.
`LoadCOREAsset(filename, startFn)` resolves BTF for the running kernel and calls `startFn`
with the pre-filled `manager.Options`.

BTF is resolved by `orderedBTFLoader`, which tries four sources in order:

1. User-supplied file (`BTFPath`)
2. Kernel's built-in `/sys/kernel/btf/vmlinux` (modern kernels)
3. Embedded minimized BTF tarball shipped with the agent (`minimized-btfs.tar.xz`)
4. Remote Configuration download

Result codes are defined in `BTFResult` / `COREResult` (`status_codes.go`):
`SuccessCustomBTF`, `SuccessDefaultBTF`, `SuccessEmbeddedBTF`, `SuccessRemoteConfigBTF`,
`BtfNotFound`, `AssetReadError`, `VerifierError`, `LoaderError`.

`GetKernelSpec()` returns a cached `*btf.Spec` for the running kernel. `FlushBTF()` clears
the cache. `GetBTFLoaderInfo()` returns a human-readable string explaining which BTF source
was used.

#### Program loading: Runtime compilation (`bytecode/runtime/`)

`runtime.Asset` represents a `.c` source file embedded in the binary. Calling
`asset.Compile(cfg, flags)` or `asset.CompileWithOptions(cfg, opts)` invokes `clang` on-host
and writes the output `.o` to `RuntimeCompilerOutputDir`. Integrity is verified via SHA-256.

`CompileOptions` allows injecting extra compiler flags and a `ModifyCallback` that
pre-processes the source before compilation.

#### Program loading: Prebuilt (`prebuilt/`)

Pre-compiled `.o` files shipped with the agent, read via `bytecode.GetReader`.

#### Program loading: Asset reader (`bytecode/`)

`bytecode.AssetReader` is an `io.Reader + io.ReaderAt + io.Closer` interface satisfied by
all three loading strategies. `bytecode.GetReader(dir, filename)` opens the object file from
the given directory. On platforms where bindata is embedded, platform-specific
`asset_reader_bindata_amd64.go` / `asset_reader_bindata_arm64.go` handle in-memory reads.

### Key interfaces

#### `Manager` and `Modifier` (`manager.go`)

`Manager` wraps `ebpf-manager.Manager` and adds a named list of `Modifier` instances:

```go
type Manager struct {
    *manager.Manager
    Name             names.ModuleName
    EnabledModifiers []Modifier
}
```

Constructors:
- `NewManager(mgr, name, modifiers...)` — explicit modifier list.
- `NewManagerWithDefault(mgr, name, modifiers...)` — adds `PrintkPatcherModifier` by default.

`Modifier` is an interface composed of optional lifecycle sub-interfaces:

| Sub-interface | Lifecycle point |
|---|---|
| `ModifierBeforeInit` | Before `InitWithOptions` |
| `ModifierAfterInit` | After `InitWithOptions` |
| `ModifierPreStart` | Before `Start` |
| `ModifierBeforeStop` | Before `Stop` |
| `ModifierAfterStop` | After `Stop` |

`Manager.InitWithOptions(bytecode, opts)` loads the ELF, runs `BeforeInit` modifiers, calls
the underlying `InitWithOptions`, then runs `AfterInit` modifiers. Errors from `BeforeInit`
abort the init; errors from stop-phase modifiers are logged but do not prevent stopping.

Notable built-in modifiers:
- `PrintkPatcherModifier` — patches `bpf_trace_printk` newline behaviour across kernel
  versions (default for all managers).
- `telemetry.ErrorsTelemetryModifier` — hooks into `BeforeInit`, `AfterInit`, and
  `BeforeStop` to instrument map and helper call errors at the eBPF level.

#### Map utilities (`maps/`, `map_cleaner.go`)

**`maps.GenericMap[K, V]`** — type-safe wrapper around `*ebpf.Map` with batch-iteration
support (kernel ≥ 5.6). `maps.Map[K, V](emap)` converts a raw `*ebpf.Map`.
`maps.BatchAPISupported()` feature-detects batch support at runtime.

**`MapCleaner[K, V]`** — periodically sweeps an eBPF map and deletes entries matching a
predicate:

```go
mc, _ := ebpf.NewMapCleaner[Key, Val](emap, batchSize, "map_name", "module")
mc.Start(30*time.Second, preClean, postClean, func(nowTS int64, k Key, v Val) bool {
    return nowTS - v.Timestamp > int64(timeout)
})
// ...
mc.Stop()
```

`nowTS` in the predicate is in nanoseconds and is directly comparable to timestamps produced
by `bpf_ktime_get_ns()`. Uses batch API automatically when available.

#### Perf / ring buffer event handling (`perf.go`, `perf_ring_buffer.go`)

**`PerfHandler`** implements `EventHandler` for perf buffers:

```go
ph := ebpf.NewPerfHandler(channelSize)
// wire ph.RecordHandler and ph.LostHandler into manager.PerfMapOptions
for event := range ph.DataChannel() { ... }
for lost := range ph.LostChannel() { ... }
ph.Stop()
```

Records are pooled to reduce allocations. A ring-buffer variant lives in
`perf_ring_buffer.go`.

#### Uprobes (`uprobes/`)

`UprobeAttacher` attaches uprobes to user-space processes and shared libraries dynamically.
It monitors new processes and `open` syscalls for new library loads.

Key types:

| Type | Description |
|---|---|
| `AttacherConfig` | Configuration: rules, exclusion mode, process monitor |
| `AttachRule` | Maps a `LibraryNameRegex` and `AttachTarget` to a list of probe selectors |
| `AttachTarget` | `AttachToExecutable` or `AttachToSharedLibraries` (bit flags) |
| `ExcludeMode` | `ExcludeSelf`, `ExcludeInternal`, `ExcludeBuildkit`, `ExcludeContainerdTmp` |
| `ProbeOptions` | Per-probe overrides: `IsManualReturn`, `Symbol` |

```go
ua, err := uprobes.NewUprobeAttacher("ssl", "usm", cfg, mgr, callback, deps)
ua.Start()
// ...
ua.Stop()
```

#### Telemetry (`telemetry/`)

Instruments eBPF internals at two levels:

**Error telemetry** (`errors_telemetry.go`, `modifier.go`) — `ErrorsTelemetryModifier` (a
`Modifier`) reads two eBPF maps (`map_err_telemetry_map`, `helper_err_telemetry_map`) that
the BPF programs populate via the `bpf_probe_read` / `bpf_perf_event_output` / etc. helpers.
Errors are surfaced as Prometheus counters with `module` and `map`/`helper` labels.

**Debugfs / kprobe stats** (`debugfs.go`) — `GetProbeStats()` reads
`tracefs/kprobe_profile` and returns hit/miss counts per kprobe name.

**CO-RE telemetry** (`co_re_telemetry.go`) — `StoreCORETelemetryForAsset` / `GetCORETelemetry`
maintain a per-asset `COREResult` accessible from the agent status page.

#### Feature detection (`features/`)

`features.SupportsFentry(funcName)` probes whether the running kernel supports fentry/fexit
programs for a specific function and checks known kernel bugs (e.g.
`HasTasksRCUExitLockSymbol`) that make fentry unsafe.

#### Lock contention (`lockcontention.go`)

On kernels where lock contention eBPF programs are supported, `LockContentionCollector`
reads `LockRange` / `ContentionData` structs from a BPF map and exposes them as agent
telemetry. A no-op implementation (`lockcontention_noop.go`) is used when the feature is
unavailable.

#### Kernel symbol helpers (`ksyms.go`, `ksyms_bpf.go`)

`SymbolTable` loads `/proc/kallsyms` and resolves kernel symbol addresses. Used by probes
that need to know a symbol's address before attaching.

#### Time (`time.go`)

`NowNanoseconds()` returns the current monotonic time in nanoseconds using the same clock as
`bpf_ktime_get_ns()`, allowing direct comparison with BPF timestamps in map values.

---

### Configuration and build flags

| Tag | Effect |
|---|---|
| `linux_bpf` | Enables all eBPF functionality (required for the whole package) |
| `linux_bpf` + `!no_co_re` | CO-RE loader is active |

Most files outside `config.go` and `common.go` are gated on `linux_bpf`. Non-Linux builds
receive `ErrNotImplemented` from any function that would need BPF.

---

## Usage

### Starting the subsystem (system-probe)

```go
// In cmd/system-probe, once config is available:
cfg := ebpf.NewConfig()
if err := ebpf.Setup(cfg, rcClient); err != nil {
    return fmt.Errorf("ebpf setup: %w", err)
}
defer ebpf.Reset()
```

### Loading a CO-RE program

```go
err := ebpf.LoadCOREAsset("my_feature.o", func(ar bytecode.AssetReader, opts manager.Options) error {
    opts.MapSpecEditors = map[string]manager.MapSpecEditor{ ... }
    return mgr.InitWithOptions(ar, &opts)
})
```

### Creating a manager with telemetry

```go
rawMgr := &manager.Manager{Probes: probes, Maps: maps}
mgr := ebpf.NewManagerWithDefault(rawMgr, "my_module",
    &telemetry.ErrorsTelemetryModifier{},
)
```

### Periodically cleaning a map

```go
cleaner, err := ebpf.NewMapCleaner[ConnKey, ConnStats](connMap, 256, "conn_stats", "tracer")
cleaner.Start(30*time.Second, nil, nil, func(nowTS int64, k ConnKey, v ConnStats) bool {
    return nowTS-v.LastUpdateEpoch > int64(idleTimeout)
})
```

### Attaching uprobes to OpenSSL

```go
cfg := &uprobes.AttacherConfig{
    Rules: []*uprobes.AttachRule{{
        LibraryNameRegex: regexp.MustCompile(`libssl\.so`),
        Targets:          uprobes.AttachToSharedLibraries,
        ProbesSelector:   []manager.ProbesSelector{...},
    }},
    ExcludeTargets: uprobes.ExcludeInternal | uprobes.ExcludeSelf,
    EbpfConfig:     ebpfCfg,
}
ua, err := uprobes.NewUprobeAttacher("ssl", "usm", cfg, mgr, onAttach, deps)
ua.Start()
```

See [uprobes.md](ebpf/uprobes.md) for the full `UprobeAttacher` API and probe naming conventions.

### Program-loading fallback chain

The three loading strategies form an ordered fallback chain driven by `ebpf.Config`:

```
CO-RE (LoadCOREAsset)
  └─ fails / disabled → Runtime compilation (bytecode/runtime.Asset.Compile)
                          └─ fails / disabled → Prebuilt (bytecode.GetReader from disk/bindata)
```

`COREResult` / `BTFResult` status codes (see `status_codes.go`) record which path succeeded
and are surfaced on the agent status page via `GetCORETelemetry()`. For details on asset
delivery and the `AssetReader` interface, see [bytecode.md](ebpf/bytecode.md). For verifier
complexity analysis of the loaded programs, see [verifier.md](ebpf/verifier.md).

### Enabling error telemetry for a new module

```go
// At process startup (once per agent process):
telemetry.NewEBPFErrorsCollector()

// When constructing the manager:
mgr := ebpf.NewManagerWithDefault(rawMgr, "my_module",
    &telemetry.ErrorsTelemetryModifier{},
)

// After mgr.InitWithOptions, register ring-buffer or perf maps:
telemetry.ReportRingBufferTelemetry(myRingBuffer)
```

See [telemetry.md](ebpf/telemetry.md) for the full telemetry API including kprobe stats and perf-buffer usage metrics.

---

## Sub-package documentation

Each sub-package has its own reference page:

| Sub-package | Doc |
|---|---|
| `pkg/ebpf/bytecode` | [bytecode.md](ebpf/bytecode.md) — `AssetReader`, runtime compilation, bindata vs disk modes |
| `pkg/ebpf/maps` | [maps.md](ebpf/maps.md) — `GenericMap[K,V]`, batch iteration, per-CPU maps |
| `pkg/ebpf/uprobes` | [uprobes.md](ebpf/uprobes.md) — `UprobeAttacher`, attach rules, shared-library hooks |
| `pkg/ebpf/telemetry` | [telemetry.md](ebpf/telemetry.md) — `ErrorsTelemetryModifier`, kprobe stats, perf-buffer metrics |
| `pkg/ebpf/verifier` | [verifier.md](ebpf/verifier.md) — `BuildVerifierStats`, complexity analysis, CI calculator |

---

## Related packages

- `pkg/network/` — primary consumer: network tracer, USM, DNS monitoring. See [network.md](network/network.md). `pkg/network/config.Config` extends `ebpf.Config` with NPM options; `pkg/network/tracer/connection` uses all three loading strategies.
- `pkg/network/usm/` — Universal Service Monitoring; uses `UprobeAttacher` (via `uprobes/`) to attach TLS probes and `sharedlibraries` to detect library opens. See [usm.md](network/usm.md).
- `pkg/security/probe` — CWS (Cloud Workload Security) eBPF probes. `EBPFProbe` uses `ebpf.Manager`, `telemetry.ErrorsTelemetryModifier`, and `constantfetch` for CO-RE offset resolution. See [probe.md](security/probe.md).
- `pkg/dyninst` — dynamic instrumentation (Live Debugger): uses `pkg/ebpf/uprobes` to attach stack-machine uprobes to Go processes. See [dyninst.md](dyninst.md).
- `pkg/collector/corechecks/ebpf/` — eBPF-backed core checks (OOM kill, TCP queue length, etc.).
- `cmd/system-probe/` — the binary that hosts all eBPF modules.
- `github.com/DataDog/ebpf-manager` — upstream manager library wrapped by `pkg/ebpf`.
- `github.com/cilium/ebpf` — low-level eBPF library used for map/program operations.
