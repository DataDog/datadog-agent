> **TL;DR:** The check execution engine root package, providing the `CheckScheduler` that bridges autodiscovery configs to running check instances by resolving them through a priority-ordered loader registry and submitting them to the collector component.

# Package `pkg/collector`

## Purpose

`pkg/collector` is the root package of the check execution engine. It owns the **`CheckScheduler`**, which bridges autodiscovery configs to running check instances: it resolves `integration.Config` objects into `check.Check` instances (via a registry of loaders), submits them to the collector component for execution, and removes them when the config disappears.

The package also provides:
- infrastructure-mode gating (`IsCheckAllowed`) so that checks can be silently suppressed in restricted deployment modes.
- a thread-safe error tracking store (`collectorErrors`) for loader and run errors, exposed through `expvar` at the `CheckScheduler` key.

The **collector component** (`comp/collector/collector`) is the fx component that glues together the lower-level building blocks in `pkg/collector`:

| Sub-package | Responsibility |
|---|---|
| `pkg/collector/scheduler` | Interval-based job queues; pushes checks onto a channel at the right cadence |
| `pkg/collector/runner` | Pulls checks from the channel; manages a pool of Workers |
| `pkg/collector/worker` | Executes a single check, records stats, emits service checks |
| `pkg/collector/loaders` | Registry of `check.Loader` factories, sorted by priority |

### Related documentation

| Document | What it covers |
|---|---|
| [`pkg/collector/check`](check.md) | `Check`, `Loader`, `Info` interfaces; `CheckBase`; check IDs and stats |
| [`pkg/collector/runner`](runner.md) | Worker pool management and dynamic scaling |
| [`pkg/collector/scheduler`](scheduler.md) | Interval queues, jitter, and one-shot dispatch |
| [`pkg/collector/worker`](worker.md) | Per-goroutine check execution, HA gating, service check emission |
| [`pkg/collector/loaders`](loaders.md) | Loader registration, priority ordering, and the loader catalog |
| [`pkg/collector/corechecks`](corechecks.md) | Built-in Go checks and `CheckBase` |
| [`comp/collector/collector`](../../comp/collector/collector.md) | fx component wrapping runner + scheduler; entry point for the main Agent |

## Key Elements

### Key types

### `CheckScheduler`

`CheckScheduler` is the autodiscovery subscriber for check configs. It is distinct from `pkg/collector/scheduler.Scheduler` (which handles *timing*) — `CheckScheduler` handles *config-to-check resolution*.

```go
type CheckScheduler struct {
    configToChecks map[string][]checkid.ID
    loaders        []check.Loader
    collector      option.Option[collector.Component]
    senderManager  sender.SenderManager
    m              sync.RWMutex
}
```

The central object. Created with `InitCheckScheduler` and kept as a package-level singleton.

| Method | Description |
|---|---|
| `InitCheckScheduler(...)` | Constructs the scheduler and registers all loaders from `loaders.LoaderCatalog` |
| `Schedule(configs []integration.Config)` | Loads checks from configs and calls `collector.RunCheck` for each |
| `Unschedule(configs []integration.Config)` | Stops checks that correspond to the given configs |
| `GetChecksFromConfigs(configs, populateCache bool)` | Instantiates checks without scheduling them; optionally updates the internal config→IDs cache |
| `GetChecksByNameForConfigs(name, configs)` | Package-level helper to find running checks by name |

For each config, `Schedule` iterates the loaders returned by `loaders.LoaderCatalog` in priority order (see [`loaders.md`](loaders.md)). The first loader whose `Load()` call succeeds produces the `check.Check`. `check.ErrSkipCheckInstance` is handled gracefully — the check is silently skipped rather than logged as an error.

### `IsCheckAllowed`

```go
func IsCheckAllowed(checkName string, cfg pkgconfigmodel.Reader) bool
```

Gate used before scheduling each check. Returns `false` when `integration.enabled` is `false`, or when the check is on the `integration.excluded` list, or when the check is not in the `integration.<mode>.allowed` list (unless the check name starts with `custom_`). Infrastructure mode is read from `infrastructure_mode`.

This function runs before `loaders` are consulted, so a rejected check never reaches any loader.

### `collectorErrors`

Internal thread-safe struct that accumulates:
- **loader errors** – per check name, per loader: which loader failed and why.
- **run errors** – per check ID: the most recent scheduling or run failure.

Both are exposed via `expvar` through the `CheckScheduler` map and via `GetLoaderErrors()`.

### `loaders.LoaderCatalog`

```go
func LoaderCatalog(senderManager, logReceiver, tagger, filter) []check.Loader
```

Built once (via `sync.Once`). Factories registered with `RegisterLoader` are sorted by their declared priority integer. The scheduler tries each loader in order until one succeeds for a given instance. See [`loaders.md`](loaders.md) for the full priority table and how to register a new loader.

### `scheduler.Scheduler`

Manages one `jobQueue` per unique interval. Checks with `Interval() == 0` are enqueued once immediately. A `Scheduler` must be started with `Run()` before it begins dispatching. See [`scheduler.md`](scheduler.md) for full details including the jitter mechanism.

| Method | Description |
|---|---|
| `Enter(check)` | Adds a check to its interval queue |
| `Cancel(id)` | Removes a check from its queue |
| `IsCheckScheduled(id)` | Returns whether a check is still in the schedule |
| `Stop()` | Shuts down all queues gracefully |

Minimum allowed interval: **1 second**.

### `runner.Runner`

Owns a pool of `worker.Worker` goroutines. The pool size scales with the number of scheduled checks (unless `check_runners` is set explicitly). Workers pull from the shared `pendingChecksChan` channel. See [`runner.md`](runner.md) for the dynamic scaling thresholds and shutdown behaviour.

