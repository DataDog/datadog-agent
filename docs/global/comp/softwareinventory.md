# comp/softwareinventory

**Team:** windows-products
**Package:** `github.com/DataDog/datadog-agent/comp/softwareinventory`

## Purpose

The `softwareinventory` component collects and reports a snapshot of software installed on a Windows host. It queries the System Probe's `SoftwareInventoryModule` over its Unix socket, caches the results in memory, and periodically forwards a JSON payload to the Datadog backend via the Event Platform forwarder. The component is Windows-only and is disabled unless `software_inventory.enabled: true` is set in the agent configuration.

The component is useful for fleet-level visibility: it lets operators see what software is installed across their Windows hosts without running a separate inventory tool.

## Key Elements

### Component interface

`comp/softwareinventory/def/component.go`

```go
type Component interface{}
```

The public interface is intentionally empty. The component's value comes from the side-effects it registers: an HTTP endpoint, a flare provider, and a status header provider.

### `Requires` / `Provides` (impl)

| Field | Type | Description |
|---|---|---|
| `Requires.Config` | `config.Component` | Reads `software_inventory.enabled`, `.jitter`, `.interval` |
| `Requires.SysprobeConfig` | `sysprobeconfig.Component` | Reads the System Probe socket path |
| `Requires.EventPlatform` | `eventplatform.Component` | Forward payloads to the backend |
| `Requires.Hostname` | `hostnameinterface.Component` | Annotates the payload with the host name |
| `Provides.FlareProvider` | `flaretypes.Provider` | Dumps `inventorysoftware.json` into agent flares |
| `Provides.StatusHeaderProvider` | `status.HeaderInformationProvider` | Adds a "Software Inventory Metadata" section to `agent status` |
| `Provides.Endpoint` | `api.AgentEndpointProvider` | Serves current inventory at `GET /metadata/software` |

### `Payload` / `HostSoftware`

```go
type Payload struct {
    Hostname string       `json:"hostname"`
    Metadata HostSoftware `json:"host_software"`
}

type HostSoftware struct {
    Software []software.Entry `json:"software"`
}
```

`software.Entry` (defined in `pkg/inventory/software`) carries fields such as `DisplayName`, `Version`, `Publisher`, `InstallDate`, and `Source` (the registry hive or other origin).

### Collection loop

On start, the component launches a goroutine (`startSoftwareInventoryCollection`) that:

1. Retries fetching from System Probe every 10 seconds until the probe is ready (up to the `check_system_probe_startup_time` window).
2. Waits a random jitter (0 – `software_inventory.jitter` seconds, minimum 60 s) before sending the first payload.
3. Sends the payload immediately on startup and then re-fetches and re-sends on every `software_inventory.interval` tick (minimum 10 minutes).

The cached inventory is protected by a `sync.RWMutex`; reads from the HTTP endpoint or status page never block collection.

### Configuration keys

| Key | Default | Description |
|---|---|---|
| `software_inventory.enabled` | `false` | Enable the component |
| `software_inventory.jitter` | `60` | Max jitter in seconds before the first send |
| `software_inventory.interval` | `10` | Polling interval in minutes |

## Usage

The component is wired into the Windows agent binary only, in `cmd/agent/subcommands/run/command_windows.go`:

```go
import softwareinventoryfx "github.com/DataDog/datadog-agent/comp/softwareinventory/fx"
// ...
softwareinventoryfx.Module(),
```

No other component holds a direct reference to `softwareinventory.Component`. The component self-registers its HTTP endpoint and flare/status providers through `Provides`, so callers only need to include the fx module.

For testing, `comp/softwareinventory/impl/mock.go` provides a no-op mock that satisfies the `Component` interface.

### Data flow

```
pkg/inventory/software.GetSoftwareInventory()
  │  called inside System Probe's SoftwareInventoryModule HTTP handler
  │  serialised as []SoftwareInventoryWireEntry JSON
  ▼
comp/softwareinventory (via sysprobeclient.GetCheck over Unix socket)
  │  deserialises wire entries to []*software.Entry
  │  wraps in Payload{Hostname, HostSoftware{Software}}
  ▼
comp/forwarder/eventplatform
  │  SendEventPlatformEvent(msg, EventTypeSoftwareInventory)
  │  BatchStrategy → softinv-intake.
  ▼
Datadog backend (software inventory UI)
```

The component polls System Probe on startup (retrying every 10 s until `check_system_probe_startup_time` is exceeded), then re-fetches on every `software_inventory.interval` tick.  The current snapshot is also served at `GET /metadata/software` and included in agent flares (`inventorysoftware.json`).

## Related components and packages

| Component / Package | Relationship |
|---|---|
| [`pkg/inventory/software`](../../pkg/inventory.md) | Provides `GetSoftwareInventory()` and the `Entry` / `SoftwareInventoryWireEntry` types. System Probe calls `GetSoftwareInventory()` inside its `SoftwareInventoryModule`; `comp/softwareinventory` deserialises the wire format returned over the Unix socket. |
| [`pkg/system-probe`](../../pkg/system-probe.md) | Hosts the `SoftwareInventoryModule` (build tag `darwin \|\| windows`) that `comp/softwareinventory` queries. `comp/softwareinventory` uses `sysprobeconfig.Component` (a wrapper around `pkg/system-probe/config`) to locate the Unix socket, and `sysprobeclient.GetCheckClient` to issue the HTTP GET. |
| [`comp/forwarder/eventplatform`](forwarder/eventplatform.md) | Receives the `Payload` JSON as an `EventTypeSoftwareInventory` message and routes it to the `softinv-intake.` pipeline. `comp/softwareinventory` calls `epForwarder.SendEventPlatformEvent` after each successful collection cycle. |
