> **TL;DR:** Decides when checks run by maintaining one interval-based job queue per unique check interval, spreading checks across time buckets with jitter, and pushing due checks onto the runner's channel.

# pkg/collector/scheduler

## Purpose

The `scheduler` package decides *when* checks run. It maintains one `jobQueue` per unique check interval, ticks each queue with a 1-second timer, and pushes due checks onto the channel that the `runner` package consumes. Checks with an interval of `0` are treated as one-shot: they are dispatched immediately and only once.

The scheduler is responsible for:

- Grouping checks by interval into independent queues.
- Spreading checks within a queue across multiple time buckets (jitter) so that all checks at the same interval do not fire simultaneously.
- Providing `IsCheckScheduled` so the runner can decide whether to record statistics for a finished check.

## Key elements

### Key types

| Type | Description |
|------|-------------|
| `Scheduler` | Top-level struct. Owns a map of `jobQueue` objects keyed by interval, the write side of the runner's channel, and control channels for lifecycle management. |
| `jobQueue` (unexported) | Per-interval queue. Contains one or more `jobBucket` objects and a ticker that fires every second. On each tick it dispatches the checks in the current bucket to the runner channel. |
| `jobBucket` (unexported) | Slice of `check.Check` objects assigned to the same time slot within a queue. Checks are round-robined across buckets at schedule time using a sparse step to maximize spread. |

### Constructor

```go
func NewScheduler(checksPipe chan<- check.Check) *Scheduler
```

Accepts the write side of the runner's pending-checks channel. Does not start any goroutines; call `Run()` to activate.

### Key functions

### Lifecycle methods

| Method | Description |
|--------|-------------|
| `Run()` | Starts the scheduler loop and all existing queues in background goroutines. Blocks until all queues are ready. |
| `Stop() error` | Stops the main loop and all queues. Cancels pending one-time dispatch goroutines and waits for them to exit. Blocks until halted. |

### Scheduling methods

| Method | Description |
|--------|-------------|
| `Enter(check check.Check) error` | Adds a check to the appropriate interval queue, creating the queue if needed. Returns an error if the interval is non-zero but less than 1 second. Interval `0` triggers immediate one-shot dispatch. |
| `Cancel(id checkid.ID) error` | Removes a check from its queue. Noop if not found. |
| `IsCheckScheduled(id checkid.ID) bool` | Thread-safe lookup; returns `true` if the check is still in a queue. Called by the runner to decide whether to record stats after execution. |

### Observability

Scheduler state is exported through two mechanisms:

| Mechanism | Key/Metric | Content |
|-----------|-----------|---------|
| `expvar` | `scheduler.QueuesCount` | Total number of queues ever created. |
| `expvar` | `scheduler.ChecksEntered` | Current number of checks tracked by the scheduler. |
| `expvar` | `scheduler.Queues` | JSON array with per-queue stats: interval, bucket count, check count. |
| Telemetry | `scheduler.checks_entered{check_name}` | Gauge of tracked checks per check name (only for checks that have telemetry enabled). |
| Telemetry | `scheduler.queues_count` | Counter of queues opened (only for checks that have telemetry enabled). |

### Constraints

- Minimum allowed interval: **1 second**. Attempting to schedule a check with a shorter non-zero interval returns an error.
- Each `jobQueue` registers a liveness health check under `collector-queue-<N>s` so the agent's health monitoring can detect a stalled queue.

### Jitter mechanism

When a check is added to a `jobQueue`, it is assigned to a bucket using a sparse round-robin with a step computed from the number of buckets. For a 30-second interval the queue has 30 buckets; for a 15-second interval, 15 buckets; and so on. The step is chosen to be approximately half the bucket count (adjusted to be odd when possible) so consecutive checks land far apart in time.

On each tick the current bucket's checks are pushed one-by-one to the runner channel. If the runner is slow and the channel blocks, the queue remains blocked (but will stop blocking if a `stop` signal arrives).

## Usage

The scheduler is created and wired up by the `collector` component:

```go
// Inside comp/collector or pkg/collector
runner   := runner.NewRunner(senderManager, haAgent, healthPlatform)
scheduler := scheduler.NewScheduler(runner.GetChan())
scheduler.Run()
runner.SetScheduler(scheduler)
```

Autodiscovery calls `scheduler.Enter(check)` when a check config is resolved, and `scheduler.Cancel(id)` when the config is removed. On shutdown, the collector calls `scheduler.Stop()` before `runner.Stop()` to ensure no new checks are pushed onto the (about-to-be-closed) runner channel.
