# pkg/util/winutil

## Purpose

`pkg/util/winutil` provides Windows-specific utility functions used across the Datadog Agent. It is a **Windows-only** package: the root package files carry `//go:build windows` (with a thin `doc.go` stub for non-Windows builds). It covers:

- Windows Service lifecycle management (start, stop, restart, wait for state).
- SCM (Service Control Manager) monitoring — mapping PIDs to service names.
- Windows Event Log writing (`LogEventViewer`).
- Windows version / PE file version information.
- User and SID utilities.
- Elevated-privilege helpers, string conversion, memory, network utilities.

The package has several sub-packages:

| Sub-package | Purpose |
|-------------|---------|
| `etw/` | Event Tracing for Windows (ETW): stop sessions, process ETL files. |
| `eventlog/` | Windows Event Log reader using pull subscriptions (EvtSubscribe API). |
| `iphelper/` | IP Helper API wrappers: TCP connection table, routing table, interface table. |
| `servicemain/` | High-level `Service` interface and `Run()` for implementing Windows Service binaries. |
| `iisconfig/` | Parse and watch IIS `applicationHost.config`. |
| `messagestrings/` | Message catalog constants for Event Log entries. |
| `datadoginterop/` | Shared memory interop with the .NET tracer. |
| `winmem/` | Windows memory utilities. |

---

## Key Elements

### Root package (`pkg/util/winutil`)

#### Service management

| Symbol | Description |
|--------|-------------|
| `StartService(name string, args ...string) error` | Starts a service via SCM. Does not block until running. Handles `StopPending` by waiting first. |
| `StopService(name string) error` | Stops a service and all its dependents. Uses `SERVICE_STATE_ALL` to catch services starting concurrently (avoids a race condition where a dependent starts after the dependent list is enumerated). |
| `RestartService(name string) error` | Stops (with dependents) then starts. |
| `ControlService(name string, cmd svc.Cmd, to svc.State, access uint32, timeout uint64) error` | Sends an arbitrary SCM control code and waits for the service to reach state `to`. |
| `WaitForState(ctx context.Context, name string, state svc.State) error` | Polls SCM until the service reaches the desired state, or the context expires. |
| `WaitForPendingStateChange(ctx context.Context, name string, current svc.State) (svc.State, error)` | Waits for the service to leave a pending state (`StartPending` / `StopPending`). |
| `IsServiceRunning(name string) (bool, error)` | Returns true if the service is in `SERVICE_RUNNING`. |
| `IsServiceDisabled(name string) (bool, error)` | Returns true if the service start type is `SERVICE_DISABLED`. |
| `GetServiceUser(name string) (string, error)` | Returns the service account name from the SCM config. |
| `OpenSCManager(access uint32) (*mgr.Mgr, error)` | Opens a handle to the SCM with the specified access rights. |
| `OpenService(mgr *mgr.Mgr, name string, access uint32) (*mgr.Service, error)` | Opens a handle to a named service. |
| `DefaultServiceCommandTimeout` | Constant: 30 seconds, the default timeout used by service control operations. |

#### SCM monitor

| Symbol | Description |
|--------|-------------|
| `SCMMonitor` | Struct that maintains a PID→service mapping by caching SCM data. Thread-safe. |
| `GetServiceMonitor() *SCMMonitor` | Creates a new `SCMMonitor`. |
| `(*SCMMonitor).GetServiceInfo(pid uint64) (*ServiceInfo, error)` | Returns `ServiceInfo` (service name and display name) for the given PID, or `nil` if the process is not an SCM-managed service. Uses lazy cache invalidation based on process start time. |
| `ServiceInfo` | Struct with `ServiceName []string` and `DisplayName []string` (a PID can host multiple services). |

#### Event Log

| Symbol | Description |
|--------|-------------|
| `LogEventViewer(service string, msgnum uint32, arg string) ` | Writes a single message to the Windows Event Log under the given service source. Message severity (info/warning/error) is inferred from the high nibble of `msgnum`. The message string must exist in the application's message catalog. |

#### Windows version and PE info

| Symbol | Description |
|--------|-------------|
| `GetWindowsBuildString() (string, error)` | Returns the Windows build version string (e.g. `"10.0 Build 19041"`) by querying `kernel32.dll`'s version resource. Avoids the deprecated `GetVersion()` API. |
| `GetFileVersionInfoStrings(path string) (FileVersionInfo, error)` | Returns version resource strings (CompanyName, ProductName, FileVersion, ProductVersion, OriginalFilename, InternalName) for any executable. |
| `FileVersionInfo` | Struct holding the version resource string fields listed above. |
| `ErrNoPEBuildTimestamp` | Sentinel error returned when a PE header has no build timestamp. |

#### User and security

