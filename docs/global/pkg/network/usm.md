> **TL;DR:** `pkg/network/usm` implements Universal Service Monitoring (USM), using eBPF socket filters and uprobes to classify and aggregate application-layer traffic (HTTP, HTTP/2, gRPC, Kafka, PostgreSQL, Redis, TLS) without requiring application instrumentation.

# pkg/network/usm

## Purpose

`pkg/network/usm` implements **Universal Service Monitoring (USM)**: application-layer visibility into network connections without requiring instrumentation of the application code. USM uses eBPF socket filters and uprobes to intercept, classify, and aggregate traffic for protocols such as HTTP/1, HTTP/2, gRPC, Kafka, PostgreSQL, Redis, as well as encrypted variants via OpenSSL/GnuTLS native-TLS, Go TLS, Istio, and Node.js.

USM is built on top of the NPM connection tracer: it reuses the `connection_protocol` eBPF map populated by the tracer and correlates protocol-level statistics back to connection tuples.

The package requires the `linux_bpf` build tag (Linux only; the Windows stub is in `monitor_windows.go`). The minimum supported kernel version is **4.14.0**.

---

## Key Elements

### Key interfaces

The `Monitor` type is a concrete struct; there is no `Monitor` interface. The `state/` sub-package exposes a global `MonitorState` string enum. File identity is abstracted through `PathIdentifier` and `FileRegistry` in `utils/`.

### Key types

#### `Monitor` (monitor.go)

The entry point for USM. Created and owned by `pkg/network/tracer.Tracer`.

```go
type Monitor struct {
    cfg            *config.Config
    ebpfProgram    *ebpfProgram      // wraps the eBPF manager
    processMonitor *monitor.ProcessMonitor
    closeFilterFn  func()            // cleans up the raw socket filter
    statsd         statsd.ClientInterface
}
```

**Lifecycle:**

```
usm.NewMonitor(cfg, connectionProtocolMap, statsd) → *Monitor
    ↳ newEBPFProgram()     — discovers enabled protocols, creates eBPF manager
    ↳ mgr.Init()           — loads and verifies eBPF objects
    ↳ filterpkg.HeadlessSocketFilter() — attaches the dispatcher socket filter

Monitor.Start()            — loads eBPF programs, optionally starts ProcessMonitor
Monitor.Stop()
Monitor.Pause() / Resume() — bypass/re-enable eBPF programs without stopping
```

**Primary API:**

| Method | Description |
|---|---|
| `GetProtocolStats() (map[protocols.ProtocolType]interface{}, func())` | Returns per-protocol stats maps and a cleanup function. Called by `tracer.GetActiveConnections`. |
| `GetUSMStats() map[string]any` | Returns current monitor state, startup error, blocked/traced process lists. |
| `DumpMaps(w io.Writer, maps ...string) error` | Debug dump of eBPF maps. |

**Protocol support** (`ebpf_main.go`):

USM initializes the following protocol specs from `pkg/network/protocols/`:

- `http.Spec`, `http2.Spec`, `kafka.Spec`, `postgres.Spec`, `redis.Spec`
- `opensslSpec` (native TLS via uprobes on libssl/libcrypto/GnuTLS/libnode)
- `goTLSSpec` (Go TLS via uprobes)
- `istioSpec`, `nodejsSpec`

Protocols that are not enabled in config are excluded before the eBPF program is loaded; if no protocols remain, `NewMonitor` returns `nil` (not an error) and USM is disabled.

The central eBPF program is a `BPF_PROG_TYPE_SOCKET_FILTER` attached to a raw socket (`socket__protocol_dispatcher`). It classifies connection payloads and tail-calls per-protocol handlers.

---

### Key functions

#### `config/` sub-package — support and feature checks

```go
// Minimum kernel version for USM
MinimumKernelVersion = kernel.VersionCode(4, 14, 0)

// Key exported functions
CheckUSMSupported(cfg *config.Config) error
IsUSMSupportedAndEnabled(cfg *config.Config) bool
TLSSupported(cfg *config.Config) bool
UretprobeSupported() bool
NeedProcessMonitor(cfg *config.Config) bool
ShouldUseNetifReceiveSKBCoreKprobe() bool
```

