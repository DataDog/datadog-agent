# pkg/util/funcs

Import path: `github.com/DataDog/datadog-agent/pkg/util/funcs`

## Purpose

Provides generic helpers for memoization (run-once caching) and flushable caching of function results. The package eliminates the boilerplate of pairing a `sync.Once` with a result variable everywhere a function should only be called once per process lifetime (or per unique argument). Variants cover all combinations of: with/without error return, with/without an argument key, thread-safe/unsafe.

## Key elements

### Memoize — zero-argument, with error

```go
func Memoize[T any](fn func() (T, error)) func() (T, error)
```

Wraps `fn` so it is called at most once, even if it returns an error. Subsequent calls return the cached `(T, error)` pair. Uses `sync.Once` internally, so it is goroutine-safe. The MIT license header in `memoize.go` acknowledges derivation from the `cilium/ebpf` project.

### MemoizeNoError — zero-argument, no error

```go
func MemoizeNoError[T any](fn func() T) func() T
```

Same as `Memoize` but for functions that cannot fail. Also goroutine-safe via `sync.Once`.

### MemoizeNoErrorUnsafe — zero-argument, no error, not thread-safe

```go
func MemoizeNoErrorUnsafe[T any](fn func() T) func() T
```

A lock-free variant for single-threaded call sites. Trades safety for lower overhead. Use only when the memoized function is guaranteed to be called from one goroutine at a time.

### MemoizeArg — keyed by comparable argument, with error

```go
func MemoizeArg[K comparable, T any](fn func(K) (T, error)) func(K) (T, error)
```

Memoizes per unique argument value. Results are stored in a `map[K]...` protected by a `sync.Mutex`. `fn` is called exactly once per distinct `K`, even on concurrent calls with the same key.

### MemoizeArgNoError — keyed by comparable argument, no error

```go
func MemoizeArgNoError[K comparable, T any](fn func(K) T) func(K) T
```

Same as `MemoizeArg` but for infallible functions.

### Cache — flushable single-result cache

```go
type CachedFunc[T any] interface {
    Do() (*T, error)
    Flush()
}

func Cache[T any](fn func() (*T, error)) CachedFunc[T]
func CacheWithCallback[T any](fn func() (*T, error), cb func()) CachedFunc[T]
```

Unlike `Memoize`, `Cache` allows the stored result to be invalidated. `Flush` clears the cached pointer so the next `Do` call re-invokes `fn`. `CacheWithCallback` additionally invokes a provided callback on each flush. Access is protected by a read-write mutex; concurrent `Do` calls take a read lock first (fast path) and fall back to a write lock only on a cache miss.

### Comparison table

| Function | Args | Error return | Thread-safe | Flushable |
|---|---|---|---|---|
| `Memoize` | none | yes | yes | no |
| `MemoizeNoError` | none | no | yes | no |
| `MemoizeNoErrorUnsafe` | none | no | no | no |
| `MemoizeArg` | one (`comparable`) | yes | yes | no |
| `MemoizeArgNoError` | one (`comparable`) | no | yes | no |
| `Cache` / `CacheWithCallback` | none | yes | yes | yes |

## Usage

### Package-level lazy initialization — kernel information

`pkg/util/kernel/platform.go` uses `Memoize` to initialize OS platform data once and cache it for the process lifetime:

```go
var Platform        = funcs.Memoize(func() (string, error) { ... })
var PlatformVersion = funcs.Memoize(func() (string, error) { ... })
var Family          = funcs.Memoize(func() (string, error) { ... })
var platformInformation = funcs.Memoize(getPlatformInformation)
```

### Struct field initialization — network path runner

`pkg/networkpath/traceroute/runner/runner.go` uses `MemoizeNoError` for a field that is computed once per runner instance:

```go
networkID: funcs.MemoizeNoError(func() string {
    id, _ := cloudproviders.GetNetworkID(ctx)
    return id
})
```

### Per-key memoization — sync pool telemetry

`pkg/util/sync/pool.go` uses `MemoizeArgNoError` to ensure each `(module, name)` combination registers its telemetry counters exactly once, even when multiple pools are created concurrently:

```go
var globalPoolTelemetry = funcs.MemoizeArgNoError(newPoolTelemetry)
```

### Flushable cache — eBPF BTF loader

`pkg/ebpf/btf.go` uses `CacheWithCallback` so that the cached BTF kernel spec can be invalidated (flushed) if the underlying BTF file changes:

```go
var loadKernelSpec = funcs.CacheWithCallback[btf.Spec](btf.LoadKernelSpec, btf.FlushKernelSpec)
```

### System-probe API client

`pkg/system-probe/api/client/client.go` and `client/check.go` use `Memoize` to lazily initialise the HTTP client and connection parameters exactly once per process, avoiding repeated config lookups on the hot path.

## Relationship to other packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/util/retry` | [retry.md](retry.md) | `retry` is the complement for fallible, repeated operations: it retries on error and tracks backoff state. `funcs.Memoize` is for operations that should run at most once (permanent cache). Use `retry` when you want to keep trying on failure; use `Memoize` when you want to cache the first result (success or failure) forever. |
| `pkg/util/kernel` | [kernel.md](kernel.md) | `pkg/util/kernel` is the primary consumer of `Memoize` in the `pkg/` tree. Every package-level variable that exposes kernel information (`HostVersion`, `ProcFSRoot`, `SysFSRoot`, `BootRoot`, `PossibleCPUs`, `OnlineCPUs`, `Release`, `Machine`, `Platform`, `PlatformVersion`, `Family`, `RootNSPID`) is memoized with `funcs.Memoize` or `funcs.MemoizeNoError`. |
| `pkg/util/sync` | [sync.md](sync.md) | `pkg/util/sync` uses `MemoizeArgNoError` to deduplicate telemetry registration across multiple pool instances with the same `(module, name)` key. This prevents Prometheus duplicate-registration panics when pools are created concurrently. |
