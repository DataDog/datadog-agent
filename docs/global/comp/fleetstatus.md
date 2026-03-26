> **TL;DR:** `comp/fleetstatus` contributes a "Fleet Automation" section to the agent status output, combining the `remote_updates` config flag and the installer daemon running state to report whether fleet automation is fully active.

# comp/fleetstatus

**Team:** fleet

## Purpose

`comp/fleetstatus` contributes a "Fleet Automation" section to the agent status
output (both text and HTML). It surfaces whether remote management (fleet
automation) is active on the host by combining two signals:

1. Whether the `remote_updates` configuration flag is enabled.
2. Whether the Datadog Installer daemon is currently running (via the
   `daemonchecker` component).

Fleet automation is considered fully active only when both conditions are true.
This gives operators a quick way to confirm fleet automation state via
`datadog-agent status` without inspecting config or system service state
separately.

## Key elements

### Key interfaces

```go
// comp/fleetstatus/def/component.go
type Component interface{}
```

The component has no public methods. It contributes to the agent status
framework via `status.InformationProvider` — the fx graph wires the status
output without callers needing to interact with the component directly.

### Key types

**`statusProvider`** — the concrete implementation registers itself as a `status.InformationProvider`:

```go
type statusProvider struct {
    Config        config.Component
    DaemonChecker daemonchecker.Component
}
```

It implements the `status.Provider` interface:

| Method | Output |
|--------|--------|
| `Name()` | `"Fleet Automation"` |
| `Section()` | `"Fleet Automation"` |
| `JSON(_, stats)` | Populates `stats["fleetAutomationStatus"]` |
| `Text(_, buf)` | Renders `fleetstatus.tmpl` (text template) |
| `HTML(_, buf)` | Renders `fleetstatus.tmpl` (HTML template) |

### Key functions

**`populateStatus`** produces the following map under `fleetAutomationStatus`:

```json
{
  "remoteManagementEnabled": true,
  "installerRunning": true,
  "fleetAutomationEnabled": true
}
```

- `remoteManagementEnabled` — `true` when `remote_updates: true` in
  `datadog.yaml`.
- `installerRunning` — `true` when `daemonchecker.IsRunning()` returns true.
- `fleetAutomationEnabled` — `true` only when both of the above are true.

### Configuration and build flags

The text and HTML templates are embedded at compile time from the `status_templates/` directory inside the `impl` package.

**fx wiring:**

```go
// comp/fleetstatus/fx/fx.go
func Module() fxutil.Module {
    return fxutil.Component(fx.Provide(NewComponent))
}
```

`NewComponent` depends on `config.Component` and `daemonchecker.Component`, and provides `status.InformationProvider`.

| Key | Description |
|---|---|
| `remote_updates` | When `true`, signals that fleet remote management is configured. Combined with `daemonchecker.IsRunning()` to set `fleetAutomationEnabled`. |

## Usage

The component is registered in the main agent's fx graph. No explicit
interaction is needed: it automatically appears in `datadog-agent status` under
the "Fleet Automation" section.

```
Fleet Automation
================
  Remote Management Enabled: true
  Installer Running:          true
  Fleet Automation Enabled:   true
```

To modify what is displayed, update `populateStatus` in
`comp/fleetstatus/impl/fleetstatus.go` and adjust the template in
`comp/fleetstatus/impl/status_templates/fleetstatus.tmpl`.

## Relationship to fleet and status components

| Component / Package | Relationship |
|---|---|
| [`pkg/fleet`](../../pkg/fleet/fleet.md) | The fleet daemon (`pkg/fleet/daemon.Daemon`) is started by `comp/updater` when `remote_updates: true`. `comp/fleetstatus` only *reports* whether fleet automation is active — it does not interact with the daemon directly. |
| [`comp/core/status`](core/status.md) | Provides the `status.InformationProvider` extension point. `comp/fleetstatus` registers one `InformationProvider` (section `"Fleet Automation"`) that the status component aggregates into the full `datadog-agent status` output. |
| `comp/updater` | Hosts `pkg/fleet/daemon.Daemon`. Fleet automation is only fully active when both `remote_updates` is enabled (read by `comp/fleetstatus` via `config.Component`) and this daemon is running (detected by `daemonchecker.Component`). |

### Adding new fleet status fields

1. Add the field to the `populateStatus` function in `comp/fleetstatus/impl/fleetstatus.go`.
2. Update the `fleetstatus.tmpl` template in `comp/fleetstatus/impl/status_templates/` (both text and HTML variants if they differ).
3. The JSON output consumed by automation tools is populated from the same `stats` map — no separate change is needed for `JSON()` unless you add a new sub-map.
