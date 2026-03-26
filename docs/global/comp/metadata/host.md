> **TL;DR:** Periodically generates and ships the "v5" host metadata payload (CPU, memory, cloud provider, tags, install method) to the Datadog `/intake` endpoint, establishing each host's identity in the backend.

# comp/metadata/host — Host Metadata Payload Component

**Import path:** `github.com/DataDog/datadog-agent/comp/metadata/host`
**Team:** agent-configuration
**Importers:** ~21 packages

## Purpose

`comp/metadata/host` generates and periodically ships the "v5" host metadata payload — the comprehensive snapshot sent to the Datadog backend that establishes a host's identity and characteristics. This is the payload received at the `/intake` endpoint and historically called the "v5" payload.

The payload aggregates information from several sources:
- System information via **gohai** (CPU, memory, network, platform)
- Cloud provider metadata (AWS EC2, GCP, Azure, ECS Fargate)
- Agent configuration and installation details
- Container environment metadata
- Host tags (configured and integration-sourced)

The component self-registers with the metadata runner so that collection happens automatically at a configurable interval (default: every 30 minutes, with an early run at 5 minutes after startup using exponential backoff).

## Package layout

| Package | Role |
|---|---|
| `comp/metadata/host` | Component interface (`Component`) |
| `comp/metadata/host/hostimpl` | Implementation (`host` struct), fx `Module()`, HTTP endpoints |
| `comp/metadata/host/hostimpl/utils` | Payload data types (`Payload`, `Meta`, etc.) and collection helpers |
| `comp/metadata/host/hostimpl/hosttags` | Host tag collection logic |

## Key elements

### Key interfaces

```go
type Component interface {
    // GetPayloadAsJSON returns the current host metadata payload serialized as
    // indented JSON. Useful for CLI inspection and flare inclusion.
    GetPayloadAsJSON(ctx context.Context) ([]byte, error)
}
```

The component exposes no mutation methods. Its primary job is to collect and send the payload on a schedule managed by `comp/metadata/runner`.

### Key types

#### `hostimpl.Payload`

Top-level structure sent to the backend. Composes `utils.CommonPayload` (API key, agent version, UUID, hostname) with `utils.Payload`:

```go
type Payload struct {
    utils.CommonPayload           // apiKey, agentVersion, uuid, internalHostname
    utils.Payload                 // os, systemStats, meta, host-tags, network, logs, …
    ResourcesPayload interface{}  `json:"resources,omitempty"`
    GohaiPayload     string       `json:"gohai"`
}
```

#### `utils.Payload`

Rich host details:

| Field | Description |
|---|---|
| `Os` | Operating system name |
| `SystemStats` | CPU cores, Python version, OS version strings |
| `Meta` | `Meta` struct (see below) |
| `HostTags` | Tags from `hosttags.Tags` |
| `NetworkMeta` | Network ID and public IPv4 |
| `LogsMeta` | Active logs transport, auto-multiline status |
| `InstallMethod` | Tool and installer version used to install the agent |
| `ProxyMeta` | `no_proxy_nonexact_match` configuration |
| `OtlpMeta` | Whether the OTLP pipeline is enabled |
| `FipsMode` | Whether the agent is running in FIPS mode |

#### `utils.Meta`

Host identity fields nested under `"meta"` in the payload:

| Field | Description |
|---|---|
| `SocketHostname` | OS-reported hostname (`os.Hostname()`) |
| `SocketFqdn` | FQDN derived from socket hostname |
| `EC2Hostname` | Hostname returned by the EC2 metadata service |
| `InstanceID` | EC2 instance ID |
| `HostAliases` | Additional aliases from cloud providers |
| `Timezones` | Host timezone |
| `ClusterName` | Kubernetes cluster name (if applicable) |
| `CanonicalCloudResourceID` | CCRID for cross-product host linking |

### Configuration and build flags

| Key | Default | Description |
|---|---|---|
| `enable_gohai` | `true` | Include gohai hardware/platform data in the payload |
| `metadata_providers[name=host].interval` | `1800` s | Main collection interval |
| `metadata_providers[name=host].early_interval` | `300` s | Initial fast collection interval |

## fx wiring

The component is provided by `hostimpl.Module()`, which is included in `metadata.Bundle()`. In addition to the `Component` itself, the constructor provides several fx outputs:

| Output | Description |
|---|---|
| `runnerimpl.Provider` | Registers the collection callback with the metadata runner |
| `flaretypes.Provider` | Adds `metadata/host.json` to agent flares |
| `status.HeaderInformationProvider` | Contributes hostname and version to the agent status header |
| `api.AgentEndpointProvider` (`GET /metadata/v5`) | Returns the current payload as scrubbed JSON |
| `api.AgentEndpointProvider` (`GET /metadata/gohai`) | Returns the raw gohai payload |

## Collection schedule

The component uses an exponential backoff policy for its collection interval:

- **Initial interval:** `earlyInterval` (default 5 minutes)
- **Multiplier:** 3×
- **Max interval:** `collectInterval` (default 30 minutes, configurable via `metadata_providers[name=host].interval`)
- **Accepted range:** 60 seconds – 4 hours

Both intervals can be overridden in `datadog.yaml` via the `metadata_providers` list:

```yaml
metadata_providers:
  - name: host
    interval: 1800        # seconds, main interval
    early_interval: 300   # seconds, initial interval
```

## Usage

Most code does not call `GetPayloadAsJSON` directly. The component is typically depended upon to ensure the host metadata is sent. For CLI inspection, the `agent metadata v5` subcommand calls `GetPayloadAsJSON` directly.

```go
// Depend on host.Component to ensure host metadata is collected
type dependencies struct {
    fx.In
    Host host.Component
}
```

To inspect the payload manually:

```bash
datadog-agent metadata v5
# or via HTTP (agent must be running):
curl -s http://localhost:5001/metadata/v5
```

## Related packages

| Package | Relationship |
|---------|-------------|
| [`pkg/gohai`](../../pkg/gohai.md) | Primary data source for CPU, memory, network, filesystem, platform, and process metadata. `comp/metadata/host/hostimpl/payload.go` calls `gohai.GetPayloadAsString` (when `enable_gohai: true`) to embed a double-encoded JSON string into `GohaiPayload`. The standalone module design means gohai can also be used by other binaries independently. |
| [`comp/metadata/runner`](runner.md) | Scheduling backbone. The `host` component registers its `collect` callback as a `runnerimpl.Provider` via `runnerimpl.NewProvider`. The runner drives the callback at the exponential-backoff interval (5 min → 30 min). Without the runner, no periodic host metadata would be sent. |
| [`comp/metadata/inventoryhost`](inventoryhost.md) | A sibling metadata component that sends the structured `host_metadata` inventory payload (to `/api/v2/host_metadata`). `inventoryhost` also uses `pkg/gohai` (individual `cpu.CollectInfo()` fields) but targets the modern inventory system rather than the legacy v5 intake. The two components cover overlapping hardware data with different schemas and endpoints. |