| Symbol | Description |
|--------|-------------|
| `GetSidFromUser() (*windows.SID, error)` | Returns the SID of the current process user. |
| `IsUserAnAdmin() (bool, error)` | Returns true if the current user is a member of the Administrators group. |
| `GetLocalSystemSID() (*windows.SID, error)` | Returns the SID for `NT AUTHORITY\SYSTEM`. Caller must free with `windows.FreeSid`. |
| `GetServiceUserSID(service string) (*windows.SID, error)` | Resolves the SID of a service's configured account. Handles the `LocalSystem` alias. |
| `GetDDAgentUserSID() (*windows.SID, error)` | Convenience wrapper: SID of the `datadogagent` service account. |

---

### `servicemain/` sub-package

Provides an abstraction for writing Windows Service binaries that correctly interacts with SCM state transitions and avoids common pitfalls (timeout during startup, hang on stop, false failure reports for short-lived services).

#### `Service` interface

Implementors provide three methods:

```go
type Service interface {
    Name() string                      // used as the Event Log source name
    Init() error                       // called while status is SERVICE_START_PENDING
    Run(ctx context.Context) error     // called when status is SERVICE_RUNNING
    HardStopTimeout() time.Duration    // max time to wait for Run() to return after ctx is cancelled
}
```

`DefaultSettings` can be embedded to get a default `HardStopTimeout()` (15 s, overridable via `DD_WINDOWS_SERVICE_STOP_TIMEOUT_SECONDS`).

#### Key functions

| Symbol | Description |
|--------|-------------|
| `Run(service Service)` | Entry point for service processes. Must be called early in `main()`. Calls `StartServiceCtrlDispatcher` (via `svc.Run`). Blocks until the service stops. |
| `RunningAsWindowsService() bool` | Returns true if the current process was launched by SCM. Use to decide whether to call `Run()` or run interactively. Includes a fix for Windows containers where Session ID is not 0. |
| `ErrCleanStopAfterInit` | Sentinel error that `Init()` can return to signal a successful early exit without triggering SCM failure recovery (e.g., agent disabled by configuration). |

**State machine:**
`StartPending` → `Init()` → `Running` → `Run(ctx)` → `StopPending` → `Stopped`.
If `Run()` does not return within `HardStopTimeout()` after the context is cancelled, the service is forcibly stopped.

A **5-second exit gate** (`runTimeExitGate`) keeps the service in `SERVICE_RUNNING` long enough for tools like `Restart-Service` to report success before the process exits, preventing false failure reports for short-lived services.

---

### `etw/` sub-package

**Build tag:** `windows`. Uses CGo (`etw.c`, `etw.h`).

Event Tracing for Windows utilities.

#### Key symbols

| Symbol | Description |
|--------|-------------|
| `StopETWSession(name string) error` | Stops a named ETW trace session using `ControlTraceW(EVENT_TRACE_CONTROL_STOP)`. Useful for stopping autologger sessions. |
| `ProcessETLFile(path string, cb EventCallback, opts ...ProcessOption) error` | Opens an ETL file, iterates all events, and calls `cb` for each. Blocks until complete. |
| `Event` | Represents a single ETW event. Fields: `ProviderID windows.GUID`, `EventID uint16`, `Timestamp time.Time`. Valid only during the callback. |
| `(*Event).EventProperties() (map[string]interface{}, error)` | Parses all event properties using TDH (Trace Data Helper). |
| `(*Event).GetPropertyByName(name string) (string, error)` | Retrieves a single named property directly via `TdhGetProperty`; more resilient to schema mismatches. |
| `GetEventPropertyString(e *Event, name string) string` | Convenience helper returning a named property as a string. |
| `EventRecordFilter` | `func(providerID windows.GUID, eventID uint16) bool` — fast pre-parse filter to skip unwanted events. |
| `WithEventRecordFilter(f EventRecordFilter) ProcessOption` | Option for `ProcessETLFile` that installs a filter. |

---

### `eventlog/subscription/` sub-package (`evtsubscribe`)

Pull-based Windows Event Log subscription using the `EvtSubscribe` / `EvtNext` API.

#### `PullSubscription` interface

```go
type PullSubscription interface {
    Start() error
    Stop()
    Running() bool
    GetEvents() <-chan []*evtapi.EventRecord
    Error() error
}
```

`GetEvents()` returns a channel; close signals an error, which is available via `Error()`. Event record handles must be closed by the caller; handles are automatically invalidated when `Stop()` is called.

The package is structured in layers: `api/` (raw Win32 wrappers), `session/` (connection to a remote or local log), `bookmark/` (persistence of read position), `subscription/` (high-level pull loop), `reporter/` (rendering events to strings), `publishermetadatacache/` (caching publisher metadata for formatting).

---

### `iphelper/` sub-package

Go wrappers around `Iphlpapi.dll` for network introspection.

#### Key symbols

| Symbol | Description |
|--------|-------------|
| `GetExtendedTcpV4Table() (map[uint32][]MIB_TCPROW_OWNER_PID, error)` | Returns all IPv4 TCP connections grouped by owning PID. |
| `GetIPv4RouteTable() ([]MIB_IPFORWARDROW, error)` | Returns the IPv4 routing table. |
| `GetIFTable() (map[uint32]windows.MibIfRow, error)` | Returns the interface table indexed by interface index. |
| `MIB_TCPROW_OWNER_PID` | TCP connection row: local/remote addr and port (network byte order), state, owning PID. |
| `MIB_IPFORWARDROW` | Routing table entry: destination, mask, next hop, interface index, metrics. |
| `TCP_TABLE_*` constants | Enum values for `GetExtendedTcpTable`'s `TableClass` parameter. |

