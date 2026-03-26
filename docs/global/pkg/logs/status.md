> **TL;DR:** `pkg/logs/status` provides the runtime status snapshot for the logs agent, including running state, active integrations, transport, warnings, and pipeline metrics, through a global singleton `Builder` and a set of reusable building blocks (`LogStatus`, `InfoRegistry`, `CountInfo`) that sources and tailers attach to.

# pkg/logs/status

## Purpose

Provides the runtime status snapshot for the logs agent: whether it is running, which integrations/sources are active, current transport, accumulated warnings/errors, and key pipeline metrics. The package is structured in three layers:

| Sub-package | Role |
|---|---|
| `pkg/logs/status` | Top-level singleton: global `Init`/`Get`/`Clear` API and the `Builder` that assembles the `Status` struct on demand. |
| `pkg/logs/status/statusinterface` | Narrow interface used by components that only need to post warnings (e.g., the TCP connection manager). Avoids importing the full status package. |
| `pkg/logs/status/utils` | Reusable building blocks: `LogStatus`, `InfoProvider`, `CountInfo`, `MappedInfo`, `InfoRegistry`, `ProcessingInfo`. |

## Key Elements

### Key types

#### `pkg/logs/status`

| Symbol | Description |
|---|---|
| `Status` struct | Top-level snapshot returned by `Get()`. Fields: `IsRunning`, `Endpoints`, `Integrations`, `Tailers` (verbose only), `StatusMetrics`, `ProcessFileStats`, `Warnings`, `Errors`, `UseHTTP`. |
| `Integration` / `Source` / `Tailer` | JSON-serialisable sub-structs representing a named log integration, individual log source configuration + state, and an active tailer. |
| `Transport` / `TransportHTTP` / `TransportTCP` | String constants and getter/setter for the current transport mode. |
| `StatusNotStarted` / `StatusRunning` | Integer constants (0/1) for the atomic running flag. |
| `Init(isRunning, endpoints, sources, tracker, logExpVars)` | Initializes the global singleton. Must be called before `Get()`. |
| `Get(verbose bool)` | Returns the current `Status`. Thread-safe. Returns `{IsRunning: false}` if not initialized. |
| `Clear()` | Tears down the singleton (used during agent stop/restart). |
| `AddGlobalWarning(key, message)` / `RemoveGlobalWarning(key)` | Thread-safe accumulation of per-key warning messages shown on the status page. |
| `AddGlobalError(key, message)` | Records a fatal error message. |
| `Builder` | Internal struct that reads live state from `LogSources`, `TailerTracker`, `Endpoints`, and `expvar.Map` to produce a `Status`. |

The `init()` function registers `Errors`, `Warnings`, and `IsRunning` as expvar entries under `LogsExpvars`.

### Key interfaces

#### `pkg/logs/status/statusinterface`

| Symbol | Description |
|---|---|
| `Status` interface | Minimal interface with `AddGlobalWarning(key, warning)` and `RemoveGlobalWarning(key)`. Used by `tcp.ConnectionManager` to surface connection errors without importing the full status package. |
| `NewNoopStatusProvider()` | Returns a no-op implementation (used in tests and connectivity checks). |

### Key functions

#### `pkg/logs/status/utils`

| Symbol | Description |
|---|---|
| `LogStatus` | Thread-safe three-state machine: `isPending` → `isSuccess` or `isError`. Methods: `Success()`, `Error(err)`, `IsPending()`, `IsSuccess()`, `IsError()`, `GetError()`. Permission-denied errors get a documentation link appended. |
| `InfoProvider` interface | Two-method interface (`InfoKey() string`, `Info() []string`) used to render structured info on the status page. Single-item results display inline; multi-item results display as an indented list. |
| `CountInfo` | Atomic counter implementing `InfoProvider`. Created with `NewCountInfo(key)`. |
| `MappedInfo` | Key-value map of string messages implementing `InfoProvider`. Created with `NewMappedInfo(key)`. |
| `InfoRegistry` | Thread-safe collection of `InfoProvider`s keyed by `InfoKey()`. `Rendered()` returns a `map[string][]string` for status page display. |
| `ProcessingInfo` | Implements `InfoProvider`; counts how many logs each named processing rule has matched. Displays as "Rule [name] applied to N log(s)". |

### Configuration and build flags

| Symbol / Constant | Description |
|---|---|
| `TransportHTTP` / `TransportTCP` | String constants identifying the current transport mode; set by `comp/logs/agent` via `status.SetCurrentTransport(...)`. |
| `StatusNotStarted` / `StatusRunning` | Integer constants (0/1) for the atomic running flag written by `Init`. |
| `LogsExpvars` | The `expvar.Map` key under which `Errors`, `Warnings`, and `IsRunning` are registered at `init()` time, enabling external metric collection. |

## Usage

- **`comp/logs/agent/agentimpl/agent.go`** — calls `status.Init(...)` on startup and `status.Clear()` on stop. Sets the transport via `status.SetCurrentTransport(...)`.
- **`comp/metadata/host/hostimpl/utils/host.go`** and other metadata consumers — call `status.Get(false)` to include logs agent status in host metadata payloads.
- **`pkg/logs/launchers/file`**, **`pkg/logs/launchers/integration`**, **`comp/core/autodiscovery/providers/process_log.go`** — call `status.AddGlobalWarning` / `RemoveGlobalWarning` as log sources encounter errors or recover.
- **`pkg/logs/client/tcp/connection_manager.go`** — uses `statusinterface.Status` to post/clear the `connection_error` warning key without importing the full status package.
- **`pkg/logs/sources`** and tailers — attach `LogStatus` and `InfoProvider` implementations to log sources so `Builder.BuildStatus` can render them.
