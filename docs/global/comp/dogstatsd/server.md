# comp/dogstatsd/server — DogStatsD Server Component

**Import path:** `github.com/DataDog/datadog-agent/comp/dogstatsd/server`
**Team:** agent-metric-pipelines
**Importers:** ~10 packages

## Purpose

`comp/dogstatsd/server` is the core DogStatsD server. It listens on one or more transport sockets (UDP, Unix Domain Socket, Windows named pipe), receives StatsD-formatted payloads, parses them into metrics, events, and service checks, and forwards the resulting samples to the aggregator (`pkg/aggregator.Demultiplexer`).

Beyond raw parsing the server handles:
- **Origin detection** — on UDS listeners the kernel attaches process credentials; the server resolves them to a container ID for automatic tagging.
- **Metric enrichment** — applies a hostname, global extra tags (`dogstatsd_tags`), namespace prefixes, and cardinality/entity-ID logic before handing samples off.
- **Metric mapping** — optional regex-based metric name translation controlled by `dogstatsd_mapper_profiles`.
- **Traffic capture** — integrates with `comp/dogstatsd/replay` to record and replay live traffic.
- **Serverless mode** — a lightweight variant (`NewServerlessServer`) used by the Lambda extension that skips fx and flushes on demand.
- **Packet forwarding** — optionally forwards raw packets to a second StatsD host (`statsd_forward_host`).

## Package layout

All production code lives in the package root (`comp/dogstatsd/server`). There is no separate `def/` or `impl/` sub-package — the component interface, constructor, and implementation all reside in the same package.

| File | Role |
|---|---|
| `component.go` | `Component` interface, `Mock` interface, `Module()`, `MockModule()` |
| `params.go` | `Params` struct (`Serverless bool`) |
| `server.go` | `server` struct, `newServer` fx constructor, start/stop lifecycle, packet parsing |
| `server_worker.go` | `worker` goroutines that drain the packet channel and batch samples |
| `enrich.go` | Metric/event/service-check enrichment (tags, hostname, origin) |
| `parse*.go` | Low-level StatsD protocol parsers (metrics, events, service checks) |
| `batch.go` | `batcher` — accumulates parsed samples and flushes them to the demultiplexer |
| `serverless.go` | `ServerlessDogstatsd` — fx-free entry point for Lambda extension |
| `server_mock.go` | Mock implementation for tests |
| `stats_endpoint.go` | HTTP handler for `/dogstatsd-stats` (registered as an `api.AgentEndpointProvider`) |

## Component interface

```go
type Component interface {
    // IsRunning returns true if the server is actively listening.
    IsRunning() bool

    // ServerlessFlush forces all in-flight samples to be flushed to the
    // aggregator and serialised. Used by the Lambda extension on each
    // invocation boundary.
    ServerlessFlush(sketchesBucketDelay time.Duration)

    // UDPLocalAddr returns the bound UDP address (e.g. "127.0.0.1:8125").
    // Returns an empty string when UDP is disabled.
    UDPLocalAddr() string
}
```

## fx wiring

The server is wired through `server.Module(params)`, which is assembled with its siblings in `comp/dogstatsd/bundle.go`:

```go
// comp/dogstatsd/bundle.go
func Bundle(params server.Params) fxutil.BundleOptions {
    return fxutil.Bundle(
        serverdebugimpl.Module(),
        replayfx.Module(),
        pidmapimpl.Module(),
        server.Module(params),
        statusimpl.Module(),
    )
}
```

The constructor `newServer` declares its dependencies via the `dependencies` struct (fx.In) and produces a `provides` struct (fx.Out) with both `Component` and an `api.AgentEndpointProvider` for the `/dogstatsd-stats` HTTP route.

Key fx dependencies injected into the server:

| Dependency | Purpose |
|---|---|
| `aggregator.Demultiplexer` | Receives parsed metric samples |
| `replay.Component` | Traffic capture integration |
| `serverdebug.Component` | Per-metric debug stats |
| `pidmap.Component` | PID-to-container-ID mapping for origin detection |
| `filterlist.Component` | Dynamic metric name allow/deny lists |
| `workloadmeta.Component` (optional) | Container metadata for origin detection |
| `hostnameinterface.Component` | Default hostname for enrichment |

### Params

```go
type Params struct {
    Serverless bool // Set to true in the Lambda extension
}
```

Pass `Params{}` for normal agent use, or `Params{Serverless: true}` in serverless contexts.

## Internal architecture

