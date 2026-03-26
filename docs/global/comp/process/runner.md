# comp/process/runner — Process Agent Check Runner Component

**Import path:** `github.com/DataDog/datadog-agent/comp/process/runner`
**Team:** container-experiences

## Purpose

`comp/process/runner` is the fx component that owns the check scheduling loop for the process agent. It wraps `pkg/process/runner.CheckRunner` and manages the lifecycle of periodic data collection: starting the runner on agent startup, stopping it gracefully on shutdown, and exposing the set of active checks to other components.

The runner receives all registered check components (process, connections, container, real-time container, process-discovery) via the fx `"check"` value group, filters to those that are enabled, and hands them to the underlying `CheckRunner`. It also wires the real-time notification channel so the submitter can signal when real-time mode should start or stop.

## Package layout

| Package | Role |
|---|---|
| `comp/process/runner` (root) | `Component` interface |
| `comp/process/runner/runnerimpl` | `runnerImpl` struct, `newRunner` constructor, `Module()` |

## Component interface

```go
// Package: github.com/DataDog/datadog-agent/comp/process/runner
type Component interface {
    GetChecks() []checks.Check
    GetProvidedChecks() []types.CheckComponent
}
```

| Method | Description |
|---|---|
| `GetChecks()` | Returns the enabled `checks.Check` objects that the runner is actively executing. |
| `GetProvidedChecks()` | Returns all registered `types.CheckComponent` instances, regardless of whether they are enabled. Useful for introspection (e.g., status display). |

## fx wiring

```go
import "github.com/DataDog/datadog-agent/comp/process/runner/runnerimpl"

runnerimpl.Module()
```

`runnerimpl.Module()` is included in `comp/process.Bundle()`.

**Dependencies (`runnerimpl.dependencies`):**

| Dependency | Purpose |
|---|---|
| `[]types.CheckComponent` (`group:"check"`) | All registered check components; filtered to enabled ones |
| `submitter.Component` | Assigned to `checkRunner.Submitter` for result delivery |
| `<-chan types.RTResponse` (`optional`) | Real-time notification channel from the submitter |
| `hostinfo.Component` | Passed to `processRunner.NewRunner` for host-level metadata |
| `sysprobeconfig.Component` | Passed to `processRunner.NewRunner` for system-probe awareness |
| `config.Component` | General agent configuration |
| `tagger.Component` | Used by checks for container tagging |
| `log.Component` | Logging |

**Lifecycle:**

When `agent.Enabled()` returns `true`, the runner appends fx lifecycle hooks:

- `OnStart` → `checkRunner.Run()` — starts the check scheduling goroutines.
- `OnStop` → `checkRunner.Stop()` — drains the scheduler and waits for running checks to complete.

If the agent is disabled, no lifecycle hooks are registered and the runner is a passive no-op (its methods still work for introspection).

## How the runner relates to other process components

```
comp/process/bundle
  ├── *checkimpl packages  ──→  types.ProvidesCheck ("check" group)
  │                                        │
  ├── runnerimpl  ←────────────────────────┘
  │     • filters enabled checks
  │     • calls pkg/process/runner.CheckRunner.Run()
  │     • GetChecks() / GetProvidedChecks()
  │
  ├── submitterimpl
  │     • receives Payload from CheckRunner via runner.Submitter
  │     • publishes RTResponse channel back to runner
  │
  └── agentimpl
        • reads runner.GetProvidedChecks() (indirectly via types group)
        • calls agent.Enabled() which runner also consults
```

## Usage

The component is consumed by:

- **`agentimpl`** — imports `runner.Component` as a dependency so that fx ensures the runner is initialised before the agent component, and to hold a reference for status/flare helpers.
- **Tests** — `runner.GetChecks()` and `runner.GetProvidedChecks()` are the primary assertion points in unit and integration tests that verify which checks are active under different configurations.

**Querying active checks in a test:**

```go
runnerComp := fxutil.Test[runner.Component](t, runnerimpl.Module(), ...)

checks := runnerComp.GetChecks()
// assert specific checks are or are not present

all := runnerComp.GetProvidedChecks()
// assert the full provided set, including disabled ones
```

**Real-time flow:**

The submitter publishes a `<-chan types.RTResponse` that the runner receives as an optional dependency. The underlying `CheckRunner` watches this channel to decide when to switch the process check between its standard interval and the shorter real-time interval.

## Notes

- `filterEnabledChecks` (package-private helper) iterates `[]types.CheckComponent` and calls `Object().IsEnabled()` on each, returning only the passing subset. This is called inside `newRunner` to build the initial enabled-check list for `processRunner.NewRunner`.
- `IsRealtimeEnabled()` (exposed for tests but not part of the `Component` interface) queries the underlying `CheckRunner` to verify whether real-time mode is currently active.

## Related documentation

| Document | Relationship |
|---|---|
| [comp/process/types](types.md) | Defines `CheckComponent`, `ProvidesCheck`, `Payload`, and `RTResponse`; the runner consumes the `"check"` group and the `<-chan types.RTResponse` channel |
| [comp/process/submitter](submitter.md) | The runner assigns its `Submitter` field to the submitter component and receives the `RTResponse` channel back from it |
| [pkg/process/runner](../../../global/pkg/process/runner.md) | The underlying `CheckRunner` and `CheckSubmitter` that this fx component wraps; documents queue configuration, RT signaling, and runner strategies |
| [pkg/process/checks](../../../global/pkg/process/checks.md) | Defines the `Check` interface (with `Run`, `Init`, `Cleanup`) that the runner invokes on every tick, and `RunnerWithRealTime` scheduling used for dual-frequency checks |
