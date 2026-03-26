# comp/netflow/server

**Team:** ndm-integrations

## Purpose

`comp/netflow/server` is the top-level NetFlow collection component. When
enabled, it binds one UDP listener per configured flow type (NetFlow 5/9,
IPFIX, sFlow 5), decodes incoming datagrams using the goflow library, and
feeds decoded records into the flow aggregator for space/time aggregation
before forwarding to the Datadog backend.

The component is intentionally thin: it owns the listener lifecycle and wires
together the configuration, listeners, and aggregator. The aggregation logic
lives in `comp/netflow/flowaggregator`.

The server also optionally starts a Prometheus metrics endpoint (controlled by
`network_devices.netflow.prometheus_listener_enabled`) to expose internal
goflow metrics for debugging.

## Key elements

### Component interface

```go
// comp/netflow/server/component.go
type Component interface{}
```

The component exposes no public methods. All behavior is triggered by fx
lifecycle hooks (`OnStart` / `OnStop`). The `Component` type is registered in
the fx graph so that other components can declare a dependency on the server
being present without needing to call it directly.

### Server struct

```go
type Server struct {
    Addr      string
    config    *nfconfig.NetflowConfig
    listeners []*netflowListener
    FlowAgg   *flowaggregator.FlowAggregator
    logger    log.Component
    running   bool
}
```

- `Start()` — launches the `FlowAggregator` goroutine, optionally starts the
  Prometheus HTTP server, then calls `startFlowListener` for every entry in
  `config.Listeners`.
- `Stop()` — calls `FlowAgg.Stop()`, then shuts down each listener with a
  configurable `StopTimeout` (seconds). Each listener shutdown runs
  concurrently; a per-listener select enforces the timeout.

### netflowListener

An internal struct that wraps a `goflowlib.FlowStateWrapper` (the goflow state
machine for one protocol) alongside its config, an atomic error string, and a
flow counter used by the status page.

### Configuration (`NetflowConfig`)

Read from `network_devices.netflow.*` in `datadog.yaml`:

| Key | Purpose |
|-----|---------|
| `enabled` | Master switch for the entire component |
| `listeners` | List of `ListenerConfig` (flow_type, bind_host, port, workers, namespace, mapping) |
| `aggregator_buffer_size` | Capacity of the channel between listeners and aggregator |
| `aggregator_flush_interval` | How often (seconds) to flush aggregated flows |
| `aggregator_flow_context_ttl` | TTL (seconds) for idle flow contexts |
| `aggregator_max_flows_per_flush_interval` | Cap on flows per flush period (enables TopN filtering) |
| `stop_timeout` | Listener shutdown timeout (seconds) |
| `prometheus_listener_enabled` / `prometheus_listener_address` | Optional Prometheus endpoint |
| `reverse_dns_enrichment_enabled` | Enrich flow source/dest addresses with rDNS hostnames |

### Status provider

`newServer` registers a `status.InformationProvider` that exposes per-listener
state (flow count, last error) in the agent status output. This is only
registered when the server is enabled.

### fx wiring

```go
// Module registers the server with the fx container
func Module() fxutil.Module {
    return fxutil.Component(fx.Provide(newServer))
}
```

`newServer` depends on: `nfconfig.Component`, `log.Component`,
`demultiplexer.Component`, `forwarder.Component`, `hostname.Component`,
`rdnsquerier.Component`.

## Usage

The server is included via `comp/netflow/bundle.go`, which is composed into the
main agent in `cmd/agent/subcommands/run/command.go`. No caller code is
required beyond importing the bundle — the fx lifecycle hooks drive everything.

To enable NetFlow collection, add to `datadog.yaml`:

```yaml
network_devices:
  netflow:
    enabled: true
    listeners:
      - flow_type: netflow9
        port: 2055
      - flow_type: sflow5
        port: 6343
```

To add a new flow type or extend listener config, update `ListenerConfig` in
`comp/netflow/config/config.go` and add a corresponding goflow state factory in
`comp/netflow/goflowlib/flowstate.go`.

## Related components

| Component | Relationship |
|---|---|
| [`comp/netflow/flowaggregator`](flowaggregator.md) | Created and owned by the server. Receives decoded flow records from every `netflowListener` via the shared `flowIn` channel, performs space/time aggregation and port rollup, applies TopN filtering when configured, and forwards flushed payloads to the event platform forwarder. The server starts and stops it via `go server.FlowAgg.Start()` / `server.FlowAgg.Stop()`. |
| [`comp/rdnsquerier`](../rdnsquerier.md) | Injected into the server at construction time and passed to `flowaggregator.NewFlowAggregator`. The aggregator calls `GetHostnameAsync` for source and destination IPs during flow accumulation. When `reverse_dns_enrichment_enabled` is `false`, the no-op module (`rdnsquerierfx-none`) should be used to avoid overhead. |
| [`comp/forwarder/eventplatform`](../forwarder/eventplatform.md) | The flow aggregator holds an `eventplatform.Forwarder` reference (extracted from the `eventplatform.Component` injected into the server) and calls `SendEventPlatformEventBlocking` to send flushed flow records as `EventTypeNetworkDevicesNetFlow` events to the `ndmflow-intake.` pipeline. Exporter metadata is sent separately to `EventTypeNetworkDevicesMetadata`. |
