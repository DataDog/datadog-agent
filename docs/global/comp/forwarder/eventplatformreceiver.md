> **TL;DR:** Diagnostic tap for the Event Platform pipeline that captures in-flight messages for live inspection, powering the `agent stream-event-platform` command via a `/agent/stream-event-platform` HTTP endpoint.

# comp/forwarder/eventplatformreceiver — Event Platform Receiver Component

**Import path:** `github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver`
**Team:** agent-log-pipelines
**Importers:** ~16 packages

## Purpose

`comp/forwarder/eventplatformreceiver` is the diagnostic tap for the Event Platform pipeline. While `comp/forwarder/eventplatform` owns the forwarding side (sending payloads to intake), this component sits on the receiving side of each pipeline and captures in-flight messages for inspection — specifically to implement `agent stream-event-platform`, the live debug streaming endpoint.

When diagnostic capture is enabled (via `SetEnabled(true)`), each `passthroughPipeline` in the event platform forwarder calls `HandleMessage` as it processes a message. The receiver buffers those messages and makes them available through a `Filter` channel. When disabled, `HandleMessage` is a no-op with no allocations.

The component also registers itself as a POST handler at `/agent/stream-event-platform` via the Agent API, allowing `agent stream-event-platform` to stream formatted payloads to the operator's terminal.

## Package layout

| Package | Role |
|---|---|
| `comp/forwarder/eventplatformreceiver` (root) | `Component` interface, `Mock` interface |
| `eventplatformreceiverimpl/` | `Module()`, `NewReceiver`, `epFormatter`, `MockModule()` |

## Key Elements

### Key interfaces

## Component interface

```go
type Component interface {
    // SetEnabled enables or disables message capture. Returns true if the state changed.
    SetEnabled(e bool) bool
    // IsEnabled returns whether message capture is currently active.
    IsEnabled() bool
    // HandleMessage buffers a message for diagnostic output. No-op when disabled.
    HandleMessage(m *message.Message, rendered []byte, eventType string)
    // Filter streams buffered messages (as formatted strings) to the returned channel
    // until the done channel is closed.
    Filter(filters *diagnostic.Filters, done <-chan struct{}) <-chan string
}
```

The concrete implementation is `pkg/logs/diagnostic.BufferedMessageReceiver`, shared with the log pipeline diagnostic receiver.

### Key types

## Message formatting

`eventplatformreceiverimpl.epFormatter` formats payloads per event type:

| Event type | Deserialization |
|---|---|
| `EventTypeContainerLifecycle` | Protobuf `contlcycle.EventsPayload` → JSON |
| `EventTypeContainerImages` | Protobuf `contimage.ContainerImagePayload` → JSON |
| `EventTypeContainerSBOM` | Protobuf `sbom.SBOMPayload` → JSON |
| All others | Raw bytes written as-is |

Each formatted line is prefixed with `type: <eventType> | `.

### Key functions

## API endpoint

`NewReceiver` returns both the component and an `api.AgentEndpointProvider` that registers the `/agent/stream-event-platform` POST route. The handler uses `apiutils.GetStreamFunc` to stream `Filter` output back to the HTTP client (used by `agent stream-event-platform`).

### Configuration and build flags

## fx wiring

```go
// Normal agent startup (part of the forwarder bundle):
eventplatformreceiverimpl.Module()

// Tests:
eventplatformreceiverimpl.MockModule()
```

### Dependencies injected by fx

`NewReceiver` requires:
- `hostnameinterface.Component` — used by the default log formatter for the hostname field in diagnostic output

## Usage patterns

**From `eventplatformimpl` (the primary consumer):**

```go
// Inside each passthroughPipeline, after a message is batched/sent:
p.eventPlatformReceiver.HandleMessage(e, []byte{}, eventType)
```

The event platform forwarder is the only production caller of `HandleMessage`. Other components interact with the receiver only to enable/disable it or consume filtered output.

**Enabling capture from the API handler (stream endpoint):**

The `/agent/stream-event-platform` handler calls `SetEnabled(true)`, opens a `Filter` channel with a `done` signal, streams formatted lines to the HTTP response, and calls `SetEnabled(false)` when the connection closes. This lifecycle is handled automatically by `apiutils.GetStreamFunc`.

**Direct construction without fx (non-forwarding processes):**

```go
// Used by NewNoopEventPlatformForwarder in tests and the serverless agent:
epr := eventplatformreceiverimpl.NewReceiver(hostname).Comp
```

## Key consumers

- `comp/forwarder/eventplatform/eventplatformimpl` — the primary production consumer; injects the component into every `passthroughPipeline` and calls `HandleMessage` on each forwarded message
- `comp/forwarder/eventplatform/eventplatformimpl.NewNoopEventPlatformForwarder` — constructs a receiver directly for noop/test forwarder instances
