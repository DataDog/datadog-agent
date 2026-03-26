> **TL;DR:** Builds and periodically sends the `check_metadata` inventory payload, recording per-instance check metadata, log source configuration, and on-disk config file hashes to populate the Checks tab of the Datadog Infrastructure inventory UI.

# comp/metadata/inventorychecks — Checks Inventory Payload Component

**Import path:** `github.com/DataDog/datadog-agent/comp/metadata/inventorychecks`
**Team:** agent-configuration
**Importers:** ~6 packages

## Purpose

`comp/metadata/inventorychecks` builds and periodically sends the `check_metadata` inventory payload. This payload is displayed in the **Checks** tab of the Datadog Infrastructure inventory UI and powers the check configuration visibility feature.

For every check instance currently scheduled by the collector, the payload records:
- Check name and instance ID
- Check version and source type
- Init config and instance config (scrubbed, when `inventories_checks_configuration_enabled` is true)
- Any extra metadata set by the check itself via `Set`

In addition to check metadata, the payload includes:
- **Logs metadata** — configuration and status of every active log source
- **Files metadata** — hash and raw content of check configuration files on disk

The component registers itself as a collector event listener and triggers a refresh whenever a check is added or removed.

> **Note:** The component's TODO comment acknowledges that this metadata provider logically belongs inside the collector component. The current design bridges the gap by exposing `inventorychecks.Component` via a global check context (`pkg/collector/check/context.go`) so that Go and Python checks can submit metadata without being fx components themselves.

## Package layout

| Package | Role |
|---|---|
| `comp/metadata/inventorychecks` | Component interface (`Component`) |
| `comp/metadata/inventorychecks/inventorychecksimpl` | Implementation (`inventorychecksImpl` struct), `Payload` type, fx `Module()` |

## Key elements

### Key interfaces

```go
type Component interface {
    // Set stores a metadata key/value pair for a specific check instance.
    // Triggers a refresh if the value changed.
    Set(instanceID string, key string, value interface{})

    // GetInstanceMetadata returns a copy of the metadata map for a single
    // check instance. Returns an empty map if the instance is not found.
    GetInstanceMetadata(instanceID string) map[string]interface{}

    // Refresh schedules an out-of-cycle payload send, respecting
    // inventories_min_interval.
    Refresh()
}
```

### Key types

#### `inventorychecksimpl.Payload`

The JSON structure sent to the backend:

```go
type Payload struct {
    Hostname      string                `json:"hostname"`
    Timestamp     int64                 `json:"timestamp"`
    Metadata      map[string][]metadata `json:"check_metadata"`
    LogsMetadata  map[string][]metadata `json:"logs_metadata"`
    FilesMetadata metadata              `json:"files_metadata"`
    UUID          string                `json:"uuid"`
}
```

- `check_metadata` — keyed by check name; each value is a slice of per-instance metadata maps (one entry per scheduled instance of that check).
- `logs_metadata` — keyed by log source name; contains config, status, service, source, tags, etc.
- `files_metadata` — keyed by filename; contains `raw_config` and `hash` fields.

#### Per-instance metadata map

Each entry in `check_metadata[checkName]` is built from `check.GetMetadata(c, withConfigs)` merged with any values submitted via `Set`. When `inventories_checks_configuration_enabled` is true, `init_config` and `instance_config` YAML strings are included (pre-scrubbed by the check infrastructure before storage).

#### `util.InventoryPayload` (embedded)

The implementation embeds `comp/metadata/internal/util.InventoryPayload`, which provides:
- `Refresh()` — schedule an out-of-cycle send
- `GetAsJSON()` — return current payload as scrubbed JSON
- `MetadataProvider()` — returns a `runnerimpl.Provider`
- `FlareProvider()` — adds `metadata/inventory/checks.json` to flares

### Configuration and build flags

