# pkg/networkdevice

## Purpose

`pkg/networkdevice` is the shared library for all Network Device Monitoring (NDM) integrations (SNMP, Cisco SD-WAN, Versa, Netflow, Network Configuration Management). It provides:

- **Common metadata types** used to describe devices, interfaces, topology links, VPN tunnels, and diagnostics — the exact JSON shapes that are sent to the Datadog backend.
- **Profile definitions** — the Go representation of SNMP device profiles, used by both the SNMP integration and the Datadog backend.
- **Pinger** — a cross-platform ICMP ping utility.
- **Sender** — a thin wrapper over the agent's `aggregator.Sender` with NDM-specific helpers (timestamp deduplication, per-device tag injection).
- **Diagnoses** — a collector that gathers per-device health diagnostics and converts them to both the NDM metadata format and the agent's `diagnose` CLI format.
- **Utility functions** shared across NDM integrations.

## Key elements

### `pkg/networkdevice/metadata`

The central package; defines every JSON type sent to the Datadog NDM intake.

| Type | Description |
|------|-------------|
| `NetworkDevicesMetadata` | Top-level payload. Contains slices of `DeviceMetadata`, `InterfaceMetadata`, `IPAddressMetadata`, `TopologyLinkMetadata`, `VPNTunnelMetadata`, `NetflowExporter`, `DiagnosisMetadata`, `DeviceOID`, and a `ScanStatusMetadata`. |
| `DeviceMetadata` | Identity and inventory data for a single device: IP, tags, vendor, model, OS, serial number, profile name/version, etc. |
| `InterfaceMetadata` | Per-interface data: index, name, alias, MAC, admin/oper status, physical flag, Meraki-specific fields. |
| `IPAddressMetadata` | IP address + prefix length, linked to an interface by `InterfaceID`. |
| `TopologyLinkMetadata` | Directed link between two `TopologyLinkSide` objects (each holds a device + interface reference). |
| `VPNTunnelMetadata` | IPsec tunnel with endpoints, status, and route addresses. |
| `DeviceStatus` | Enum: `DeviceStatusReachable` (1) / `DeviceStatusUnreachable` (2). |
| `IfAdminStatus` / `IfOperStatus` | IF-MIB enums with `.AsString()` helpers. |
| `ScanStatus` / `ScanType` | Enums for network scan state (`in progress`, `completed`, `error`) and trigger type (`manual`, `rc_triggered`, `default`). |
| `PayloadMetadataBatchSize` | Constant `100` — the max number of resources per metadata event payload. |

### `pkg/networkdevice/profile/profiledefinition`

Defines the Go representation of an SNMP device profile. Shared between the SNMP integration and the backend.

| Type | Description |
|------|-------------|
| `ProfileDefinition` | Root profile structure: `SysObjectIDs`, `Extends`, `Metadata`, `MetricTags`, `StaticTags`, `Metrics`, `Version`. Supports both YAML (file-based profiles) and JSON (RC/backend profiles). |
| `MetricsConfig` | A single metric declaration: scalar or table-based, with symbol, forced type, tags, etc. |
| `SymbolConfig` | Maps an OID to a metric/tag name; supports `extract_value` regex, `match_pattern`, `scale_factor`, `format`. |
| `MetricTagConfig` | Tag derived from an OID value, mapping table, or regex group. |
| `MetadataConfig` | `map[string]MetadataResourceConfig` — device/interface field definitions. |
| `ProfileMetricType` | Enum controlling the Datadog metric type emitted: `gauge`, `rate`, `monotonic_count`, `monotonic_count_and_rate`, `flag_stream`. |
| `DeviceProfileRcConfig` | Wrapper for profiles stored in Remote Config: `{ "profile_definition": ProfileDefinition }`. |
| `ProfileDefinition.SplitOIDs(includeMetadata bool)` | Returns `(scalars, columns)` — the two OID lists needed to build SNMP GET / GETBULK requests. |
| `ProfileDefinition.Clone()` | Deep-copy of a profile. |

The `schema/` sub-package ships a JSON Schema for RC profile validation (`profile_rc_schema.json`). The `normalize_cmd` and `schema_cmd` binaries are developer tools for normalising profile YAML and regenerating the schema.

### `pkg/networkdevice/pinger`

| Symbol | Description |
|--------|-------------|
| `Pinger` interface | Single method: `Ping(host string) (*Result, error)`. |
| `Config` | `UseRawSocket bool`, `Interval`, `Timeout`, `Count` (defaults: 2 pings, 20 ms interval, 3 s timeout). |
| `Result` | `CanConnect bool`, `PacketLoss float64`, `AvgRtt time.Duration`. |
| `ErrRawSocketUnsupported` / `ErrUDPSocketUnsupported` | Sentinel errors when the selected socket type is unavailable on the current OS. |

Platform-specific implementations live in `pinger_linux.go`, `pinger_darwin.go`, `pinger_windows.go`. A `mock.go` is provided for tests.

### `pkg/networkdevice/sender`

