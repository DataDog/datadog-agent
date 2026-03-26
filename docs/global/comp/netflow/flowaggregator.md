# comp/netflow/flowaggregator

**Team:** ndm-integrations

## Purpose

`comp/netflow/flowaggregator` performs space and time aggregation of raw NetFlow
records before they are sent to Datadog. Rather than forwarding every individual
flow record, the aggregator:

1. Merges flows that share the same five-tuple (and namespace) within a
   configurable time window, summing byte/packet counters and OR-ing TCP flags.
2. Applies port rollup to consolidate high-cardinality ephemeral source/dest
   ports.
3. Optionally enforces a TopN cap on the number of flows emitted per flush
   period.
4. Enriches source and destination IP addresses with reverse-DNS hostnames
   (optional, via `rdnsquerier`).
5. Sends the resulting payloads to the event-platform forwarder (for flow
   records) and the NDM metadata endpoint (for exporter metadata).

This package is not an fx component by itself. It is instantiated and owned by
`comp/netflow/server`, which passes it the configured sender, forwarder, and
hostname at construction time.

## Key elements

### FlowAggregator

```go
type FlowAggregator struct {
    flowIn          chan *common.Flow         // inbound channel from listeners
    FlushConfig     common.FlushConfig        // flush interval and tick frequency
    flowAcc         *flowAccumulator          // keyed store of in-flight flows
    sender          sender.Sender             // metric sender
    epForwarder     eventplatform.Forwarder   // event-platform forwarder
    hostname        string
    flowFilter      FlowFlushFilter           // optional TopN filter
    ...
}
```

**`NewFlowAggregator`** — constructor; accepts a `sender.Sender`,
`eventplatform.Forwarder`, `*config.NetflowConfig`, hostname string,
`log.Component`, and `rdnsquerier.Component`. Returns a ready-to-use
(but not yet started) aggregator.

**`Start()`** — launches the `run` goroutine (reads `flowIn` and feeds
`flowAccumulator`) then blocks in `flushLoop`. Should be called in its own
goroutine by the server.

**`Stop()`** — closes `stopChan`, then waits for both `flushLoopDone` and
`runDone` to confirm clean shutdown.

**`GetFlowInChan() chan *common.Flow`** — returns the inbound channel. Each
`netflowListener` writes decoded flows here.

### flowAccumulator

An internal type that stores in-flight flows keyed by their aggregation hash
(`common.Flow.AggregationHash()`).

- `add(flow)` — merges a new flow into the accumulator; applies port rollup;
  triggers async rDNS enrichment on the first occurrence of a hash.
- `flush(ctx)` — returns all flows whose `nextFlush` time has been reached,
  resets their counters, and removes contexts that have exceeded their TTL.

### FlowFlushFilter interface

```go
type FlowFlushFilter interface {
    Filter(flushCtx common.FlushContext, flows []*common.Flow) []*common.Flow
}
```

Used to apply TopN (highest bytes/packets) filtering before emission. When
`aggregator_max_flows_per_flush_interval > 0` the aggregator uses
`topn.NewPerFlushFilter`; otherwise `topn.NoopFilter{}` is used.

### Flush loop

The `flushLoop` runs two tickers:

| Ticker | Controlled by | Action |
|--------|---------------|--------|
| `flushFlowsToSendTicker` | `FlushConfig.FlushTickFrequency` (10 s fixed) | Calls `flush()`, sends flows and exporter metadata, commits metrics |
| `rollupTrackersRefresh` | `aggregator_rollup_tracker_refresh_interval` | Promotes the "new" port-rollup store to "current" |

### Sequence-number tracking

The aggregator tracks the maximum sequence number seen per exporter (keyed by
namespace + exporter IP + flow type). On each flush it computes the delta and
emits `datadog.netflow.aggregator.sequence.delta`. A large negative delta
triggers a reset event (`datadog.netflow.aggregator.sequence.reset`).

### Telemetry metrics emitted

All metrics share the prefix `datadog.netflow.`:

