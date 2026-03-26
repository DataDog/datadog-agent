# comp/snmptraps/snmplog

**Package:** `github.com/DataDog/datadog-agent/comp/snmptraps/snmplog`
**Team:** network-device-monitoring-core

## Purpose

`comp/snmptraps/snmplog` is a thin adapter that satisfies the `gosnmp.LoggerInterface` by forwarding log calls to the agent's structured `log.Component`. Its only purpose is to prevent gosnmp's internal diagnostic messages (decoded packet contents, protocol state transitions) from being lost or written to `os.Stderr`.

## Key elements

### `SNMPLogger`

```go
type SNMPLogger struct {
    gosnmp.LoggerInterface
    logger log.Component
}
```

Embeds `gosnmp.LoggerInterface` for forward-compatibility (any future interface methods are satisfied via the embedded nil value, which panics only if called — acceptable since gosnmp only calls `Print` and `Printf`).

Implements:

| Method | Maps to |
|---|---|
| `Print(v ...interface{})` | `logger.Trace(v...)` |
| `Printf(format string, v ...interface{})` | `logger.Tracef(format, v...)` |

gosnmp's internal logs are deliberately routed to `Trace` (the lowest agent log level) because they include the full decoded content of every trap packet, which would be too noisy at `Debug` or above.

### `New(logger log.Component) *SNMPLogger`

The only constructor. Pass an `log.Component` obtained from fx.

## Usage

Inject an `SNMPLogger` into a `gosnmp.TrapListener` or `gosnmp.GoSNMP` struct before starting it:

```go
import (
    "github.com/DataDog/datadog-agent/comp/snmptraps/snmplog"
    log "github.com/DataDog/datadog-agent/comp/core/log/def"
    "github.com/gosnmp/gosnmp"
)

func newListener(logger log.Component) *gosnmp.TrapListener {
    tl := gosnmp.NewTrapListener()
    tl.Params = &gosnmp.GoSNMP{
        Logger: snmplog.New(logger),
        ...
    }
    return tl
}
```

Within the SNMP traps codebase, `SNMPLogger` is instantiated once in `comp/snmptraps/config/config.go` and embedded in the `gosnmp.GoSNMP` params that are shared across listener and trap-listener setup. The logger is not an fx component — it is created directly with `snmplog.New`.

### Log level note

gosnmp emits one log line per decoded trap packet at its default log level. Routing these to `Trace` (the lowest agent level) means they are only visible when the agent is run with `log_level: trace`. At `debug` or above they are suppressed. This is intentional: during normal operation the volume of per-packet trace messages would overwhelm the log stream.

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`comp/snmptraps/server`](server.md) | The server orchestrates the inner fx sub-application that includes the listener and forwarder. Both rely on the `gosnmp.GoSNMP` params (with `SNMPLogger` embedded) that `comp/snmptraps/config` produces. Any gosnmp protocol-level debug messages from the UDP receive path or packet decoding will be routed through `SNMPLogger` to the agent log at `Trace` level. |
| [`pkg/snmp`](../../pkg/snmp.md) | Provides the shared gosnmp helpers (`gosnmplib.GetAuthProtocol`, `GetPrivProtocol`, `PDUFromSNMP`) and the `Authentication.BuildSNMPParams()` pattern used across the SNMP subsystem. `SNMPLogger` bridges the gosnmp logging interface to the agent's structured logger, but the gosnmp instance itself is configured using the patterns from `pkg/snmp/gosnmplib`. |