---

### `iisconfig/` sub-package

Parses and hot-watches the IIS `applicationHost.config` file (`%windir%\System32\inetsrv\config\applicationHost.config`).

| Symbol | Description |
|--------|-------------|
| `DynamicIISConfig` | Struct that reloads XML config on file change (via `fsnotify`). Thread-safe. |
| `NewDynamicIISConfig() (*DynamicIISConfig, error)` | Creates and starts watching. Returns an error if IIS is not installed. |

Used by the IIS check to map site IDs to names and application paths for APM tag generation.

---

## Platform notes

- All files in this package require `//go:build windows`; `doc.go` provides the package declaration stub for non-Windows builds to allow cross-compilation.
- The package lives in a separate Go module (`go.mod` at `pkg/util/winutil/go.mod`), which means it has its own dependency graph and can be used without pulling in the full agent module.
- CGo is used in `etw/` (C wrappers for `AdvApi32.dll` ETW APIs). All other files use `golang.org/x/sys/windows` and avoid CGo.

---

## Usage

### Running as a Windows Service

```go
// cmd/agent/main_windows.go
import "github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"

type agentService struct{ servicemain.DefaultSettings }

func (s *agentService) Name() string { return "datadogagent" }
func (s *agentService) Init() error  { return initAgent() }
func (s *agentService) Run(ctx context.Context) error { return runAgent(ctx) }

func main() {
    if servicemain.RunningAsWindowsService() {
        servicemain.Run(&agentService{})
        return
    }
    // interactive / CLI mode
}
```

### Starting and stopping a service

```go
import "github.com/DataDog/datadog-agent/pkg/util/winutil"

if err := winutil.StartService("datadog-trace-agent"); err != nil {
    return err
}
// ...
if err := winutil.StopService("datadogagent"); err != nil { // stops dependents first
    return err
}
```

### Processing an ETL file

```go
import (
    "github.com/DataDog/datadog-agent/pkg/util/winutil/etw"
    "golang.org/x/sys/windows"
)

myProvider := windows.GUID{...}
err := etw.ProcessETLFile("C:\\trace.etl",
    func(e *etw.Event) {
        if e.ProviderID == myProvider {
            val, _ := e.GetPropertyByName("CommandLine")
            fmt.Println(val)
        }
    },
    etw.WithEventRecordFilter(func(id windows.GUID, eventID uint16) bool {
        return id == myProvider && eventID == 1
    }),
)
```

### Looking up the service for a PID

```go
import "github.com/DataDog/datadog-agent/pkg/util/winutil"

monitor := winutil.GetServiceMonitor()
info, err := monitor.GetServiceInfo(uint64(pid))
if err == nil && info != nil {
    fmt.Println(info.ServiceName) // e.g. ["datadogagent"]
}
```

### Primary importers

The package is imported by ~79 packages, concentrated in:

- `cmd/agent/` and `cmd/*/` — Windows service entry points.
- `pkg/process/` — process monitoring and network connection attribution.
- `pkg/network/` — Windows network tracer.
- `pkg/security/` — Windows CWS probe.
- `pkg/fleet/` — installer and fleet management.
- `comp/trace/config/` — trace-agent Windows configuration.

---

## Related packages and components

| Package / Component | Doc | Relationship |
|---------------------|-----|--------------|
| `pkg/util/pdhutil` | [pkg/util/pdhutil.md](pdhutil.md) | Parallel Windows-only utility package. `pdhutil` wraps the PDH performance-counter API for metric collection; `winutil` wraps SCM, Event Log, and user/SID APIs for service management and security. Both packages live under `pkg/util/` and are compiled only on Windows. Core checks that use PDH counters (`winproc`, `cpu`, `memory`, `disk/io`) often also use `winutil` for service lifecycle operations. |
| `comp/etw` | [comp/etw.md](../../comp/etw.md) | The `comp/etw` component provides a higher-level, fx-managed interface for **real-time** ETW sessions (subscribing to live provider streams). `pkg/util/winutil/etw` provides lower-level helpers for **offline** ETL file processing (`ProcessETLFile`) and stopping autologger sessions (`StopETWSession`). The `comp/logonduration` component uses `pkg/util/winutil/etw.ProcessETLFile` to replay a boot-time ETL file, while security and network monitoring use `comp/etw` for real-time event streaming. |
| `comp/logonduration` | [comp/logonduration.md](../../comp/logonduration.md) | The logon-duration component depends directly on `pkg/util/winutil/etw.ProcessETLFile` and `etw.StopETWSession` to parse the boot-time ETL file written by the `"Datadog Logon Duration"` AutoLogger. It also uses `winutil.StartService` / `winutil.StopService` patterns indirectly through AutoLogger registry management. |
