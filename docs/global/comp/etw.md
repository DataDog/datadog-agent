> **TL;DR:** `comp/etw` provides a Go interface to Windows Event Tracing for Windows (ETW), allowing agent components to subscribe to named ETW providers and receive structured event records via callbacks without writing CGo/Win32 session management boilerplate.

# comp/etw/impl

**Package:** `github.com/DataDog/datadog-agent/comp/etw`
**Implementation:** `github.com/DataDog/datadog-agent/comp/etw/impl`
**Team:** windows-products
**Platform:** Windows only (`//go:build windows`)

## Purpose

The ETW component provides a Go interface to Windows Event Tracing for Windows (ETW), the kernel-level logging infrastructure built into Windows. It allows other agent components to subscribe to named ETW providers, receive structured event records, and process them through Go callbacks — without each consumer having to write its own CGo/Win32 session management boilerplate.

ETW is used throughout the agent for low-overhead, kernel-assisted telemetry on Windows: network traffic (HTTP), process and file system events (security probe), and APM tracing. This component centralises session lifecycle management so consumers only need to supply a GUID and a callback.

## Key elements

### Key interfaces

```go
// comp/etw/component.go
type Component interface {
    NewSession(sessionName string, f SessionConfigurationFunc) (Session, error)
    NewWellKnownSession(sessionName string, f SessionConfigurationFunc) (Session, error)
}
```

- `NewSession` creates and starts a new private real-time ETW session. Any previous session with the same name is deleted first (recovery from crashes or unclean shutdowns).
- `NewWellKnownSession` attaches to an already-running system ETW session (e.g., the NT Kernel Logger). These sessions are not owned by the agent; calling `StopTracing` on them closes the trace handle without stopping the session itself.

Both accept an optional `SessionConfigurationFunc` to tune buffer sizes and kernel flags before the session is created.

**`Session` interface:**

```go
type Session interface {
    ConfigureProvider(providerGUID windows.GUID, configurations ...ProviderConfigurationFunc)
    EnableProvider(providerGUID windows.GUID) error
    DisableProvider(providerGUID windows.GUID) error
    StartTracing(callback EventCallback) error
    StopTracing() error
    GetSessionStatistics() (SessionStatistics, error)
}
```

Typical call order:

1. `ConfigureProvider(guid, ...)` — optional, to set trace level, keyword bitmasks, PID filters, or event ID allow/deny lists.
2. `EnableProvider(guid)` — activates the provider in the session.
3. `StartTracing(callback)` — **blocking call** that processes incoming events until `StopTracing` is called from another goroutine.
4. `StopTracing()` — disables all providers and stops the session.

### Key types

**SessionConfiguration** (passed via `SessionConfigurationFunc`):

| Field | Description |
|---|---|
| `MinBuffers` | Minimum number of ETW trace buffers to allocate |
| `MaxBuffers` | Maximum number of ETW trace buffers |
| `EnableFlags` | `EVENT_TRACE_FLAG_*` bitmask for kernel-mode system providers (e.g., `EVENT_TRACE_FLAG_PROCESS`, `EVENT_TRACE_FLAG_NETWORK_TCPIP`) |

**ProviderConfiguration** (passed via `ProviderConfigurationFunc`):

| Field | Description |
|---|---|
| `TraceLevel` | Maximum event severity (`TRACE_LEVEL_CRITICAL` … `TRACE_LEVEL_VERBOSE`) |
| `MatchAnyKeyword` | 64-bit bitmask; provider writes event if any bit matches |
| `MatchAllKeyword` | 64-bit bitmask; provider writes event only if all bits match |
| `PIDs` | If non-empty, only receive events from these process IDs |
| `EnabledIDs` | Whitelist of event IDs (mutually exclusive with `DisabledIDs`) |
| `DisabledIDs` | Blacklist of event IDs (mutually exclusive with `EnabledIDs`) |

**`DDEventRecord`** — mirrors the Win32 `EVENT_RECORD` layout and is passed (by pointer) to the `EventCallback`:

```go
type EventCallback func(e *DDEventRecord)

type DDEventRecord struct {
    EventHeader   DDEventHeader       // provider GUID, event ID/version, timestamp, PID/TID
    BufferContext DDEtwBufferContext   // processor and logger IDs
    ExtendedData  *DDEventHeaderExtendedDataItem
    UserData      *uint8              // event payload bytes
    UserDataLength uint16
    ...
}
```

