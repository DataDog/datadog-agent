> **TL;DR:** `comp/snmptraps/forwarder` drains the packet channel from the listener, formats each trap as JSON via the formatter, and delivers it to the Datadog Event Platform as an `snmp-traps` event.

# comp/snmptraps/forwarder

## Purpose

The `forwarder` component is the final processing stage in the SNMP traps pipeline. It consumes raw `SnmpPacket` values from the listener, formats each packet into a JSON payload using the `formatter` component, and delivers the result to the Datadog backend via the Event Platform forwarder.

Its role is to decouple the listener (which must handle UDP packets as fast as possible) from the slower work of formatting and sending. The listener drops packets onto a channel; the forwarder drains that channel at its own pace.

## Key elements

### Key interfaces

```go
// comp/snmptraps/forwarder/component.go
type Component interface{}
```

The interface is a marker type. The component's behavior is entirely driven by its fx lifecycle hooks (start/stop).

### Key types

**`trapForwarder`** — located in `comp/snmptraps/forwarder/forwarderimpl/forwarder.go`:

| Field | Type | Role |
|---|---|---|
| `trapsIn` | `packet.PacketsChannel` | Channel of incoming packets from the listener |
| `formatter` | `formatter.Component` | Serializes each packet to JSON |
| `sender` | `sender.Sender` | Submits metrics and events to the aggregator |
| `stopChan` | `chan struct{}` | Signals the run loop to exit |

### Key functions

#### Run loop

The internal `run()` goroutine handles three cases:

- **Incoming packet** — calls `sendTrap(packet)`, which formats the packet and emits:
  - A `Count` metric: `datadog.snmp_traps.forwarded` (tagged with `snmp_device`, `device_namespace`, `snmp_version`)
  - An `EventPlatformEvent` of type `snmp-traps`
- **Flush ticker (10 s)** — calls `sender.Commit()` to flush aggregated metrics
- **Stop signal** — exits the goroutine

### Configuration and build flags

**Dependencies** — the component is constructed via fx and requires:

| Dependency | Purpose |
|---|---|
| `config.Component` | Checks whether traps are enabled before registering lifecycle hooks |
| `formatter.Component` | Converts `SnmpPacket` to JSON bytes |
| `demultiplexer.Component` | Provides the default `Sender` |
| `listener.Component` | Provides the `PacketsChannel` via `Packets()` |
| `log.Component` | Logging |

**Module registration**

```go
// comp/snmptraps/forwarder/forwarderimpl/forwarder.go
func Module() fxutil.Module {
    return fxutil.Component(fx.Provide(newTrapForwarder))
}
```

## Usage

The forwarder is wired automatically by `serverimpl.Module()` inside the SNMP traps server sub-application (`comp/snmptraps/server/serverimpl/server.go`). It is not registered directly in the main agent fx app; instead, the server creates an inner `fx.App` that includes:

```go
configimpl.Module()
formatterimpl.Module()
forwarderimpl.Module()   // <-- this component
listenerimpl.Module()
oidresolverimpl.Module()
```

The server is itself included when `snmptraps.Bundle()` is added to the agent binary.

### Testing

Tests in `forwarderimpl/forwarder_test.go` use `senderhelper.Opts` to inject a `mocksender.MockSender` into the demultiplexer, then assert that the forwarder emits the expected `EventPlatformEvent` and `Count` metric when a packet is pushed through the mock listener.
