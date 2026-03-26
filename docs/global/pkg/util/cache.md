> **TL;DR:** Provides a global TTL cache (backed by `patrickmn/go-cache`) for short-lived shared state and a lightweight `BasicCache` for per-component, no-expiry in-memory storage.

# pkg/util/cache

## Purpose

`pkg/util/cache` provides two complementary in-memory stores used throughout the agent:

1. **`Cache`** — a global TTL cache (backed by `patrickmn/go-cache`) for short-lived values
   such as cluster IDs, Kubernetes metadata, and check state. Entries expire automatically
   and a background goroutine purges them periodically.
2. **`BasicCache`** — a simple, no-expiry, thread-safe map intended for use cases where a
   component owns its own cache lifecycle (e.g. JMX check state).

## Key elements

### Key types

#### Global TTL cache

**`Cache`** — package-level `*cache.Cache` (from `patrickmn/go-cache`).

- Default item expiration: **5 minutes**.
- Purge interval: **30 seconds**.
- Uses `go-cache`'s API directly (`Cache.Get`, `Cache.Set`, `Cache.Delete`, etc.).

**`NoExpiration`** — re-exported constant from `go-cache`. Pass it as an expiry duration to
store an item indefinitely (it will only be evicted by an explicit `Cache.Delete`).

**`AgentCachePrefix`** — the string `"agent"`. All core agent packages must prefix their cache
keys with this constant (via `BuildAgentKey`) to avoid collisions with other code sharing the
same global cache.

**`BuildAgentKey(parts ...string) string`**

Builds a slash-separated cache key prefixed with `"agent"`. Allocation-optimised; do not
substitute with `path.Join` or `fmt.Sprintf`.

```go
key := cache.BuildAgentKey("clustername", "clusterID")
// returns "agent/clustername/clusterID"
```

**Generic read-through helpers**

```go
func Get[T any](key string, cb func() (T, error)) (T, error)
func GetWithExpiration[T any](key string, cb func() (T, error), expire time.Duration) (T, error)
```

These look up `key` in the global `Cache`. On a cache miss they call `cb`, store the result
(only on success — errors are never cached), and return it. `Get` stores with
`cache.NoExpiration`; `GetWithExpiration` uses the provided `expire` duration.

#### BasicCache

```go
type BasicCache struct { ... }
```

A thread-safe `map[string]interface{}` with no TTL or background purge. Suitable when the
owning component controls eviction manually.

| Method | Description |
|---|---|
| `NewBasicCache()` | Allocates and returns an empty `*BasicCache` |
| `Add(k, v)` | Inserts or overwrites an entry; updates the modification timestamp |
| `Get(k)` | Returns `(value, found)` |
| `Remove(k)` | Deletes an entry; updates the modification timestamp |
| `Size()` | Returns current number of entries |
| `GetModified()` | Returns Unix timestamp of last `Add` or `Remove` |
| `Items()` | Returns a shallow copy of the entire map |

## Usage

### Storing and retrieving a cluster ID

```go
// pkg/util/kubernetes/clustername/clustername.go
key := cache.BuildAgentKey(constants.ClusterIDCacheKey)
if id, found := cache.Cache.Get(key); found {
    return id.(string), nil
}
// ... compute clusterID ...
cache.Cache.Set(key, clusterID, cache.NoExpiration)
```

### Kubernetes metadata mapper

```go
// pkg/util/kubernetes/apiserver/apiserver.go
nodeKey := cache.BuildAgentKey(MetadataMapperCachePrefix, nodeName)
if bundle, found := cache.Cache.Get(nodeKey); found {
    return bundle.(*MetadataMapperBundle), nil
}
```

### JMX check state (BasicCache)

```go
// pkg/jmxfetch/state.go
type jmxState struct {
    configs *cache.BasicCache
}
s := &jmxState{configs: cache.NewBasicCache()}
s.configs.Add(checkID, config)
```

### Cluster-checks handler (BasicCache indirectly via global cache)

```go
// pkg/clusteragent/clusterchecks/stats.go and handler.go
key := cache.BuildAgentKey(handlerCacheKey)
cache.Cache.Set(key, stats, cache.NoExpiration)
```

### Read-through pattern with generic helper

```go
result, err := cache.GetWithExpiration("mykey", func() (MyType, error) {
    return expensiveComputation()
}, 2*time.Minute)
```

## Notes

- The global `Cache` is a single shared instance. Use `BuildAgentKey` to namespace keys and
  avoid cross-component key collisions.
- `BasicCache.Items()` returns a copy of the map, so iteration is safe after the call returns,
  but it does not reflect subsequent mutations.
- Neither cache type persists to disk; all data is lost on agent restart.

---

## Cross-references

| Topic | Document |
|---|---|
| `pkg/util/cachedfetch` — per-fetcher resilient cache for cloud provider metadata APIs; falls back to the last successful value on error rather than using the global TTL cache | [`pkg/util/cachedfetch`](cachedfetch.md) |
| `pkg/util/hostname` — uses the global `Cache` (via `BuildAgentKey`) to store the resolved hostname for the lifetime of the process; also uses `pkg/util/cachedfetch` internally for cloud-provider providers | [`pkg/util/hostname`](hostname.md) |
| `pkg/util/kubernetes/apiserver` — stores `MetadataMapperBundle` objects in the global `Cache` under `KubernetesMetadataMapping/<nodeName>` keys | [`pkg/util/kubernetes/apiserver`](kubernetes-apiserver.md) |

### Choosing between `pkg/util/cache` and `pkg/util/cachedfetch`

| Need | Package |
|---|---|
| Short-lived, TTL-based storage shared across many components | `pkg/util/cache` — global `Cache` with `BuildAgentKey` |
| No-expiry per-component storage with manual eviction | `pkg/util/cache` — `BasicCache` |
| Wrapping a single fallible fetch call (e.g. cloud IMDS) so failures return the last known good value | `pkg/util/cachedfetch` — `Fetcher` |
| Read-through with a custom expiry per call site | `pkg/util/cache.GetWithExpiration` |

### How the hostname package interacts with this cache

`pkg/util/hostname` resolves the agent hostname once and stores it under
`BuildAgentKey("hostname")` with `NoExpiration`. Subsequent calls to
`hostname.Get` return the cached value without re-running the provider chain,
making repeated lookups free. Drift detection (background goroutine) updates the
cache entry when the resolved hostname changes.
