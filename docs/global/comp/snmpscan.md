# comp/snmpscan — SNMP Device Scanner Component

**Import path:** `github.com/DataDog/datadog-agent/comp/snmpscan/def`
**Team:** network-device-monitoring-core
**Importers:** `comp/snmpscanmanager`, `cmd/agent/subcommands/snmp` (CLI), `pkg/collector/corechecks/snmp` (autodiscovery)

## Purpose

`comp/snmpscan` performs on-demand SNMP operations against network devices. It provides two capabilities:

- **Device scan** — walks the full OID tree of a device, batches the results into `NetworkDevicesMetadata` payloads, and streams them to the event platform (NDM metadata pipeline).
- **SNMP walk** — walks the OID tree starting from a given OID and prints each value to stdout in a format similar to the `snmpwalk` CLI tool.

It is the low-level execution engine used by `comp/snmpscanmanager` (periodic scheduled scans) and `cmd/agent/subcommands/snmp` (interactive CLI diagnostics). It can also be triggered remotely via a Remote Config agent task (`TaskDeviceScan`), which the component handles through its `TaskListenerProvider` side-output.

## Package layout

| Package | Role |
|---|---|
| `comp/snmpscan/def` | Component interface and `ScanParams` type |
| `comp/snmpscan/impl` | Full implementation |
| `comp/snmpscan/fx` | fx `Module()` wiring `impl` |
| `comp/snmpscan/mock` | Mock implementation for tests |

## Component interface

```go
// Package: github.com/DataDog/datadog-agent/comp/snmpscan/def

type ScanParams struct {
    ScanType     metadata.ScanType   // e.g. DefaultScan, RCTriggeredScan
    CallInterval time.Duration       // delay between SNMP calls (0 = no delay)
    MaxCallCount int                 // max SNMP calls in a scan (0 = unlimited)
}

type Component interface {
    // RunSnmpWalk prints all OIDs reachable from firstOid to stdout, snmpwalk style.
    RunSnmpWalk(snmpConnection *gosnmp.GoSNMP, firstOid string) error

    // ScanDeviceAndSendData walks the device, batches PDUs into metadata payloads,
    // and sends them to the event platform. Reports in-progress, error, and
    // completed scan-status events to the backend throughout.
    ScanDeviceAndSendData(ctx context.Context, connParams *snmpparse.SNMPConfig, namespace string, scanParams ScanParams) error
}
```

## Implementation details

**`ScanDeviceAndSendData`** follows a structured protocol:

1. Sends an `InProgress` scan-status event immediately (before connecting), so the backend can track long-running scans.
2. Establishes the SNMP connection using `snmpparse.NewSNMP`. If the connection fails, sends an `Error` scan-status event and returns a `gosnmplib.ConnectionError`.
3. Calls `gatherPDUs`, which uses `gosnmplib.ConditionalWalk` to walk the full OID tree. The walk collects one row per table (using `SkipOIDRowsNaive`) to keep payload sizes manageable. `CallInterval` and `MaxCallCount` in `ScanParams` are forwarded to the walk to rate-limit aggressive scans.
4. Converts raw PDUs to `metadata.DeviceOID` records and batches them using `metadata.BatchDeviceScan` (respecting `PayloadMetadataBatchSize`).
5. Sends each batch to the event platform via `eventplatform.EventTypeNetworkDevicesMetadata`.
6. Sends a `Completed` scan-status event on success, or an `Error` event if the walk or send fails.

**Remote Config integration:** `NewComponent` also registers an `rctypes.TaskListenerProvider`. When the RC backend sends a `TaskDeviceScan` task (containing `ip_address` and optionally `namespace`), the component resolves the SNMP config via `snmpparse.GetParamsFromAgent` (which queries the running agent over IPC) and calls `ScanDeviceAndSendData` with `ScanType: RCTriggeredScan`.

## fx wiring

```go
import snmpscanfx "github.com/DataDog/datadog-agent/comp/snmpscan/fx"

// In your fx app:
snmpscanfx.Module()
```

The module requires `log.Component`, `config.Component`, `eventplatform.Component`, and `ipc.HTTPClient` (used to resolve device configs from the running agent during RC-triggered scans).

In addition to `snmpscan.Component`, it provides an `rctypes.TaskListenerProvider` that is automatically picked up by the RC client.

## Usage

**Scheduled periodic scans** (via `comp/snmpscanmanager`):

```go
import snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"

type Requires struct {
    fx.In
    Scanner snmpscan.Component
}

err := c.scanner.ScanDeviceAndSendData(ctx, snmpConfig, namespace, snmpscan.ScanParams{
    ScanType:     metadata.DefaultScan,
    CallInterval: snmpCallInterval,
})
```

**Interactive CLI walk** (via `cmd/agent/subcommands/snmp`):

```go
conn, _ := snmpparse.NewSNMP(params, log)
conn.Connect()
scanner.RunSnmpWalk(conn, "1.3.6")
```

**Key callers:**

- `comp/snmpscanmanager/impl` — the scan manager schedules periodic device scans and calls `ScanDeviceAndSendData` with `ScanType: DefaultScan`.
- `cmd/agent/subcommands/snmp` — the `snmp walk` CLI subcommand calls `RunSnmpWalk` for interactive diagnostics.
- `pkg/collector/corechecks/snmp` — the SNMP check's autodiscovery integration can trigger scans via the RC task mechanism.

## Notes

- `RunSnmpWalk` writes directly to stdout and is intended for interactive use only; it should not be called in production data paths.
- The `gatherPDUs` walk skips sibling rows of the same table OID (via `SkipOIDRowsNaive`) to avoid sending enormous payloads for large tables. Only one representative row per table is included in device scan results.
- `MaxCallCount: 0` disables the call limit entirely. Use a non-zero value when scanning potentially very large devices to avoid overloading them.
- The mock (`comp/snmpscan/mock`) is provided for unit tests in `comp/snmpscanmanager` and other callers.
- Scan-status events (InProgress, Completed, Error) are sent as part of `EventTypeNetworkDevicesMetadata` payloads, sharing the same NDM intake pipeline as device metadata from the SNMP check.

## Related components

| Component / Package | Relationship |
|---|---|
| [`comp/snmpscanmanager`](snmpscanmanager.md) | Orchestrates when `ScanDeviceAndSendData` is called. The scan manager maintains a persistent scan-history cache, a bounded worker pool, and a retry scheduler with exponential back-off. `snmpscan` is the low-level execution engine; `snmpscanmanager` is the policy layer that decides which devices to scan and when. |
| [`comp/forwarder/eventplatform`](forwarder/eventplatform.md) | `ScanDeviceAndSendData` sends batched `DeviceOID` records and scan-status events as `EventTypeNetworkDevicesMetadata` payloads via this forwarder. The `BatchStrategy` pipeline batches records up to `PayloadMetadataBatchSize` (100) before forwarding. |
| [`pkg/snmp`](../pkg/snmp.md) | `ScanDeviceAndSendData` builds the SNMP session using `snmpparse.NewSNMP` from this package. RC-triggered scans use `snmpparse.GetParamsFromAgent` to resolve device credentials from the running agent over IPC. The walk itself uses `gosnmplib.ConditionalWalk` and `gosnmplib.SkipOIDRowsNaive` for row-skipping. |
| [`pkg/networkdevice`](../pkg/networkdevice/networkdevice.md) | Defines the payload types used by the scanner: `metadata.DeviceOID`, `metadata.NetworkDevicesMetadata`, `metadata.ScanType`, `metadata.ScanStatus`, and `metadata.PayloadMetadataBatchSize`. These types encode the wire format sent to the NDM intake. |
