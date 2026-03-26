# comp/snmptraps/listener

**Package:** `github.com/DataDog/datadog-agent/comp/snmptraps/listener`
**Team:** network-device-monitoring-core

## Purpose

The listener component opens a UDP socket and receives SNMP trap packets (v1, v2c, v3) sent by network devices. It validates each incoming packet, wraps it with metadata (source address, timestamp, namespace), and publishes it on a channel for downstream processing.

The component decouples ingestion from processing: the listener does only the minimum work needed to accept a datagram—credential validation and channel publish—while the heavier formatting and forwarding work is handled by separate components.

## Key elements

### Interface

```go
// comp/snmptraps/listener/component.go
type Component interface {
    Packets() packet.PacketsChannel
}
```

`Packets()` returns the `packet.PacketsChannel` (a `chan *packet.SnmpPacket`) to which the listener publishes every accepted trap packet. Downstream consumers (the forwarder) read from this channel.

### Implementation: trapListener

`listenerimpl/listener.go` provides the concrete implementation via `listenerimpl.Module()`.

The `trapListener` struct holds:

| Field | Description |
|---|---|
| `config` | `*config.TrapsConfig` — bind address, port, community strings, SNMP params, stop timeout |
| `packets` | Buffered `packet.PacketsChannel` (capacity from `config.GetPacketChannelSize()`) |
| `listener` | `*gosnmp.TrapListener` — underlying UDP listener from the gosnmp library |
| `errorsChannel` | Buffered channel used to surface bind/listen errors back to the startup path |
| `sender` | `sender.Sender` for internal telemetry counters |
| `status` | `status.Component` for exposing trap counts to the agent status page |

### Lifecycle

Registration uses `fx.Lifecycle`. When `config.Enabled` is true:

- **OnStart**: calls `trapListener.start()`, which launches `listener.Listen(addr)` in a goroutine (blocking call) and then blocks until the `gosnmp.TrapListener.Listening()` channel fires or an error arrives from `errorsChannel`. This guarantees the socket is bound before `Start` returns.
- **OnStop**: calls `trapListener.stop()`, which closes the gosnmp listener and enforces a timeout (`config.StopTimeout` seconds) via a `select` on a goroutine-closed channel.

### Packet validation

SNMPv3 packets are authenticated/decrypted by gosnmp before the callback fires—the listener passes them through without additional checks.

For v1/v2c packets, `validatePacket` performs a constant-time comparison of the packet's community string against every entry in `config.CommunityStrings`. If none matches, the packet is dropped, `datadog.snmp_traps.invalid_packet` is incremented with `reason:unknown_community_string`, and the `status` component records the unknown-community-string count.

### SnmpPacket type

```go
// comp/snmptraps/packet
type SnmpPacket struct {
    Content   *gosnmp.SnmpPacket
    Addr      *net.UDPAddr
    Timestamp int64          // Unix milliseconds at receive time
    Namespace string
}
```

`GetTags()` on `SnmpPacket` produces the Datadog tags (`snmp_device:<ip>`, `namespace:<ns>`, etc.) that are propagated to both telemetry and the formatted payload.

### Internal telemetry metrics

| Metric | Meaning |
|---|---|
| `datadog.snmp_traps.received` | Total datagrams received (before validation) |
| `datadog.snmp_traps.invalid_packet` | Datagrams rejected for unknown community string |

### Mock

`listenerimpl/mock_listener.go` provides `MockListener`, which wraps an in-memory channel. Tests can inject packets by writing directly to that channel. The `mock.go` at the package root exposes the mock for use in external test packages.

## Usage

The listener is started as part of the SNMP traps server assembled in `comp/snmptraps/server/serverimpl`. The server creates an inner fx application containing all trap sub-components:

```go
// serverimpl/server.go
app := fx.New(
    ...
    listenerimpl.Module(),
    forwarderimpl.Module(),   // reads from listener.Packets()
    ...
)
```

The forwarder injects `listener.Component` and consumes `Packets()` in its processing loop. No other component in the codebase reads from the listener directly.

To use the listener in isolation (e.g. for testing), apply `listenerimpl.Module()` in an fx app alongside `configimpl.Module()` and `demultiplexer`.
