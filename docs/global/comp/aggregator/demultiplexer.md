> **TL;DR:** Central hub between metric producers (checks, DogStatsD, APM) and the serialization/forwarding pipeline, owning multiple samplers and fanning samples out to the default, orchestrator, and event platform forwarders.

# comp/aggregator/demultiplexer — Metric Demultiplexer Component

**Import path:** `github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer`
**Team:** agent-metric-pipelines
**Importers:** ~32 packages

## Purpose

`comp/aggregator/demultiplexer` is the central hub that sits between metric producers (checks, DogStatsD, APM) and the serialization/forwarding pipeline. It owns multiple samplers — sharded DogStatsD time samplers, a check sampler inside the `BufferedAggregator`, an optional no-aggregation pass-through pipeline — and fans collected samples out to three forwarder paths:

1. **Default forwarder** (`comp/forwarder/defaultforwarder`) — metrics series, sketches, service checks, events
2. **Orchestrator forwarder** (`comp/forwarder/orchestrator`) — Kubernetes resource manifests
3. **Event Platform forwarder** (`comp/forwarder/eventplatform`) — DBM, container lifecycle, SBOM, etc.

It also provides the `SenderManager` interface used by checks (`Sender`) so that every check can submit metrics without knowing which sampler or forwarder they end up in.

## Package layout

| Package | Role |
|---|---|
| `comp/aggregator/demultiplexer` (root) | `Component`, `Mock`, `FakeSamplerMock` interfaces |
| `demultiplexerimpl/` | `Module()`, `Params`, `newDemultiplexer`, status provider, test helpers |
| `pkg/aggregator/` | `AgentDemultiplexer`, `Demultiplexer`, `DemultiplexerWithAggregator`, `BufferedAggregator`, time samplers |

The component struct in `demultiplexerimpl` is a thin wrapper over `aggregator.AgentDemultiplexer` from `pkg/aggregator`. The heavy logic (flush loops, bucket aggregation, serialization) lives in `pkg/aggregator`.

## Key Elements

### Key interfaces

## Component interface

```go
type Component interface {
    sender.SenderManager          // GetSender, SetSender, DestroySender, GetDefaultSender

    Serializer() serializer.MetricSerializer

    aggregator.DemultiplexerWithAggregator
    // which embeds aggregator.Demultiplexer, adding:
    //   Aggregator() *BufferedAggregator
    //   AggregateCheckSample(sample MetricSample)
    //   GetEventPlatformForwarder() (eventplatform.Forwarder, error)
    //   GetEventsAndServiceChecksChannels() (chan []*event.Event, chan []*servicecheck.ServiceCheck)
    //   DumpDogstatsdContexts(io.Writer) error

    AddAgentStartupTelemetry(agentVersion string)
}
```

`aggregator.Demultiplexer` (embedded transitively) adds the DogStatsD-facing API:

```go
AggregateSample(sample MetricSample)
AggregateSamples(shard TimeSamplerID, samples MetricSampleBatch)
SendSamplesWithoutAggregation(metrics MetricSampleBatch)   // no-aggregation pipeline
ForceFlushToSerializer(start time.Time, waitForSerializer bool)
GetMetricSamplePool() *metrics.MetricSamplePool
SetSamplersFilterList(filterList, histoFilterList utilstrings.Matcher)
```

### SenderManager

`sender.SenderManager` is the interface checks use to obtain a `Sender`:

```go
type SenderManager interface {
    GetSender(id checkid.ID) (Sender, error)
    SetSender(Sender, checkid.ID) error
    DestroySender(id checkid.ID)
    GetDefaultSender() (Sender, error)
}
```

The `Sender` returned to each check exposes `Gauge`, `Rate`, `Count`, `Histogram`, `Distribution`, `ServiceCheck`, `Event`, `EventPlatformEvent`, and related methods. Calling `Commit()` on a `Sender` moves the buffered samples into the appropriate time or check sampler.

### Key types

## fx wiring

```go
// cmd/agent/subcommands/run/command.go
demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams(
    demultiplexerimpl.WithDogstatsdNoAggregationPipelineConfig(),
)),
```

The constructor `newDemultiplexer` resolves the hostname, builds `AgentDemultiplexerOptions`, then calls `aggregator.InitAndStartAgentDemultiplexer`. It registers an `OnStop` lifecycle hook that calls `agentDemultiplexer.Stop(true)` (flush before stop).

### Provides (fx outputs)

In addition to `demultiplexer.Component`, the constructor provides:

| fx type | Description |
|---|---|
| `sender.SenderManager` | Used by the collector to inject `Sender`s into checks |
| `status.InformationProvider` | Powers the Aggregator section of `agent status` |
| `aggregator.Demultiplexer` | Non-component interface for packages in `pkg/` that predate the component system |

### Dependencies

| Dependency | Purpose |
|---|---|
| `defaultforwarder.Component` | Receives serialized metric payloads |
| `orchestratorforwarder.Component` | Receives orchestrator manifests |
| `eventplatform.Component` | Receives event platform events from the aggregator |
| `haagent.Component` | HA agent coordination |
| `tagger.Component` | Tag enrichment inside the aggregator |
| `compression.Component` | Metric payload compression |
| `hostnameinterface.Component` | Hostname attached to all metrics |
| `filterlist.Component` | Metric filter list applied inside time samplers |

### Key functions

## Params