| Method | Description |
|---|---|
| `NewRunner(...)` | Creates a runner and spawns the initial worker pool |
| `AddWorker()` | Adds one more worker to the pool |
| `UpdateNumWorkers(numChecks)` | Grows the pool if needed (stepped thresholds: 4/10/15/20/`MaxNumWorkers`) |
| `StopCheck(id)` | Signals a running check to stop; times out after 500 ms |
| `Stop()` | Closes the channel, cancels context, waits up to 2 s for all workers |

### `worker.Worker`

Each worker loops over the `pendingChecksChan`. For every check it:
1. Checks HA leadership (skips if not leader for HA-supported checks).
2. Guards against concurrent runs via `RunningChecksTracker`.
3. Calls `check.Run()`.
4. Emits a `datadog.agent.check_status` service check if `integration_check_status_enabled` is set.
5. Commits sender stats and records them in expvars.
6. Reports failures to the health platform component.

Worker utilization is tracked with an exponential moving average and exposed as the `collector.worker_utilization` telemetry gauge. See [`worker.md`](worker.md) for the full execution sequence and utilization monitoring.

### `collector.Component` (fx interface at `comp/collector/collector`)

```go
type Component interface {
    RunCheck(inner check.Check) (checkid.ID, error)
    StopCheck(id checkid.ID) error
    MapOverChecks(cb func([]check.Info))
    GetChecks() []check.Check
    ReloadAllCheckInstances(name string, newInstances []check.Check) ([]checkid.ID, error)
    AddEventReceiver(cb EventReceiver)
}
```

The fx-managed wrapper around `Runner` + `Scheduler`. Emits `CheckRun` and `CheckStop` events to registered receivers. Each `check.Check` is wrapped in `middleware.CheckWrapper` which handles mutex-guarded `Cancel()`, `DestroySender` cleanup, and optional agent-telemetry spans. See [`comp/collector/collector.md`](../../comp/collector/collector.md) for the full fx wiring.

## Usage

### Wiring at agent startup

The main agent (`cmd/agent/subcommands/run`) wires things together via fx:

```go
collectorimpl.Module()  // provides collector.Component, status provider, metadata provider
```

`collectorimpl.Module()` registers an fx lifecycle hook that calls `runner.NewRunner` + `scheduler.NewScheduler` on `OnStart` and tears them down on `OnStop`. See [`comp/collector/collector.md`](../../comp/collector/collector.md) for the full fx dependency list.

### Scheduling checks from autodiscovery

`CheckScheduler` is the `scheduler` subscriber registered with the autodiscovery component. It acts as the glue between autodiscovery and the `collector.Component`:

```go
ac.AddScheduler("check",
    pkgcollector.InitCheckScheduler(
        option.New(collectorComponent), demultiplexer, logReceiver, tagger, filterStore,
    ),
    true,
)
// autodiscovery calls cs.Schedule / cs.Unschedule as configs appear/disappear
```

The `SenderManager` injected here is provided by `comp/aggregator/demultiplexer` (see [`demultiplexer.md`](../../comp/aggregator/demultiplexer.md)). It flows through to each check via the `Loader.Load` call and is stored on the check for sender access during `Run()`.

### One-shot check execution (CLI)

`pkg/cli/subcommands/check` loads checks via `GetChecksByNameForConfigs`, then calls each check's `Run()` directly.

### Loader registration

Each loader package registers itself in its `init()` function:

```go
func init() {
    loaders.RegisterLoader(func(...) (check.Loader, int, error) {
        return myLoader, priorityOrder, nil
    })
}
```

Import the loader package as a blank import to activate it. See [`loaders.md`](loaders.md) for priority values of existing loaders and a full registration guide.

### Check lifecycle summary

```
autodiscovery config arrives
        |
        v
CheckScheduler.Schedule()
    └─ IsCheckAllowed() gate
    └─ loaders.LoaderCatalog() → Loader.Load() → check.Check
    └─ collector.Component.RunCheck(check)
            └─ middleware.CheckWrapper wraps the check
            └─ scheduler.Scheduler.Enter(check)  ← timing
            └─ runner.Runner (via pendingChecksChan)
                    └─ worker.Worker.Run()
                            └─ check.Run()
                            └─ sender.Commit() (called by check)
                            └─ expvars / check_status service check

autodiscovery config removed
        |
        v
CheckScheduler.Unschedule()
    └─ collector.Component.StopCheck(id)
            └─ scheduler.Scheduler.Cancel(id)
            └─ runner.Runner.StopCheck(id)  (if running)
            └─ check.Cancel()
            └─ senderManager.DestroySender(id)
```

## Configuration Keys

| Key | Default | Description |
|---|---|---|
| `check_runners` | 0 (dynamic) | Fixed number of runner workers; 0 enables auto-scaling |
| `check_cancel_timeout` | — | How long to wait for a check to cancel before timing out |
| `check_watchdog_warning_timeout` | — | Emit a warning log if a check runs longer than this |
| `check_runner_utilization_threshold` | — | Per-worker utilization threshold for warnings |
| `check_runner_utilization_monitor_interval` | — | How often to log utilization warnings |
| `integration.enabled` | true | Master switch; false disables all integrations |
| `integration.excluded` | — | List of check names always suppressed by `IsCheckAllowed` |
| `infrastructure_mode` | — | Mode name used to look up the `integration.<mode>.allowed` list |
| `integration_check_status_enabled` | — | Whether to emit `datadog.agent.check_status` service checks |
| `prioritize_go_check_loader` | false | Lower Go loader priority to 10 (runs before Python) |
| `logging_frequency` | — | After initial 5 runs, log check events every N runs |
