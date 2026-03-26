> **TL;DR:** `comp/collector/collector` is the check scheduling and execution engine that accepts Go and Python check instances, runs them through a worker pool, and notifies autodiscovery and metadata components of lifecycle events.

# comp/collector/collector

**Team:** agent-runtimes
**Import path:** `github.com/DataDog/datadog-agent/comp/collector/collector`
**fx module:** `github.com/DataDog/datadog-agent/comp/collector/collector/collectorimpl`

## Purpose

`comp/collector/collector` is the check scheduling and execution engine for
the Datadog Agent. It accepts `check.Check` instances (Go or Python),
schedules them via a `scheduler.Scheduler`, and executes them through a pool
of `runner.Runner` workers.

Responsibilities:

- Runs and stops individual check instances on demand.
- Dynamically adjusts the worker pool size as check instances are added and
  removed (long-running checks get a dedicated extra worker).
- Handles check reloads atomically: stop all old instances, start new ones.
- Emits `CheckRun` / `CheckStop` events to registered observers (used by
  autodiscovery, inventory metadata, etc.).
- Exposes a status provider and metadata provider as fx outputs (no separate
  registration needed).
- Serves the `/py/status` HTTP endpoint with Python runtime information.

## Key elements

### Key interfaces

#### Interface (`comp/collector/collector/component.go`)

```go
type Component interface {
    // RunCheck enqueues a check for execution, starts a worker if needed,
    // and returns the assigned check ID. Fails if the collector is not
    // running or the check is already registered.
    RunCheck(inner check.Check) (checkid.ID, error)

    // StopCheck cancels the schedule, waits for any running execution to
    // finish (with a configurable timeout), and calls check.Cancel().
    StopCheck(id checkid.ID) error

    // MapOverChecks calls cb with a snapshot of running check.Info values
    // while holding the read lock.
    MapOverChecks(cb func([]check.Info))

    // GetChecks returns a copy of all currently registered check.Check
    // instances.
    GetChecks() []check.Check

    // ReloadAllCheckInstances stops every instance of a named check and
    // starts the provided replacements. Returns the IDs of the stopped
    // instances.
    ReloadAllCheckInstances(name string, newInstances []check.Check) ([]checkid.ID, error)

    // AddEventReceiver registers a callback invoked on CheckRun and
    // CheckStop events.
    AddEventReceiver(cb EventReceiver)
}
```

### Key types

#### EventType / EventReceiver

```go
type EventType uint32
const (
    CheckRun  EventType = iota // check was registered and scheduled
    CheckStop                  // check was stopped and removed
)

type EventReceiver func(checkid.ID, EventType)
```

### Key functions

#### `NoneModule()`

`NoneModule()` provides `option.None[Component]()` for processes that need to
satisfy optional collector dependencies without linking the full implementation
(e.g. serverless, some test helpers).

#### Internal structure (`collectorimpl`)

`collectorImpl` holds:

| Field | Purpose |
|---|---|
| `scheduler *scheduler.Scheduler` | Dispatches checks to the runner channel on their configured interval. |
| `runner *runner.Runner` | Pool of goroutines that call `check.Run()`. |
| `checks map[checkid.ID]*middleware.CheckWrapper` | Live check registry. |
| `eventReceivers []EventReceiver` | Notification callbacks. |
| `state *atomic.Uint32` | `stopped` (0) or `started` (1). |
| `cancelCheckTimeout` | Max wait for `check.Cancel()` (`check_cancel_timeout`). |
| `watchdogWarningTimeout` | Threshold before a watchdog warning is emitted. |

#### CheckWrapper (internal middleware)

`middleware.CheckWrapper` decorates each `check.Check` with:

- A `sync.Mutex` to prevent `Cancel()` from racing with `Run()`.
- Sender cleanup (`senderManager.DestroySender`) after cancellation.
- Optional agent-telemetry span around each `Run()` call.

#### fx outputs

`collectorimpl.Module()` provides four values:

| Type | Description |
|---|---|
| `collector.Component` | The main scheduler/runner component. |
| `option.Option[collector.Component]` | A non-None optional for callers that accept `option.Option[collector.Component]`. |
| `status.InformationProvider` | Feeds the collector section in `agent status`. |
| `metadata.Provider` | Reports check metadata to the Agent's metadata pipeline. |
| `api.AgentEndpointProvider` | Registers the `GET /py/status` endpoint. |

