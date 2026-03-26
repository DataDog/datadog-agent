# comp/networkpath/npcollector — Network Path Collector Component

**Import path:** `github.com/DataDog/datadog-agent/comp/networkpath/npcollector`
**Team:** network-path
**Importers:** ~13 packages

## Purpose

`comp/networkpath/npcollector` (Network Path Collector) continuously monitors live network connections observed by system-probe and runs traceroutes to the remote endpoints of those connections. It is the automated, traffic-driven counterpart to the on-demand `network_path` check: instead of a user scheduling individual checks, the collector subscribes to the agent's connection stream and automatically discovers which destinations to probe.

The workflow is:

1. **Scheduling** — callers pass an iterator of observed connections via `ScheduleNetworkPathTests`. Each connection is filtered (intra-host, intra-VPC, CIDR/domain exclusions, and custom `ConnFilter` rules) and transformed into a `Pathtest` (hostname + port + protocol).
2. **Deduplication** — the `PathtestStore` deduplicates and rate-limits pathtests so that the same destination is not re-probed more often than configured.
3. **Execution** — a pool of worker goroutines drains the processing channel and calls `traceroute.Component.Run` for each pathtest.
4. **Enrichment & forwarding** — completed paths are optionally enriched with reverse-DNS hostnames (via `rdnsquerier`) and forwarded to the Datadog event platform as `EventTypeNetworkPath` events.

When `network_path.connections_monitoring.enabled` is `false` the component is instantiated as a no-op that silently discards all scheduling requests.

## Package layout

| Package | Role |
|---|---|
| `comp/networkpath/npcollector` (root) | `Component` interface and `component_mock.go` |
| `npcollectorimpl/model` | `NetworkPathConnection` input type |
| `npcollectorimpl` | Core implementation: scheduling, flush loop, worker pool |
| `npcollectorimpl/pathteststore` | Deduplication and rate-limit store for pathtests |
| `npcollectorimpl/connfilter` | Configurable inclusion/exclusion filters for connections |
| `npcollectorimpl/common` | Shared constants and `Pathtest` / `PathtestMetadata` types |

## Component interface

```go
// Package: github.com/DataDog/datadog-agent/comp/networkpath/npcollector
type Component interface {
    ScheduleNetworkPathTests(conns iter.Seq[npmodel.NetworkPathConnection])
}
```

`ScheduleNetworkPathTests` is the only public method. It must be non-blocking: connections are enqueued into an internal buffered channel (`pathtestInputChan`) and dropped with a warning log if the channel is full.

### `npmodel.NetworkPathConnection` (key fields)

Defined in `comp/networkpath/npcollector/model`:

| Field | Description |
|---|---|
| `Source` / `Dest` | Source and destination `netip.AddrPort` |
| `TranslatedDest` | NAT-translated destination (used for VPC subnet filtering) |
| `Type` | `ConnectionType` (TCP, UDP, …) |
| `Direction` | Only `outgoing` connections are tested |
| `Family` | IPv4 or IPv6 (IPv6 is skipped unless a domain name is present) |
| `Domain` | Reverse-DNS hostname of the destination, used as probe target when available |
| `IntraHost` | If true, the connection is skipped |
| `SystemProbeConn` | If true, the connection is skipped (avoids self-monitoring) |
| `SourceContainerID` | Propagated to the path's source metadata |

## fx wiring

```go
import "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"

npcollectorimpl.Module()
```

The implementation depends on:
- `eventplatform.Component` — to send completed path events
- `traceroute.Component` — to execute individual traceroutes
- `rdnsquerier.Component` — for reverse-DNS enrichment of hop IPs
- `config.Component` — all tuning parameters are read from `network_path.collector.*`
- `log.Component` and `statsd.ClientInterface` for observability

Lifecycle hooks start the flush loop and worker goroutines on `OnStart` and shut them down gracefully (draining all channels) on `OnStop`.

## Configuration

All settings live under `network_path.collector.*` in `datadog.yaml`:

