# pkg/util/maps

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/maps`

## Purpose

`maps` provides generic, nil-safe utility functions for transforming and
filtering Go maps. The functions follow functional-programming conventions
(transform into a new map, never mutate in place) and handle `nil` input
gracefully by returning `nil`.

The package complements the standard library's `maps` package (Go 1.21+), which
covers equality and cloning. `pkg/util/maps` focuses on key/value transformation
and predicate-based filtering, which the standard library does not provide.

> **Alias note:** Because the standard library package is also named `maps`,
> importers typically alias this package: `import ddmaps
> "github.com/DataDog/datadog-agent/pkg/util/maps"`.

## Key elements

### Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `Map` | `Map[K1, K2 comparable, V1, V2 any](m map[K1]V1, kmap func(K1) K2, vmap func(V1) V2) map[K2]V2` | Returns a new map with both keys and values transformed. |
| `MapKeys` | `MapKeys[K1, K2 comparable, V any](m map[K1]V, kmap func(K1) K2) map[K2]V` | Returns a new map with only the keys transformed; values are copied. |
| `MapValues` | `MapValues[K comparable, V1, V2 any](m map[K]V1, vmap func(V1) V2) map[K]V2` | Returns a new map with only the values transformed; keys are copied. |
| `CastIntegerKeys` | `CastIntegerKeys[K1, K2 constraints.Integer, V any](m map[K1]V) map[K2]V` | Convenience wrapper for `MapKeys` using a plain integer cast. |
| `Filter` | `Filter[K comparable, V any](m map[K]V, fn func(K, V) bool) map[K]V` | Returns a new map containing only the entries for which `fn` returns `true`. |

All functions return `nil` when `m` is `nil`.

## Usage

### Filtering a map

`Filter` is the most common function in the codebase. In
`pkg/network/sender/docker_proxy.go` it extracts proxies whose IP has not yet
been discovered:

```go
undiscoveredProxies := ddmaps.Filter(d.proxyByPID, func(_ uint32, p *proxy) bool {
    return !p.ip.IsValid()
})
```

### Transforming keys and values

`Map` is used in `pkg/collector/corechecks/ebpf/probe/ebpfcheck/probe.go` to
convert a specialised eBPF map into a generic stats map:

```go
statsGenericMap, err := ddmaps.Map[cookie, kprobeKernelStats](k.cookieToKprobeStats)
```

Here both type parameters are concrete types from the eBPF check; the two
transformer arguments are omitted because the call uses the one-argument form
`Map[K, K, V, V]` with identity functions.

### Casting integer key types

`CastIntegerKeys` avoids a boilerplate `MapKeys` call when the only change is
a numeric widening or narrowing:

```go
// Convert map[uint32]Foo to map[uint64]Foo
wide := ddmaps.CastIntegerKeys[uint32, uint64](narrowMap)
```

### Nil safety

All functions return `nil` for a `nil` input map, so callers do not need a
`nil` guard before calling them:

```go
var m map[string]int // nil
result := ddmaps.Filter(m, func(k string, v int) bool { return v > 0 })
// result == nil, no panic
```

## Cross-references

### `pkg/network` — `Filter` on connection maps

`pkg/network/sender/docker_proxy.go` is the known direct importer in the network package. It uses `ddmaps.Filter` to extract a subset of entries from a `map[uint32]*proxy` by predicate. This is the canonical pattern: rather than a manual `for` loop with conditional accumulation, `Filter` expresses intent clearly and handles the nil case automatically.

See: [`pkg/network`](../network/network.md)

### `pkg/ebpf` — map key type casting in eBPF checks

eBPF probes commonly work with maps whose keys are C-level integer types that don't match the Go types expected by higher-level code. `CastIntegerKeys` and `Map` are used in `pkg/collector/corechecks/ebpf/probe/ebpfcheck/probe.go` to convert specialised eBPF map types into generic stats maps without boilerplate loops.

See: [`pkg/ebpf`](../ebpf.md)

### `pkg/util/slices` — complementary package

`pkg/util/maps` and `pkg/util/slices` are the map and slice halves of the same design: generic, functional-style combinators that return new collections and treat nil input as a no-op. When you need to project a map's values into a slice (or vice versa), you will often combine both packages.

See: [`pkg/util/slices`](slices.md)

### Related utility packages

| Package | Relationship |
|---------|--------------|
| [`pkg/util/slices`](slices.md) | Provides `Map` for slice-to-slice projection. Often used alongside `pkg/util/maps` when a map's values need to be converted to a slice. |
| [`pkg/util/sort`](sort.md) | Sorting and deduplication helpers. Independent of `pkg/util/maps`; useful as a post-processing step after extracting a key list from a map. |
