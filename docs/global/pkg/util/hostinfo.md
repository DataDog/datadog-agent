> **TL;DR:** Returns a cached snapshot of host OS metadata (hostname, uptime, boot time, platform, kernel version, virtualization, UUID) used by the host metadata pipeline and agent telemetry.

# pkg/util/hostinfo

## Purpose

`pkg/util/hostinfo` provides a single function, `GetInformation()`, that returns an aggregated snapshot of host operating system metadata (hostname, uptime, boot time, OS/platform, kernel version, virtualization info, host UUID). Results are cached in `pkg/util/cache` so repeated calls are cheap. The package has platform-specific implementations for Linux/macOS and Windows.

---

## Key elements

### `GetInformation() *<InfoStat>`

The only exported entry point. Returns a pointer to a populated info struct. On failure the function logs an error and returns an empty struct rather than propagating the error, so callers never need to handle a nil return.

**Linux / macOS** (build tag `!windows`): delegates to `github.com/shirou/gopsutil/v4/host.Info()` and returns a `*host.InfoStat` from that library. Fields of note:

| Field | Description |
|---|---|
| `Hostname` | System hostname |
| `Uptime` | Seconds since boot |
| `BootTime` | Unix timestamp of last boot |
| `OS` / `Platform` / `PlatformFamily` | e.g. `"linux"`, `"ubuntu"`, `"debian"` |
| `PlatformVersion` | Full OS version string |
| `KernelVersion` / `KernelArch` | Kernel version and CPU architecture |
| `VirtualizationSystem` / `VirtualizationRole` | Hypervisor type and guest/host role |
| `HostID` | Host UUID |

**Windows**: returns a locally-defined `*hostinfo.InfoStat` (same field names and JSON tags as the gopsutil struct). Populated by combining:
- `os.Hostname()` for the hostname
- `GetTickCount64` (kernel32.dll) for uptime / boot time
- `Pids()` (via `w32.EnumProcesses`) for the running process count
- `pkg/gohai/platform` for OS/platform fields
- `pkg/util/winutil.GetWindowsBuildString()` for the platform version
- `pkg/util/uuid.GetUUID()` for the host ID

**`Pids() ([]int32, error)`** (Windows only) — enumerates all running process IDs using `EnumProcesses`. Automatically retries with a larger buffer if the initial 1024-entry buffer is too small.

---

## Usage

**`comp/metadata/host/hostimpl/utils/host.go`** (and its platform-specific variants) — calls `GetInformation()` to populate the host metadata payload sent to the Datadog backend at startup and on a periodic schedule. Fields like `PlatformVersion`, `KernelVersion`, and `HostID` are surfaced in the Datadog UI under Infrastructure > Host Map.

**`comp/core/agenttelemetry/impl/sender.go`** — includes host metadata in internal agent telemetry payloads.

**`pkg/process/checks/net.go`** — uses `GetInformation()` to attach host context to network connection data reported by the process agent.

**`comp/metadata/host/hostimpl/utils/host.go`** example pattern:

```go
info := hostinfo.GetInformation()
payload.KernelVersion = info.KernelVersion
payload.Platform = info.Platform
```

---

## Relationship to other packages

| Package / component | Relationship |
|---|---|
| `pkg/util/hostname` ([docs](hostname.md)) | `pkg/util/hostname` resolves the agent's logical hostname (the string that identifies a host in Datadog) using a prioritized provider chain (config, GCE, AWS EC2, kubelet, etc.) with drift detection. `pkg/util/hostinfo` returns low-level OS metadata (uptime, kernel version, platform, UUID) from `gopsutil` or WinAPI — it does not resolve the Datadog hostname. The two packages are consumed together by `comp/metadata/host` to build complete host metadata payloads. |
| `comp/metadata/inventoryhost` ([docs](../../comp/metadata/inventoryhost.md)) | `comp/metadata/inventoryhost` builds the structured `host_metadata` inventory payload using gohai, DMI/SMBIOS, and cloud provider detection — fields like `kernel_name`, `kernel_release`, and `os` overlap with what `pkg/util/hostinfo` returns. However `inventoryhost` collects data directly from gohai rather than through `pkg/util/hostinfo`; the two are parallel sources for related but distinct payloads (`inventoryhost` → Infrastructure inventory, `comp/metadata/host` via `hostinfo` → legacy host metadata). |
| `pkg/util/winutil` ([docs](winutil.md)) | On Windows, `GetInformation()` calls `pkg/util/winutil.GetWindowsBuildString()` to populate the `PlatformVersion` field (e.g. `"10.0 Build 19041"`). `pkg/util/winutil` is also used for `pkg/util/uuid.GetUUID()` to obtain the host ID on Windows. |
