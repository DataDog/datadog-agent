# pkg/system-probe/utils

## Purpose

Shared utilities for system-probe HTTP handlers: JSON response helpers, a concurrency limiter middleware, and a cgroup memory monitor. These are used across all system-probe modules to give handlers consistent behavior with minimal boilerplate.

## Key Elements

### HTTP helpers (`utils.go`)

| Symbol | Description |
|---|---|
| `FormatOptions` | Boolean type alias. `CompactOutput = false`, `PrettyPrint = true`. |
| `PrettyPrintQueryParam` | Query parameter name `"pretty_print"`. Truthy values: `"1"`, `"t"`, `"true"`, `"yes"`, `"y"`, `"on"` (case-insensitive). |
| `GetPrettyPrintFromQueryParams(req)` | Returns `PrettyPrint` if the `pretty_print` query param is set to a truthy value, else `CompactOutput`. |
| `WriteAsJSON(req, w, data, outputOptions)` | Marshals `data` to JSON and writes it to `w`. Uses `json.NewEncoder` with optional indentation. Logs an error and returns HTTP 500 on encode failure; silently swallows broken-pipe errors (`EPIPE`). |
| `GetClientID(req)` | Returns the `client_id` query parameter, defaulting to `"-1"`. Used by modules to track which agent client is making the request. |

### Concurrency limiter (`limiter.go`)

| Symbol | Description |
|---|---|
| `DefaultMaxConcurrentRequests` | `2` — one slot for the regular agent check, one for manual troubleshooting. |
| `WithConcurrencyLimit(limit, handler)` | Middleware that wraps an `http.HandlerFunc`. Uses an atomic counter to track in-flight requests; returns HTTP 429 and logs a warning if the limit is exceeded. |

### Memory monitor (`memory_monitor_linux.go` / `memory_monitor_stub.go`)

| Symbol | Description |
|---|---|
| `MemoryMonitor` | On Linux: type alias for `pkg/util/cgroups/memorymonitor.MemoryController`. On non-Linux: an empty stub with no-op `Start()`/`Stop()`. |
| `NewMemoryMonitor(kind, containerized, pressureLevels, thresholds)` | Constructs a `MemoryMonitor` from two maps. `pressureLevels` maps cgroup pressure level names to actions (`"gc"`, `"log"`, `"profile"`). `thresholds` maps size strings (e.g., `"512MiB"`) or percentage strings (e.g., `"80%"`) to actions. On Linux, registers cgroup memory event callbacks; on other platforms returns an empty stub. |

Supported actions for both pressure and threshold monitors:
- `"gc"` — triggers `runtime.GC()`
- `"log"` — no-op (logs the event but takes no action)
- `"profile"` — writes a pprof heap profile to a temp file (max 10 profiles retained)

## Usage

`pkg/system-probe/utils` is imported by virtually all system-probe modules:

- **`cmd/system-probe/subcommands/run/command.go`** — calls `NewMemoryMonitor` using values from the system-probe config to protect against OOM in containerized deployments.
- **`cmd/system-probe/modules/*`** (e.g., `network_tracer_linux.go`, `oom_kill_probe.go`, `ping.go`, `tcp_queue_tracer.go`, `noisy_neighbor.go`) — wrap their HTTP handlers with `WithConcurrencyLimit(DefaultMaxConcurrentRequests, handler)` and use `WriteAsJSON` + `GetPrettyPrintFromQueryParams` to respond to agent check requests.
- **`pkg/dyninst/module`** and other system-probe sub-modules — use the same JSON and concurrency helpers for their own module HTTP endpoints.

Typical handler pattern:

```go
func (m *MyModule) GetStats(w http.ResponseWriter, req *http.Request) {
    stats := m.collectStats()
    utils.WriteAsJSON(req, w, stats, utils.GetPrettyPrintFromQueryParams(req))
}

// At registration time:
router.HandleFunc("/check/my-module", utils.WithConcurrencyLimit(
    utils.DefaultMaxConcurrentRequests, myModule.GetStats))
```
