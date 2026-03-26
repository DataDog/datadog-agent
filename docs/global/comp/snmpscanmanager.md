> **TL;DR:** `comp/snmpscanmanager` orchestrates periodic SNMP device profile scans, managing a bounded worker pool, deduplication, persistent scan history, and exponential back-off retries triggered by the SNMP discovery subsystem.

# comp/snmpscanmanager

## Purpose

The `snmpscanmanager` component manages SNMP "default scans" — full device profile scans that are triggered automatically when a new SNMP device is discovered. It serialises and throttles scan work across a bounded worker pool, persists results to survive agent restarts, and schedules re-scans (refreshes and retries with exponential back-off).

Relevant configuration key: `network_devices.default_scan.enabled`. When `false` all `RequestScan` calls are silently dropped.

## Key elements

### Key interfaces

| Symbol | Description |
|--------|-------------|
| `Component` | Public interface: single method `RequestScan(req ScanRequest, forceQueue bool)`. |
| `ScanRequest` | Value type carrying `DeviceIP string`. |

### Key types

| Type | Description |
|------|-------------|
| `snmpScanManagerImpl` | Main struct. Owns a `scanQueue` channel, an `allRequestedIPs` set (deduplication), a `deviceScans` map (per-IP scan history), a `scanScheduler`, and a `snmpConfigProvider`. Protected by `sync.Mutex`. |
| `deviceScan` | Per-device scan record: `DeviceIP`, `ScanStatus` (`"success"` / `"failed"`), `ScanEndTs`, `Failures` count. Serialised to JSON in the persistent cache under key `"snmp_scanned_devices"`. |
| `scanScheduler` / `scanSchedulerImpl` | Min-heap priority queue of `scanTask` entries sorted by `nextScanTs`. Exposes `QueueScanTask` and `PopDueScans(now)`. |
| `snmpConfigProvider` | Thin interface wrapping `snmpparse.GetParamsFromAgent`; separated to allow mocking in tests. |

### Key functions

| Function | Description |
|----------|-------------|
| `NewComponent(reqs Requires) (Provides, error)` | Constructor. Loads the persistent cache, registers `OnStart`/`OnStop` hooks, and provides `status.InformationProvider` and `flare.Provider` outputs. |
| `RequestScan(req, forceQueue)` | Thread-safe. Drops if disabled or already queued (unless `forceQueue`). Non-blocking channel send; drops with warning if queue is full. |
| `scanWorker()` | Worker goroutine (2 instances). Pulls from `scanQueue`, calls `processScanRequest`, updates `deviceScans`. |
| `scanSchedulerWorker()` | Single goroutine; wakes every 10 minutes, calls `scanScheduler.PopDueScans`, re-queues due scans with `forceQueue=true`. |
| `processScanRequest(req)` | Resolves SNMP config via `snmpConfigProvider`, delegates to `snmpscan.Component.ScanDeviceAndSendData`, then calls `onDeviceScanSuccess` or `onDeviceScanFailure`. |
| `loadCache()` / `writeCache()` | Persist `deviceScans` via `pkg/persistentcache`. Called at startup and after every scan result. |

### Configuration and build flags

| Constant | Value | Meaning |
|----------|-------|---------|
| `scanWorkers` | 2 | Maximum concurrent scan goroutines. |
| `scanQueueSize` | 10 000 | Bounded channel buffer; excess requests are dropped with a warning. |
| `snmpCallsPerSecond` | 8 | SNMP call rate limit passed to the scanner. |
| `maxSnmpCallCount` | 100 000 | Maximum SNMP calls per single scan. |
| `scanRefreshInterval` | 182 days | How long before a successfully scanned device is rescanned. |
| `scanRefreshJitter` | ±2 weeks | Random jitter applied to `scanRefreshInterval`. |

The feature is enabled with `network_devices.default_scan.enabled: true`.

Retry delays after a failed scan: 1 h → 12 h → 1 day → 3 days → 1 week (up to 5 attempts). Only `gosnmplib.ConnectionError` failures are retried; other errors are permanent.

### `comp/snmpscanmanager/mock`

Provides `SnmpScanManagerMock` (testify mock) implementing `Component`. Use `mock.NewMockSnmpScanManager()` in tests.

### `comp/snmpscanmanager/fx`

Wraps `NewComponent` in an fx module via `fxutil.ProvideComponentConstructor`.

## Usage

The component is instantiated in the main agent (`cmd/agent/subcommands/run/command.go`) and on Windows (`command_windows.go`). The primary caller is the SNMP discovery subsystem:

```go
// pkg/collector/corechecks/snmp/internal/discovery/discovery.go
d.scanManager.RequestScan(snmpscanmanager.ScanRequest{DeviceIP: deviceIP}, false)
```

Discovery calls `RequestScan` whenever it encounters a new device IP. The manager handles deduplication, rate limiting, persistence, and retry scheduling transparently.

Status information is surfaced in the agent status page and flare via the registered `status.InformationProvider` and `flare.Provider`.

### Scan lifecycle

```
pkg/collector/corechecks/snmp (discovery)
  │  RequestScan({DeviceIP})
  ▼
comp/snmpscanmanager
  │  deduplication via allRequestedIPs set
  │  non-blocking send to scanQueue channel (capacity 10 000)
  ▼
scanWorker goroutine (×2)
  │  snmpConfigProvider.GetParamsFromAgent(deviceIP)
  │       → snmpparse.GetParamsFromAgent (queries running agent over IPC)
  │  comp/snmpscan.ScanDeviceAndSendData(ctx, snmpConfig, namespace, ScanParams)
  ▼
comp/snmpscan
  │  walks full OID tree; batches DeviceOID records
  │  SendEventPlatformEvent(msg, EventTypeNetworkDevicesMetadata)
  ▼
comp/forwarder/eventplatform → ndm-intake.

  (on failure) ←── exponential back-off retry via scanScheduler
                    1 h → 12 h → 1 day → 3 days → 1 week (up to 5 attempts)
                    only gosnmplib.ConnectionError triggers retry
```

`scanSchedulerWorker` wakes every 10 minutes to re-queue devices whose `nextScanTs` has passed (either a retry or a `scanRefreshInterval` refresh after 182 days ± 2-week jitter). The per-device `deviceScan` record is persisted to the agent's `persistentcache` after every scan outcome and reloaded on startup so history survives agent restarts.

## Related components and packages

| Component / Package | Relationship |
|---|---|
| [`comp/snmpscan`](snmpscan.md) | The low-level execution engine. `snmpscanmanager` is the policy layer (when to scan, retries, throttling); `snmpscan.ScanDeviceAndSendData` is what does the actual OID walk and forwards results. The scan manager passes `ScanParams{ScanType: DefaultScan, CallInterval: snmpCallInterval, MaxCallCount: 100 000}` to `snmpscan`. |
| [`pkg/snmp`](../pkg/snmp.md) | `snmpConfigProvider` calls `snmpparse.GetParamsFromAgent` (from `pkg/snmp/snmpparse`) to resolve device credentials and namespace from the running agent over IPC before each scan. `ListenerConfig` subnets from `pkg/snmp` also inform which IPs are candidates for scanning. |
| [`pkg/networkdevice`](../pkg/networkdevice/networkdevice.md) | Scan results are sent as `EventTypeNetworkDevicesMetadata` payloads. The payload types (`metadata.DeviceOID`, `metadata.ScanStatus`, `metadata.ScanType`, `metadata.PayloadMetadataBatchSize`) are all defined in `pkg/networkdevice/metadata`. |