```
Listeners (UDP / UDS datagram / UDS stream / named pipe)
    │  chan packets.Packets
    ▼
[optional forwarder goroutine → raw UDP forward]
    │
    ▼  chan packets.Packets
Workers (N goroutines, one per aggregator pipeline shard)
    │  parser → enrichMetricSample / enrichEvent / enrichServiceCheck
    ▼
Batcher → Demultiplexer (aggregator)
```

The number of worker goroutines is determined by `aggregator.GetDogStatsDWorkerAndPipelineCount()` and can be overridden with `dogstatsd_workers_count`.

Workers receive live filter-list updates from `filterlist.Component` via a dedicated channel so metric filtering can be changed at runtime without a restart.

## Configuration keys (selected)

| Key | Default | Description |
|---|---|---|
| `dogstatsd_port` | `8125` | UDP port; set to `0` to disable, `"__random__"` for an OS-assigned port |
| `dogstatsd_socket` | `""` | UDS datagram socket path |
| `dogstatsd_stream_socket` | `""` | UDS stream socket path (experimental) |
| `dogstatsd_pipe_name` | `""` | Windows named pipe name |
| `dogstatsd_origin_detection` | `false` | Enable UDS credential-based origin detection |
| `dogstatsd_tags` | `[]` | Extra tags appended to every metric |
| `dogstatsd_mapper_profiles` | `[]` | Regex metric name mapping rules |
| `dogstatsd_eol_required` | `[]` | Require newline termination on `udp`, `uds`, or `named_pipe` |
| `statsd_forward_host` | `""` | Forward raw packets to this host |
| `dogstatsd_metrics_stats_enable` | `false` | Collect per-metric debug statistics |

## Telemetry

The server registers several Prometheus-compatible metrics under the `dogstatsd` namespace:
- `dogstatsd_processed` (counter, labels: `message_type`, `state`, `origin`) — packets processed ok/error
- `dogstatsd_channel_latency` (histogram) — time to push a sample into the aggregator channel
- `dogstatsd_metric_type_count` (counter, label: `metric_type`) — count of each metric type received

Expvar counters (accessible at `/debug/vars`) track parse errors and packet counts per message type.

## Usage patterns

**Normal agent startup (via the bundle):**

```go
// cmd/agent/subcommands/run/command.go
dogstatsd.Bundle(server.Params{})
```

**Checking server state from a gRPC handler:**

```go
type Requires struct {
    compdef.In
    DogstatsdServer dogstatsdServer.Component
}

func (s *server) GetStatus() {
    if s.dogstatsdServer.IsRunning() { ... }
    addr := s.dogstatsdServer.UDPLocalAddr()
}
```

**Serverless (no fx):**

```go
srv, err := server.NewServerlessServer(demux, extraTags)
// later, at invocation boundary:
srv.ServerlessFlush(10 * time.Second)
```

**Testing with the mock:**

```go
fx.Provide(server.MockModule())
// or directly:
mock := &serverMock{} // see server_mock.go
```

### End-to-end metric flow

The following shows the full path a UDP metric takes from the socket to Datadog:

```
1. UDPListener (comp/dogstatsd/listeners)
       reads datagram
       → packets.Pool.Get() → Packet
       → packets.Assembler.AddMessage()
       → packets.Buffer.Append()
       → chan packets.Packets → server.packetsIn

2. server worker goroutine
       → parse*(rawPacket) → MetricSample / Event / ServiceCheck
       → enrich.go: tagger.EnrichTags(tb, OriginInfo{ContainerIDFromSocket, ...})
                    adds container/pod tags resolved via pkg/tagger
       → if replay active: replay.Enqueue(CaptureBuffer{...})
       → serverDebug.StoreMetricStats(sample)  (if debug enabled)
       → batcher.appendSample(sample)

3. batcher
       → demux.AggregateSamples(shardID, batch)
           (comp/aggregator/demultiplexer)

4. AgentDemultiplexer → TimeSampler → pkg/serializer → defaultforwarder
       → Datadog intake
```

### Registering the /dogstatsd-stats HTTP endpoint

The server's provides struct emits an `api.AgentEndpointProvider` so
`comp/api/api` wires the endpoint automatically — no direct dependency on the
HTTP server is needed:

```go
// server.go constructor output
return provides{
    Component: s,
    Endpoint:  api.NewAgentEndpointProvider(s.writeStats, "/dogstatsd-stats", "GET"),
}
```

### Enabling debug stats

```bash
# Toggle at runtime (no restart required)
datadog-agent config set dogstatsd_metrics_stats_enable true

# Read current per-metric stats
datadog-agent dogstatsd-stats
```

