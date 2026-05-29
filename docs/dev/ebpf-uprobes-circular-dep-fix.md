# Breaking the pkg/ebpf/uprobes <-> pkg/network Circular Dependency

## Problem

`pkg/ebpf/uprobes/` and `pkg/network/usm/sharedlibraries/` had a circular
dependency that prevented extracting either `pkg/ebpf/` or `pkg/network/` as
independent Go modules:

```
pkg/ebpf/uprobes/attacher.go
  -> imports pkg/network/usm/sharedlibraries  (for EbpfProgram, Libset, LibPath)
  -> sharedlibraries/ebpf.go imports pkg/ebpf  (for Manager, Config, etc.)
  = CYCLE
```

This circular dependency blocks the module extraction needed to upstream the
CNM OTel receiver to the OTel Collector contrib repository.

## Root Cause

The `UprobeAttacher` in `pkg/ebpf/uprobes/` directly imported the concrete
`sharedlibraries.EbpfProgram` type and called `sharedlibraries.GetEBPFProgram()`
and `sharedlibraries.IsSupported()`. Since `EbpfProgram` itself imports from
`pkg/ebpf/` (for the eBPF manager, bytecode loading, etc.), this created the
cycle.

However, the attacher only needed:
1. **Pure types** -- `Libset` (string enum), `LibPath` (struct), `LibsetToLibSuffixes` (map)
2. **Three methods** -- `InitWithLibsets()`, `Subscribe()`, `Stop()`
3. **A support check** -- `IsSupported()`

The pure types had zero dependency on `pkg/ebpf/`. Only the `EbpfProgram`
concrete type and its factory function created the coupling.

## Solution

### 1. Extract pure types into `pkg/network/usm/sharedlibraries/types/`

New package with zero internal dependencies containing:
- `Libset` type + constants (`LibsetCrypto`, `LibsetGPU`, `LibsetLibc`)
- `LibsetToLibSuffixes` map
- `IsLibsetValid()` function
- `LibPath` struct + `LibPathMaxSize` constant
- `ToBytes()` function

### 2. Define `SharedLibraryWatcher` interface in `pkg/ebpf/uprobes/`

```go
type SharedLibraryWatcher interface {
    InitWithLibsets(libsets ...sharedlibtypes.Libset) error
    Subscribe(callback func(sharedlibtypes.LibPath), libsets ...sharedlibtypes.Libset) (func(), error)
    Stop()
    IsSupported() bool
}
```

This interface is satisfied by `sharedlibraries.EbpfProgram` without that
package needing to know about the interface.

### 3. Inject the watcher via `AttacherDependencies`

Added `SharedLibWatcher SharedLibraryWatcher` field to `AttacherDependencies`.
Callers that configure shared library tracing now create the `EbpfProgram` and
pass it in:

```go
uprobes.AttacherDependencies{
    Inspector:        &uprobes.NativeBinaryInspector{},
    ProcessMonitor:   monitor.GetProcessMonitor(),
    Telemetry:        telemetry.GetCompatComponent(),
    SharedLibWatcher: sharedlibraries.GetEBPFProgram(&cfg.Config),  // NEW
}
```

### 4. Backward-compatible re-exports

Both `sharedlibraries/libset.go` and `sharedlibraries/types_linux.go` now
re-export from the new `types/` package via type aliases, so existing code
importing `sharedlibraries.Libset` or `sharedlibraries.LibPath` continues
to work unchanged.

## Files Changed

| File | Change |
|------|--------|
| `pkg/network/usm/sharedlibraries/types/libset.go` | **New** -- pure types extracted |
| `pkg/network/usm/sharedlibraries/types/libpath_linux.go` | **New** -- LibPath struct extracted |
| `pkg/ebpf/uprobes/sharedlib_iface.go` | **New** -- SharedLibraryWatcher interface |
| `pkg/ebpf/uprobes/attacher.go` | Import types/ instead of sharedlibraries; use interface |
| `pkg/network/usm/sharedlibraries/libset.go` | Re-export from types/ |
| `pkg/network/usm/sharedlibraries/types_linux.go` | Re-export from types/ |
| `pkg/network/usm/sharedlibraries/ebpf.go` | Add IsSupported() method on EbpfProgram |
| `pkg/network/usm/ebpf_ssl.go` | Inject SharedLibWatcher |
| `pkg/network/usm/nodejs.go` | Inject SharedLibWatcher |
| `pkg/network/tracer/connection/ssl-uprobes/ebpf_ssl.go` | Inject SharedLibWatcher |
| `pkg/gpu/probe.go` | Inject SharedLibWatcher |

## Result

After this change, `pkg/ebpf/uprobes/` no longer imports `pkg/network/usm/sharedlibraries`
(only `pkg/network/usm/sharedlibraries/types/` which has no `pkg/ebpf` dependency).
The circular dependency is broken, unblocking the extraction of `pkg/ebpf/` as an
independent Go module.
