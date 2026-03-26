> **TL;DR:** `comp/snmptraps/server` is the top-level orchestrator for the SNMP traps subsystem, running a UDP listener that receives SNMPv1/v2c/v3 traps, enriching them with OID metadata, and forwarding JSON payloads to the Datadog event platform.

# comp/snmptraps/server — SNMP Traps Server Component

**Import path:** `github.com/DataDog/datadog-agent/comp/snmptraps/server`
**Team:** network-device-monitoring-core
**Importers:** `cmd/agent/subcommands/run` (core agent and Windows agent), `comp/snmptraps` (bundle)

## Purpose

`comp/snmptraps/server` runs a UDP server that listens for SNMP trap messages (SNMPv1, v2c, v3), validates credentials, enriches trap data with OID names and descriptions, formats the result as a JSON event, and forwards it to the Datadog backend via the event platform pipeline. It is the top-level orchestrator for the entire SNMP traps subsystem.

## Package layout

The server component is the entry point for a bundle of sub-components. Each sub-component lives in its own package:

| Package | Role |
|---|---|
| `comp/snmptraps/server` | Component interface |
| `comp/snmptraps/server/serverimpl` | `TrapsServer` implementation; wires the sub-app |
| `comp/snmptraps/listener/listenerimpl` | UDP socket, credential validation, packet channel |
| `comp/snmptraps/forwarder/forwarderimpl` | Dequeues packets, formats and sends to event platform |
| `comp/snmptraps/formatter/formatterimpl` | Formats `SnmpPacket` to JSON bytes |
| `comp/snmptraps/oidresolver/oidresolverimpl` | Resolves numeric OIDs to human-readable names |
| `comp/snmptraps/config/configimpl` | Parses and validates `TrapsConfig` from agent config |
| `comp/snmptraps/status/statusimpl` | Tracks running state and startup errors |
| `comp/snmptraps` | `Bundle()` — convenience function that includes `serverimpl.Module()` |

## Key elements

### Key interfaces

```go
// Package: github.com/DataDog/datadog-agent/comp/snmptraps/server
type Component interface {
    // Running reports whether the server is currently accepting traps.
    Running() bool
    // Error returns any error recorded during startup. Non-nil implies Running() == false.
    Error() error
}
```

### Key functions

`serverimpl.newServer` checks `trapsconfig.IsEnabled` first. If SNMP traps are not enabled in the agent configuration, it returns a `TrapsServer` with `running: false` and no lifecycle hooks registered — zero cost at runtime.

When traps are enabled, the server creates a **nested `fx.App`** to wire the sub-components. The outer `Lifecycle` hooks start and stop the inner fx app. If the inner app fails to initialize (e.g. invalid config, port already in use), the error is recorded via `status.SetStartError` and `Running()` returns false without crashing the outer agent.

### Configuration and build flags

Key fields in `TrapsConfig` (under `network_devices.snmp_traps`):

| Field | Description |
|---|---|
| `enabled` | Whether to start the listener |
| `port` | UDP port to listen on (default 9162) |
| `community_strings` | Allowed community strings for v1/v2c |
| `users` | SNMPv3 user credentials |
| `namespace` | Namespace tag attached to all traps |
| `stop_timeout` | Seconds to wait for listener shutdown |

## Implementation details

`serverimpl.newServer` checks `trapsconfig.IsEnabled` first. If SNMP traps are not enabled in the agent configuration, it returns a `TrapsServer` with `running: false` and no lifecycle hooks registered — zero cost at runtime.

When traps are enabled, the server creates a **nested `fx.App`** to wire the sub-components. This inner app receives the outer app's `config`, `hostname`, `demultiplexer`, and `log` components via `fx.Supply`, along with a shared `status.Component`. It then constructs the full pipeline:

```
[UDP socket] → listener → packet channel → forwarder → formatter → event platform
                                                      └─ OID resolver (enrichment)
```

The outer `Lifecycle` hooks start and stop the inner fx app:

- `OnStart` — calls `app.Start`, which starts the listener (binds UDP port) and forwarder (begins draining the packet channel). Sets `running = true`.
- `OnStop` — sets `running = false`, then calls `app.Stop`.

If the inner app fails to initialize (e.g. invalid config, port already in use), the error is recorded via `status.SetStartError` and `Running()` returns false without crashing the outer agent.

The server also registers a `coreStatus.InformationProvider` so the agent's `status` command can display traps server health.

**Listener** (`listenerimpl`): Opens a UDP socket using `gosnmp.TrapListener`. For SNMPv1/v2c packets, validates the community string against the configured list using constant-time comparison to prevent timing attacks. SNMPv3 packets are validated and decrypted by gosnmp itself. Valid packets are put on a `packet.PacketsChannel`. Counts `datadog.snmp_traps.received` and `datadog.snmp_traps.invalid_packet` metrics.

