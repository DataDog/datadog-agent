# pkg/gohai

## Purpose

`pkg/gohai` collects static host hardware and OS metadata and packages it into a JSON payload that is submitted to Datadog as part of the **host metadata** check-in. The name "gohai" is a portmanteau of "Go" and "hai" (the original Python library it replaced). This metadata powers the Infrastructure list view in the Datadog UI (CPU model, OS version, IP address, memory size, etc.).

The package is a **standalone Go module** (`pkg/gohai/go.mod`) with minimal dependencies, making it easy to embed in other binaries or test independently.

---

## Package Layout

| Sub-package | What it collects |
|---|---|
| `cpu` | Vendor, model name, core/logical processor count, frequency, cache sizes (L1/L2/L3), family/model/stepping. ARM64 and Windows have dedicated collection paths. |
| `filesystem` | Mounted filesystems: device name, size in KB, mount point. Collection runs with a 2-second timeout to avoid hanging on network filesystems. |
| `memory` | Total RAM in bytes, swap size in KB (Unix only). |
| `network` | Primary MAC address, IPv4 address, IPv6 address (optional), and a per-interface breakdown of all up non-loopback interfaces. |
| `platform` | Kernel name/release/version, hostname, machine architecture, OS description, Go version/GOOS/GOARCH. Windows additionally reports the OS family. |
| `processes` | Top-20 process groups by resource usage: usernames, CPU%, memory%, VMS, RSS, name, PID count. |
| `utils` | Generic `Value[T]` type (result-or-error), `AsJSON` reflection helper. |

---

## Key Elements

### Root package

**`Payload`** — the JSON-serialisable wrapper:

```go
type Payload struct {
    Gohai *gohai `json:"gohai"`
}
```

The inner `gohai` struct holds one field per sub-package (`cpu`, `filesystem`, `memory`, `network`, `platform`, `processes`).

**`GetPayload(hostname, useHostnameResolver, isContainerized) *Payload`** — collects all metadata except processes. This is the standard entry point used by the host metadata component.

**`GetPayloadWithProcesses(hostname, useHostnameResolver, isContainerized) *Payload`** — same as above but also includes the top-20 process snapshot. Used by the legacy resources metadata.

**`GetPayloadAsString(hostname, useHostnameResolver, isContainerized) (string, error)`** — marshals the gohai struct to JSON **twice** (the result is a JSON-encoded string). This double-encoding is required to match the Agent v5 host metadata format where the `gohai` field in the outer payload is a serialised string, not a nested object.

**Container/Docker detection** — `getGohaiInfo` skips network collection when running containerised (`isContainerized=true`) unless the `docker0` interface is present (indicating host-network mode). This avoids reporting container-internal interfaces in the host metadata.

**Hostname resolution** — when `useHostnameResolver=true`, the collected network info's primary IP is overridden by a reverse DNS lookup of `hostname`, controlled by `metadata_ip_resolution_from_hostname` in the agent config.

### Sub-package patterns

Every sub-package exposes the same two-function API:

```go
// Collect returns an Info struct; partial results are always returned (no all-or-nothing).
info := cpu.CollectInfo()       // *cpu.Info
info := memory.CollectInfo()    // *memory.Info
info, err := network.CollectInfo()    // *network.Info, error
info, err := filesystem.CollectInfo() // filesystem.Info ([]MountInfo), error
info := platform.CollectInfo()  // *platform.Info
info, err := processes.CollectInfo()  // processes.Info ([]ProcessGroup), error

// AsJSON returns a JSON-compatible interface{}, a slice of non-fatal warnings, and a fatal error.
payload, warnings, err := info.AsJSON()
```

