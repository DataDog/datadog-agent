# comp/snmptraps/formatter

**Package:** `github.com/DataDog/datadog-agent/comp/snmptraps/formatter`
**Team:** network-device-monitoring-core

## Purpose

The formatter component converts raw SNMP trap packets into JSON payloads suitable for sending to the Datadog log intake via the event platform forwarder. It enriches the raw binary/numeric data from trap variables with human-readable names and symbolic values drawn from MIB databases, so that Datadog receives structured, self-describing trap records rather than opaque OID strings and integer codes.

Without this component, consumers of trap packets would receive low-level gosnmp structures. The formatter bridges between the raw listener output and the forwarder that sends data to Datadog.

## Key elements

### Interface

```go
// comp/snmptraps/formatter/component.go
type Component interface {
    FormatPacket(packet *packet.SnmpPacket) ([]byte, error)
}
```

`FormatPacket` accepts a `*packet.SnmpPacket` (the parsed UDP datagram with source address and timestamp) and returns a JSON-encoded byte slice or an error.

### Implementation: JSONFormatter

The sole concrete implementation is `JSONFormatter` in `formatterimpl/formatter.go`. It is provided as a fx component via `formatterimpl.Module()`.

Dependencies injected at construction time:

| Dependency | Role |
|---|---|
| `oidresolver.Component` | Resolves numeric OIDs to trap and variable names from MIB databases |
| `demultiplexer.Component` | Provides a `sender.Sender` for internal telemetry counters |
| `log.Component` | Structured logger |

### Output JSON shape

```json
{
  "trap": {
    "ddsource": "snmp-traps",
    "ddtags": "namespace:default,snmp_device:10.0.0.2,...",
    "timestamp": 1712345678000,
    "snmpTrapOID": "1.3.6.1.6.3.1.1.5.4",
    "snmpTrapName": "linkDown",
    "snmpTrapMIB": "IF-MIB",
    "uptime": 123456,
    "variables": [
      { "oid": "1.3.6.1.2.1.2.2.1.1.2", "type": "integer", "value": 2 }
    ],
    "ifAdminStatus": "down"
  }
}
```

The top-level object always has a single `trap` key. SNMPv1 traps additionally carry `enterpriseOID`, `genericTrap`, and `specificTrap` fields.

### Variable enrichment

For each variable bound to the trap, the formatter:

1. Converts the raw gosnmp `SnmpPDU` to a `trapVariable` struct (`oid`, `type`, `value`).
2. Calls `oidresolver.GetVariableMetadata(trapOID, varOID)` to look up the MIB definition.
3. If the variable has an **enumeration** mapping, replaces the integer value with its symbolic string (`down`, `up`, etc.) and adds it as a named top-level key alongside the `variables` array.
4. If the variable uses **BITS** syntax, expands each set bit to its symbolic name and formats the raw bytes as a hex string.
5. If no metadata is found, keeps the raw value and emits `datadog.snmp_traps.vars_not_enriched` telemetry.

If the trap OID itself cannot be resolved, `snmpTrapName` and `snmpTrapMIB` are omitted and `datadog.snmp_traps.traps_not_enriched` is incremented.

### Internal telemetry metrics

| Metric | Meaning |
|---|---|
| `datadog.snmp_traps.traps_not_enriched` | Trap OID could not be resolved to a name |
| `datadog.snmp_traps.vars_not_enriched` | One or more variable OIDs could not be resolved |
| `datadog.snmp_traps.incorrect_format` | v2/v3 packet had fewer than 2 PDU variables or malformed sysUpTime/trapOID |

### Mock

`formatterimpl/mock.go` provides a `MockFormatter` for use in tests. It is configured with a fixed byte slice to return from `FormatPacket`.

## Usage

The formatter is consumed exclusively by `comp/snmptraps/forwarder` (`forwarderimpl`), which:

1. Reads packets from `listener.Component.Packets()`.
2. Calls `formatter.FormatPacket(packet)` to produce the JSON body.
3. Wraps the result in a `message.Message` and sends it to the event platform forwarder as a log event.

The full pipeline is assembled by `comp/snmptraps/server/serverimpl`:

```
listener → packets channel → forwarder → formatter → event platform → Datadog
                                              ↑
                                        oidresolver
```

To register the formatter in an fx application, import and apply `formatterimpl.Module()`.
