> **TL;DR:** The execution hub of the collector, owning a dynamically-scaled pool of worker goroutines that pull checks from a shared channel, monitor utilization, and coordinate graceful shutdown.

# pkg/collector/runner

## Purpose

The `runner` package is the execution hub of the collector. It owns a pool of worker goroutines and feeds them checks via an unbuffered channel. It bridges the `scheduler` (which decides *when* to run a check) and the `worker` (which actually calls `check.Run()`). It also monitors worker utilization and coordinates graceful shutdown.

## Key elements

### Key types

| Type | Description |
|------|-------------|
| `Runner` | Central struct. Holds the worker pool, the pending-checks channel, the `RunningChecksTracker`, and a reference to the associated `Scheduler`. |

### Sub-packages

| Package | Description |
|---------|-------------|
| `runner/tracker` | Thread-safe `RunningChecksTracker` that records which checks are currently executing, preventing the same check from running concurrently. |
| `runner/expvars` | Publishes runtime statistics (runs, errors, warnings, per-check stats, per-worker utilization) under the `runner` expvar key. |

### Constructor

```go
func NewRunner(senderManager sender.SenderManager, haAgent haagent.Component, healthPlatform healthplatform.Component) *Runner
```

Reads `check_runners` from the agent config. When the value is `0` (the default), the worker count is managed dynamically via `UpdateNumWorkers`; otherwise it is fixed at the configured value. Immediately spawns the initial set of workers and starts the utilization monitor goroutine.

### Key functions

### Important methods

| Method | Description |
|--------|-------------|
| `GetChan() chan<- check.Check` | Returns the write side of the pending-checks channel. The `Scheduler` uses this to dispatch checks. |
| `SetScheduler(s *scheduler.Scheduler)` | Associates a scheduler with the runner so it can query `IsCheckScheduled`. |
| `ShouldAddCheckStats(id checkid.ID) bool` | Returns `true` if statistics for a completed check should be recorded (i.e., the check is still scheduled or no scheduler is set). |
| `UpdateNumWorkers(numChecks int64)` | Scales the worker pool up (never down) according to the number of scheduled checks. No-op when `isStaticWorkerCount` is set. |
| `AddWorker()` | Adds a single worker to the pool at runtime. |
| `StopCheck(id checkid.ID) error` | Calls `Stop()` on a currently-running check. Times out after `stopCheckTimeout` (500 ms). |
| `Stop()` | Closes the pending-checks channel, cancels the runner context, signals all running checks to stop, and waits up to `stopAllChecksTimeout` (2 s) for them to finish. |

### Constants

| Constant | Value | Purpose |
|----------|-------|---------|
| `stopCheckTimeout` | 500 ms | Maximum wait for a single check to stop. |
| `stopAllChecksTimeout` | 2 s | Maximum wait for all checks to stop during shutdown. |

### Configuration and build flags

| Key | Default | Description |
|-----|---------|-------------|
| `check_runners` | `0` (dynamic) | Fixed number of worker goroutines; `0` enables dynamic scaling. |
| `check_runner_utilization_threshold` | — | Fraction of time (0–1) above which a worker is considered over-utilized. |
| `check_runner_utilization_monitor_interval` | — | How often to log utilization warnings. |
| `check_runner_utilization_warning_cooldown` | — | Minimum interval between repeated utilization warnings. |
| `check_watchdog_warning_timeout` | — | Duration after which a still-running check triggers a watchdog warning. |

### Build tags

| Tag | Effect |
|-----|--------|
| `python` | Enables `terminateChecksRunningProcesses()` which kills Python subprocesses on shutdown (`runner_python.go`). Without the tag the function is a no-op (`runner_nopython.go`). |

### Dynamic worker scaling

`UpdateNumWorkers` applies the following step function:

| Scheduled checks | Worker target |
|-----------------|---------------|
| ≤ 10 | 4 |
| ≤ 15 | 10 |
| ≤ 20 | 15 |
| ≤ 25 | 20 |
| > 25 | `pkgconfigsetup.MaxNumWorkers` |

The pool only ever grows; workers that have been added are never removed.

## Usage

The runner is constructed once by `comp/collector` and wired into the rest of the collector pipeline:

```
Scheduler --[chan check.Check]--> Runner --[chan check.Check]--> Workers
```

1. `NewRunner` is called with a `SenderManager`, `haAgent`, and `healthPlatform` component.
2. The caller calls `runner.GetChan()` and passes the result to `scheduler.NewScheduler(...)`.
3. The scheduler is then attached to the runner with `runner.SetScheduler(s)`.
4. Autodiscovery calls `collector.RunCheck(c)` which pushes the check onto the pending channel; the runner's workers consume it.
5. On agent shutdown, `runner.Stop()` is called after the scheduler has been stopped (to prevent new items being enqueued on the closed channel).

The `runner/expvars` sub-package exposes all metrics under `/expvar` (port 5000 by default) and via the `datadog-agent status` command.

## Related packages

| Package | Relationship |
|---------|-------------|
| [`pkg/collector/check`](check.md) | Defines the `Check` interface that the runner's channel carries and that workers invoke. The `check/stats` sub-package holds the `Stats` struct updated after every run. |
| [`pkg/collector/worker`](worker.md) | Each `Worker` is spawned and owned by the runner. Workers pull from `pendingChecksChan`, execute `check.Run()`, and call back into the runner's expvars via `ShouldAddCheckStats`. |
| [`pkg/collector/scheduler`](scheduler.md) | The scheduler holds the write end of the same `pendingChecksChan` (obtained via `runner.GetChan()`). `runner.SetScheduler(s)` links the two so `ShouldAddCheckStats` can query whether a check is still scheduled. |
| [`comp/collector/collector`](../../comp/collector/collector.md) | The fx component that constructs and wires the runner. It calls `runner.NewRunner`, then `runner.SetScheduler`, and calls `runner.Stop()` on `OnStop`. Long-running checks (interval `0`) get an additional dedicated worker via `runner.AddWorker()`. |
| [`pkg/aggregator/sender`](../aggregator/sender.md) | The `SenderManager` passed to `NewRunner` is forwarded to each worker so it can emit the `datadog.agent.check_status` service check after every run. |
