> **TL;DR:** `comp/snmptraps/packet` defines the `SnmpPacket` struct and `PacketsChannel` type alias that connect all pipeline stages of the SNMP traps subsystem without creating circular dependencies.

# comp/snmptraps/packet

**Package:** `github.com/DataDog/datadog-agent/comp/snmptraps/packet`
**Team:** network-device-monitoring-core

## Purpose

`comp/snmptraps/packet` defines the shared data types and channel type that flow between the SNMP traps pipeline stages. It is a pure-data package with no fx wiring — every other SNMP traps component imports it to pass trap data around without creating circular dependencies.

## Key elements

### Key types

#### `SnmpPacket`

```go
type SnmpPacket struct {
    Content   *gosnmp.SnmpPacket  // decoded SNMP payload (gosnmp library)
    Addr      *net.UDPAddr        // source address of the sender
    Namespace string              // agent namespace for multi-tenancy
    Timestamp int64               // receive time, Unix milliseconds
}
```

Wraps the raw `gosnmp.SnmpPacket` with the additional metadata the agent needs:

- **`Addr`**: the IP/port of the sending device, used as the `snmp_device` tag and for logging.
- **`Namespace`**: the device namespace configured on the listener (e.g. `"default"`, `"prod"`), emitted as the `device_namespace` tag.
- **`Timestamp`**: populated by the listener at receive time and forwarded to the Datadog backend as the event timestamp.

#### `PacketsChannel`

```go
type PacketsChannel = chan *SnmpPacket
```

A type alias for the channel that connects the listener to the forwarder. Using an alias (rather than a named type) means any `chan *SnmpPacket` value satisfies the type without an explicit cast.

### Key functions

#### `GetTags() []string`

Returns the Datadog tags derived from the packet:

| Tag | Value |
|---|---|
| `snmp_version` | `"1"`, `"2"`, or `"3"` (mapped from `gosnmp.Version1/2c/3`) |
| `device_namespace` | Value of `SnmpPacket.Namespace` |
| `snmp_device` | IP string from `SnmpPacket.Addr` |

These tags are applied both to agent telemetry metrics and to the formatted trap event payload.

### Configuration and build flags

**Test helpers** (`test_helpers.go` — compiled under `!serverless && test` build constraint) — provides pre-built `*SnmpPacket` values and `gosnmp.SnmpTrap` fixtures:

| Symbol | Description |
|---|---|
| `NetSNMPExampleHeartbeatNotification` | SNMPv2c heartbeat trap (NET-SNMP example MIB) |
| `LinkDownv1GenericTrap` | SNMPv1 generic link-down trap |
| `AlarmActiveStatev1SpecificTrap` | SNMPv1 enterprise-specific alarm trap |
| `Unknownv1Trap` | SNMPv1 trap with an unknown OID |
| `CreateTestPacket(trap)` | Wraps a trap in a v2c `SnmpPacket` with a fixed source IP and namespace |
| `CreateTestV1GenericPacket()` | Returns a ready-to-use v1 link-down packet |
| `CreateTestV1SpecificPacket()` | Returns a ready-to-use v1 alarm packet |
| `CreateTestV1Packet(trap)` | Wraps an arbitrary trap in a v1 `SnmpPacket` |

## Usage

The `packet` package is imported by every stage of the SNMP traps pipeline:

- **`comp/snmptraps/listener`**: the listener component's `Packets() PacketsChannel` return type is defined here. The listener writes `*SnmpPacket` values onto the channel after validating each datagram.
- **`comp/snmptraps/formatter`**: the formatter component receives `*SnmpPacket` and uses `GetTags()` when building the outgoing JSON event.
- **`comp/snmptraps/forwarder`**: the forwarder reads from the channel and passes packets to the formatter before submitting them to the event platform.

There is no fx module in this package. Add it as a Go import directly:

```go
import "github.com/DataDog/datadog-agent/comp/snmptraps/packet"

func (f *myForwarder) process(p *packet.SnmpPacket) {
    tags := p.GetTags()
    ...
}
```

### Where `PacketsChannel` flows in the pipeline

```
listenerimpl  ─── writes *SnmpPacket ──► PacketsChannel ──► forwarderimpl
                                                               │
                                                    formatter.FormatPacket(p)
                                                               │
                                                    EventPlatformEvent("snmp-traps")
```

The channel capacity is set by `config.GetPacketChannelSize()`. If the forwarder falls behind (e.g. during a Datadog intake outage), the listener drops incoming packets when the channel is full rather than blocking the UDP receive loop.

## Cross-references

| Related component | Relationship |
|-------------------|--------------|
| [`comp/snmptraps/listener`](listener.md) | The listener is the **producer**: it validates each incoming UDP datagram, wraps it as `*SnmpPacket`, and publishes it on `PacketsChannel`. The `Packets() PacketsChannel` method on the listener interface is typed directly with the `PacketsChannel` alias defined in this package. |
| [`comp/snmptraps/forwarder`](forwarder.md) | The forwarder is the **consumer**: it drains `PacketsChannel` in its `run()` loop, calls `formatter.FormatPacket` on each `*SnmpPacket`, and emits both a `datadog.snmp_traps.forwarded` count metric (tagged with the values from `GetTags()`) and an event platform event of type `snmp-traps`. |
| [`comp/snmptraps/server`](server.md) | The top-level orchestrator. It assembles the inner fx sub-application that wires the listener, forwarder, formatter, and OID resolver together. The `PacketsChannel` is the internal contract that decouples the listener's UDP receive path from the forwarder's formatting and delivery path. |
