# comp/rdnsquerier — Reverse DNS Querier Component

**Import path:** `github.com/DataDog/datadog-agent/comp/rdnsquerier/def`
**Team:** ndm-integrations
**Importers:** `comp/netflow` (flow accumulator, server), `comp/networkpath/npcollector`

## Purpose

`comp/rdnsquerier` resolves private IP addresses to hostnames using reverse DNS lookups. It enriches network telemetry — NetFlow records and Network Path traces — with human-readable hostnames without blocking data pipelines.

The component is only active when at least one of the following is enabled:

- `network_devices.netflow.reverse_dns_enrichment_enabled: true`
- Both `network_path.connections_monitoring.enabled: true` and `network_path.collector.reverse_dns_enrichment.enabled: true`

When neither condition holds, `NewComponent` returns a no-op implementation (`impl-none`) transparently, so callers need not check whether the feature is on.

## Package layout

| Package | Role |
|---|---|
| `comp/rdnsquerier/def` | Component interface and `ReverseDNSResult` type |
| `comp/rdnsquerier/impl` | Full implementation with worker pool, cache, and rate limiter |
| `comp/rdnsquerier/impl-none` | No-op implementation (all methods return immediately) |
| `comp/rdnsquerier/fx` | fx `Module()` wiring `impl` |
| `comp/rdnsquerier/fx-none` | fx `Module()` wiring `impl-none` |
| `comp/rdnsquerier/fx-mock` | fx mock module for tests |
| `comp/rdnsquerier/mock` | Mock implementation for unit tests |

## Component interface

```go
// Package: github.com/DataDog/datadog-agent/comp/rdnsquerier/def

type ReverseDNSResult struct {
    IP       string
    Hostname string
    Err      error
}

type Component interface {
    // GetHostnameAsync resolves a single IP (as a []byte) without blocking.
    // If the result is already cached, updateHostnameSync is called immediately.
    // Otherwise the lookup is queued and updateHostnameAsync is called when done.
    // Only IPs in private address space are resolved; others are silently ignored.
    GetHostnameAsync(ipAddr []byte, updateHostnameSync func(string), updateHostnameAsync func(string, error)) error

    // GetHostname resolves a single IP string synchronously, honouring ctx for timeout.
    GetHostname(ctx context.Context, ipAddr string) (string, error)

    // GetHostnames resolves multiple IPs concurrently and returns a map keyed by IP.
    GetHostnames(ctx context.Context, ipAddrs []string) map[string]ReverseDNSResult
}
```

## Implementation internals

The full implementation (`comp/rdnsquerier/impl`) is composed of three layers:

**Querier** — a pool of worker goroutines (`reverse_dns_enrichment.workers`, default 10) that drain a buffered channel (`chan_size`, default 5000) of pending lookups. Each worker calls `net.LookupAddr` and invokes the callback with the result.

**Rate limiter** — an adaptive token-bucket limiter that automatically throttles to a lower rate (`limit_throttled_per_sec`, default 1/s) when lookups start failing, and recovers back to the normal rate (`limit_per_sec`, default 1000/s) after a configurable number of quiet intervals. Controlled by the `reverse_dns_enrichment.rate_limiter.*` config keys.

**Cache** — a read-through in-memory map keyed by IP string. On a cache hit the sync callback is invoked inline; on a miss a worker query is enqueued and async callbacks are registered on the in-progress entry. Failed lookups are retried up to `max_retries` times. The cache is periodically cleaned (`clean_interval`) and persisted to disk (`persist_interval`) via `pkg/persistentcache` so warm entries survive agent restarts. Controlled by the `reverse_dns_enrichment.cache.*` config keys.

## fx wiring

```go
import rdnsquerierfx "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx"

// In your fx app:
rdnsquerierfx.Module()
```

The module requires `config.Component`, `log.Component`, and `telemetry.Component`. It registers `OnStart`/`OnStop` hooks on `compdef.Lifecycle` to start/stop the worker pool and cache.

For processes that never need DNS enrichment, use the no-op module instead:

```go
import rdnsquerierfxnone "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx-none"
rdnsquerierfxnone.Module()
```

## Usage

**Injecting as a dependency:**

