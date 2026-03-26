> **TL;DR:** RC client-side state machine that handles TUF/Uptane root and target verification, config file caching, apply-status tracking, and product-config delivery — used by agent sub-processes via `pkg/config/remote/client` and directly by the Go tracer.

# pkg/remoteconfig/state

This package is the RC client-side state machine. It handles TUF/Uptane root and target verification, config file caching, and apply-status tracking for any process that needs to receive Remote Configuration updates from the Datadog backend (directly or via the core agent's IPC gRPC channel).

It is consumed by both the in-process `pkg/config/remote/client` package (agent sub-processes) and directly by the Go tracer. Adding a new RC product only requires changes here.

---

## Key types

### Repository

`Repository` is the central object. Create one per RC consumer:

```go
// With TUF verification (recommended)
repo, err := state.NewRepository(embeddedRootBytes)

// Without TUF verification (internal/testing use only)
repo, err := state.NewUnverifiedRepository()
```

**Methods:**

| Method | Description |
|--------|-------------|
| `Update(Update) ([]string, error)` | Applies an update from the backend. Returns the list of product names whose configs changed. Validates TUF roots and target hashes before mutating state; rolls back on any error. |
| `GetConfigs(product) map[string]RawConfig` | Returns the current config map for a product. |
| `CurrentState() (RepositoryState, error)` | Returns the full client state (root version, targets version, cached files, per-config apply statuses, opaque backend state). Used to construct the next `ClientGetConfigsRequest`. |
| `UpdateApplyStatus(cfgPath, ApplyStatus)` | Records whether a config was successfully applied, or if an error occurred. |

Product-specific typed accessors (built on top of `GetConfigs`):
- `ASMFeaturesConfigs() map[string]ASMFeaturesConfig`
- `ASMDDConfigs() map[string]ConfigASMDD`

### Update

```go
type Update struct {
    TUFRoots      [][]byte          // ordered new root documents
    TUFTargets    []byte            // latest signed targets.json
    TargetFiles   map[string][]byte // raw config bytes by TUF path
    ClientConfigs []string          // TUF paths designated for this client
}
```

Populated from a `pbgo.ClientGetConfigsResponse` by `pkg/config/remote/client`.

### RepositoryState

```go
type RepositoryState struct {
    Configs            []ConfigState
    CachedFiles        []CachedFile
    TargetsVersion     int64
    RootsVersion       int64
    OpaqueBackendState []byte
}
```

Serialised into the `pbgo.ClientGetConfigsRequest` on every poll.

### Metadata / ConfigState / CachedFile

`Metadata` is stored per config path and tracks product, config ID, name, version, hashes, raw length, and `ApplyStatus`. `ConfigState` and `CachedFile` are outbound projections of `Metadata` used in `RepositoryState`.

### RawConfig

```go
type RawConfig struct {
    Config   []byte
    Metadata Metadata
}
```

The default representation for all products that are not parsed inline. Delivered to listeners via `OnUpdate`.

### ApplyState / ApplyStatus

```go
type ApplyState uint64
const (
    ApplyStateUnknown        ApplyState = 0
    ApplyStateUnacknowledged ApplyState = 1
    ApplyStateAcknowledged   ApplyState = 2
    ApplyStateError          ApplyState = 3
)

type ApplyStatus struct {
    State ApplyState
    Error string
}
```

Listeners must call `applyStateCallback(path, status)` after processing each config. The status is propagated back to the backend on the next poll.

---

## Key functions

| Function | Description |
|----------|-------------|
| `state.NewRepository(embeddedRootBytes)` | Create a `Repository` with TUF verification enabled. |
| `state.NewUnverifiedRepository()` | Create a `Repository` that skips TUF verification (testing/internal only). |
| `repo.Update(Update)` | Apply a backend response; validates TUF and mutates state atomically. |
| `repo.GetConfigs(product)` | Return the current config map for a product as `map[string]RawConfig`. |
| `repo.CurrentState()` | Return the full `RepositoryState` used to build the next poll request. |
| `repo.UpdateApplyStatus(cfgPath, ApplyStatus)` | Report whether a config was successfully applied. |
| `state.MergeRCAgentConfig(applyStatus, updates)` | Merge layered `AGENT_CONFIG` updates into a `ConfigContent`. |

---

## Products

All valid product identifiers are declared as `const` in `products.go` and registered in the `validProducts` map. Both maps must be kept in sync.

Selected products (full list in `products.go`):

| Constant | String value | Description |
|----------|-------------|-------------|
| `ProductAgentConfig` | `AGENT_CONFIG` | Dynamic agent configuration (e.g. log level) |
| `ProductAgentTask` | `AGENT_TASK` | One-shot tasks (e.g. trigger a flare) |
| `ProductAPMSampling` | `APM_SAMPLING` | APM head-based sampling rates |
| `ProductAPMTracing` | `APM_TRACING` | APM tracing configuration |
| `ProductCWSDD` | `CWS_DD` | Cloud Workload Security — Datadog-managed rules |
| `ProductCWSCustom` | `CWS_CUSTOM` | Cloud Workload Security — customer rules |
| `ProductASMFeatures` | `ASM_FEATURES` | ASM activation flag |
| `ProductASMDD` | `ASM_DD` | ASM WAF rules (Datadog-managed) |
| `ProductASMData` | `ASM_DATA` | ASM WAF rules data (e.g. IP blocklists) |
| `ProductLiveDebugging` | `LIVE_DEBUGGING` | Dynamic Instrumentation |
| `ProductInstallerConfig` | `INSTALLER_CONFIG` | Datadog Installer configuration |
| `ProductAgentFailover` | `AGENT_FAILOVER` | Multi-region failover configuration |
| `ProductHaAgent` | `HA_AGENT` | High-Availability Agent |
| `ProductBTFDD` | `BTF_DD` | BTF catalog for eBPF |

To add a new product: add a `const` and an entry in `validProducts`.

---

## Config path format

TUF target paths follow one of two schemas:

- `datadog/<org_id>/<product>/<config_id>/<name>` — customer-facing configs.
- `employee/<product>/<config_id>/<name>` — Datadog-internal configs.

`parseConfigPath` (internal) decodes these into a `configPath` struct. The public equivalent in `pkg/config/remote/data` is `data.ParseConfigPath`.

---

## TUF verification (tuf.go)

`tufRootsClient` wraps `github.com/DataDog/go-tuf/client.Client` to handle incremental root chain updates:

1. New roots in `Update.TUFRoots` are fed to `updateRoots`, which calls `tufClient.UpdateRoots()`.
2. The updated targets file is verified with `validateTargets`, which reconstructs the TUF `verify.DB` from the latest root's keys and validates the targets document signature.
3. Each raw config file's SHA-256 hash is checked against the TUF metadata with `validateTargetFileHash`.
4. State is only committed after all checks pass.

In unverified mode (`NewUnverifiedRepository`), steps 1–3 are skipped and files are accepted as-is. The repository starts with `latestRootVersion = 1` so the backend does not resend the initial root.

---

## AGENT_CONFIG merge logic (agent_config.go)

The `AGENT_CONFIG` product uses a layered merge model:

- One config file acts as the **order file** (config ID `configuration_order`); it lists the order in which other layers should be applied.
- `MergeRCAgentConfig(applyStatus, updates)` iterates layers in order and builds a `ConfigContent` (currently only `LogLevel`). The last layer wins.
- Internal layers (from `InternalOrder`) override customer layers.

---

## ASM typed configs (configs_asm.go)

Three ASM products are parsed inline rather than returned as `RawConfig`:

- `ASMFeaturesConfig` — `ASMFeaturesData` with `ASM.Enabled` and `APISecurity.RequestSampleRate`.
- `ConfigASMDD` — raw bytes (the WAF rules are consumed by the WAF library directly).
- `ASMDataConfig` — `ASMDataRulesData`, a list of WAF data entries used for IP blocking etc.

---

## Usage

### In sub-processes (via pkg/config/remote/client)

`pkg/config/remote/client.Client` owns a `*state.Repository` and calls `repo.Update(update)` after each successful gRPC poll. Subscribers receive `map[string]state.RawConfig` and must call the provided `applyStateCallback`.

### Direct usage (Go tracer, security agent)

The tracer and security agent create a `Repository` directly, without going through the agent gRPC channel. They call `repo.Update` and `repo.CurrentState` manually. This is the path described in the package README.

### Example: checking apply status

```go
func onUpdate(configs map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
    for path, cfg := range configs {
        if err := applyConfig(cfg.Config); err != nil {
            applyStateCallback(path, state.ApplyStatus{
                State: state.ApplyStateError,
                Error: err.Error(),
            })
            continue
        }
        applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
    }
}
```

---

## Notes for contributors

- The package is a standalone Go module (`go.mod` inside `pkg/remoteconfig/state/`). It is also used by the Go tracer (`dd-trace-go`), so changes to the public API require coordination.
- `validProducts` acts as a compile-time registry. Any product not listed here will cause `parseConfig` to return an error, preventing `Repository.Update` from succeeding.
- Config state is stored in `sync.Map` (metadata) and plain `map` (configs per product). The `Repository` is not safe to call `Update` concurrently; callers must serialize updates externally.
- `opaqueBackendState` is an opaque byte blob extracted from `targets.signed.custom.opaque_client_state`. It is sent back verbatim on the next poll and must never be modified by the client.

## Cross-references

| Topic | See also |
|-------|----------|
| Higher-level RC client used by sub-processes; owns the `Repository` | [`pkg/config/remote`](config/remote.md) |
| fx component wrapping `client.Client` for in-process use | [`comp/remote-config/rcclient`](../comp/remote-config/rcclient.md) |
| RC service that polls the backend and distributes updates | [`comp/remote-config/rcservice`](../comp/remote-config/rcservice.md) |
| Fleet daemon that subscribes to `ProductUpdaterTask` / `ProductInstallerConfig` | [`pkg/fleet`](fleet/fleet.md) |

### Where `state.Repository` lives in the stack

```
Datadog backend  (TUF-signed payloads)
    │
    ▼
pkg/config/remote/uptane.CoreAgentClient   ← full Uptane verification (BoltDB store)
    │  on every poll
    ▼
pkg/config/remote/service.CoreAgentService ← drives the uptane client, distributes via gRPC
    │  gRPC ClientGetConfigsResponse
    ▼
pkg/config/remote/client.Client            ← owns state.Repository per sub-process
    │  repo.Update(update)
    ▼
pkg/remoteconfig/state.Repository          ← TUF root chain + per-config apply-status tracking
    │  GetConfigs(product)
    ▼
Subscriber callbacks (OnUpdate)
```

The `state` package is deliberately kept lean (no agent config dependency) so it can be embedded in the Go tracer without pulling in the full agent module graph. Any new product constant added here must also be registered in `pkg/config/remote/data/product.go` for the service layer.
