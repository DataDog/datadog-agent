# comp/forwarder/eventplatform — Event Platform Forwarder Component

**Import path:** `github.com/DataDog/datadog-agent/comp/forwarder/eventplatform`
**Team:** agent-log-pipelines
**Importers:** ~49 packages

## Purpose

`comp/forwarder/eventplatform` routes structured, typed event payloads to the Datadog Event Platform intake. Unlike the `defaultforwarder`, which handles raw metric and check-run series, this component owns a separate set of HTTP pipelines — one per event type — that apply their own batching, compression, and retry tuning independently of the metric pipeline.

Producers call `SendEventPlatformEvent` (non-blocking, drops on full channel) or `SendEventPlatformEventBlocking` (blocks until capacity is available) with a `*message.Message` and an event-type string constant such as `eventplatform.EventTypeSnmpTraps`. The component manages transport entirely; callers do not interact with HTTP, endpoints, or compression directly.

## Package layout

| Package | Role |
|---|---|
| `comp/forwarder/eventplatform` (root) | `Component` and `Forwarder` interfaces, event-type constants |
| `eventplatformimpl/` | `Module()`, `Params`, pipeline construction, `defaultEventPlatformForwarder` |

## Component interface

```go
// Component wraps an optional Forwarder. Get() returns (nil, false) when the
// forwarder is disabled (e.g. in processes that do not forward events).
type Component interface {
    Get() (Forwarder, bool)
}

type Forwarder interface {
    // Non-blocking send. Returns an error if the pipeline channel is full.
    SendEventPlatformEvent(e *message.Message, eventType string) error
    // Blocking send. Waits until the pipeline channel has space.
    SendEventPlatformEventBlocking(e *message.Message, eventType string) error
    // Purge drains all pipeline input channels and returns their contents.
    Purge() map[string][]*message.Message
}
```

`Component` is implemented as an `option.Option[Forwarder]`: it is absent when neither `UseEventPlatformForwarder` nor `UseNoopEventPlatformForwarder` is set in `Params`.

## Event types

The constants below are defined in the root package and used by producers to address the correct pipeline:

| Constant | Pipeline destination |
|---|---|
| `EventTypeNetworkDevicesMetadata` | `ndm-intake.` |
| `EventTypeSnmpTraps` | `snmp-traps-intake.` |
| `EventTypeNetworkDevicesNetFlow` | `ndmflow-intake.` |
| `EventTypeNetworkPath` | `netpath-intake.` |
| `EventTypeNetworkConfigManagement` | `ndm-intake.` |
| `EventTypeContainerLifecycle` | `contlcycle-intake.` (Protobuf) |
| `EventTypeContainerImages` | `contimage-intake.` (Protobuf) |
| `EventTypeContainerSBOM` | `sbom-intake.` (Protobuf) |
| `EventTypeSoftwareInventory` | `softinv-intake.` (guarded by `software_inventory.enabled`) |
| `EventTypeSynthetics` | `http-synthetics.` |
| `EventTypeEventManagement` | `event-management-intake.` (stream strategy, one event at a time) |

Private event types used internally by DBM and Data Streams (`dbm-samples`, `dbm-metrics`, `dbm-activity`, `dbm-metadata`, `dbm-health`, `data-streams-message`) are registered but their constants are not exported from the root package.

## Pipeline internals

Each registered event type gets a `passthroughPipeline`:

- An **input channel** sized by `defaultInputChanSize` (overridable via config prefix `<feature>.forwarder.input_chan_size`).
- A **strategy**: `BatchStrategy` for JSON types (groups messages up to `batch_max_size` / `batch_max_content_size` before sending), or `StreamStrategy` for Protobuf types and event-management (sends each message individually).
- An **HTTP sender** (`pkg/logs/sender/http`) that flushes to the configured endpoint(s) with up to `batch_max_concurrent_send` parallel requests.
- Optional **compression** read from the endpoint config; falls back to no compression.

Endpoint URLs are resolved from config using the per-pipeline `endpointsConfigPrefix` (e.g. `network_devices.metadata.`), with an additional `hostnameEndpointPrefix` used to build the default hostname.

## fx wiring

```go
// eventplatformimpl.Module is provided via the forwarder bundle in agent startup:
eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
eventplatformreceiverimpl.Module(),
```

### Params

```go
// NewDefaultParams — full forwarder enabled (normal agent start)
eventplatformimpl.NewDefaultParams()

// NewDisabledParams — no forwarder instantiated; Component.Get() returns (nil, false)
eventplatformimpl.NewDisabledParams()

// Noop variant — pipelines exist but senders are removed (useful for tests / non-forwarding processes)
// Pass Params{UseNoopEventPlatformForwarder: true, UseEventPlatformForwarder: false}
```

