# comp/systray/systray

## Purpose

The `systray` component implements the Windows system tray application (`ddtray.exe`). It creates a notification icon in the Windows taskbar notification area that gives users a graphical interface to start, stop, and restart the Datadog Agent Windows service, open the web-based configuration UI, submit a diagnostic flare, and exit the tray application.

This component is **Windows-only** (`//go:build windows` on all implementation files). There is no macOS implementation; the build tag restricts compilation entirely.

## Key elements

### Component interface

```go
// comp/systray/systray/def/component.go (build: windows)
type Component interface{}
```

The interface is a marker type. All behaviour is managed through the fx lifecycle hooks registered in `NewComponent`.

### Params

```go
type Params struct {
    LaunchGuiFlag      bool   // open the web UI on start
    LaunchElevatedFlag bool   // (reserved) request UAC elevation
    LaunchCommand      string // run a named service command immediately on start
}
```

`Params` is supplied by the CLI command and drives initial behaviour.

### Implementation: `systrayImpl`

Located in `comp/systray/systray/impl/systray.go` (Windows only).

| Field | Type | Role |
|---|---|---|
| `shutdowner` | `compdef.Shutdowner` | Triggers fx app shutdown when the tray icon is closed |
| `config` | `config.Component` | Reads agent config (IPC address, port) |
| `flare` | `flare.Component` | Creates and uploads flare bundles |
| `diagnose` | `diagnose.Component` | Runs local diagnostics for the flare |
| `client` | `ipc.HTTPClient` | HTTP client for Agent IPC calls |
| `singletonEventHandle` | `windows.Handle` | Named Windows event ensuring only one ddtray instance runs |

### Lifecycle

**`start`** (fast, called by fx):
1. If `LaunchGuiFlag` is set, opens the configuration browser in a goroutine.
2. Acquires the `ddtray-event` named Windows event to enforce single-instance semantics. Fails if another instance is already running.
3. Spawns `windowRoutine` in a new goroutine (OS-thread-locked per `lxn/walk` requirements).
4. If `LaunchCommand` is non-empty, executes that service command immediately.

**`stop`** (called by fx):
1. Posts `WM_QUIT` to the message loop via `notifyWindowToStop`.
2. Waits for `windowRoutine` to exit via `routineWaitGroup`.
3. Closes the singleton event handle.

### Window and message loop

`windowRoutine` runs on an OS-locked goroutine (required by `lxn/walk`/Win32). It:

1. Creates a `walk.MainWindow` to host the message loop.
2. Creates a `walk.NotifyIcon` (system tray icon) with the Datadog icon loaded from the binary's resource section (ID `RSRC_MAIN_ICON = 1`).
3. Falls back to `LoadImageW` for Windows 7/2008 R2 where `LoadIconWithScaleDown` is unavailable.
4. Attaches a left-click handler and builds the context menu.
5. Runs `mw.Run()` (blocking message loop).

### Context menu items

| Label | Command |
|---|---|
| Agent version (greyed out) | — |
| Start | `controlsvc.StartService()` |
| Stop | `controlsvc.StopService()` |
| Restart | `controlsvc.RestartService()` |
| Configure | Opens `http://127.0.0.1:<gui_port>/` via `rundll32` (launches non-elevated via `LaunchUnelevated` C helper) |
| Flare | Opens a Win32 dialog box to collect ticket/email, then calls the Agent IPC `/agent/flare` endpoint or falls back to a local flare |
| Exit | Calls `shutdowner.Shutdown()` |

### UAC and elevation

The `uac.c` / `uac.h` CGo helper implements `LaunchUnelevated`, which uses `IShellDispatch2::ShellExecute` to open a URL as the non-elevated user even when `ddtray.exe` runs elevated. This prevents the configuration browser from inheriting administrator privileges.

`NewComponent` fails immediately if the current process is not running as an administrator, since service control operations require it.

### Flare flow

When the user clicks "Flare":