| Key | Default | Description |
|---|---|---|
| `inventories_enabled` | `true` | Master switch for all inventory metadata |
| `inventories_checks_configuration_enabled` | `true` | Include init/instance config in check metadata and log source metadata |
| `inventories_min_interval` | `60s` | Minimum time between two payload submissions |
| `inventories_max_interval` | `600s` | Maximum time between submissions |
| `inventories_first_run_delay` | `60s` | Delay before the first send after startup |

## fx wiring

Provided by `inventorychecksimpl.Module()`, included in `metadata.Bundle()`. Constructor outputs:

| Output | Description |
|---|---|
| `inventorychecks.Component` | The component itself |
| `runnerimpl.Provider` | Periodic collection callback registered with the runner |
| `flaretypes.Provider` | Adds `metadata/inventory/checks.json` to flares |
| `api.AgentEndpointProvider` (`GET /metadata/inventory-checks`) | Returns current payload as scrubbed JSON |

Dependencies injected via fx:
- `collector.Component` (optional) — used to enumerate running checks
- `logagent.Component` (optional) — used to enumerate active log sources
- `hostnameinterface.Component`, `config.Component`, `serializer.MetricSerializer`

## Check access pattern

Because checks are not fx components, they access `inventorychecks` through a global check context:

```go
// In pkg/collector/check/context.go — set once at agent startup
check.InitializeInventoryChecksContext(ic inventorychecks.Component)

// Inside any check (Go or Python, via cgo bridge)
ic, err := check.GetInventoryChecksContext()
if err == nil {
    ic.Set(string(c.ID()), "version", checkVersion)
}
```

Python checks reach this via the `datadog_checks_base` Python binding which calls `SetCheckMetadata` through the cgo bridge.

## Configuration

| Key | Default | Description |
|---|---|---|
| `inventories_enabled` | `true` | Master switch for all inventory metadata |
| `inventories_checks_configuration_enabled` | `true` | Include init/instance config in check metadata and log source metadata |
| `inventories_min_interval` | `60s` | Minimum time between two payload submissions |
| `inventories_max_interval` | `600s` | Maximum time between submissions (forces a send) |
| `inventories_first_run_delay` | `60s` | Delay before the first send after startup |

## Usage

### From a Go check

```go
import (
    "github.com/DataDog/datadog-agent/pkg/collector/check"
)

func (c *MyCheck) Run() error {
    ic, err := check.GetInventoryChecksContext()
    if err == nil {
        ic.Set(string(c.ID()), "version", c.version)
        ic.Set(string(c.ID()), "flavor", "go")
    }
    // ...
}
```

### Triggering an immediate refresh

```go
// From pkg/cli/subcommands/check/command.go — force a refresh after running
// a single check so the result is reflected in the next payload
ic.Refresh()
```

## Related packages

| Package | Relationship |
|---------|-------------|
| [`comp/metadata/runner`](runner.md) | Scheduling backbone. `inventorychecks` registers its `collect` callback as a `runnerimpl.Provider` via `util.InventoryPayload.MetadataProvider()`. The runner drives the callback at up to `inventories_max_interval`. `Refresh()` (triggered by check add/remove or explicit call) can schedule an earlier send, but never faster than `inventories_min_interval`. |
| [`comp/metadata/inventoryagent`](inventoryagent.md) | Sibling inventory component. Both embed `util.InventoryPayload` and share the same `inventories_*` configuration namespace. `inventoryagent` covers agent-wide feature flags and configuration layers; `inventorychecks` covers per-check-instance metadata. They are submitted to different backend endpoints (`/api/v2/agents` vs `/api/v2/check_run`). |
| [`pkg/collector/check`](../../pkg/collector/check.md) | Defines the `Info` interface and `GetMetadata(c Info, includeConfig bool)` helper that `inventorychecks` calls to build each per-instance metadata map. Also defines `InitializeInventoryChecksContext` / `GetInventoryChecksContext`, the package-level singleton bridge that lets Go and Python checks call `inventorychecks.Set` without being fx components. |
