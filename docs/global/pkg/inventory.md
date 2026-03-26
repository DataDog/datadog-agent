> **TL;DR:** `pkg/inventory` collects installed software applications (Windows, macOS) and static hardware information (manufacturer, model, serial number) from the host, forwarding them to the Datadog backend via system-probe and the metadata components.

# pkg/inventory

Two sub-packages that collect host hardware and software metadata for Datadog's inventory features.

---

## pkg/inventory/software

### Purpose

Collects the list of installed applications from the host system. The data is used by the `comp/softwareinventory` component and surfaced through the `software_inventory` metadata payload sent to the Datadog backend. Collection is triggered from within System Probe (on Windows and macOS), then forwarded to the main Agent over IPC.

### Key elements

### Key interfaces

**`Collector` interface** (`collector.go`)

```go
type Collector interface {
    Collect() ([]*Entry, []*Warning, error)
}
```

Each platform-specific collector (e.g. registry, MSI database, `.app` bundles, Homebrew, PKG receipts) implements this interface and is responsible for a single software source.

### Key types

**`Entry`** (`collector.go`)

The primary data type. Fields that are serialized in the backend payload:

| Field | JSON key | Notes |
|---|---|---|
| `Source` | `software_type` | Source type: `desktop`, `msstore`, `app`, `pkg`, `homebrew`, `kext`, `sysext`, etc. |
| `DisplayName` | `name` | Human-readable application name |
| `Version` | `version` | Version string |
| `InstallDate` | `deployment_time` | RFC3339 UTC timestamp (omitempty) |
| `Publisher` | `publisher` | Vendor name |
| `Is64Bit` | `is_64_bit` | Architecture flag |
| `Status` | `deployment_status` | `"installed"` or `"broken"` |
| `ProductCode` | `product_code` | Package-manager-specific unique ID |
| `UserSID` | `user` | Windows user SID for per-user installs (omitempty) |

Fields tagged `json:"-"` (`BrokenReason`, `InstallSource`, `PkgID`, `InstallPath`, `InstallPaths`) are used internally and in the Agent↔System Probe wire format but are not sent to the backend.

**`Entry.GetID()`**

Returns a stable identifier: `"{source}:{ProductCode|DisplayName}:{InstallPath}"`. Used to correlate entries between the MSI database and the Windows registry to detect broken installations.

**`SoftwareInventoryWireEntry` / `EntryToWire` / `WireToEntry`** (`wire.go`)

Wire format for Agent↔System Probe communication. Includes all fields (including internal ones) so they survive the IPC round-trip. The backend payload uses `Entry` directly, which omits internal fields via `json:"-"`.

**`Warning`**

A non-fatal collection issue. A collector returns warnings instead of errors when partial data is available.

### Key functions

**Top-level functions**

- `GetSoftwareInventory() ([]*Entry, []*Warning, error)` — calls `defaultCollectors()` for the current platform and aggregates results.
- `GetSoftwareInventoryWithCollectors(collectors []Collector)` — same, but accepts an explicit list; useful in tests.

**Platform-specific collectors**

| Platform | Collectors |
|---|---|
| Windows | `registryCollector` (Uninstall registry keys), `mSICollector` (MSI database), `msStoreAppsCollector`; wrapped by `desktopAppCollector` which cross-references MSI and registry to flag broken entries |
| macOS | `applicationsCollector` (`.app` bundles in `/Applications`), `pkgReceiptsCollector` (pkgutil receipts), `kernelExtensionsCollector`, `systemExtensionsCollector`, `homebrewCollector`, `macPortsCollector` |
| Linux | No collectors implemented yet (stub returns empty list) |

### Usage

The System Probe registers a `SoftwareInventory` HTTP module (`cmd/system-probe/modules/software_inventory.go`, build tag `darwin || windows`) that calls `GetSoftwareInventory()` and serializes results as `[]SoftwareInventoryWireEntry` JSON. The Agent's `comp/softwareinventory` component calls that endpoint via `sysprobeclient.GetCheck` on a configurable interval (`software_inventory.interval`, default 10 minutes) and forwards the payload to the event platform (`eventplatform.EventTypeSoftwareInventory`).

---

## pkg/inventory/systeminfo

### Purpose

Collects static hardware metadata about the host: manufacturer, model, serial number, and chassis type. Used exclusively for the `host_system_info` metadata payload, which is only sent in `infrastructure_mode: end_user_device` on Windows and macOS.

### Key elements

### Key types

**`SystemInfo`** (`collector.go`)

```go
type SystemInfo struct {
    Manufacturer string
    ModelNumber  string
    SerialNumber string
    ModelName    string
    ChassisType  string  // "Desktop", "Laptop", "Virtual Machine", or "Other"
    Identifier   string  // e.g. MacBook Pro model identifier, Windows SKU number
}
```

### Key functions

**`Collect() (*SystemInfo, error)`**

Single entry point. Delegates to a platform-specific `collect()` function:

- **Windows** (`collector_windows.go`): queries WMI classes `Win32_ComputerSystem`, `Win32_BIOS`, and `Win32_SystemEnclosure`. Detects Hyper-V/Azure (`model == "Virtual Machine"`) and AWS EC2 (`manufacturer == "Amazon EC2"`) as virtual machines.
- **macOS** (`collector_darwin.go`): calls Objective-C code via cgo (`systeminfo_darwin.m`) that reads IOKit and Foundation frameworks to get the model identifier, serial number, and product name.
- **Linux** (`collector_nix.go`): returns `nil, nil` (not implemented).

`ChassisType` is normalized to one of `"Desktop"`, `"Laptop"`, `"Virtual Machine"`, or `"Other"` across both Windows (WMI `Win32_SystemEnclosure.ChassisTypes`) and macOS (product name heuristic).

### Usage

`comp/metadata/hostsysteminfo/impl/hostsysteminfo.go` calls `systeminfo.Collect()` in its `fillData()` method and maps the result to a `Payload` struct sent to the backend. Collection is gated by `infrastructure_mode == "end_user_device"` and limited to Windows/macOS. The metadata runner triggers a refresh every hour.

---

## Related packages

| Package | Relationship |
|---|---|
| [`comp/softwareinventory`](../../comp/softwareinventory.md) | The primary consumer of `pkg/inventory/software`. The component queries the System Probe's `SoftwareInventoryModule` HTTP endpoint (which calls `GetSoftwareInventory()`), deserializes the wire format into `[]*software.Entry`, and forwards the payload to the Datadog backend via the Event Platform forwarder. The component is Windows-only; the Linux stub in this package returns an empty list. |
| [`comp/metadata/inventoryhost`](../../comp/metadata/inventoryhost.md) | A sibling metadata component that handles structured host hardware metadata (CPU, memory, OS). Together with `pkg/inventory/systeminfo`, these two packages cover the full "inventory" surface area for host-level data — `inventoryhost` for cloud/hardware dimensions and `systeminfo` for physical chassis details. |
| [`pkg/util/containerd`](util/containerd.md) | Not a direct dependency of `pkg/inventory`, but both packages contribute to the broader software bill-of-materials story. `pkg/util/containerd` supports image mounting for SBOM scanning (`pkg/sbom/collectors/containerd`), which complements the installed-application inventory from `pkg/inventory/software` for full fleet software visibility. |
