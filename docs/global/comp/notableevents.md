> **TL;DR:** `comp/notableevents` subscribes to predefined Windows Event Log entries (reboots, crashes, failed updates) and forwards them to the Datadog Event Management v2 API using a persistent bookmark to avoid re-emission across restarts.

# comp/notableevents

**Team:** windows-products
**Platform:** Windows only (build tag `windows`)

## Purpose

`comp/notableevents` monitors the Windows Event Log for pre-defined notable
system events and forwards them to the Datadog Event Management v2 API. It
gives operators visibility into significant host-level events — such as
unexpected reboots, application crashes, and failed updates — directly in
Datadog Event Management, without requiring a separate log collection pipeline.

The component is enabled with `notable_events.enabled: true` in `datadog.yaml`.
If disabled, a no-op implementation is returned and no Event Log subscription
is opened.

## Key elements

### Key interfaces

```go
// comp/notableevents/def/component.go
type Component interface{}
```

The component has no public methods. All behavior is managed by fx lifecycle
hooks (`OnStart` / `OnStop`). Start/stop order is enforced internally: the
submitter (consumer) starts before the collector (producer), and the collector
stops before the channel is closed.

### Key types

**Internal architecture:**

```
Windows Event Log
       |
  [collector]  --eventChan-->  [submitter]  -->  Event Platform Forwarder
```

**`collector`** (Windows-only, `collector.go`)

- Opens a single WEL pull subscription using a combined XPath query covering
  all registered event definitions.
- Persists a bookmark via the agent's `persistentcache` so events are not
  re-emitted across restarts.
- On subscription failure, retries with exponential backoff (1 s → 1 min) until
  the context is cancelled.
- For each received event, renders the event XML, parses it, applies the
  definition's `FormatPayload` function if present, and sends an `eventPayload`
  to `eventChan`.

**`submitter`** (`submitter.go`)

- Reads `eventPayload` structs from the channel.
- Formats each as an Event Management v2 JSON payload with integration ID
  `system-notable-events`.
- Calls `eventplatform.Forwarder.SendEventPlatformEventBlocking` with event
  type `EventTypeEventManagement`.

**Monitored events** — defined in `getEventDefinitions()` (`collector.go`):

| Provider | Event ID | Type |
|----------|----------|------|
| `Microsoft-Windows-Kernel-Power` | 41 | Unexpected reboot |
| `Application Error` | 1000 | Application crash |
| `Application Hang` | 1002 | Application hang |
| `Microsoft-Windows-WindowsUpdateClient` | 20 | Failed Windows update |
| `MsiInstaller` | 1033 | Failed application installation |
| `MsiInstaller` | 1034 | Failed application removal |

Each definition includes the XPath channel/query, a human-readable title and
message, and an optional `FormatPayload` function to extract structured fields
from the event XML (e.g., process name for application crashes).

**`eventPayload`** — the internal event structure:

```go
type eventPayload struct {
    Timestamp time.Time
    EventType string                 // e.g., "Unexpected reboot"
    Title     string
    Message   string
    Custom    map[string]interface{} // contains "windows_event_log" map
}
```

### Configuration and build flags

**fx wiring:**

```go
// comp/notableevents/fx/fx.go
func Module() fxutil.Module {
    return fxutil.Component(fx.Provide(NewComponent))
}
```

Depends on: `configcomp.Component`, `logcomp.Component`,
`eventplatform.Component`, `hostname.Component`, `compdef.Lifecycle`.

## Usage

The component is included in the main agent's Windows build
(`cmd/agent/subcommands/run/command_windows.go`). No calling code is needed
beyond ensuring it is in the fx graph — the lifecycle hooks drive everything.

To enable:

```yaml
# datadog.yaml
notable_events:
  enabled: true
```

To add a new monitored event type:

1. Add an `eventDefinition` entry to `getEventDefinitions()` in
   `comp/notableevents/impl/collector.go`.
2. Optionally implement a `PayloadFormatter` in
   `comp/notableevents/impl/format.go` to extract structured fields from the
   event XML and assign it to `FormatPayload`.

The bookmark ensures that events already seen are not re-forwarded after an
agent restart. The bookmark is stored under the key
`notable_events:event_log_bookmark` in the agent's persistent cache.

## Related components

| Component / Package | Relationship |
|---|---|
| [`pkg/util/winutil`](../pkg/util/winutil.md) | Provides the `eventlog/subscription` sub-package (`evtsubscribe.PullSubscription`) that `comp/notableevents` uses internally to open and drive the Windows Event Log pull subscription. The `eventlog/` layer owns the `EvtSubscribe` / `EvtNext` Win32 wrappers, bookmark persistence in the persistent cache, and event-record rendering; `comp/notableevents` sits above it, mapping rendered XML to `eventPayload` structs and forwarding them. |
| [`comp/forwarder/eventplatform`](forwarder/eventplatform.md) | The primary transport for notable events. `comp/notableevents` injects `eventplatform.Component`, unwraps it with `Get()`, and calls `SendEventPlatformEventBlocking` with event type `EventTypeEventManagement` for every received Windows Event Log entry. Each payload becomes a single Event Management v2 JSON message (stream strategy — one event at a time) routed to the `event-management-intake.` pipeline. |
