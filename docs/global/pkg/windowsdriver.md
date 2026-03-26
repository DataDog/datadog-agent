# pkg/windowsdriver

## Purpose

`pkg/windowsdriver` provides Go bindings for Datadog's custom Windows kernel drivers. It exposes lifecycle management for driver services and communicates with the drivers using Windows I/O Control Codes (IOCTLs) over device handles. Two drivers are currently integrated: the **ddprocmon** process-monitoring driver and the **DDInjector** APM-injection driver.

All code in this package carries `//go:build windows` and is therefore excluded from Linux/macOS builds.

## Sub-packages

### `driver/`

Generic helpers for Windows Service Control Manager (SCM) operations applied to any kernel driver service.

| Symbol | Description |
|--------|-------------|
| `EnableDriverService(name string) error` | Changes a disabled service's start type to `SERVICE_DEMAND_START`. |
| `StartDriverService(name string) error` | Enables (if disabled) and starts the named driver service. |
| `StopDriverService(name string, disable bool) error` | Stops the service; optionally sets it to `SERVICE_DISABLED`. |

### `olreader/`

`OverlappedReader` is a generic, reusable component for performing asynchronous (overlapped) reads from a Windows device handle using an I/O Completion Port (IOCP). It hides the complexity of Win32 overlapped I/O behind a simple callback interface.

| Symbol | Description |
|--------|-------------|
| `OverlappedCallback` interface | Implemented by callers; has `OnData([]uint8)` and `OnError(error)`. |
| `OverlappedReader` struct | Manages a device handle + IOCP + a pool of `count` raw read buffers of `bufsz` bytes each. |
| `NewOverlappedReader(cb, bufsz, count)` | Constructor. |
| `Open(name string) error` | Opens the device with `FILE_FLAG_OVERLAPPED` and attaches an IOCP. |
| `Read() error` | Queues all buffers and starts a goroutine that calls `cb.OnData` for every completed read, then immediately re-queues the buffer. |
| `Stop()` | Closes the IOCP and device handle, causing the goroutine to exit; waits for it. |
| `Ioctl(...)` | Passes a `DeviceIoControl` call through to the underlying handle. |

Buffer memory is allocated via C `malloc` (to keep the `windows.Overlapped` structure pointer-stable across GC cycles) and freed in `cleanBuffers`.

### `procmon/`

Go interface to the `ddprocmon` kernel driver, which delivers process-start and process-stop notifications to user space via overlapped reads.

| Symbol | Description |
|--------|-------------|
| `WinProcmon` struct | Top-level object; wraps an `OverlappedReader`. |
| `ProcessStartNotification` struct | Pid, PPid, creating process/thread IDs, owner SID string, image path, command line, and full environment block for a new process. |
| `ProcessStopNotification` struct | Pid of the terminated process. |
| `NewWinProcMon(onStart, onStop, onError, bufsize, numbufs)` | Starts the `ddprocmon` driver service and opens the device. Callers supply three channels and optional buffer tuning (defaults: 140 KB buffers, 50 buffers). |
| `Start() error` | Issues the `ProcmonStartIOCTL` IOCTL to begin receiving events. |
| `Stop()` | Issues the `ProcmonStopIOCTL` IOCTL, stops the reader goroutine, and stops the driver service. |
| `OnData([]uint8)` | Internal callback (implements `OverlappedCallback`); decodes the raw kernel notification and sends to the appropriate channel. |

Key constants (defined in the cgo-generated `types_windows.go`):

- `ProcmonSignature = 0xdd0100000005` — driver handshake value
- `ProcmonStartIOCTL`, `ProcmonStopIOCTL`, `ProcmonStatsIOCTL` — IOCTL codes
- `ProcmonNotifyStart = 1`, `ProcmonNotifyStop = 0` — notification type discriminator
- `ProcmonDefaultReceiveSize = 140 * 1024`, `ProcmonDefaultNumBufs = 50`

The notification wire format is described by `DDProcessNotification` (generated from the C header `include/procmonapi.h`).

### `ddinjector/`

Go interface to the `DDInjector` kernel driver, which is responsible for injecting APM instrumentation into Windows processes at creation time.

