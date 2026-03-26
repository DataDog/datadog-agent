# comp/logonduration/impl

**Package:** `github.com/DataDog/datadog-agent/comp/logonduration`
**Definition:** `comp/logonduration/def`
**Implementation:** `comp/logonduration/impl`
**fx wiring:** `comp/logonduration/fx`
**Team:** windows-products
**Platform:** Windows (full implementation), macOS (stub for testing), other platforms (no-op)

## Purpose

The logon duration component measures how long Windows takes to boot and present an interactive desktop to the user after a reboot. It parses a Windows ETL (Event Trace Log) file written by a pre-configured ETW AutoLogger session, extracts a timeline of key OS milestones (kernel start, Winlogon, Explorer initialisation, desktop ready), calculates durations, and submits a single "Logon duration" event to the Datadog Event Management v2 API after every detected reboot.

This data lets operators monitor user experience degradation from slow Group Policy processing, profile loading, or startup application delays on managed Windows machines.

## Key elements

### Component interface

```go
// comp/logonduration/def/component.go
type Component interface{}
```

The interface is intentionally empty. All work is managed internally via fx.Lifecycle `OnStart`/`OnStop` hooks. The component is used for its side effects (one-shot analysis and event submission at agent startup).

### Dependencies (Windows)

Injected via `Requires` struct in `impl/impl_windows.go`:

| Dependency | Role |
|---|---|
| `compdef.Lifecycle` | Registers `start`/`stop` hooks |
| `configcomp.Component` | Reads `logon_duration.enabled` config key |
| `eventplatform.Component` | Provides the `Forwarder` used to submit events to Datadog |
| `hostname.Component` | Used to populate the `host` field in the event payload |

### Boot analysis workflow

The component's `run()` method executes once at agent startup, after `OnStart` fires:

1. **Stop the active ETL session.** The AutoLogger was started by the OS at boot time. The agent stops it to flush and finalise the ETL file.
2. **Re-arm the AutoLogger.** Sets the `Start` registry value to `1` under `HKLM\SYSTEM\CurrentControlSet\Control\WMI\Autologger\Datadog Logon Duration` so the trace session will run again on the next boot.
3. **Reboot detection.** Reads the current boot time (via `gopsutil/host.BootTime`) and compares it to the value stored in the agent's persistent cache (`logon_duration:last_boot_time`). If the value has changed or is absent, a reboot is considered to have occurred.
4. **ETL parsing.** Calls `analyzeETL` on the file at `%ProgramData%\logonduration\logon_duration.etl`. This opens the ETL file, registers per-provider event parsers, and processes all events through the ETW `pkg/util/winutil/etw.ProcessETLFile` helper.
5. **Event submission.** Builds an Event Management v2 payload and sends it via `eventplatform.Forwarder.SendEventPlatformEventBlocking`.
6. **Cache update.** Writes the current boot time to the persistent cache so the next agent run skips analysis if no reboot has occurred.

### ETW providers monitored

The analyzer subscribes to six ETW providers to build the `BootTimeline`:

| Provider | GUID | Events tracked |
|---|---|---|
| Kernel-General | `{A68CA8B7-...}` | Boot start timestamp (Event 12) |
| Kernel-Process | `{22FB2CD6-...}` | Process starts: smss.exe, winlogon.exe, userinit.exe, explorer.exe (Event 1) |
| Winlogon | `{DBE9B383-...}` | Winlogon init, Login UI, logon start/stop, shell command execution (Events 101–104, 5001–5002, 9–10) |
| User Profile Service | `{89B1E9F0-...}` | Profile load and creation (Events 1–2, 1001–1002) |
| Group Policy | `{AEA1B4FA-...}` | Machine and user GP processing (Events 4000–4001, 8000–8001) |
| Shell-Core | `{30336ED4-...}` | Explorer initialisation steps: desktop create, desktop visible, startup apps, finalize (Events 9601–9602, 9611–9612, 9648–9649) |

### Key types

**BootTimeline** — struct with one `time.Time` field per milestone. Only non-zero fields are included in the output.