```go
// Default (no special options)
demultiplexerimpl.NewDefaultParams()

// Honour dogstatsd_no_aggregation_pipeline config key
demultiplexerimpl.NewDefaultParams(
    demultiplexerimpl.WithDogstatsdNoAggregationPipelineConfig(),
)

// Continue even if hostname resolution fails (e.g. check runner)
demultiplexerimpl.NewDefaultParams(
    demultiplexerimpl.WithContinueOnMissingHostname(),
)

// Override the flush interval (e.g. test agent, short-lived processes)
demultiplexerimpl.NewDefaultParams(
    demultiplexerimpl.WithFlushInterval(10 * time.Second),
)
```

### Configuration and build flags

## AgentDemultiplexerOptions

`createAgentDemultiplexerOptions` maps `Params` to `aggregator.AgentDemultiplexerOptions`:

| Field | Default | Description |
|---|---|---|
| `FlushInterval` | 15 s | How often samplers flush to the serializer |
| `EnableNoAggregationPipeline` | false | Enable pass-through for pre-timestamped metrics |
| `DontStartForwarders` | false | Unit-test flag — skip forwarder startup |
| `UseDogstatsdContextLimiter` | false | Cap number of active DogStatsD contexts |

## Mock and test helpers

```go
// MockModule provides a real AgentDemultiplexer with forwarders disabled,
// plus a Mock interface with SetDefaultSender.
demultiplexerimpl.MockModule()

// FakeSamplerMockModule replaces the DogStatsD time samplers with a fake
// implementation that buffers samples for assertion.
// Use WaitForSamples() / WaitForNumberOfSamples() in tests.
demultiplexer.FakeSamplerMock
```

`FakeSamplerMock.Reset()` clears the sample buffer between test cases.

## Usage patterns

**Obtaining a Sender in an fx component (e.g. a check runner):**

```go
type deps struct {
    fx.In
    SenderManager sender.SenderManager
}

func (c *myComp) runCheck(id checkid.ID) error {
    s, err := c.SenderManager.GetSender(id)
    if err != nil { return err }
    s.Gauge("my.metric", 42.0, "", []string{"tag:value"})
    s.Commit()
    return nil
}
```

**Sending DogStatsD samples (from `comp/dogstatsd`):**

```go
// Batch submit to a specific time-sampler shard
demux.AggregateSamples(shardID, sampleBatch)
```

**Forcing an immediate flush (e.g. on agent shutdown):**

```go
demux.ForceFlushToSerializer(time.Now(), true /* waitForSerializer */)
```

**Getting the event platform forwarder from within the aggregator:**

```go
fwd, err := demux.GetEventPlatformForwarder()
if err == nil {
    fwd.SendEventPlatformEvent(msg, eventplatform.EventTypeContainerLifecycle)
}
```

## Related components and packages

| Component / Package | Relationship |
|---|---|
| [`comp/forwarder/defaultforwarder`](../forwarder/defaultforwarder.md) | Primary downstream: receives serialized `BytesPayloads` from `pkg/serializer`; wired into the demultiplexer's `AgentDemultiplexer` at construction |
| [`comp/forwarder/orchestrator`](../forwarder/orchestrator.md) | Optional downstream for Kubernetes/ECS orchestrator manifests; injected as `orchestratorforwarder.Component` and forwarded through `pkg/serializer.SendOrchestratorMetadata` |
| [`comp/forwarder/eventplatform`](../forwarder/eventplatform.md) | Downstream for structured event payloads (DBM, container lifecycle, SBOM, etc.); exposed via `GetEventPlatformForwarder()` on the aggregator |
| [`pkg/aggregator/aggregator`](../../pkg/aggregator/aggregator.md) | Houses the concrete `AgentDemultiplexer`, `BufferedAggregator`, and `TimeSampler` implementations that this component wraps |
| [`pkg/aggregator/sender`](../../pkg/aggregator/sender.md) | Defines the `Sender` and `SenderManager` interfaces; the demultiplexer provides `SenderManager` so checks can call `GetSender()` without depending on the aggregator directly |
| [`pkg/serializer`](../../pkg/serializer.md) | Constructed by this component and passed to `AgentDemultiplexer`; encodes aggregated payloads and routes them to the default forwarder |
| [`comp/serializer/metricscompression`](../serializer/metricscompression.md) | Injected into the demultiplexer fx graph and forwarded to the serializer for payload compression |

## Pipeline overview

```
DogStatsD server          Checks (Go / Python via Sender)
      |                           |
      | AggregateSamples()        | sender.Commit()
      v                           v
comp/aggregator/demultiplexer (AgentDemultiplexer)
  ├─ TimeSampler(s) — sharded DogStatsD aggregation
  ├─ BufferedAggregator — per-check CheckSampler + event/service-check queues
  └─ NoAggregation pipeline — pre-timestamped samples bypass aggregation
      |                           |                         |
      v                           v                         v
pkg/serializer          eventplatform.Forwarder    orchestrator.Forwarder
      |
      v
comp/forwarder/defaultforwarder → Datadog intake
```

## Key consumers

- `comp/dogstatsd/server` — pushes DogStatsD metric samples via `AggregateSamples`
- `comp/collector` / check runner — calls `GetSender` / `DestroySender` around each check run
- `cmd/agent`, `cmd/dogstatsd`, `cmd/cluster-agent`, `cmd/cluster-agent-cloudfoundry` — agent processes that start the demultiplexer
- `comp/snmptraps/forwarder` — retrieves the default sender to emit trap metrics
- `cli check` subcommand — uses `GetDefaultSender()` for one-shot check runs