- `CheckUSMSupported` gates the entire USM subsystem on kernel version and the `EnableEbpfless` flag (eBPF-less mode does not support USM).
- `TLSSupported` adds an ARM-specific floor of 5.5.0 and requires runtime compilation or CO-RE.
- `UretprobeSupported` checks for the kernel bug that causes segfaults with uretprobes + seccomp filters.
- `NeedProcessMonitor` returns `true` when any TLS or Istio monitoring feature is enabled — the process monitor is needed to track execve/exit events for uprobe attachment.

---

#### `sharedlibraries/` sub-package — shared library open event tracking

Detects when a process opens a library from a known set (crypto, GPU, libc) by hooking `open`/`openat`/`openat2` syscalls. This is the trigger for attaching TLS uprobes to the process.

**Key types:**

| Symbol | Description |
|---|---|
| `Libset` | String identifier for a group of libraries (`LibsetCrypto`, `LibsetGPU`, `LibsetLibc`) |
| `LibsetToLibSuffixes` | Maps each `Libset` to filename suffix patterns (e.g., `"libssl"`, `"crypto"`, `"gnutls"`) |
| `LibPath` | C struct (`lib_path_t`) carrying the path string from kernel to userspace via perf ring |
| `LibraryCallback` | `func(LibPath)` — called when a matching library is opened |
| `EbpfProgram` | Singleton eBPF program managing all libsets; ref-counted |

**Key functions:**

| Function | Description |
|---|---|
| `GetEBPFProgram(cfg)` | Returns the singleton `EbpfProgram`, incrementing its ref-count |
| `EbpfProgram.InitWithLibsets(libsets...)` | (Re-)initializes the program for the given libset set; idempotent |
| `EbpfProgram.Subscribe(callback, libsets...)` | Registers a callback for library-open events; returns an unsubscribe function |
| `EbpfProgram.Stop()` | Decrements ref-count; stops the program when it reaches zero |
| `IsSupported(cfg)` | Checks kernel version (≥ 4.14, or ≥ 5.5 on ARM with CORE/RC) |

**Probe selection:** The program chooses the most efficient available probe type at startup: fexit (kernel ≥ 5.6 with fexit support) → tracepoints (≥ 4.15) → kprobe fallback.

---

#### `state/` sub-package — USM monitor lifecycle state

A tiny thread-safe global tracking the monitor's operational state.

```go
const (
    Disabled   MonitorState = "Disabled"
    Running    MonitorState = "Running"
    NotRunning MonitorState = "Not running"
    Stopped    MonitorState = "Stopped"
)

func Set(state MonitorState)
func Get() MonitorState
```

Used by `Monitor` and exposed through `GetUSMStats()`. The `GetUSMStats` API reports this state to system-probe's debug endpoint.

---

#### `maps/` sub-package — eBPF map leak detection

Provides tooling to detect leaked entries in PID-keyed TLS eBPF maps (entries whose owning PID has exited without cleanup).

**Key types:**

| Type | Description |
|---|---|
| `MapLeakInfo` | Per-map report: total entries, leaked entries, leak rate, dead PIDs |
| `LeakDetectionReport` | Aggregated report across all checked maps |

**Key functions:**

| Function | Description |
|---|---|
| `CheckPIDKeyedMaps() (*LeakDetectionReport, error)` | Enumerates all system-wide eBPF maps, finds maps owned by USM (via `usm.GetPIDKeyedTLSMapNames()`), validates each for leaked entries |

Map names are truncated to 15 chars by the kernel; all USM map names are designed to be unique within their first 15 characters.

---

#### `utils/` sub-package — file registry and path utilities

Provides `FileRegistry`, the core data structure for tracking which processes currently have a given shared library open, and driving activation/deactivation callbacks.

```go
// FileRegistry tracks reference counts per PathIdentifier (device+inode).
// Activation callback fires when count goes 0 → 1.
// Deactivation callback fires when count goes 1 → 0.
type FileRegistry struct { ... }

type FilePath struct {
    HostPath string         // path resolved from /proc/<pid>/root/
    ID       PathIdentifier // (dev, inode) tuple
    PID      uint32
}
```

Supporting utilities:
- `NewFilePath(procRoot, namespacedPath, pid)` — resolves a namespaced path to a host-visible path.
- `PathIdentifier` — device+inode tuple used as stable file identity across mounts.
- `GetBlockedPathIDsList`, `GetTracedProgramList` — expose blocked/traced file lists for debug endpoints.

---

## Usage

`pkg/network/tracer.Tracer` creates and manages the `Monitor`:

