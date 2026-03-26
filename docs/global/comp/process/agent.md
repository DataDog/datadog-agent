# comp/process/agent — Process Agent Orchestration Component

**Import path:** `github.com/DataDog/datadog-agent/comp/process/agent`
**Team:** container-experiences
**Importers:** ~6 packages

## Purpose

`comp/process/agent` is the top-level orchestration component for the process-agent subsystem. Its primary responsibilities are:

1. **Enablement decision** — determine at startup whether the process-agent functionality should be active, based on the current agent flavor, enabled checks, and configuration.
2. **Status and flare integration** — expose process check output through the agent's status page and flare bundle when the process subsystem runs embedded in the core agent (i.e., not as a standalone `process-agent` binary).

The component intentionally has a thin interface: it does not start goroutines or own I/O. The actual check scheduling is handled by `comp/process/runner`, and result submission by `comp/process/submitter`. The agent component simply ties the enablement logic together and acts as the gateway for status/flare providers.

## Package layout

| Package | Role |
|---|---|
| `comp/process/agent` (root) | `Component` interface, `Enabled()` helper, `FlareHelper`, `StatusProvider`, platform-specific enablement logic |
| `comp/process/agent/agentimpl` | `newProcessAgent` constructor, `Module()` |
| `agent_linux.go` | Linux-specific `Enabled()`: process checks run in the core agent; standalone process-agent only needed for NPM (connections check) |
| `agent_fallback.go` | Non-Linux fallback `Enabled()` |

## Component interface

```go
// Package: github.com/DataDog/datadog-agent/comp/process/agent
type Component interface {
    Enabled() bool
}
```

`Enabled()` returns whether the process subsystem is active. It is evaluated once and memoised (via `sync.Once` on Linux) to prevent duplicate log output.

## fx wiring

```go
import "github.com/DataDog/datadog-agent/comp/process/agent/agentimpl"

agentimpl.Module()
```

`agentimpl.Module()` is included in `comp/process.Bundle()` and should not normally need to be added manually.

**Dependencies (`agentimpl.dependencies`):**

| Dependency | Purpose |
|---|---|
| `[]types.CheckComponent` (`group:"check"`) | Determines which checks are enabled; used by `Enabled()` |
| `runner.Component` | Holds the running check scheduler |
| `submitter.Component` | Holds the result submitter |
| `config.Component` | Used by `Enabled()` to read process configuration |
| `sysprobeconfig.Component` | Passed to `expvars.InitProcessStatus` when running embedded |
| `hostinfo.Component` | Passed to `expvars.InitProcessStatus` |
| `hostname.Component` | Used by the status provider |
| `log.Component` | Logging during enablement check |

**Outputs (`agentimpl.provides`):**

| Output | Description |
|---|---|
| `agent.Component` | The component itself |
| `statusComponent.InformationProvider` | Registered only when running embedded in the core agent (not as `process-agent` flavor) |
| `flaretypes.Provider` | Adds check output JSON files to the flare; also only registered when embedded |

## Enablement logic

`Enabled()` (and the underlying `enabledHelper` on Linux) applies the following rules:

- **CLC Runner** (cluster-level check runner): always disabled.
- **`ProcessAgent` flavor** (standalone `process-agent` binary):
  - On Linux: only enabled when the connections (NPM) check is active, since process checks run in the core agent on Linux.
  - On other platforms: standard flag/config check.
- **`DefaultAgent` flavor** (embedded in core agent):
  - Always enabled (returns `true`) so that process checks run in-process.
  - If NPM is also enabled, a warning is logged that a separate `process-agent` will be needed for NPM.
- **Other flavors**: disabled.

If `Enabled()` returns `false`, or if no checks pass `IsEnabled()`, the component is created in a disabled state that simply returns `false` from `Enabled()` with no side effects.

## Flare integration

`FlareHelper` iterates over the enabled non-realtime checks and calls `checks.GetCheckOutput(name)` to retrieve the last check result, serialising it as indented JSON into `<checkname>_check_output.json` files inside the flare bundle.

## Status page

When embedded in the core agent, the component registers a `StatusProvider` backed by `agent.NewStatusProvider`, which renders check metadata and process collection status into the agent's status page template (`processcomponent.tmpl`).

## Usage

The component is used in three ways:

1. **`comp/process/runner/runnerimpl`** — calls `agent.Enabled()` to decide whether to attach `OnStart`/`OnStop` lifecycle hooks to the runner (i.e., whether to actually start the check scheduler). See [comp/process/runner](runner.md) for the lifecycle details.

2. **`comp/process/submitter/submitterimpl`** — not a direct consumer, but submitter checks `Enabled()` indirectly through `agentEnabled` to decide whether to hook its lifecycle. See [comp/process/submitter](submitter.md) for submission flow details.

3. **Agent binary startup** (`cmd/agent/subcommands/run`, `cmd/process-agent/command/main_common`) — includes `comp/process.Bundle()` which pulls in `agentimpl.Module()`.

**Example: checking enablement at startup**

```go
// Inside cmd/process-agent or cmd/agent startup
// (done automatically via comp/process.Bundle)
if !agentComp.Enabled() {
    log.Info("Process agent component disabled, skipping")
    return
}
```

## Related documentation

| Document | Relationship |
|---|---|
| [comp/process/types](types.md) | Defines `CheckComponent` and `ProvidesCheck`; this component injects the `"check"` group to evaluate which checks are active |
| [comp/process/runner](runner.md) | Depends on `agent.Component`; registers `OnStart`/`OnStop` hooks only when `Enabled()` is true |
| [pkg/process/process.md](../../../global/pkg/process/process.md) | Describes the overall startup sequence and the role of `CheckRunner` that this component orchestrates |