The callback pointer lives only for the duration of the call. Any data that must outlive the callback must be copied.

### Key functions

**`GetUserData`** helper — `GetUserData(event *DDEventRecord) UserData` (in `impl/eventrecord.go`) wraps `UserData` in a `UserData` interface for structured field extraction:

```go
type UserData interface {
    ParseUnicodeString(offset int) (string, int, bool, int)
    GetUint64(offset int) uint64
    GetUint32(offset int) uint32
    GetUint16(offset int) uint16
    Bytes(offset int, length int) []byte
    Length() int
}
```

All reads are by offset. Fields in ETW user-data buffers are packed sequentially; consumers must know the schema for their provider.

### Configuration and build flags

- The implementation (`comp/etw/impl`) uses CGo to call `StartTraceW`, `EnableTraceEx2`, `ProcessTrace`, and `ControlTraceW` from the Win32 ETW API. The C helper `DDEnableTrace` (in `session.c`/`session.h`) reconstructs `EVENT_FILTER_DESCRIPTOR` arrays from flat slices because Go pointers containing Go pointers cannot be passed directly to C.
- The `ddEtwCallbackC` C-exported function bridges the C ETW callback to the Go `EventCallback` using `cgo.Handle`.
- Sessions run in real-time mode (`EVENT_TRACE_REAL_TIME_MODE`); there is no file-based logging.
- Platform: Windows only (`//go:build windows`).

## Usage

### In the codebase

| Consumer | What it traces |
|---|---|
| `comp/trace/etwtracer/etwtracerimpl` | APM span collection via ETW on Windows |
| `pkg/network/protocols/http/etw_http_service.go` | Windows HTTP.sys kernel events for HTTP monitoring |
| `pkg/security/probe/probe_windows.go` | Security-relevant process, file, and registry events |

### Typical consumer pattern

```go
// 1. Inject etw.Component via fx
etwComp etw.Component

// 2. Create a session
session, err := etwComp.NewSession("my-session", func(cfg *etw.SessionConfiguration) {
    cfg.MaxBuffers = 64
})

// 3. Configure and enable a provider
providerGUID := windows.GUID{...}
session.ConfigureProvider(providerGUID, func(cfg *etw.ProviderConfiguration) {
    cfg.TraceLevel = etw.TRACE_LEVEL_INFORMATION
    cfg.MatchAnyKeyword = 0xFFFFFFFF
})
_ = session.EnableProvider(providerGUID)

// 4. Start tracing (blocking) in a goroutine
go func() {
    _ = session.StartTracing(func(e *etw.DDEventRecord) {
        ud := etwimpl.GetUserData(e)
        // parse ud fields by offset
    })
}()

// 5. Stop from another goroutine on shutdown
_ = session.StopTracing()
```

### Registration

Import `etwimpl.Module` (from `comp/etw/impl`) in the Windows agent build:

```go
etwimpl.Module  // provides etw.Component
```

The component has no fx lifecycle hooks; session lifecycle is entirely managed by the consumer.

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/util/winutil/etw`](../pkg/util/winutil.md) | A lower-level, CGo-based sub-package that provides **offline** ETL file processing (`ProcessETLFile`) and session teardown (`StopETWSession`). `comp/etw` is for **real-time** streaming sessions via a live ETW session; `pkg/util/winutil/etw` is used when replaying a previously recorded `.etl` file or stopping an AutoLogger session. The two packages share the same underlying Win32 ETW APIs but address different use cases. |
| [`comp/logonduration`](logonduration.md) | Consumes `pkg/util/winutil/etw.ProcessETLFile` (not `comp/etw`) to parse the boot-time ETL file written by the `"Datadog Logon Duration"` AutoLogger. It also calls `etw.StopETWSession` to finalise the session before reading the file. `comp/etw` is the right choice for components that need a **live** real-time session; `comp/logonduration` does not need a live session because all events were already recorded before the agent started. |
| [`comp/checks/windowseventlog`](checks/windowseventlog.md) | A Windows-only check that reads Windows Event Log entries (via the EvtSubscribe API in `pkg/util/winutil/eventlog/`). It targets the **Windows Event Log** (a separate subsystem from ETW), but both components are Windows-only and share the same team (`windows-products`). When monitoring Windows system behavior, `windowseventlog` and `comp/etw`-based consumers are often complementary: ETW captures kernel and application instrumentation events at high throughput; the Event Log captures well-known operational records written by Windows components. |