Fields that cannot be collected (e.g. L3 cache on a platform that doesn't expose it) are stored as `utils.Value[T]` errors and are **silently omitted** from the JSON output rather than causing the whole collection to fail.

### `utils.Value[T]`

A generic result-or-error type used as the field type in all `Info` structs:

```go
// Constructors
utils.NewValue(val)          // wraps a successful value
utils.NewErrorValue[T](err)  // wraps a failure
utils.NewValueFrom(val, err) // convenience from a (value, error) pair

// Access
val, err := v.Value()
val      := v.ValueOrDefault()
err      := v.Error()
```

`AsJSON` (in `utils`) iterates struct fields via reflection; a field is included in the output only when its `Value()` returns `nil` error.

### Platform-specific collection

| Sub-package | Linux | macOS | Windows |
|---|---|---|---|
| `cpu` | `/proc/cpuinfo` (default), `lscpu` (ARM64) | `sysctl` | Win32 WMI via CGo (`cpu_windows.c`) |
| `filesystem` | `df -k` output parsing | same | `GetDiskFreeSpaceExW` via syscall |
| `memory` | `/proc/meminfo` | `sysctl hw.memsize` | `GlobalMemoryStatusEx` |
| `network` | `net.Interfaces()` | same | same |
| `platform` | `uname` syscall | `uname` syscall | `RtlGetVersion`, registry reads |
| `processes` | `gopsutil` | same | `gops` (embedded under `processes/gops/`) |

---

## Usage

### In the host metadata component

`comp/metadata/host/hostimpl/payload.go` calls `gohai.GetPayloadAsString` when `enable_gohai: true` (the default):

```go
gohaiPayload, err := gohai.GetPayloadAsString(
    h.hostname,
    h.config.GetBool("metadata_ip_resolution_from_hostname"),
    env.IsContainerized(),
)
p.GohaiPayload = gohaiPayload
```

The resulting host metadata payload is sent to Datadog's `/intake` endpoint every 30 minutes (the standard metadata flush interval).

### In the resources metadata component

`comp/metadata/resources/resourcesimpl/resources.go` calls `gohai.GetPayloadWithProcesses` to include the top-20 process snapshot in the resources metadata payload.

### In the inventory host component

`comp/metadata/inventoryhost/inventoryhostimpl/inventoryhost.go` reads individual fields from `cpu.CollectInfo()` (e.g. `CPUCores`, `CPULogicalProcessors`) to populate the `host_cpu_cores` / `host_logical_processors` inventory metadata fields sent to `datadog-agent/api/v2/host_metadata`.

### Direct sub-package use

The `cpu` sub-package is also imported by `pkg/collector/corechecks/system/cpu/cpu` on Windows for the CPU check.

---

---

## Related packages

| Package | Relationship |
|---|---|
| [`comp/metadata/host`](../../comp/metadata/host.md) | Primary consumer. `hostimpl/payload.go` calls `gohai.GetPayloadAsString` (when `enable_gohai: true`) and embeds the result as the `GohaiPayload` field in the v5 host metadata payload sent to `/intake` every 30 minutes. The component also exposes a `/metadata/gohai` HTTP endpoint that returns the raw gohai payload. |
| [`comp/metadata/inventoryhost`](../../comp/metadata/inventoryhost.md) | Sibling inventory consumer. Imports individual sub-packages (`cpu.CollectInfo()`, `memory.CollectInfo()`, etc.) directly to populate the structured `host_metadata` payload sent to `/api/v2/host_metadata`. The two components overlap in hardware data but use different schemas and endpoints. |
| [`pkg/opentelemetry-mapping-go`](opentelemetry-mapping-go.md) | The `inframetadata` sub-package derives `payload.HostMetadata` from OTel resource attributes. This is an alternative, OTLP-sourced path for reporting host metadata to the Datadog infrastructure list — it does not call gohai directly, but fills a structurally similar payload. |

---

## Notes for contributors

- When adding a new field to an `Info` struct, use `utils.Value[T]` so that a collection failure does not block the rest of the metadata.
- Fields that are platform-specific should be documented with a comment like `// Linux only` or `// Windows and Linux ARM64 only`.
- The `filesystem` package uses a goroutine + channel pattern to enforce a 2-second timeout; follow the same pattern if adding a sub-package that may block on I/O.
- Do not add agent-internal dependencies to `pkg/gohai` — it is a standalone module intended to remain lightweight and reusable outside the agent.
