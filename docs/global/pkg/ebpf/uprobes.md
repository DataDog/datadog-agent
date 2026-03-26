# pkg/ebpf/uprobes

## Purpose

Provides a high-level `UprobeAttacher` that monitors running processes and automatically attaches eBPF uprobes to matching binaries (shared libraries and/or executables). It handles process birth/death notifications, library `open` events, ELF inspection, and `ebpf-manager` probe lifecycle—so callers only need to declare _what_ to attach rather than orchestrating each step manually.

## Key elements

### Build flags

Most files are gated on `//go:build linux_bpf`. `procfs.go` uses `//go:build linux` (no BPF dependency).

### Core types

| Type | Description |
|------|-------------|
| `UprobeAttacher` | Main type. Created with `NewUprobeAttacher`. Starts background goroutines that react to process and shared-library events. |
| `AttacherConfig` | Configuration struct: list of `AttachRule`s, `ExcludeTargets` bitmask, and a reference to `pkg/ebpf.Config`. |
| `AttachRule` | Declares which targets a set of probes applies to. Fields: `LibraryNameRegex`, `ExecutableFilter`, `Targets`, `ProbesSelector`, and `ProbeOptionsOverride`. |
| `AttachTarget` | Bitmask: `AttachToExecutable` and/or `AttachToSharedLibraries`. |
| `ExcludeMode` | Bitmask to skip certain processes: `ExcludeSelf`, `ExcludeInternal` (other Datadog agents), `ExcludeBuildkit`, `ExcludeContainerdTmp`. |
| `ProbeOptions` | Per-probe overrides: `IsManualReturn` (attach at all return sites instead of using uretprobes) and `Symbol` (useful for Go function names with non-C characters). |
| `BinaryInspector` | Interface that extracts function offsets from a binary given a set of `SymbolRequest`s. |
| `NativeBinaryInspector` | Built-in `BinaryInspector` that reads ELF symbol tables. Only supports 64-bit binaries matching the agent's own architecture. |
| `ProcInfo` | Thin procfs wrapper that lazily reads `/proc/<pid>/exe` and `/proc/<pid>/comm`. Used in `ExecutableFilter` callbacks. |
| `InspectionResult` | Map of symbol name to `bininspect.FunctionMetadata` (entry offset + optional return locations). |
| `SymbolRequest` | Describes a single symbol fetch: `Name`, `BestEffort` flag, and `IncludeReturnLocations`. |

### Key errors

| Error | Meaning |
|-------|---------|
| `ErrSelfExcluded` | PID matches the agent's own PID. |
| `ErrInternalDDogProcessRejected` | Path matches the internal Datadog process regex. |
| `ErrNoMatchingRule` | No `AttachRule` matched the library or executable path. |

### Linux-specific requirements

- Requires Linux kernel 4.1+ (uprobe infrastructure).
- The shared-library watcher depends on `pkg/network/usm/sharedlibraries`, which hooks `do_sys_open` via eBPF. Any new library name must also be registered in `pkg/network/ebpf/c/shared-libraries/probes.h:do_sys_open_helper_exit`.
- The recommended process monitor source is the event stream consumer API (`pkg/process/events`). `monitor.GetProcessMonitor()` is available but intended for USM only and will be deprecated.

### Probe name conventions

By default, probe names are parsed to infer the symbol and return mode:

- `uprobe__<symbol>` — attaches to the entry point of `<symbol>`.
- `uretprobe__<symbol>` — attaches using a uretprobe on `<symbol>`.
- Manual-return probes (e.g., for Go functions) can override this via `ProbeOptions.IsManualReturn`.

## Usage

A typical setup for TLS inspection via `libssl`:

```go
connectProbeID := manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect"}

attacherCfg := uprobes.AttacherConfig{
    Rules: []*uprobes.AttachRule{
        {
            LibraryNameRegex: regexp.MustCompile(`libssl.so`),
            Targets:          uprobes.AttachToSharedLibraries,
            ProbesSelector: []manager.ProbesSelector{
                &manager.ProbeSelector{ProbeIdentificationPair: connectProbeID},
            },
        },
    },
    ExcludeTargets: uprobes.ExcludeInternal | uprobes.ExcludeSelf,
    EbpfConfig:     ebpfCfg,
}

ua, err := uprobes.NewUprobeAttacher("mymodule", "ssl", attacherCfg, mgr, onAttach, uprobes.AttacherDependencies{
    Inspector:      &uprobes.NativeBinaryInspector{},
    ProcessMonitor: processMonitor,
})
if err != nil { /* handle */ }
ua.Start()
// ... later:
ua.Stop()
```

Existing callers in the codebase: `pkg/network/usm/ebpf_ssl.go`, `pkg/network/usm/ebpf_gotls.go`, `pkg/network/usm/istio.go`, `pkg/network/usm/nodejs.go`, `pkg/gpu/probe.go`, `pkg/network/tracer/connection/ssl-uprobes/ebpf_ssl.go`.

---

## Integration with shared-library detection

`UprobeAttacher` does not discover shared-library opens on its own. The typical flow in USM is:

1. `pkg/network/usm/sharedlibraries.GetEBPFProgram` hooks `open`/`openat`/`openat2` via eBPF
   and fires a `LibraryCallback` when a matching library suffix is seen.
2. The callback resolves the host path via `utils.NewFilePath` and calls into
   `UprobeAttacher.AttachToLibrary` (or equivalent) for the specific PID + library.
3. `UprobeAttacher` invokes `BinaryInspector` to extract ELF symbol offsets, then calls
   `ebpf-manager` to attach the uprobe probes.

Any new library pattern must be registered in **both** the `LibsetToLibSuffixes` map in
`pkg/network/usm/sharedlibraries` and the corresponding `AttachRule.LibraryNameRegex` in the
attacher config. See [usm.md](../network/usm.md) for the `sharedlibraries` sub-package API.

## Go function uprobes

For Go binaries, function names contain non-C characters (e.g., `(*net/http.Transport).roundTrip`).
Use `ProbeOptions.Symbol` to provide the exact ELF symbol name and set `IsManualReturn = true`
instead of uretprobes, because Go's stack-copying goroutine model makes uretprobes unreliable.
The `NativeBinaryInspector` handles Go ABI (register-based, ≥ Go 1.17) automatically.

## Dynamic instrumentation (pkg/dyninst)

`pkg/dyninst` re-implements the uprobe attachment pipeline independently for its stack-machine
use case (see [dyninst.md](../dyninst.md)). The two implementations share the same kernel
uprobe mechanism but `dyninst` compiles custom per-probe eBPF programs rather than using
pre-compiled probes registered with `ebpf-manager`. When adding uprobe support for a new use
case, prefer `UprobeAttacher` unless the per-probe eBPF program needs to be generated
dynamically from a user-supplied probe definition.

## Related packages

- [pkg/ebpf](../ebpf.md) — parent package; `UprobeAttacher` is exposed through the `uprobes/` sub-package and summarised in the parent doc's "Uprobes" section.
- [pkg/network/usm](../network/usm.md) — primary consumer; attaches TLS probes via `openssl`, `gotls`, `istio`, and `nodejs` rules.
- [pkg/dyninst](../dyninst.md) — alternative uprobe pipeline for dynamic instrumentation / Live Debugger.