### Dependencies injected by fx

`eventplatformimpl.newEventPlatformForwarder` requires:
- `configcomp.Component` — reads per-pipeline endpoint config
- `hostnameinterface.Component` — used to build default endpoint hostnames and Data Streams headers
- `eventplatformreceiver.Component` — receives a copy of each message for debug streaming to console
- `logscompression.Component` — constructs per-pipeline compressors

## Connectivity diagnosis

`eventplatformimpl.Diagnose()` iterates all registered pipelines and performs a live connectivity check to each pipeline's primary endpoint. Results are returned as `[]diagnose.Diagnosis` and surfaced in `agent diagnose`. The event-management pipeline is skipped because its intake does not accept the empty probe payload.

## Usage patterns

**Sending from a component that has access to `eventplatform.Component`:**

```go
type deps struct {
    fx.In
    EPForwarder eventplatform.Component
}

func (c *myComp) sendPayload(msg *message.Message) error {
    fwd, ok := c.EPForwarder.Get()
    if !ok {
        return nil // forwarder disabled in this process
    }
    return fwd.SendEventPlatformEvent(msg, eventplatform.EventTypeSnmpTraps)
}
```

**Sending from a component that holds a `Forwarder` directly** (e.g. after extracting it at construction time):

```go
if err := c.epFwd.SendEventPlatformEventBlocking(msg, eventplatform.EventTypeNetworkDevicesNetFlow); err != nil {
    log.Warnf("failed to forward flow: %v", err)
}
```

`SendEventPlatformEvent` is appropriate when the caller is on a hot path (e.g. an aggregator loop) and dropping events under back-pressure is acceptable. Use the blocking variant for low-volume, high-priority payloads where dropping is not acceptable.

## Related components

| Component / Package | Relationship |
|---|---|
| [`comp/forwarder/defaultforwarder`](defaultforwarder.md) | Handles metric/check-run payloads over a separate HTTP pipeline; the two forwarders are independent — events never flow through the default forwarder |
| [`comp/forwarder/eventplatformreceiver`](eventplatformreceiver.md) | Diagnostic tap wired into every `passthroughPipeline`; calls `HandleMessage` on each forwarded message and powers `agent stream-event-platform` |
| [`comp/aggregator/demultiplexer`](../../comp/aggregator/demultiplexer.md) | Injects this component at construction time; exposes `GetEventPlatformForwarder()` so the aggregator can forward DBM, Data Streams, and container lifecycle events |
| [`pkg/sbom`](../../pkg/sbom.md) | The SBOM check (`pkg/collector/corechecks/sbom`) sends scan results as `EventTypeContainerSBOM` payloads through this forwarder |
| [`pkg/containerlifecycle`](../../pkg/containerlifecycle.md) | The container lifecycle check flushes `contlcycle.EventsPayload` messages as `EventTypeContainerLifecycle` via `sender.EventPlatformEvent`, which is ultimately routed here |

## Key consumers

- `comp/netflow/flowaggregator` — NetFlow flows (`EventTypeNetworkDevicesNetFlow`)
- `comp/snmptraps/forwarder` — SNMP traps (`EventTypeSnmpTraps`)
- `comp/networkpath/npcollector` — network path data (`EventTypeNetworkPath`)
- `comp/notableevents` / `comp/logonduration` — event management events
- `comp/syntheticstestscheduler` — Synthetics results
- `comp/softwareinventory` — software inventory payloads
- `pkg/aggregator/aggregator.go` — DBM, Data Streams, and container lifecycle/image/SBOM events forwarded through the aggregator's event platform bridge

## Data-flow context

The full path from a check to the intake for a structured event payload is:

```
Check / collector
  │  sender.EventPlatformEvent(rawBytes, eventType)
  ▼
pkg/aggregator.BufferedAggregator
  │  routes to the event platform forwarder via GetEventPlatformForwarder()
  ▼
comp/forwarder/eventplatform.Forwarder
  │  SendEventPlatformEvent(msg, eventType)
  ▼
passthroughPipeline (per event type)
  │  batches/streams → HTTP sender
  │  side-copies each message → eventplatformreceiver (diagnostic tap)
  ▼
Datadog Event Platform intake
```

Contrast with the metric pipeline, where data flows:
`pkg/aggregator → pkg/serializer → comp/forwarder/defaultforwarder`.