| Key | Default | Description |
|---|---|---|
| `network_path.connections_monitoring.enabled` | `false` | Master switch; disables the component entirely when `false` |
| `network_path.collector.workers` | — | Number of concurrent traceroute workers |
| `network_path.collector.max_ttl` | — | Maximum TTL (hops) per traceroute |
| `network_path.collector.timeout` | — | Per-probe timeout in milliseconds |
| `network_path.collector.pathtest_interval` | — | Minimum interval between re-probing the same destination |
| `network_path.collector.pathtest_contexts_limit` | — | Max number of tracked destinations in the store |
| `network_path.collector.flush_interval` | — | How often the store flushes due pathtests to workers |
| `network_path.collector.reverse_dns_enrichment.enabled` | `false` | Enable reverse-DNS enrichment of hop IPs |
| `network_path.collector.disable_intra_vpc_collection` | `false` | Skip connections whose translated destination is in the host's VPC subnets |
| `network_path.collector.source_excludes` / `dest_excludes` | — | CIDR-based exclusion maps |
| `network_path.collector.filters` | — | Domain/IP inclusion filter rules (`connfilter.Config` slice) |
| `network_path.collector.tcp_method` | — | `SYN` or `ACK` for TCP probes |
| `network_path.collector.icmp_mode` | — | Override to force ICMP probes |

## Usage

The component is consumed in two main patterns:

**1. Connection check integration (`connectionscheckimpl`)**

The connections check calls `ScheduleNetworkPathTests` with the current connection snapshot after each collection cycle. This is the primary source of connections.

```go
// comp/process/connectionscheck/connectionscheckimpl/check.go
checks.NewConnectionsCheck(..., deps.NpCollector, ...)
```

**2. Direct network sender integration**

`pkg/network/sender` (Linux) calls `ScheduleNetworkPathTests` directly from the network tracer subsystem:

```go
// pkg/network/sender/sender_linux.go
s.npCollector.ScheduleNetworkPathTests(connections)
```

**Internal flow:**

```
ScheduleNetworkPathTests
  └─ filter connections
       └─ pathtestInputChan (buffered)
            └─ listenPathtests goroutine → PathtestStore
                 └─ flushLoop (ticker) → pathtestProcessingChan
                      └─ worker goroutines → traceroute.Run()
                           └─ reverse-DNS enrichment
                                └─ epForwarder.SendEventPlatformEventBlocking(EventTypeNetworkPath)
```

## Observability

Internal StatsD metrics are emitted under the `datadog.network_path.collector.*` prefix, including:

- `schedule.conns_received`, `schedule.pathtest_count`, `schedule.pathtest_dropped` — scheduling throughput
- `flush.pathtest_count`, `flush.pathtest_dropped` — flush loop metrics
- `worker.task_duration`, `worker.pathtest_processed`, `worker.pathtest_interval` — worker performance
- `reverse_dns_lookup.failures`, `reverse_dns_lookup.successes` — rDNS enrichment outcomes

## Related components and packages

| Component / Package | Relationship |
|---|---|
| [`comp/networkpath/traceroute`](traceroute.md) | Executes individual traceroutes on behalf of this collector. `npcollector` injects `traceroute.Component` and calls `Run` per pathtest. The local or remote implementation is selected by the binary's fx wiring (core agent vs. process-agent). `npcollector` sets `ReverseDNS: false` in the config it passes to `Run` and handles rDNS itself via `rdnsquerier`. |
| [`pkg/networkpath`](../../pkg/networkpath.md) | Defines the `payload.NetworkPath` return type from `traceroute.Run`, the `config.Config` input, and `payload.ValidateNetworkPath` used before forwarding. Also defines the `PathOrigin`, `TestRunType`, and `SourceProduct` enums embedded in forwarded events. |
| [`comp/rdnsquerier`](../rdnsquerier.md) | Resolves destination and hop IP addresses to hostnames after a traceroute completes. The collector calls `GetHostnames` in batch; rDNS enrichment is gated on `network_path.collector.reverse_dns_enrichment.enabled`. When the feature is off, the no-op implementation is injected and the call is a no-op. |
| [`comp/forwarder/eventplatform`](../forwarder/eventplatform.md) | Completed, enriched `NetworkPath` payloads are forwarded as `EventTypeNetworkPath` events via `SendEventPlatformEventBlocking`. The blocking variant is used here because dropping path events under back-pressure is not acceptable. |
