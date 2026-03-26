# pkg/collector/worker

## Purpose

The `worker` package implements the individual goroutine that executes checks. Each `Worker` reads `check.Check` values from a shared channel, runs them one at a time, emits a `datadog.agent.check_status` service check to the aggregator, and records runtime statistics. Multiple worker instances run concurrently under a single `Runner`.

Workers also track their own utilization (fraction of time spent running checks vs. idle) and expose it through expvars and a Prometheus-compatible telemetry gauge.

## Key elements

### Types

| Type | Description |
|------|-------------|
| `Worker` | A single check-execution goroutine. Identified by an integer `ID` and a string `Name` (`worker_<ID>`). |
| `UtilizationMonitor` | Queries expvars to compute per-worker utilization and aggregate overview data. Used by the runner to log warnings when workers are overloaded. |
| `OverviewData` | Summary returned by `UtilizationMonitor.GetWorkerOverview()`: total worker count, threshold, list of workers over threshold, and average utilization. |
| `CheckLogger` | Helper that logs check start, finish, and error events. Applies a frequency-based throttle: the first 5 runs are always logged at INFO; subsequent runs are logged every `logging_frequency` runs (config key). |

### Constructor

```go
func NewWorker(
    senderManager sender.SenderManager,
    haAgent haagent.Component,
    healthPlatform healthplatform.Component,
    runnerID int,
    ID int,
    pendingChecksChan chan check.Check,
    checksTracker *tracker.RunningChecksTracker,
    shouldAddCheckStatsFunc func(id checkid.ID) bool,
    watchdogWarningTimeout time.Duration,
) (*Worker, error)
```

Returns an error if any required argument is `nil`. Workers are created by `Runner.newWorker()`; callers should not instantiate them directly.

### Main loop

```go
func (w *Worker) Run(ctx context.Context)
```

Runs until the `pendingChecksChan` is closed (which happens when `Runner.Stop()` is called). For each check received:

1. **HA check**: if `haAgent` is enabled and the current agent is not the active leader, and the check declares HA support, the check is silently skipped.
2. **Concurrency guard**: `checksTracker.AddCheck(check)` returns `false` if the check is already running; the duplicate is skipped.
3. **Watchdog**: if `watchdogWarningTimeout > 0`, a background goroutine logs a warning if the check does not finish within the timeout.
4. **Execution**: `check.Run()` is called synchronously.
5. **Service check**: a `datadog.agent.check_status` service check (`OK` / `Warning` / `Critical`) is submitted via the default sender, gated on `integration_check_status_enabled`. Long-running checks (interval `0`) do not emit this service check.
6. **Stats**: `expvars.AddCheckStats(...)` is called if the check is still in the scheduler (`shouldAddCheckStatsFunc`).

The context passed to `Run` is used for cancellable operations inside the check (such as EC2 IMDS hostname resolution). It is cancelled by `Runner.Stop()`.

### Utilization tracking

Each worker maintains a `UtilizationTracker` (exponential moving average with α = 0.25). It calls `Started()` immediately before `check.Run()` and `Finished()` immediately after. A separate ticker goroutine calls `Tick()` every `pollingInterval` (15 seconds) to drive the EMA calculation.

Utilization values are published to:

- `expvar`: under `runner.Workers.<worker_name>.Utilization`
- Telemetry gauge: `collector.worker_utilization{worker_name=<name>}`

### Configuration keys

| Key | Description |
|-----|-------------|
| `integration_check_status_enabled` | Whether to emit `datadog.agent.check_status` service checks. |
| `logging_frequency` | How often (in check runs) to log check start/stop after the initial 5 runs. |
| `check_watchdog_warning_timeout` | Duration before the watchdog logs a warning for a still-running check. |

## Usage

Workers are managed exclusively by `Runner`. Contributors do not interact with the `Worker` type directly. The typical extension points are:

- **Implementing a new check**: implement `check.Check` in `pkg/collector/corechecks/` or `pkg/collector/check/`. The worker calls `check.Run()` without knowing the implementation.
- **Observing worker load**: read `UtilizationMonitor.GetWorkerOverview()` from the runner, or query the `collector.worker_utilization` telemetry gauge / the expvar endpoint.
- **Adjusting worker count**: set `check_runners` in `datadog.yaml` or rely on `Runner.UpdateNumWorkers`.

The `CheckLogger` in this package is also used internally for standardised, frequency-throttled logging of check lifecycle events and should not be instantiated outside the worker.
