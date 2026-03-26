# pkg/util/pointer

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/pointer`

## Purpose

`pkg/util/pointer` provides small generic helpers for working with pointers in Go. The primary problem it solves is the common need to take the address of a literal or local value when a struct field or function parameter requires a pointer. Without a helper you must assign the value to a named variable first, which is verbose and clutters the call site. The package also offers a typed conversion helper used when bridging integer-based monitoring APIs with float-based metrics senders.

## Key Elements

### `Ptr[T any](v T) *T`

Returns a pointer to a heap-allocated copy of `v`. The type parameter is inferred from the argument in almost all cases, so the explicit form `Ptr[T]` is only needed when the compiler cannot infer it (e.g. when passing an untyped literal to a `*uint64` field).

### `UIntPtrToFloatPtr(u *uint64) *float64`

Converts a `*uint64` to a `*float64`, propagating `nil` as `nil`. Used when raw byte/nanosecond counters from container runtimes (which expose `*uint64`) need to be forwarded to the agent's metrics layer (which uses `*float64`).

## Usage

### Taking the address of a literal value

```go
import "github.com/DataDog/datadog-agent/pkg/util/pointer"

// Instead of:
v := true
cfg.AutoMultiLine = &v

// Write:
cfg.AutoMultiLine = pointer.Ptr(true)

// Works for any type:
cfg.MinReplicas = pointer.Ptr[int32](2)
cfg.CPURequest   = pointer.Ptr(500.0)
```

### Converting container stats from uint64 to float64

```go
outStats.CPU.Total      = pointer.UIntPtrToFloatPtr(kubeStats.CPU.UsageCoreNanoSeconds)
outStats.Memory.RSS     = pointer.UIntPtrToFloatPtr(kubeStats.Memory.RSSBytes)
outStats.Memory.Pgfault = pointer.UIntPtrToFloatPtr(kubeStats.Memory.PageFaults)
```

If the source pointer is `nil`, the result is also `nil`, so callers do not need to guard the call.

### Real-world patterns

- **Autoscaling** (`pkg/clusteragent/autoscaling/`) uses `Ptr[int32]` to fill optional fields in autoscaler recommendation structs (min/max replicas, utilization targets).
- **Container metrics collectors** (`pkg/util/containers/metrics/kubelet/`) use `UIntPtrToFloatPtr` to convert all Kubelet stat fields (CPU, memory, network I/O) into the agent's internal `ContainerStats` format.
- **Helm check** (`pkg/collector/corechecks/cluster/helm/`) uses `Ptr` to set optional fields in Kubernetes API objects.
- **SBOM / security** (`pkg/security/resolvers/sbom/`) uses `Ptr` when constructing CycloneDX protobuf messages that require pointer fields.
- **Tests** — `Ptr` is heavily used in test files to build fixture structs with optional pointer fields inline, keeping test data readable.

## Cross-references

### `pkg/util/option` — conceptual relationship

Both `pkg/util/pointer` and `pkg/util/option` address the "value that may be absent" problem, but at different layers:

- `pointer.Ptr(v)` solves a **syntactic** problem: taking the address of a literal so it can be assigned to a `*T` field. The pointer itself is the canonical Go way to express optionality in a struct.
- `option.Option[T]` solves a **semantic** problem: it makes absence explicit and self-documenting, avoids nil-pointer dereferences, and works cleanly in Go's fx dependency-injection graph where a `*T` cannot distinguish "not provided" from "provided as nil".

Use `pointer.Ptr` when an API (Kubernetes CRD, proto struct, config field) already declares a `*T` field and you need a non-nil value from a literal. Use `option.Option[T]` when designing new APIs where absence is a meaningful state.

See: [`pkg/util/option`](option.md)

### `pkg/metrics` — pointer fields in metric types

`pkg/metrics` declares several optional `*float64` and `*uint64` fields on stats types (container metrics, resource usage) that use `UIntPtrToFloatPtr` for conversion. The pattern appears throughout `pkg/util/containers/metrics/kubelet/` where every Kubelet stat field (CPU, memory, network I/O) is a `*uint64` in the source struct and a `*float64` in the agent's `ContainerStats`.

See: [`pkg/metrics`](../metrics/metrics.md)

### Related utility packages

| Package | Relationship |
|---------|--------------|
| [`pkg/util/option`](option.md) | Semantic optionality type for the fx component graph and config structs. Prefer `option.Option[T]` over `*T` when designing new optional fields in component `Requires`/`Provides` structs. |
| [`pkg/util/maps`](maps.md) | Orthogonal. `pkg/util/maps` transforms maps; `pkg/util/pointer` converts individual values. No direct dependency. |