**Milestone** — a single timeline entry in the JSON payload:
```go
type Milestone struct {
    Name      string  `json:"name"`
    OffsetS   float64 `json:"offset_s"`    // seconds since BootStart
    DurationS float64 `json:"duration_s"`  // duration of the phase (0 if point-in-time)
    Timestamp string  `json:"timestamp"`
}
```

**AnalysisResult** — wrapper around `BootTimeline` returned by `analyzeETL`.

### Event payload

The submitted event uses the Event Management v2 format. The `custom` field contains:

```json
{
  "boot_timeline": [
    { "name": "Boot Start", "offset_s": 0, "duration_s": 0, "timestamp": "..." },
    { "name": "Winlogon Start", "offset_s": 3.2, "duration_s": 0, "timestamp": "..." },
    ...
  ],
  "durations": {
    "Boot Duration (ms)": 12400,
    "Logon Duration (ms)": 8300,
    "Total Boot Duration (ms)": 20700
  }
}
```

Durations are defined as:
- **Boot Duration**: `BootStart` → `LoginUIStart` (time until the login screen appears)
- **Logon Duration**: `LogonStart` → `DesktopVisibleStart` (time from credential submission to desktop visible)
- **Total Boot Duration**: sum of the two above (excludes idle time at the login screen)

### AutoLogger prerequisite

The component will not run unless an ETW AutoLogger named `"Datadog Logon Duration"` exists in the registry. This AutoLogger is created by the agent's Windows installer. If the key is absent at startup, the component logs a warning and returns without doing anything. This is a silent no-op on machines where the agent was not installed via the MSI.

### Configuration

| Key | Default | Description |
|---|---|---|
| `logon_duration.enabled` | `false` | Must be set to `true` to enable the component |

If disabled at startup, the component also disables the AutoLogger (sets `Start=0`) so it does not consume resources on the next boot.

## Usage

The component is registered in the main agent run command (`cmd/agent/subcommands/run/command.go`) via:

```go
logondurationfx.Module()
```

`logondurationfx.Module()` (in `comp/logonduration/fx/fx.go`) wires `NewComponent` and calls `fx.Invoke` to force instantiation even though the `Component` interface is empty. Start/stop are handled entirely through the fx lifecycle.

No other component depends on `logonduration.Component` or calls methods on it. It is a self-contained, side-effect-only component.

## Related components and packages

| Component / Package | Relationship |
|---|---|
| [`pkg/logonduration`](../pkg/logonduration.md) | Provides the **macOS** counterpart of this component. Where `comp/logonduration` parses a Windows ETL file, `pkg/logonduration` queries the macOS OSLogStore for `LoginWindowTime`, `LoginTime`, and `DesktopReadyTime`. The macOS path is exposed as a system-probe HTTP module (`/check` endpoint) rather than a standalone fx component. Both implementations share the same high-level goal — measuring user login latency — but differ entirely in OS primitives used. |
| [`comp/etw`](etw.md) | Provides the **real-time** ETW session interface used by network, security, and APM monitoring components. `comp/logonduration` does **not** use `comp/etw`; instead it relies on the lower-level `pkg/util/winutil/etw.ProcessETLFile` and `etw.StopETWSession` helpers to replay the boot-time ETL file written by the `"Datadog Logon Duration"` AutoLogger offline. The distinction is important: `comp/etw` manages live kernel sessions; `pkg/util/winutil/etw` reads pre-recorded trace files. |
| [`pkg/persistentcache`](../pkg/persistentcache.md) | Used on both Windows and macOS to persist the last recorded boot/login time across agent restarts. On Windows, `comp/logonduration` writes the current boot time under the key `logon_duration:last_boot_time` after each successful analysis; on subsequent starts it reads this key to determine whether a reboot has occurred since the last run. This avoids re-submitting a logon-duration event when the agent restarts without a preceding reboot. |
| [`comp/forwarder/eventplatform`](forwarder/eventplatform.md) | The transport for the logon-duration event. After building the `boot_timeline` and `durations` payload, the component calls `eventplatform.Forwarder.SendEventPlatformEventBlocking` with event type `EventTypeEventManagement`. The payload is a single Event Management v2 JSON message routed through the `event-management-intake.` stream pipeline — the same pipeline used by `comp/notableevents`. |