| Metric | Type | Description |
|--------|------|-------------|
| `aggregator.flows_received` | MonotonicCount | Total flows received |
| `aggregator.flows_flushed` | Count | Flows forwarded per flush |
| `aggregator.flows_contexts` | Gauge | Number of active flow contexts |
| `aggregator.hash_collisions` | MonotonicCount | Aggregation hash collisions |
| `aggregator.flush_interval` | Gauge | Actual interval between flushes |
| `aggregator.input_buffer.length` | Gauge | Current depth of `flowIn` channel |
| `aggregator.port_rollup.current_store_size` | Gauge | Port rollup store size |
| `aggregator.sequence.delta` | Count | Per-exporter sequence number delta |

In addition, Prometheus metrics collected from goflow are translated and
submitted under the same prefix.

## Usage

`FlowAggregator` is created inside `comp/netflow/server.newServer`:

```go
flowAgg := flowaggregator.NewFlowAggregator(
    sender, deps.Forwarder, conf,
    deps.Hostname.GetSafe(ctx), deps.Logger, rdnsQuerier,
)
```

Each listener then writes to `flowAgg.GetFlowInChan()`. The server starts the
aggregator with `go server.FlowAgg.Start()` and stops it via
`server.FlowAgg.Stop()`.

To tune aggregation behavior, adjust the `aggregator_*` keys under
`network_devices.netflow` in `datadog.yaml`. To extend the port-rollup logic,
see `comp/netflow/portrollup`. To add a new flush filter strategy, implement
`FlowFlushFilter` and wire it in `NewFlowAggregator`.

## Related components

| Component | Relationship |
|---|---|
| [`comp/netflow/server`](server.md) | Owns the `FlowAggregator` instance. The server constructs it via `NewFlowAggregator`, calls `go server.FlowAgg.Start()` after starting each UDP listener, and calls `server.FlowAgg.Stop()` on shutdown. Each `netflowListener` inside the server writes decoded `*common.Flow` records to `flowAgg.GetFlowInChan()`. |
| [`comp/rdnsquerier`](../rdnsquerier.md) | Injected at construction time. The `flowAccumulator.add` method calls `rdnsQuerier.GetHostnameAsync` for source and destination IPs on the first occurrence of each aggregation hash. A cache hit triggers the sync callback inline; a cache miss enqueues the lookup and the async callback updates `flow.SrcHostname` / `flow.DstHostname` when the lookup completes. Enable with `network_devices.netflow.reverse_dns_enrichment_enabled: true`; when disabled, the no-op module (`rdnsquerierfx-none`) is injected so the calls are free. |
| [`comp/forwarder/eventplatform`](../forwarder/eventplatform.md) | The aggregator holds an `eventplatform.Forwarder` reference extracted from the `eventplatform.Component` passed by the server. On every flush it calls `SendEventPlatformEventBlocking` with `EventTypeNetworkDevicesNetFlow` for flow records and `EventTypeNetworkDevicesMetadata` for exporter metadata. The blocking variant is used because the flush loop runs on a dedicated goroutine and dropping flows under back-pressure is not acceptable. |

### Lifecycle and shutdown safety

`FlowAggregator` uses two done-channels (`flushLoopDone`, `runDone`) to signal completion of its goroutines. `Stop()` closes `stopChan`, then waits on both channels before returning. This guarantees that no goroutine writes to `flowIn` or calls the forwarder after `Stop()` returns — avoiding the send-on-closed-channel panic that is a common pitfall in components with concurrent goroutines.

### Data flow summary

```
UDP datagrams
        │  (one listener per flow type: netflow5/9, ipfix, sflow5)
        ▼
comp/netflow/server.netflowListener  (goflow decoder)
        │  *common.Flow  →  flowAgg.GetFlowInChan()
        ▼
flowAccumulator.add()
        ├─ merges byte/packet counters by AggregationHash
        ├─ applies port rollup (comp/netflow/portrollup)
        └─ triggers rDNS enrichment (comp/rdnsquerier.GetHostnameAsync)
        │
        │  (every FlushConfig.FlushTickFrequency, default 10 s)
        ▼
flowAccumulator.flush()
        │  applies FlowFlushFilter (TopN or noop)
        ▼
eventplatform.Forwarder.SendEventPlatformEventBlocking
        │  EventTypeNetworkDevicesNetFlow  →  ndmflow-intake.
        │  EventTypeNetworkDevicesMetadata →  ndm-intake.
        ▼
Datadog Event Platform intake
```
