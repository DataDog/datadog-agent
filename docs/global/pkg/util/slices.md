> **TL;DR:** Provides a generic `Map` combinator for slices that transforms each element with a function and returns a new slice, filling the gap left by Go's standard library.

# pkg/util/slices

Import path: `github.com/DataDog/datadog-agent/pkg/util/slices`

## Purpose

Provides generic slice helpers that complement Go's standard `slices` package (added in Go 1.21). The package currently contains a single function, `Map`, which transforms a slice by applying a function to each element. The standard library does not include a `Map` combinator, making this a commonly needed addition across the codebase.

## Key elements

### Key functions

#### `Map[S ~[]E, E any, RE any](s S, fn func(E) RE) []RE`

Applies `fn` to every element of `s` and returns a new slice of results, preserving order and pre-allocating to `len(s)` capacity. The input slice is not modified.

Type parameters:
- `S` — any slice type whose element type is `E` (accepts named slice types, not just `[]E`)
- `E` — element type of the input slice
- `RE` — element type of the result slice; can differ from `E`

```go
// Convert a slice of int to a slice of string
strs := slices.Map([]int{1, 2, 3}, strconv.Itoa)
// → []string{"1", "2", "3"}
```

## Usage

`Map` is used wherever the codebase needs to project one slice type onto another without a manual loop. Representative examples:

**Network tracer — probe name to struct conversion**

```go
// pkg/network/tracer/connection/kprobe/manager.go
mgr.Probes = append(mgr.Probes, slices.Map(mainProbes, funcNameToProbe)...)
```

Here `mainProbes` is `[]probes.ProbeFuncName` (a named string type) and `funcNameToProbe` converts each name to a `*manager.Probe`. The `S ~[]E` constraint allows the named slice type to be passed directly.

**Network sender — DNS hostname serialisation**

```go
// pkg/network/sender/encode.go
uniqDNSStringList := ddslices.Map(dnsSet.UniqueKeys(), func(h dns.Hostname) string {
    return dns.ToString(h)
})
```

**Autodiscovery — container port to string list**

```go
// comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go
ports = slices.Map(containerPorts, func(port workloadmeta.ContainerPort) string {
    return strconv.Itoa(int(port.Port))
})
```

### Relationship to `golang.org/x/exp/slices` and standard `slices`

The standard `slices` package deliberately omits functional combinators like `Map` to keep the API minimal. This package fills that gap. If the standard library adds `Map` in a future Go version, this package should be updated or deprecated accordingly.

## Cross-references

### How `pkg/util/slices` is used in `pkg/network`

The network package is the primary consumer. Two usage patterns appear:

1. **`pkg/network/tracer/connection/kprobe/manager.go`** — converts a `[]probes.ProbeFuncName` (a named string slice type) to a `[]*manager.Probe` slice using `slices.Map`. The `S ~[]E` type constraint is what makes this work without an intermediate conversion.

2. **`pkg/network/sender/encode.go`** — converts `[]dns.Hostname` to `[]string` via `ddslices.Map`, extracting the serialisable string representation from each hostname value on the DNS encoding path.

See: [`pkg/network`](../network/network.md)

### `pkg/util/maps` — complementary package

`pkg/util/slices.Map` transforms slices; `pkg/util/maps` provides the equivalent for maps (`Map`, `MapKeys`, `MapValues`, `Filter`). They share the same functional-combinator philosophy and the same nil-safety conventions.

See: [`pkg/util/maps`](maps.md)

### Related utility packages

| Package | Relationship |
|---------|--------------|
| [`pkg/util/maps`](maps.md) | Map equivalents: `Map`, `MapKeys`, `MapValues`, `Filter`. Use `pkg/util/maps` when transforming map keys or values; use `pkg/util/slices` for slice projection. |
| [`pkg/util/sort`](sort.md) | Provides `UniqInPlace` for in-place sorting and deduplication of string slices — a common companion operation after a `Map` projection that produces strings. |
