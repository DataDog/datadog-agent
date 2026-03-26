# comp/metadata/inventoryhost — Host Inventory Payload Component

**Import path:** `github.com/DataDog/datadog-agent/comp/metadata/inventoryhost`
**Team:** agent-configuration

## Purpose

`comp/metadata/inventoryhost` builds and periodically sends the `host_metadata` inventory payload. This payload populates the **Infrastructure** inventory in Datadog with detailed, structured hardware and OS information about each host — distinct from the legacy "v5" payload produced by `comp/metadata/host`.

The component collects data entirely at payload-generation time (no caching between runs) from:
- **gohai** — CPU, memory, network, platform (kernel, OS, architecture)
- **DMI/SMBIOS** — hypervisor guest UUID, product UUID, board asset tag and vendor
- **Cloud provider detection** — provider name, source, account ID, host ID, CCRID, instance type
- **Package signing** — Linux GPG signing policies (global and RPM repo checks)
- Agent version

Unlike `inventoryagent`, this component has no `Set` method. Its data is fully derived from the system at each collection cycle.

## Package layout

| Package | Role |
|---|---|
| `comp/metadata/inventoryhost` | Component interface (`Component`) |
| `comp/metadata/inventoryhost/inventoryhostimpl` | Implementation (`invHost` struct), `Payload` and `hostMetadata` types, fx `Module()` |

## Component interface

```go
type Component interface {
    // Refresh schedules an out-of-cycle payload send,
    // respecting inventories_min_interval.
    Refresh()
}
```

`Refresh` is the only public method. Call it when you know host information has changed (e.g., after a cloud provider detection update) and want to avoid waiting for the next scheduled collection.

## Key types

### `inventoryhostimpl.Payload`

Top-level JSON structure sent to the backend:

```go
type Payload struct {
    Hostname  string        `json:"hostname"`
    Timestamp int64         `json:"timestamp"`
    Metadata  *hostMetadata `json:"host_metadata"`
    UUID      string        `json:"uuid"`
}
```

### `inventoryhostimpl.hostMetadata`

Flat struct covering all host dimensions:

| Field group | Fields |
|---|---|
| CPU (gohai) | `cpu_cores`, `cpu_logical_processors`, `cpu_vendor`, `cpu_model`, `cpu_model_id`, `cpu_family`, `cpu_stepping`, `cpu_frequency`, `cpu_cache_size` |
| Platform (gohai) | `kernel_name`, `kernel_release`, `kernel_version`, `os`, `cpu_architecture` |
| Memory (gohai) | `memory_total_kb`, `memory_swap_total_kb` |
| Network (gohai) | `ip_address`, `ipv6_address`, `mac_address`, `interfaces` |
| Agent | `agent_version` |
| Cloud | `cloud_provider`, `cloud_provider_source`, `cloud_provider_account_id`, `cloud_provider_host_id`, `ccrid`, `instance-type` |
| OS | `os_version` |
| DMI/SMBIOS | `hypervisor_guest_uuid`, `dmi_product_uuid`, `dmi_board_asset_tag`, `dmi_board_vendor` |
| Package signing | `linux_package_signing_enabled`, `rpm_global_repo_gpg_check_enabled` |

When `metadata_ip_resolution_from_hostname` is enabled, the primary IP addresses are resolved by DNS lookup on the agent hostname rather than taken from the network interface.

### `util.InventoryPayload` (embedded)

The implementation embeds `comp/metadata/internal/util.InventoryPayload`, providing:
- `Refresh()` — schedule an out-of-cycle send
- `GetAsJSON()` — return current payload as scrubbed JSON
- `MetadataProvider()` — returns a `runnerimpl.Provider`
- `FlareProvider()` — adds `metadata/inventory/host.json` to flares

## fx wiring

Provided by `inventoryhostimpl.Module()`, included in `metadata.Bundle()`. Constructor outputs:

| Output | Description |
|---|---|
| `inventoryhost.Component` | The component itself |
| `runnerimpl.Provider` | Periodic collection callback registered with the runner |
| `flaretypes.Provider` | Adds `metadata/inventory/host.json` to flares |
| `api.AgentEndpointProvider` (`GET /metadata/inventory-host`) | Returns current payload as scrubbed JSON |

Dependencies injected via fx:
- `config.Component`, `log.Component`, `serializer.MetricSerializer`, `hostnameinterface.Component`

## Configuration

| Key | Default | Description |
|---|---|---|
| `inventories_enabled` | `true` | Master switch for all inventory metadata |
| `inventories_collect_cloud_provider_account_id` | `true` | Include cloud provider account ID in the payload |
| `metadata_ip_resolution_from_hostname` | `false` | Resolve IP by DNS instead of network interface enumeration |
| `inventories_min_interval` | `60s` | Minimum time between two payload submissions |
| `inventories_max_interval` | `600s` | Maximum time between submissions (forces a send) |
| `inventories_first_run_delay` | `60s` | Delay before the first send after startup |

## Usage

### Triggering a refresh from another component

```go
type dependencies struct {
    fx.In
    InvHost inventoryhost.Component
}

// After detecting that the cloud provider has changed:
c.invHost.Refresh()
```

### Inspecting the payload

```bash
# Via the agent HTTP API (agent must be running):
curl -s http://localhost:5001/metadata/inventory-host | python3 -m json.tool
```

The payload file is also included in agent flares at `metadata/inventory/host.json`.

## Related packages

| Package | Relationship |
|---------|-------------|
| [`comp/metadata/runner`](runner.md) | Scheduling backbone. `inventoryhost` registers its `collect` callback as a `runnerimpl.Provider` via `util.InventoryPayload.MetadataProvider()`. The runner drives the callback at up to `inventories_max_interval`. `Refresh()` schedules an early send (e.g. after cloud-provider detection completes), subject to `inventories_min_interval`. |
| [`pkg/gohai`](../../pkg/gohai.md) | Primary data source. At each collection cycle `inventoryhost` calls the individual `cpu.CollectInfo()`, `memory.CollectInfo()`, `network.CollectInfo()`, and `platform.CollectInfo()` sub-package APIs directly (not the combined `GetPayload`/`GetPayloadAsString` used by `comp/metadata/host`). Fields are mapped 1:1 into `hostMetadata`. Partial collection failures are tolerated because gohai fields use the `utils.Value[T]` result-or-error pattern. |
| [`pkg/util/hostname`](../../pkg/util/hostname.md) | Used via `hostnameinterface.Component` (the fx-injectable wrapper around `pkg/util/hostname`) to stamp `hostname` and `uuid` on every payload. The `metadata_ip_resolution_from_hostname` config key controls whether the primary IP is resolved by DNS lookup on the agent hostname (via `pkg/util/hostname`) rather than taken from network interface enumeration in gohai. |
| [`comp/metadata/host`](host.md) | Sibling component that sends the legacy v5 host metadata payload to `/intake`. Both use gohai data, but `comp/metadata/host` calls `gohai.GetPayloadAsString` (a double-encoded JSON blob) while `inventoryhost` reads individual gohai sub-package structs to fill a typed `hostMetadata` struct sent to `/api/v2/host_metadata`. The two components cover overlapping hardware data with different schemas, endpoints, and collection schedules. |
