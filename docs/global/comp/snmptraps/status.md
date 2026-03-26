> **TL;DR:** `comp/snmptraps/status` tracks runtime metrics for the SNMP traps server (packets received, authentication failures, startup errors) and exposes them via `agent status` text/HTML output.

# comp/snmptraps/status

## Purpose

The `status` component tracks runtime metrics for the SNMP traps server and makes them available to the agent's status output subsystem. It maintains in-process counters (backed by `expvar`) for packets received and authentication failures, records any start-up error, and provides a `status.InformationProvider` that renders the data in both text and HTML formats.

## Key elements

### Key interfaces

```go
// comp/snmptraps/status/component.go
type Component interface {
    AddTrapsPackets(int64)
    GetTrapsPackets() int64
    AddTrapsPacketsUnknownCommunityString(int64)
    GetTrapsPacketsUnknownCommunityString() int64
    SetStartError(error)
    GetStartError() error
}
```

| Method | Description |
|---|---|
| `AddTrapsPackets(n)` | Increments the received-packets counter by `n` |
| `GetTrapsPackets()` | Returns the current received-packets count |
| `AddTrapsPacketsUnknownCommunityString(n)` | Increments the counter for packets rejected due to unknown SNMP community string |
| `GetTrapsPacketsUnknownCommunityString()` | Returns that counter |
| `SetStartError(err)` | Records an error that prevented the server from starting |
| `GetStartError()` | Returns the recorded start error (nil if none) |

### Key types

**`manager`** — located in `comp/snmptraps/status/statusimpl/status.go`.

The counters are stored as package-level `expvar.Int` values registered under the `snmp_traps` expvar map:

```
snmp_traps
  Packets                      ← total received traps
  PacketsUnknownCommunityString← authentication failures
```

`PacketsDropped` is derived on demand from the aggregator's `EventPlatformEventsErrors` expvar map (key `snmp-traps`) and appended to JSON/text output only when non-zero.

`startError` is a package-level `error` variable (not thread-safe by design; it is written once during startup and read-only afterward).

### Key functions

**Status provider** — `statusimpl.Provider` implements `status.InformationProvider`. It is registered in `serverimpl.Module()` via `coreStatus.NewInformationProvider(statusimpl.Provider{})`.

| Method | Output |
|---|---|
| `Name()` | `"SNMP Traps"` |
| `Section()` | `"SNMP Traps"` |
| `JSON(verbose, stats)` | Writes `snmpTrapsStats` key into the map |
| `Text(verbose, w)` | Renders `status_templates/snmp.tmpl` |
| `HTML(verbose, w)` | Renders `status_templates/snmpHTML.tmpl` |

The text template displays the error (if any) and then all metric keys formatted with `formatTitle` and `humanize`.

### Configuration and build flags

**Module registration**

```go
// The status component is not registered via a Module() call.
// It is created directly by serverimpl.newServer() and injected into the sub-app:
stat := statusimpl.New()
fx.Supply(injections{..., Status: stat, ...})
```

This design keeps the status object accessible to the outer agent app (for `TrapsServer.Error()`) while the inner fx sub-app also uses it to record incoming packets.

### Mock

`statusimpl/mock.go` provides `MockModule()` for tests that need to inject a `status.Component` without standing up the full server.

## Usage

The status component is created and owned by `serverimpl.newServer()`. Callers interact with it in two ways:

1. **Listener increments counters** — the listener calls `AddTrapsPackets` and `AddTrapsPacketsUnknownCommunityString` each time it receives a UDP packet.

2. **Agent status command** — the `Provider` is registered as a `coreStatus.InformationProvider`, so `agent status` automatically includes the SNMP traps section in its output.

3. **Server health** — `TrapsServer.Error()` delegates to `stat.GetStartError()`, which lets health-check code determine whether the server started successfully.