| Symbol | Description |
|--------|-------------|
| `Injector` struct | Holds a single `windows.Handle` to the `\\.\DDInjector` device. |
| `NewInjector() (*Injector, error)` | Opens a read-only handle to the device. |
| `GetCounters(counters *InjectorCounters) error` | Calls `IOCTL_GET_COUNTERS` (Method: `METHOD_OUT_DIRECT`) and populates the caller-supplied `InjectorCounters`. |
| `Close() error` | Closes the handle. |
| `InjectorCounters` struct | 17 `telemetry.SimpleGauge` fields covering injection successes/failures, process tracking, and PE (Portable Executable) manipulation metrics. |
| `DDInjectorCountersV1` | Cgo-mapped C struct mirroring `_DRIVER_COUNTERS_V1` from `include/ddinjector_public.h`. Versioning policy: V1 fields are frozen; new versions add a nested struct. |

The counter structure is versioned: `DDInjectorCounterRequest.RequestedVersion` is set to `countersVersion = 1` in the current implementation. The public header documents how to extend to V2+ without breaking existing clients.

## Usage

**`procmon`** is used by the Windows security probe (`pkg/security/probe/probe_windows.go`) to receive kernel-level process-start and process-stop events:

```go
onStart := make(chan *procmon.ProcessStartNotification, 100)
onStop  := make(chan *procmon.ProcessStopNotification, 100)
onError := make(chan bool, 1)
wp, err := procmon.NewWinProcMon(onStart, onStop, onError,
    procmon.ProcmonDefaultReceiveSize, procmon.ProcmonDefaultNumBufs)
wp.Start()
// consume onStart / onStop channels ...
wp.Stop()
```

**`ddinjector`** is exposed as a system-probe module (`cmd/system-probe/modules/injector_windows.go`). The module implements `prometheus.Collector` and periodically calls `injector.GetCounters` to populate telemetry gauges that are scraped by the agent's telemetry pipeline. The module is enabled via `injector.enable_telemetry` in the system-probe configuration.

**`driver`** functions are called internally by `procmon` (start/stop of the `ddprocmon` service) and can be used directly when a caller needs lifecycle control over any named Windows driver service.

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/util/winutil`](util/winutil.md) | Provides the higher-level Windows Service lifecycle API (`StartService`, `StopService`, `SCMMonitor`) that the rest of the agent uses. `pkg/windowsdriver/driver` duplicates a thin subset of that API, but is scoped exclusively to kernel-driver services and avoids the heavier `winutil` dependency. For agent-level Windows service management (the `datadogagent` service, fleet management, trace-agent) use `winutil` directly. |
| [`pkg/network`](network/network.md) | The Windows network tracer (`cmd/system-probe/modules/network_tracer.go`) uses the ETW-based connection tracer (`pkg/network/tracer/connection/tracer.go`, `TracerTypeEbpfless` on Windows) instead of `ddprocmon`. For network monitoring on Windows the NPM module relies on the Windows kernel driver interface exposed through `pkg/network/config` and the ETW path, not on `pkg/windowsdriver` directly. |
| [`pkg/security/probe`](security/probe.md) | `pkg/security/probe/probe_windows.go` is the primary consumer of `pkg/windowsdriver/procmon`. The Windows CWS probe creates a `WinProcMon` instance, starts it, and consumes `ProcessStartNotification` / `ProcessStopNotification` events to feed the SECL event pipeline alongside ETW-based file and registry events. The `ddinjector` module is consumed by the system-probe injector module (`cmd/system-probe/modules/injector_windows.go`), which implements `prometheus.Collector` and reports `InjectorCounters` as telemetry. |

## Platform considerations

All files carry `//go:build windows`. There are no stub implementations for other platforms; importing these packages outside a Windows build will fail at compile time. The cgo type definitions in `procmon/types.go` are compiled with `//go:build ignore` and regenerated via `cgo -godefs`; the actual types live in the generated `procmon/types_windows.go`.

### Relationship between `pkg/windowsdriver` and `pkg/util/winutil`

Both packages manipulate Windows services via the SCM, but at different layers:

- `pkg/util/winutil` — agent-level services (`datadogagent`, `datadog-trace-agent`, etc.), service lifecycle helpers, ETW session management, Event Log writing, and the full Windows utility toolkit.
- `pkg/windowsdriver` — kernel-mode driver services (`ddprocmon`, `DDInjector`) only, with device-handle I/O and IOCTL communication that `winutil` does not provide.

When writing Windows-specific agent code that does not need IOCTL communication with a kernel driver, prefer `pkg/util/winutil` to keep the dependency surface minimal.
