# pkg/util/cachedfetch

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/cachedfetch`

## Purpose

`pkg/util/cachedfetch` provides a **read-through cache for values that are fetched from an external source** (typically a cloud provider metadata API). On every `Fetch` call it attempts a fresh retrieval. If the attempt fails it returns the last successfully fetched value instead of propagating the error. This "ride out" behaviour is especially valuable for the agent's cloud metadata providers (EC2, GCE, Azure, Oracle, IBM), whose instance metadata services can suffer transient unavailability without affecting the running agent.

Cached values **do not expire**. The design philosophy is that stale cloud metadata (instance ID, hostname, instance type) is better than no metadata during an API outage. If you need TTL-based expiry, add it in the `Attempt` function or use a different mechanism.

## Key Elements

### `Fetcher` (struct)

The only public type. Callers declare one `Fetcher` per piece of data they want to cache. Fields:

| Field | Type | Description |
|---|---|---|
| `Attempt` | `func(context.Context) (interface{}, error)` | **Required.** Called on every `Fetch` to retrieve a fresh value. |
| `Name` | `string` | Human-readable name used in the default failure log message. At least one of `Name` or `LogFailure` must be set. |
| `LogFailure` | `func(error, interface{})` | Optional custom logger called on failure when a cached value is available. Receives the error and the last successful value. If nil, a default `log.Debugf` message using `Name` is emitted. |

The struct embeds `sync.Mutex`, making it safe to call `Fetch` from multiple goroutines concurrently. Concurrent calls will each invoke `Attempt` independently (no coalescing), but the cached value is updated under the lock.

### `(*Fetcher).Fetch(ctx context.Context) (interface{}, error)`

Core method. Calls `Attempt(ctx)`. On success, stores and returns the result. On failure, returns the last cached value (if any) without an error. If no successful attempt has ever been made, returns the error from `Attempt` directly.

Context cancellation and deadline-exceeded are treated as ordinary errors: they fall back to the cache just like any other failure.

### `(*Fetcher).FetchString(ctx context.Context) (string, error)`

Convenience wrapper that type-asserts the result of `Fetch` to `string`. Panics if `Attempt` returns a non-string on success — only use when the `Attempt` function is known to return a `string`.

### `(*Fetcher).FetchStringSlice(ctx context.Context) ([]string, error)`

Same as `FetchString` but for `[]string` results.

### `(*Fetcher).Reset()`

Clears the cached value. Intended for testing to force `Fetch` to behave as if no successful call has ever been made.

## Usage

### Declaring a package-level fetcher

The standard pattern in cloud-provider packages is a package-level `Fetcher` variable with an inline `Attempt` closure:

```go
import "github.com/DataDog/datadog-agent/pkg/util/cachedfetch"

var instanceIDFetcher = cachedfetch.Fetcher{
    Name: "EC2 instance ID",
    Attempt: func(ctx context.Context) (interface{}, error) {
        return getInstanceIDFromIMDS(ctx)
    },
}

func GetInstanceID(ctx context.Context) (string, error) {
    return instanceIDFetcher.FetchString(ctx)
}
```

This is exactly how `pkg/util/ec2/ec2.go` declares `instanceIDFetcher`, `hostnameFetcher`, and `instanceTypeFetcher`.

### Custom failure logging

```go
var networkFetcher = cachedfetch.Fetcher{
    Attempt: fetchNetworkInterfaces,
    LogFailure: func(err error, lastValue interface{}) {
        log.Warnf("Failed to fetch network interfaces, using cached value %v: %v", lastValue, err)
    },
}
```

### Known callers

- `pkg/util/ec2/` — instance ID, hostname, instance type, network interfaces, account ID, spot instance data
- `pkg/util/cloudproviders/gce/` — GCE instance metadata
- `pkg/util/cloudproviders/azure/` — Azure instance metadata (vmID, resourceGroup, instanceType, CCRID, publicIPv4)
- `pkg/util/cloudproviders/oracle/` — Oracle Cloud metadata (instanceID, CCRID)
- `pkg/util/cloudproviders/ibm/` — IBM Cloud metadata
- `pkg/util/cloudproviders/alibaba/` — Alibaba Cloud metadata
- `pkg/util/cloudproviders/tencent/` — Tencent Cloud metadata

## Relationship to other packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/util/cache` | [cache.md](cache.md) | A different caching approach: `pkg/util/cache` is a global TTL store (backed by `patrickmn/go-cache`) with key-based lookup and configurable expiry. `cachedfetch` is a per-fetcher, indefinitely-cached, failure-tolerant wrapper around a single fetch function. Choose `cachedfetch` when you need "last-good-value" semantics for a remote call; choose `pkg/util/cache` when you need TTL-based eviction and multi-key storage. |
| `pkg/util/cloudproviders` | [cloudproviders.md](cloudproviders.md) | `cloudproviders` is the fan-out entry point. Every provider sub-package it delegates to uses `cachedfetch.Fetcher` to cache its IMDS responses for the process lifetime. The concurrency notes in the `cloudproviders` doc about simultaneous calls to `DetectCloudProvider` and `GetHostAliases` being safe apply because each underlying `Fetcher` is protected by a `sync.Mutex`. |
| `pkg/util/ec2` | [ec2.md](ec2.md) | The most common consumer. `ec2.go`, `network.go`, `spot.go`, `ec2_account_id.go`, and `ccrid_fetch.go` each declare one or more package-level `Fetcher` variables. The `Reset()` method is exercised by `ec2` unit tests to force fresh IMDS calls between test cases. |