```go
import rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"

type Requires struct {
    fx.In
    RDNSQuerier rdnsquerier.Component
}

// Batch synchronous resolution (used by npcollector):
results := c.rdnsQuerier.GetHostnames(ctx, []string{"10.0.0.1", "192.168.1.1"})
for ip, r := range results {
    if r.Err == nil && r.Hostname != "" {
        // use r.Hostname
    }
}

// Async enrichment during flow accumulation (used by netflow):
err := c.rdnsQuerier.GetHostnameAsync(
    ipBytes,
    func(h string) { flow.SrcHostname = h },          // sync: cache hit
    func(h string, err error) { flow.SrcHostname = h }, // async: resolved later
)
```

**Key callers:**

- [`comp/netflow/flowaggregator`](netflow/flowaggregator.md) — calls `GetHostnameAsync` for source and destination IPs of each flow during accumulation. When `network_devices.netflow.reverse_dns_enrichment_enabled` is `false`, the netflow server injects the no-op module so calls are free.
- [`comp/networkpath/npcollector`](networkpath/npcollector.md) — calls `GetHostnames` to batch-resolve destination and hop IPs for each completed traceroute path before forwarding the event to the event platform. When `network_path.collector.reverse_dns_enrichment.enabled` is `false`, the no-op module is used.

**Choosing the right fx module:**

```go
// Processes that may need rDNS enrichment (netflow server, npcollector):
import rdnsquerierfx "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx"
rdnsquerierfx.Module()  // activates the real implementation only if config enables it

// Processes that never need rDNS enrichment (e.g. process-agent, security-agent):
import rdnsquerierfxnone "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx-none"
rdnsquerierfxnone.Module()  // always wires the no-op; zero overhead
```

## Configuration keys

| Key | Default | Description |
|---|---|---|
| `reverse_dns_enrichment.workers` | 10 | Number of lookup worker goroutines |
| `reverse_dns_enrichment.chan_size` | 5000 | Capacity of the pending-lookup channel |
| `reverse_dns_enrichment.cache.enabled` | — | Enable the in-memory + persistent cache |
| `reverse_dns_enrichment.cache.entry_ttl` | 24h | Time before a cached entry is considered stale |
| `reverse_dns_enrichment.cache.max_size` | 1 000 000 | Maximum number of cached entries |
| `reverse_dns_enrichment.cache.max_retries` | 10 | Retries for a failed lookup before giving up |
| `reverse_dns_enrichment.rate_limiter.enabled` | — | Enable the adaptive rate limiter |
| `reverse_dns_enrichment.rate_limiter.limit_per_sec` | 1000 | Normal lookup rate |
| `reverse_dns_enrichment.rate_limiter.limit_throttled_per_sec` | 1 | Throttled lookup rate after repeated errors |

## Telemetry

The implementation emits counters and gauges under the `reverse_dns_enrichment` module name, covering total requests, cache hits/misses/expiries, dropped requests (channel full or rate-limiter), lookup errors by category (not-found, timeout, temporary, other), and successful resolutions. The current cache size is tracked as a gauge updated every 30 s.

## Related components

| Component | Relationship |
|---|---|
| [`comp/netflow/flowaggregator`](netflow/flowaggregator.md) | The flow accumulator enriches source and destination IPs of each aggregated flow with reverse-DNS hostnames. It calls `GetHostnameAsync` inside `flowAccumulator.add`, so that enrichment happens asynchronously without blocking the `flowIn` channel. The sync callback (`updateHostnameSync`) is invoked inline on a cache hit; the async callback updates `flow.SrcHostname` / `flow.DstHostname` when the lookup completes later. Enrichment is gated on `network_devices.netflow.reverse_dns_enrichment_enabled`. |
| [`comp/networkpath/npcollector`](networkpath/npcollector.md) | After each traceroute completes, the collector batch-resolves all destination and intermediate hop IPs by calling `GetHostnames`. The resolved hostnames are embedded in the `NetworkPath` payload before it is forwarded to the event platform as `EventTypeNetworkPath`. Enrichment is gated on `network_path.collector.reverse_dns_enrichment.enabled`; when the feature is off, the no-op module is injected so the `GetHostnames` call is free. |