### Configuration and build flags

#### Dependencies

| Dep | Purpose |
|---|---|
| `config.Component` | Reads `check_cancel_timeout`, `python_lazy_loading`, `shared_library_check.enabled`, etc. |
| `sender.SenderManager` | Passed to each check so it can emit metrics. |
| `haagent.Component` | HA-aware routing for check execution. |
| `hostnameinterface.Component` | Provides hostname to the runner. |
| `option.Option[healthplatform.Component]` | Optional health-platform integration. |
| `option.Option[serializer.MetricSerializer]` | Required for the metadata provider; omitted if not set. |

## Usage

### Starting the collector in the core agent

`cmd/agent/subcommands/run/command.go` includes `collectorimpl.Module()` in
the main fx app. After the app starts, autodiscovery connects the collector
via `InitCheckScheduler`:

```go
ac.AddScheduler("check",
    pkgcollector.InitCheckScheduler(
        option.New(collectorComponent), demultiplexer, logReceiver, tagger, filterStore,
    ),
    true,
)
```

`InitCheckScheduler` wraps the component in an autodiscovery-compatible
scheduler interface. When autodiscovery discovers a new check configuration it
calls `RunCheck`; when it removes one it calls `StopCheck` or
`ReloadAllCheckInstances`.

### Cluster agent and Cloud Foundry agent

`cmd/cluster-agent` and `cmd/cluster-agent-cloudfoundry` also use
`collectorimpl.Module()` to run cluster-level checks.

### Check CLI (`agent check`)

`pkg/cli/subcommands/check/command.go` imports the component interface to
introspect running checks via `GetChecks()` / `MapOverChecks()`.

### Inventory metadata

`comp/metadata/inventorychecks/inventorychecksimpl` subscribes to
`AddEventReceiver` to track which checks are running and include that
information in inventory payloads sent to Datadog.

### Diagnose

`comp/core/diagnose/local/local.go` calls `collector.Diagnose(component, log)`
(defined in `comp/collector/collector/diagnose.go`) to run check-level
diagnose routines on demand.

### Disabling the collector

When a process should not run any checks but still needs to satisfy optional
dependencies, use:

```go
collector.NoneModule()
```

This avoids linking the full runner/scheduler code.

---

## Related packages and components

| Package / Component | Doc | Relationship |
|---|---|---|
| `pkg/collector` | [../../pkg/collector/collector.md](../../pkg/collector/collector.md) | Lower-level building blocks. This component wraps `runner.Runner` + `scheduler.Scheduler` from `pkg/collector`. `InitCheckScheduler` (from `pkg/collector`) bridges autodiscovery to the component's `RunCheck` / `StopCheck` / `ReloadAllCheckInstances` methods. |
| `pkg/collector/runner` | [../../pkg/collector/runner.md](../../pkg/collector/runner.md) | Worker pool. `collectorimpl` constructs a `*runner.Runner`, obtains the pending-checks channel via `runner.GetChan()`, and passes it to the scheduler. The runner scales the worker pool with `UpdateNumWorkers` as checks are added. Long-running (interval `0`) checks get a dedicated extra worker via `runner.AddWorker()`. |
| `pkg/collector/scheduler` | [../../pkg/collector/scheduler.md](../../pkg/collector/scheduler.md) | Interval-based dispatch. The scheduler receives checks from `RunCheck` via `scheduler.Enter`, groups them by interval into `jobQueue` objects, and dispatches due checks onto the runner channel. On shutdown the collector calls `scheduler.Stop()` before `runner.Stop()` to prevent writes to the closed channel. |
| `comp/core/autodiscovery` | [../core/autodiscovery.md](../core/autodiscovery.md) | Config source. After the fx app starts, `InitCheckScheduler` wraps this component in an autodiscovery-compatible `Scheduler` interface and registers it with `ac.AddScheduler("check", ...)`. Autodiscovery calls `Schedule` / `Unschedule` as container/pod configs appear and disappear, which in turn calls `RunCheck` / `StopCheck` / `ReloadAllCheckInstances` on this component. |
