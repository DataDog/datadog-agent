> **TL;DR:** `pkg/ebpf/maps` wraps `cilium/ebpf`'s `Map` with Go generics to provide type-safe CRUD and efficient batch iteration over eBPF maps, eliminating `unsafe.Pointer` boilerplate throughout the codebase.

# pkg/ebpf/maps

## Purpose

Wraps `github.com/cilium/ebpf`'s `ebpf.Map` with Go generics to give callers a type-safe CRUD and iteration API. Eliminates boilerplate `unsafe.Pointer` casts and adds transparent batch iteration to reduce the number of syscalls when draining large maps.

## Key elements

### Configuration and build flags

The entire package is gated on `//go:build linux_bpf`. It compiles only on Linux with eBPF support enabled.

### Key interfaces

| Type | Description |
|------|-------------|
| `GenericMapIterator[K, V any]` | Interface with `Next(*K, *V) bool` and `Err() error`. Returned by `Iterate()` and `IterateWithBatchSize()`. |

### Key types

| Type | Description |
|------|-------------|
| `GenericMap[K, V any]` | Core wrapper. Holds an `*ebpf.Map` and a flag indicating whether the key type supports the batch API. |
| `IteratorOptions` | Configures batch size and forces single-item iteration when needed. |

### Key functions

| Function | Description |
|----------|-------------|
| `NewGenericMap[K, V](spec *ebpf.MapSpec)` | Creates a new map from a spec. Key and value sizes are inferred from the type parameters via `unsafe.Sizeof`. Per-CPU maps require `V` to be a slice. |
| `Map[K, V](m *ebpf.Map)` | Wraps an existing `*ebpf.Map`. Validates that the map's declared key/value sizes match the sizes of `K` and `V`. |
| `GetMap[K, V](mgr *manager.Manager, name string)` | Convenience: looks up a named map from an `ebpf-manager` instance and returns a typed `GenericMap`. |

### Methods on `GenericMap`

| Method | Description |
|--------|-------------|
| `Put(key *K, value *V) error` | Insert or overwrite. |
| `Lookup(key *K, valueOut *V) error` | Point lookup. Returns `ebpf.ErrKeyNotExist` if absent. |
| `Update(key *K, value *V, flags ebpf.MapUpdateFlags) error` | Update with explicit flags (e.g., `UpdateExist`, `UpdateNoExist`). |
| `Delete(key *K) error` | Remove a key. |
| `BatchDelete(keys []K) (int, error)` | Remove multiple keys in one syscall. Returns `ErrBatchAPINotSupported` when unavailable. |
| `BatchUpdate(keys []K, values []V, opts *ebpf.BatchOptions) (int, error)` | Insert/update multiple entries in one syscall. |
| `Iterate() GenericMapIterator[K, V]` | Returns a single-item iterator (default). |
| `IterateWithBatchSize(n int) GenericMapIterator[K, V]` | Returns a batch iterator when `n > 1` and the batch API is available; otherwise falls back to item-by-item. |
| `CanUseBatchAPI() bool` | True when: kernel supports batch API (feature-detected via a probe map at startup), key type is `binary.Read`-able, and map is not per-CPU. |

### Sentinel errors

| Error | Meaning |
|-------|---------|
| `ErrBatchAPINotSupported` | The caller asked for batch operations but the map or kernel does not support them. |

### Per-CPU maps

Per-CPU map types (`PerCPUHash`, `PerCPUArray`, `LRUCPUHash`) require `V` to be a slice type (one element per logical CPU). The constructors enforce this at runtime.

### Batch API detection

`BatchAPISupported` is a memoized function that creates a temporary probe map and attempts a `BatchUpdate` to determine support. This is preferred over a kernel-version check for accuracy across backport kernels.

## Usage

`GetMap` is the primary entry point for eBPF programs managed by `ebpf-manager`:

```go
// After manager.Init():
connMap, err := maps.GetMap[ConnTupleType, ConnStatsType](mgr, "conn_stats")
if err != nil { /* handle */ }

// Drain the entire map efficiently:
it := connMap.IterateWithBatchSize(100)
var key ConnTupleType
var val ConnStatsType
for it.Next(&key, &val) {
    process(key, val)
}
if err := it.Err(); err != nil { /* handle */ }
```

Example callers in the codebase: `pkg/network/tracer/connection/ebpf_tracer.go`, `pkg/network/tracer/offsetguess/tracer.go`, `pkg/collector/corechecks/ebpf/probe/ebpfcheck/probe.go`, `pkg/network/protocols/events/batch_consumer.go`.

---

## Relationship to `MapCleaner`

`pkg/ebpf` exports a higher-level `MapCleaner[K, V]` (see [pkg/ebpf](../ebpf.md)) that
internally uses `GenericMap` (with its batch API) to sweep and delete stale entries
periodically. When writing new code that needs both typed access and automatic expiry,
prefer `MapCleaner` over a bare `GenericMap` + manual deletion loop.

## Relationship to telemetry

`ErrorsTelemetryModifier` (see [telemetry.md](../ebpf/telemetry.md)) instruments map
operations at the eBPF level (kernel side). `GenericMap` covers the user-space side (Go
`bpf()` syscall errors). Both layers surface failures via Prometheus counters so operators
can distinguish kernel-side map errors from user-space iteration failures.

## Related packages

- [pkg/ebpf](../ebpf.md) — wraps `GenericMap` in `MapCleaner` and documents the full eBPF infrastructure.
- [pkg/ebpf/bytecode](bytecode.md) — loads the eBPF object whose `CollectionSpec` declares the maps that `GetMap` looks up.
- [pkg/network](../network/network.md) — primary consumer; `pkg/network/tracer/connection/ebpf_tracer.go` wraps connection maps via `maps.GetMap` and iterates them with batch size 100.
- [pkg/security/probe](../security/probe.md) — CWS uses `GenericMap` for kfilter approver maps and discarder maps written during rule-set application.