```go
// pkg/network/tracer/tracer.go
monitor, err := usm.NewMonitor(c, connectionProtocolMap, statsd)
monitor.Start()
...
usmStats, cleanup := monitor.GetProtocolStats()
defer cleanup()
delta := state.GetDelta(clientID, latestTime, active, dnsStats, usmStats)
```

The `connectionProtocolMap` eBPF map is shared between NPM and USM: NPM populates it with connection tuples; USM reads it to correlate protocol classifications back to connections.

TLS protocol monitors (OpenSSL, Go TLS) use `sharedlibraries.GetEBPFProgram` to subscribe to library-open events, then attach uprobes to the specific process/function combination via `utils.FileRegistry`.

### Configuration and build flags

| Tag | Meaning |
|---|---|
| `linux_bpf` | Required for the full USM implementation |
| no tag | Windows stub in `monitor_windows.go` (no-op) |

### Configuration options (abbreviated)

| Config field | Effect |
|---|---|
| `ServiceMonitoringEnabled` | Master switch; gates `IsUSMSupportedAndEnabled` |
| `EnableNativeTLSMonitoring` | OpenSSL/GnuTLS uprobes |
| `EnableGoTLSSupport` | Go TLS uprobes |
| `EnableIstioMonitoring` | Istio sidecar monitoring |
| `EnableNodeJSMonitoring` | Node.js TLS monitoring |
| `EnableUSMEventStream` | Use the event stream (vs. polling) for process events |
| `EnableRuntimeCompiler` / `EnableCORE` | Required for TLS on ARM |

---

## Architecture overview

```
pkg/network/tracer.Tracer
    |
    +--> usm.NewMonitor(cfg, connectionProtocolMap, statsd)
            |
            v
        ebpfProgram (wraps pkg/ebpf.Manager)
            +--> protocols.ProtocolSpec factories  [pkg/network/protocols/]
            |        +--> http.Spec, http2.Spec, kafka.Spec, postgres.Spec, redis.Spec
            |        +--> opensslSpec, goTLSSpec, istioSpec, nodejsSpec
            |
            +--> socket__protocol_dispatcher (BPF_PROG_TYPE_SOCKET_FILTER)
            |        tail-calls per-protocol handlers
            |
            +--> sharedlibraries.EbpfProgram  (track library opens)
            |        +--> utils.FileRegistry  (device+inode reference counting)
            |        +--> pkg/ebpf/uprobes.UprobeAttacher  (attach TLS uprobes)
            |
            v
        Monitor.GetProtocolStats()
            |
            v
        network.State.GetDelta()  -->  Connections.USMData
```

TLS uprobes for Go binaries additionally use `pkg/network/go/bininspect` to locate function
entry and return offsets inside each target ELF before `UprobeAttacher` attaches the probes.

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/network/protocols` | [protocols.md](protocols.md) | Defines the `Protocol` interface, `ProtocolSpec`, and all per-protocol implementations loaded by USM. `usm/ebpf_main.go` registers every `ProtocolSpec`. |
| `pkg/network/tracer` | [tracer.md](tracer.md) | Creates and owns the `Monitor`. Passes the shared `connectionProtocolMap` eBPF map to `NewMonitor` and calls `GetProtocolStats()` at each check interval. |
| `pkg/network/go` | [go.md](go.md) | Used by `ebpf_gotls.go` to inspect Go binary ELF files for TLS function offsets before uprobe attachment via `bininspect.InspectNewProcessBinary`. |
| `pkg/ebpf/uprobes` | [../../pkg/ebpf/uprobes.md](../../pkg/ebpf/uprobes.md) | Provides `UprobeAttacher`, which USM uses in `ebpf_ssl.go`, `ebpf_gotls.go`, `istio.go`, and `nodejs.go` to attach TLS uprobes to target processes. |
| `pkg/ebpf` | [../../pkg/ebpf.md](../../pkg/ebpf.md) | Provides `pkg/ebpf.Manager`, CO-RE/RC/prebuilt program loading, `MapCleaner`, `PerfHandler`, and `NowNanoseconds`. The USM `ebpfProgram` is a thin wrapper around a `pkg/ebpf.Manager`. |
| `pkg/network` | [network.md](network.md) | Defines the `Connections.USMData` map and `State` interface. USM protocol stats flow into `State.GetDelta` then into `Connections.USMData`. |