**Forwarder** (`forwarderimpl`): Drains the packet channel in a goroutine, calls `formatter.FormatPacket` on each, then calls `sender.EventPlatformEvent(data, EventTypeSnmpTraps)`. Commits metrics every 10 seconds. Counts `datadog.snmp_traps.forwarded`.

**Formatter** (`formatterimpl`): Resolves OID names via the OID resolver and serializes the trap to JSON. The event type is `snmp-traps` in the event platform.

## fx wiring

The recommended way to include the server is via the bundle:

```go
import "github.com/DataDog/datadog-agent/comp/snmptraps"

// In your fx app:
snmptraps.Bundle()
```

This is equivalent to `serverimpl.Module()`. The server requires `config.Component`, `hostname.Component`, `demultiplexer.Component`, and `log.Component` from the outer app.

## Usage

The server component is consumed by the core agent's run command, which injects it as a dependency to ensure its lifecycle is managed:

```go
// In the agent's run function signature:
func run(
    ...
    _ snmptrapsServer.Component,
    ...
) error { ... }
```

Callers typically do not call methods on the component directly — the server manages itself through fx lifecycle hooks. The `Running()` and `Error()` methods are mainly useful for health checks and the status page.

**Event-platform back-pressure:** The forwarder inside the server calls `SendEventPlatformEvent` (non-blocking). If the `snmp-traps-intake.` pipeline input channel of [`comp/forwarder/eventplatform`](../forwarder/eventplatform.md) is full, trap payloads are silently dropped. Monitor `datadog.snmp_traps.forwarded` alongside the event-platform metrics to detect drops under sustained load.

**Namespace tagging:** Every trap payload carries a `namespace` tag derived from `TrapsConfig.Namespace`, which defaults to `network_devices.namespace` in `datadog.yaml`. Set this to the same namespace used for SNMP polling (see [`pkg/snmp`](../../pkg/snmp.md)) so that trap events and device metadata are associated consistently in the Datadog backend.

**Checking server state:**

```go
import snmptrapsServer "github.com/DataDog/datadog-agent/comp/snmptraps/server"

type Requires struct {
    fx.In
    TrapsServer snmptrapsServer.Component
}

if !c.trapsServer.Running() {
    log.Warnf("SNMP traps server not running: %v", c.trapsServer.Error())
}
```

## Notes

- The inner fx app pattern (`fx.New` within an `fx.Provide` function) is used here to isolate the traps sub-components. The code comments note this is non-standard and should not be copied elsewhere if avoidable.
- If the agent is reconfigured and traps were not enabled at startup, the server will not start dynamically — a restart is required.
- OID name resolution is best-effort: if an OID cannot be resolved, the raw numeric OID is used in the formatted event.

## Related components

| Component | Relationship |
|---|---|
| [`comp/snmptraps/config`](config.md) | Parses and validates `TrapsConfig` from `network_devices.snmp_traps.*`. The server reads the stop timeout and packet channel size from `Get()`; the listener uses `Get()` for the bind address, port, and SNMP params. `IsEnabled()` is the fast-path guard checked before constructing the inner fx app. |
| [`comp/snmptraps/listener`](listener.md) | Opened inside the inner fx app. Binds the UDP socket, validates community strings with constant-time comparison, and publishes accepted `SnmpPacket` values on `Packets()`. The server's pipeline depends on the listener completing its `OnStart` (socket bind) before the forwarder begins draining packets. |
| [`comp/snmptraps/formatter`](formatter.md) | Called by the forwarder for each dequeued packet. Resolves OID names via `oidresolver` and serializes the trap to a JSON payload. Enrichment is best-effort — unresolvable OIDs use the raw numeric string. |
| [`comp/snmptraps/oidresolver`](oidresolver.md) | Loaded inside the inner fx app. Scans `conf.d/snmp.d/traps_db/` for MIB databases at startup. Provides `GetTrapMetadata` and `GetVariableMetadata` to the formatter. User-provided files override the Datadog-shipped `dd_traps_db*` files. |
| [`comp/forwarder/eventplatform`](../forwarder/eventplatform.md) | The forwarder (`forwarderimpl`) sends formatted trap payloads as `EventTypeSnmpTraps` events. It uses the non-blocking `SendEventPlatformEvent` variant — traps are dropped if the pipeline input channel is full. The pipeline destination is `snmp-traps-intake.`. |
| [`pkg/snmp`](../../pkg/snmp.md) | Provides the shared `gosnmplib` helpers (protocol-string-to-constant conversion, PDU conversion, `ConditionalWalk`) used throughout the traps subsystem. `TrapsConfig.BuildSNMPParams()` in `comp/snmptraps/config` mirrors the `Authentication.BuildSNMPParams()` pattern from `pkg/snmp`. Both packages share the `network_devices.namespace` configuration key and the same `gosnmplib.GetAuthProtocol` / `GetPrivProtocol` utilities for USM credential resolution. |
