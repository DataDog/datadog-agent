# comp/checks/windowseventlog

**Package:** `github.com/DataDog/datadog-agent/comp/checks/windowseventlog`
**Team:** windows-products
**Platform:** Windows only (build tag `windows`)

## Purpose

`windowseventlog` registers the `win32_event_log` agent check, which reads entries from the Windows Event Log (via the native EvtSubscribe API) and forwards them to Datadog as Events, Logs, or security telemetry.

Common use cases:

- Forwarding application/system/security event log channels to Datadog Events
- Sending Windows event log data as Datadog Logs (integration logs path)
- Monitoring for Windows Defender, audit, or other security events using the built-in `dd_security_events` profile

## Key Elements

### Interface

```go
// def/component.go
type Component interface{}
```

The interface is a marker. The component's value is the side effect of registering the check factory with the collector on startup.

### `Check` struct (`impl/check/check.go`)

The core check type. Notable fields:

| Field | Type | Purpose |
|-------|------|---------|
| `config` | `*Config` | Parsed instance and init configuration |
| `sub` | `evtsubscribe.PullSubscription` | Background event subscription |
| `bookmarkManager` | `evtbookmark.Manager` | Persists position in the event stream across restarts |
| `publisherMetadataCache` | `publishermetadatacache.Component` | Caches publisher metadata for message rendering |
| `logsAgent` | `option.Option[logsAgent.Component]` | Optional logs pipeline for integration log output |
| `ddSecurityEventsFilter` | `eventdatafilter.Filter` | Filter for the built-in DD security events profile |

### `Config` / `instanceConfig`

Key instance configuration fields (all `option.Option` to distinguish absent from zero value):

| Field | YAML key | Description |
|-------|----------|-------------|
| `ChannelPath` | `path` | Windows Event Log channel (e.g. `System`, `Application`) |
| `DDSecurityEvents` | `dd_security_events` | Use built-in security profile (`"high"` or `"low"`); mutually exclusive with `path` |
| `Query` | `query` | XPath query to filter events (default: `*`) |
| `Start` | `start` | Where to begin reading: `"now"` or `"oldest"` |
| `BookmarkFrequency` | `bookmark_frequency` | Save position every N events |
| `Filters` | `filters` | Filter by source, type, and event ID |
| `IncludedMessages` | `included_messages` | Regex whitelist applied to rendered message text |
| `ExcludedMessages` | `excluded_messages` | Regex blacklist applied to rendered message text |
| `AuthType` | `auth_type` | Remote auth type (`default`, `negotiate`, `kerberos`, `ntlm`) |
| `Server` / `User` / `Domain` / `Password` | — | Remote event log credentials |

### Event pipeline

Event collection runs in a background goroutine (`fetchEventsLoop`) rather than synchronously in `Run()`. This decouples the check scheduler from event arrival latency. `Run()` is responsible for:

1. Starting (or restarting) the subscription if it is not already running.
2. Persisting the bookmark via `bookmarkManager.Save()`.

### Bookmark persistence

Position in the event log is stored in the agent's persistent cache (key `<checkID>_bookmark`). On restart the check reopens the subscription at the last saved position, preventing duplicate or missed events.

### FX wiring and dependencies

`Requires` in the implementation:

| Dependency | Required? | Purpose |
|------------|-----------|---------|
| `configComponent.Component` | Yes | Agent configuration |
| `logsAgent.Component` | Optional | Send events as Datadog Logs |
| `publishermetadatacache.Component` | Yes | Render event messages |

`NewComponent` registers the check factory with `core.RegisterCheck` during `OnStart`.

## Usage

The component is wired into the Windows-specific agent run command via `cmd/agent/subcommands/run/command_windows.go`:

```go
windowseventlogfx.Module()
// ...
fx.Invoke(func(_ windowseventlog.Component) {})
```

A minimal `conf.yaml` instance for the check:

```yaml
instances:
  - path: System
    start: now
    filters:
      type:
        - Error
        - Warning
```

To forward as Datadog Logs instead of Events, ensure `logs_enabled: true` in `datadog.yaml` and use the `dd_security_events` option or configure a logs section.

The check can also be exercised from the CLI:

```bash
agent check win32_event_log
```

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/util/winutil`](../../pkg/util/winutil.md) | The check is built on top of `pkg/util/winutil/eventlog/` sub-packages. `evtsubscribe.PullSubscription` drives the background event-collection loop via the `EvtSubscribe`/`EvtNext` Windows API. `evtbookmark.Manager` (using `evtapi` from `pkg/util/winutil/eventlog/api/windows`) manages the XML bookmark that records the last-read position. `evtsession.Session` handles connections to remote event log sources. `publishermetadatacache` (under `comp/publishermetadatacache`) relies on `winutil/eventlog` APIs to render provider-specific message strings. |
| [`pkg/persistentcache`](../../pkg/persistentcache.md) | Bookmark persistence is implemented by `persistentCacheSaver` (in `impl/check/bookmark_saver.go`), which writes and reads the event log bookmark XML as a plain string via `persistentcache.Write` / `persistentcache.Read`. The cache key is derived from the check ID (e.g. `win32_event_log:<hash>`), which maps to `<run_path>/win32_event_log/<hash>`. On restart the check calls `bookmarkManager.Load()` to resume from the saved position, preventing duplicate or missed events. `bookmark_frequency` controls how often `bookmarkManager.Save()` is called inside the `fetchEventsLoop`. |
| [`comp/core/config`](../core/config.md) | `configComponent.Component` is injected into the check factory to read agent-wide settings (e.g. `logs_enabled`) that affect how events are routed between the Datadog Events pipeline and the Logs pipeline. |

### Event routing: Events vs. Logs pipeline

The check supports two output paths depending on how the instance is configured:

- **Datadog Events** (default): events are submitted via `sender.Event(...)` using `ddevent_submitter.go`. Each Windows event record becomes one Datadog event.
- **Datadog Logs** (integration logs path): when `logsAgent.Component` is present (injected as `option.Option`), events are forwarded through `ddlog_submitter.go` to the logs pipeline. This path is used when `logs_enabled: true` in `datadog.yaml` and a logs section is configured, or when `dd_security_events` is specified.

### Background collection loop

Unlike most agent checks where `Run()` performs all collection, `win32_event_log` runs a persistent goroutine (`fetchEventsLoop`) that consumes `sub.GetEvents()` continuously. `Run()` only:

1. Starts (or restarts) the `PullSubscription` if it is not running.
2. Saves the bookmark at the configured `bookmark_frequency`.

This design keeps event latency low and decouples the check scheduler's tick rate from the Windows Event Log arrival rate.