Internally: `server` checks `serverDebug.IsDebugEnabled()` before calling
`serverDebug.StoreMetricStats(sample)` on every parsed metric, keeping the
hot path allocation-free when debug is off. See
[serverDebug.md](serverDebug.md) for the full debug component API.

### Traffic capture and replay

```go
// Trigger via gRPC (comp/api/grpcserver/impl-agent/grpc.go):
path, err := capture.StartCapture(req.GetPath(), duration, compressed)
// ...
capture.StopCapture()
```

Internally, when a capture starts:
1. `replay.Component` switches packet-pool managers to non-passthrough mode.
2. Each listener wraps packets in a `CaptureBuffer` and calls `replay.Enqueue`.
3. On stop, `TrafficCaptureWriter` finalises the `.dog` file and appends a
   snapshot of tagger state for tag-accurate replay.

See [replay.md](replay.md) for the full capture API and file format.

## Related components

The server sits at the junction of several other components. Understanding their roles helps when tracing the path of a metric from socket to Datadog intake.

| Component | Doc | Relationship to server |
|---|---|---|
| `comp/dogstatsd/listeners` | [listeners.md](listeners.md) | Library (not a component) instantiated directly by the server during `start()`. Provides `UDPListener`, `UDSDatagramListener`, `UDSStreamListener`, and `NamedPipeListener`, each pushing `packets.Packets` batches into the server's `packetsIn` channel. |
| `comp/dogstatsd/packets` | [packets.md](packets.md) | Defines `Packet`, `Packets`, `Pool`, `PoolManager`, `Buffer`, and `Assembler`. The server holds a `PoolManager[Packet]` and calls `Put` after parsing each batch. When replay is active `SetPassthru(false)` is called so both the server and the capture writer hold a reference. |
| `comp/dogstatsd/replay` | [replay.md](replay.md) | Traffic-capture component. Injected into the server; listeners call `replay.Enqueue` per-packet when a capture is active. The server registers the shared pool managers with `replay.RegisterSharedPoolManager` / `RegisterOOBPoolManager` and calls `StopCapture` on shutdown. Triggered via the gRPC API (see `comp/api/grpcserver`). |
| `comp/dogstatsd/serverDebug` | [serverDebug.md](serverDebug.md) | Per-metric debug statistics. The server calls `StoreMetricStats` on every processed sample (guarded by `IsDebugEnabled()`). Toggle at runtime with `dogstatsd_metrics_stats_enable`. |
| `comp/aggregator/demultiplexer` | [../../aggregator/demultiplexer.md](../../aggregator/demultiplexer.md) | Primary downstream. The server's batcher calls `demux.AggregateSamples(shardID, batch)` to hand off parsed metric samples. The demultiplexer's time samplers aggregate them and flush to `pkg/serializer` → `comp/forwarder/defaultforwarder`. |
| `comp/api/api` | [../../api/api.md](../../api/api.md) | The server's constructor produces an `api.AgentEndpointProvider` that registers the `/dogstatsd-stats` HTTP route on the CMD API server. |
| `comp/api/grpcserver` | [../../api/grpcserver.md](../../api/grpcserver.md) | The agent's gRPC implementation (`impl-agent`) injects `dogstatsdServer.Component` to expose `IsRunning`, `UDPLocalAddr`, and replay control (`DogstatsdCaptureTrigger` / `DogstatsdStopCapture`) over the authenticated `AgentSecureServer`. |
| `pkg/tagger` | [../../../pkg/tagger.md](../../../pkg/tagger.md) | Tag enrichment. `enrich.go` calls `tagger.EnrichTags(tb, OriginInfo{...})` on every metric sample to attach container/pod tags resolved from UDS peer credentials (`ContainerIDFromSocket`), inline client IDs, and admission-webhook external data. Origin detection on UDS sockets is a Linux-only kernel feature. |

## Key importers

| Package | How it uses the server |
|---|---|
| `comp/dogstatsd/bundle.go` | Wires the bundle |
| `cmd/agent/subcommands/run/command.go` | Starts the server as part of the main agent |
| `cmd/dogstatsd/subcommands/start/command.go` | Standalone DogStatsD binary |
| `comp/api/grpcserver/impl-agent/grpc.go` | Exposes server state / capture control over gRPC |
| `pkg/serverless/metrics/metric.go` | Lambda extension: creates a serverless server |
| `pkg/jmxfetch/jmxfetch.go` | Reads `UDPLocalAddr()` to tell JMXFetch where to send metrics |