1. A modal Win32 dialog (resource `IDD_DIALOG1 = 101`) collects ticket number and email. The OK button is disabled until the email field matches a basic regex.
2. The component POSTs to `https://<ipc_address>:<cmd_port>/agent/flare` via `ipc.HTTPClient`. If the Agent is responsive, it builds the flare and returns the file path.
3. If the Agent IPC call fails, the component builds a local flare by running diagnose suites (port conflicts, event platform connectivity, autodiscovery connectivity, core endpoint connectivity, firewall scan) and calling `flare.Create`.
4. Either way, `flare.Send(filePath, caseID, email, ...)` uploads the bundle and shows the response in a `MessageBox`.

## Usage

`ddtray.exe` is a standalone binary built from `cmd/systray/`. Its `command.go` wires the fx app:

```go
import (
    systray    "github.com/DataDog/datadog-agent/comp/systray/systray/def"
    systrayfx  "github.com/DataDog/datadog-agent/comp/systray/systray/fx"
)

// The systray.Params are provided by cobra flags parsed in the command.
fx.Supply(systray.Params{LaunchGuiFlag: ..., LaunchCommand: ...})
systrayfx.Module()
```

The component is **not** included in the main `datadog-agent.exe` binary; it runs only as the standalone tray application.

### Flare flow — IPC vs. local fallback

The systray uses `ipc.HTTPClient` to POST to `https://<ipc_address>:<cmd_port>/agent/flare`.
The Agent's CMD API server protects that endpoint with `ipc.HTTPMiddleware`
(bearer-token authentication). If the agent is unreachable:

1. The systray calls `diagnose.Component` to run local diagnostic suites
   (port conflicts, event-platform connectivity, autodiscovery, firewall scan).
2. It calls `flare.Component.Create` to build an archive from locally available
   data only (same as `datadog-agent flare --local`).
3. It calls `flare.Component.Send` to upload the archive.

This mirrors the CLI fallback documented in
[comp/core/flare](../comp/core/flare.md#triggering-a-flare-from-the-cli).

### Service control operations

The context menu items **Start**, **Stop**, and **Restart** delegate to
`pkg/util/winutil` (`controlsvc.StartService`, `controlsvc.StopService`,
`controlsvc.RestartService`). These wrappers handle `StopPending`/`StartPending`
races and stop dependent services before stopping the main service. See
[pkg/util/winutil](../../pkg/util/winutil.md) for the full SCM API.

Administrator privileges are enforced at `NewComponent` time via
`winutil.IsUserAnAdmin()`. The process fails immediately if not elevated,
preventing confusing partial failures later.

### Opening the GUI without elevation

The Configure menu item opens the local web UI at
`http://127.0.0.1:<gui_port>/`. Because `ddtray.exe` runs elevated,
it uses the `LaunchUnelevated` CGo helper (`uac.c`) to invoke
`IShellDispatch2::ShellExecute` and open the URL as the non-elevated user,
preventing the browser from inheriting administrator privileges.

### Mock

`comp/systray/systray/mock/mock.go` provides a no-op `Component` for use in tests that need to satisfy the fx dependency graph without instantiating a real Win32 window.

---

## Related packages and components

| Package / Component | Doc | Relationship |
|---|---|---|
| `comp/core/ipc` | [../core/ipc.md](../core/ipc.md) | Provides the `ipc.HTTPClient` used for both the flare IPC call (`POST /agent/flare`) and any other agent API calls from the tray. The client adds bearer-token auth and mutual-TLS; the running agent validates this via `HTTPMiddleware`. |
| `comp/core/flare` | [../core/flare.md](../core/flare.md) | Provides `flare.Component` used for local fallback flare creation (when the Agent is unreachable). The systray calls `Create` to build the archive and `Send` to upload it — the same interface used by the CLI `datadog-agent flare` subcommand. |
| `pkg/util/winutil` | [../../pkg/util/winutil.md](../../pkg/util/winutil.md) | Provides the `StartService`, `StopService`, `RestartService`, and `IsUserAnAdmin` functions used by the tray's context menu and its elevation guard. The `servicemain` sub-package documents the broader Windows Service lifecycle model. |