| Symbol | Description |
|--------|-------------|
| `Sender` interface | Extends `aggregator/sender.Sender` with `GaugeWithTimestampWrapper`, `CountWithTimestampWrapper`, `UpdateTimestamps`, `SetDeviceTagsMap`, `GetDeviceTags`, `ShouldSendEntry`. |
| `IntegrationSender` | Concrete implementation. Tracks `lastTimeSent` per metric key to avoid re-sending stale timestamped metrics. Expires entries older than 6 hours on `Commit()`. |
| `NewSender(s sender.Sender, integration, namespace string)` | Constructor. |
| `GetDeviceTags(defaultIPTag, deviceIP string) []string` | Returns pre-populated device tags from the internal map, or falls back to `[<tag>:<ip>, device_namespace:<ns>]`. |
| `ShouldSendEntry(key string, ts float64) bool` | Returns `false` if `ts` is not newer than the last sent timestamp for `key`. |

### `pkg/networkdevice/diagnoses`

| Symbol | Description |
|--------|-------------|
| `Diagnoses` | Holds a list of `metadata.Diagnosis` items for one NDM resource. Thread-safe flush. |
| `NewDeviceDiagnoses(deviceID string)` | Creates a `Diagnoses` scoped to a device resource. |
| `Add(result, code, message string)` | Appends a diagnosis. `result` is a severity string: `"success"`, `"error"`, or `"warn"`. |
| `Report() []metadata.DiagnosisMetadata` | Flushes and returns metadata-ready diagnoses. |
| `ReportAsAgentDiagnoses() []diagnose.Diagnosis` | Converts the last flushed diagnoses to the agent `diagnose` CLI format (used by `agent diagnose`). |

### `pkg/networkdevice/integrations`

The `Integration` string enum lists all NDM integrations: `snmp`, `cisco-sdwan`, `versa`, `netflow`, `network-configuration-management`. Used as the `integration` field in `NetworkDevicesMetadata`.

### `pkg/networkdevice/utils`

Small utilities used across NDM:

| Function | Description |
|----------|-------------|
| `CopyStrings(tags []string) []string` | Returns a copy of a string slice. |
| `BoolToFloat64(val bool) float64` | Converts a bool to 1.0 / 0.0. |
| `GetAgentVersionTag() string` | Returns `"agent_version:<version>"`. |
| `GetCommonAgentTags() []string` | Returns `["agent_host:<hostname>", "agent_version:<version>"]`. |

## Usage

### Building and sending a metadata payload (SNMP integration pattern)

```go
import (
    "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
    "github.com/DataDog/datadog-agent/pkg/networkdevice/sender"
)

// Wrap the aggregator sender
ndmSender := sender.NewSender(agentSender, "snmp", "default")
ndmSender.SetDeviceTagsMap(deviceTagsMap)

// Build the payload
payload := metadata.NetworkDevicesMetadata{
    Namespace:        "default",
    Integration:      integrations.SNMP,
    CollectTimestamp: time.Now().Unix(),
    Devices: []metadata.DeviceMetadata{
        {
            ID:        "default:192.168.1.1",
            IPAddress: "192.168.1.1",
            Status:    metadata.DeviceStatusReachable,
        },
    },
}

// Batch into slices of PayloadMetadataBatchSize (100) before serialising
```

### Sending a timestamped metric safely

```go
// Only send the metric if the timestamp is newer than what was last sent
if ndmSender.ShouldSendEntry("ifInOctets:eth0", ts) {
    ndmSender.GaugeWithTimestampWrapper("snmp.ifInOctets", value, tags, ts)
}
// At the end of the collection cycle:
ndmSender.UpdateTimestamps(collectedTimestamps)
ndmSender.Commit()
```

### Using the pinger

```go
import "github.com/DataDog/datadog-agent/pkg/networkdevice/pinger"

cfg := pinger.Config{UseRawSocket: false, Count: 3, Timeout: 5 * time.Second}
p, err := pinger.New(cfg) // platform-specific constructor
result, err := p.Ping("192.168.1.1")
if result.CanConnect {
    // device is reachable
}
```

### Where these packages are consumed

- `pkg/collector/corechecks/snmp/` — SNMP check (main consumer of metadata, profiledefinition, pinger, sender, diagnoses)
- `pkg/collector/corechecks/network-devices/cisco-sdwan/` — Cisco SD-WAN check
- `pkg/collector/corechecks/network-devices/versa/` — Versa check
- `pkg/networkconfigmanagement/` — Network Configuration Management
- `comp/netflow/` — Netflow (uses metadata for netflow exporters; see [`comp/netflow/server`](../../comp/netflow/server.md) for the server component that wires listeners and the flow aggregator)
- `comp/snmpscan/` — SNMP network scanner (uses metadata for scan status)

## Related documentation

| Document | Relationship |
|----------|-------------|
| [`pkg/snmp`](../snmp.md) | Provides the SNMP session, credential, and listener-config building blocks consumed by the SNMP check. `pkg/networkdevice/profile/profiledefinition` defines the profile types that `pkg/snmp` indirectly references for autodiscovery. |
| [`pkg/networkpath`](../networkpath.md) | The network path feature is a parallel NDM capability; both packages share the `pkg/networkdevice/pinger` for ICMP reachability and emit payloads through the same event-platform pipeline. |
| [`comp/netflow/server`](../../comp/netflow/server.md) | Top-level NetFlow component. Sends `NetflowExporter` records using `metadata.NetworkDevicesMetadata` from this package. |
| [`comp/snmptraps/server`](../../comp/snmptraps/server.md) | Top-level SNMP traps component. Uses the `network_devices.namespace` value (same namespace model as NDM) and produces trap events alongside NDM device metadata on the Datadog backend. |
