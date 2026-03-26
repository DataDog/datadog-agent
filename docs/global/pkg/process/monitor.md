# pkg/process/monitor

## Purpose

`pkg/process/monitor` provides a singleton process lifecycle monitor that fires registered callbacks whenever a process starts (exec) or exits on the host. Consumers subscribe to exec or exit events and receive the affected PID; the monitor handles all event sourcing, multiplexing, and callback dispatch internally.

The primary use cases are:
- Universal Service Monitoring (USM) detecting when new TLS-capable processes start so it can attach uprobes.
- Go TLS, Node.js, Istio, and eBPF SSL monitors that need to react to process exec/exit events to load or unload eBPF programs.

**Platform:** Linux only (`//go:build linux`). The package has no-op stubs on other platforms.

## Key elements

### ProcessMonitor (`process_monitor.go`)

```go
type ProcessMonitor struct { ... }
```

A package-level singleton is created at program start:

```go
var processMonitor = &ProcessMonitor{
    processExecCallbacks: make(map[*ProcessCallback]struct{}),
    processExitCallbacks: make(map[*ProcessCallback]struct{}),
    ...
}
```

Multiple callers share the same instance; a reference counter (`refcount atomic.Int32`) tracks how many callers have acquired it.

### ProcessCallback

```go
type ProcessCallback = func(pid uint32)
```

The type accepted by both subscribe methods. Callbacks receive only the PID; callers must read `/proc/<pid>` themselves if additional process information is needed.

### GetProcessMonitor

```go
func GetProcessMonitor() *ProcessMonitor
```

Returns the singleton and increments the reference counter. Every call must be paired with a later `Stop()` call.

### Initialize

```go
func (pm *ProcessMonitor) Initialize(useEventStream bool) error
```

Idempotent (uses `sync.Once`). On the first call it:

1. Starts a pool of callback worker goroutines (one per logical CPU, capped by `pendingCallbacksQueueSize = 5000`).
2. If `useEventStream` is `false`: opens a netlink `PROC_EVENT_EXEC | PROC_EVENT_EXIT` socket via `github.com/vishvananda/netlink` inside the root network namespace, and starts `mainEventLoop` in a goroutine.
3. If `useEventStream` is `true`: skips netlink setup; event delivery is expected from an external `consumers.ProcessConsumer` wired up via `InitializeEventConsumer`.
4. Scans all existing `/proc/<pid>` entries and fires exec callbacks for already-running processes (skipped when no exec callbacks are registered).

### SubscribeExec / SubscribeExit

```go
func (pm *ProcessMonitor) SubscribeExec(callback ProcessCallback) func()
func (pm *ProcessMonitor) SubscribeExit(callback ProcessCallback) func()
```

Register a callback. Both methods return an unsubscribe closure that removes the callback from the active set. Callbacks may be registered before or after `Initialize`.

Internally, callbacks are stored as `map[*ProcessCallback]struct{}` (keyed by pointer so each registration is independent) under a read-write mutex. An `atomic.Bool` flag (`hasExecCallbacks` / `hasExitCallbacks`) short-circuits the mutex acquisition on the hot path when no callbacks are registered.

### Stop

```go
func (pm *ProcessMonitor) Stop()
```

Decrements the reference counter. When the counter reaches zero, closes the `done` channel, waits for the event loop and all callback workers to drain (`processMonitorWG`, `callbackRunnersWG`), then resets internal state so the monitor can be re-initialized. The reset is primarily for test isolation, since in production the monitor is initialized only once.

### InitializeEventConsumer

```go
func InitializeEventConsumer(consumer *consumers.ProcessConsumer)
```

Alternative event source for environments where the event monitor subsystem (eBPF-based) is used instead of netlink. The provided `consumers.ProcessConsumer` is subscribed to exec/exit events, which are forwarded to the same internal callback dispatch path as the netlink events.

### mainEventLoop (internal)

Runs as a goroutine. Selects on:
- `netlinkEventsChannel` — dispatches `ExecProcEvent` and `ExitProcEvent` to the callback worker pool.
- `netlinkErrorsChannel` — reinitializes the netlink socket after a brief delay (self-healing on buffer overflow or socket errors).
- `logTicker` — logs a summary of telemetry counters every 2 minutes.
- `done` — tears down on `Stop()`.

### Callback dispatch

Callbacks are never called directly from the event loop. Instead, a closure is sent over `callbackRunner` (a `chan func()` of size 5000) to one of the worker goroutines. If the channel is full, the event is dropped and `process_exec_channel_is_full` / `process_exit_channel_is_full` telemetry counters are incremented. This prevents slow callbacks from stalling the netlink event loop.

### Telemetry

All metrics are registered under the `usm.process.monitor` Prometheus metric group:

| Metric | Type | Description |
|--------|------|-------------|
| `events` | Counter | Total netlink events received |
| `exec` | Counter | Exec events dispatched |
| `exit` | Counter | Exit events dispatched |
| `restart` | Counter | Netlink socket reinitializations |
| `reinit_failed` | Counter | Failed reinitializations |
| `process_scan_failed` | Counter | Failures during initial `/proc` scan |
| `callback_executed` | Counter | Total callbacks executed |
| `process_exec_channel_is_full` | Counter | Dropped exec callbacks (channel full) |
| `process_exit_channel_is_full` | Counter | Dropped exit callbacks (channel full) |

## Usage

### Typical initialization pattern

```go
mon := monitor.GetProcessMonitor()
unsubExec := mon.SubscribeExec(func(pid uint32) { /* handle new process */ })
unsubExit := mon.SubscribeExit(func(pid uint32) { /* handle exit */ })
if err := mon.Initialize(false); err != nil {
    return err
}

// later, on shutdown:
unsubExec()
unsubExit()
mon.Stop()
```

`SubscribeExec` must be called before `Initialize` if you want callbacks for processes that were already running at init time (the initial `/proc` scan only fires exec callbacks for registered subscribers).

### In USM (Universal Service Monitoring)

`pkg/network/usm/monitor.go` calls `GetProcessMonitor()` and `Initialize(useEventStream)`. The `useEventStream` flag is set to `true` when the system-probe event monitor is running (eBPF-based approach). In that case, `InitializeEventConsumer` is called to connect the eBPF consumer to the singleton's dispatch logic.

### In eBPF uprobes

`pkg/network/usm/ebpf_ssl.go`, `ebpf_gotls.go`, `nodejs.go`, and `istio.go` all subscribe to exec events to detect when new instances of target processes (nginx, Node.js, Istio, etc.) start and attach uprobes. They subscribe to exit events to clean up per-PID uprobe state.

### In system-probe modules

`cmd/system-probe/modules/eventmonitor_linux.go` initializes the event monitor that feeds `consumers.ProcessConsumer`, which in turn drives the monitor via `InitializeEventConsumer`.
