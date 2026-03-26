# pkg/config/remote

Remote Configuration (RC) is the system that delivers signed, versioned configuration files from the Datadog backend to agents at runtime — without a restart. `pkg/config/remote` is the agent-side implementation of that system. It is split across sub-packages that each cover a distinct layer of the architecture.

## Architecture overview

```
Datadog RC backend
        |
        | HTTPS (protobuf)
        v
  pkg/config/remote/api      ← HTTP client wrapping /configurations, /org, /status
        |
        v
  pkg/config/remote/service  ← CoreAgentService: polls backend, drives Uptane verification,
        |                       serves downstream gRPC clients (ClientGetConfigs)
        |
        | gRPC (secure IPC channel)
        v
  pkg/config/remote/client   ← Client: used by sub-processes (trace-agent, system-probe, …)
                                to subscribe to product updates
```

TUF/Uptane metadata verification happens in `uptane/`. Embedded root certificates live in `meta/`. Config path and product types are defined in `data/`.

---

## Sub-packages

### api

**Purpose:** HTTP client that calls the Datadog RC backend.

**Key elements:**
- `API` interface — `Fetch`, `FetchOrgData`, `FetchOrgStatus`.
- `HTTPClient` — concrete implementation; constructs requests as protobuf (`pbgo.LatestConfigsRequest`) and returns `pbgo.LatestConfigsResponse`.
- Sentinel errors: `ErrUnauthorized`, `ErrProxy`, `ErrGatewayTimeout`, `ErrServiceUnavailable` — used by the service layer to decide log severity and backoff behaviour.
- Endpoints: `POST /api/v0.1/configurations`, `GET /api/v0.1/org`, `GET /api/v0.1/status`.

---

### meta

**Purpose:** Embeds the TUF root certificates for each Datadog environment so they ship inside the binary.

**Key elements:**
- Six embedded JSON files (`prod.director.json`, `prod.config.json`, `staging.*`, `gov.*`) loaded via `//go:embed`.
- `EmbeddedRoot` — wraps a raw root JSON blob and its parsed version number.
- `RootsDirector(site, override)` / `RootsConfig(site, override)` — select the correct embedded root for prod, staging (`datad0g.com`), or gov (`ddog-gov.com`/`*.ddog-gov.com`) sites, or accept a runtime override for testing.

---

### data

**Purpose:** Shared type definitions for products and config file paths.

**Key elements:**
- `Product` (string type) — the RC product identifier, e.g. `ProductAPMSampling`, `ProductAgentConfig`, `ProductCWSDD`.
- `ConfigPath` struct with `Source`, `OrgID`, `Product`, `ConfigID`, `Name` — parsed from TUF target paths of the form `datadog/<org_id>/<product>/<config_id>/<name>` or `employee/<product>/<config_id>/<name>`.
- `ParseConfigPath(path)` — decodes a TUF path into its structured fields.
- `ProductListToString` / `StringListToProduct` — conversion helpers used when serialising gRPC requests.

---

### uptane

**Purpose:** Full Uptane verification of RC payloads. Maintains two parallel TUF repositories (config and director) backed by a BoltDB transactional store.

**Key elements:**
- `CoreAgentClient` — wraps a base `Client` and adds `Update(*pbgo.LatestConfigsResponse) error`. All writes go through a transaction; failures trigger an automatic rollback.
- `Client` — holds separate `configTUFClient` / `directorTUFClient` instances (from `github.com/DataDog/go-tuf`), a `targetStore`, an `orgStore`, and a `transactionalStore` (BoltDB).
- `transactionalStore` — write-ahead in-memory layer over BoltDB; calls `commit()` only after full verification.
- `Metadata` struct — database file path, agent version, API key and URL; used to open or recreate the BoltDB file.
- `NewCoreAgentClientWithNewTransactionalStore` / `NewCoreAgentClientWithRecreatedTransactionalStore` — factory functions used by `service`.
- Options: `WithOrgIDCheck`, `WithDirectorRootOverride`, `WithConfigRootOverride`.
- Exported on `Client`: `State()`, `Targets()`, `TargetFile(path)`, `TargetFiles(paths)`, `DirectorRoot(version)`, `TUFVersionState()`, `TimestampExpires()`.

---

### service

**Purpose:** The core agent's RC service. Runs in the main agent process, polls the Datadog backend, verifies payloads with Uptane, and serves downstream sub-processes over gRPC.

**Key elements:**
- `CoreAgentService` struct — the central type. Fields of note:
  - `api` (`api.API`) — backend HTTP client.
  - `uptane` (`coreAgentUptaneClient`) — verifies each backend response.
  - `clients` — tracks connected downstream gRPC clients and their last-seen state.
  - `subscriptions` — streaming subscriptions for system-probe / other consumers via `CreateConfigSubscription`.
  - `backoffPolicy`, `refreshBypassLimiter` — exponential backoff (2–5 min) and rate limiting for cache-bypass requests.
  - `orgStatusPoller` — separate goroutine that polls `/status` every minute to log RC enablement state.
- `RcTelemetryReporter` interface — implemented by the agent to emit RC-specific metrics (rate-limit hits, timeouts, active subscriptions).
- `uptaneClient` / `coreAgentUptaneClient` interfaces — seam used in tests to substitute the Uptane client.
- Constants: `defaultRefreshInterval` (1 min), `minimalRefreshInterval` (5 s), `defaultClientsTTL` (30 s).
- gRPC endpoints served: `ClientGetConfigs` (request/response), `ClientGetConfigsHA` (MRF failover), `CreateConfigSubscription` (streaming for system-probe).

The service is wired up as an Fx component via `comp/remote-config/rcservice/rcserviceimpl`.

---

### client

**Purpose:** Lightweight RC client used inside sub-processes (trace-agent, system-probe, security-agent, etc.) to receive configuration updates from the core agent over the secure gRPC IPC channel.

**Key elements:**
- `Client` struct — poll loop (`Start()`/`Close()`), backoff (max 90 s), listener registry.
- `ConfigFetcher` interface — `ClientGetConfigs`; supplied either as a gRPC stub or any custom fetcher in tests.
- `Listener` interface — three methods components must implement:
  - `OnUpdate(map[string]state.RawConfig, applyStateCallback)` — called on every config change for subscribed products.
  - `OnStateChange(bool)` — called when RC connectivity is gained or lost (after the first successful connection).
  - `ShouldIgnoreSignatureExpiration() bool` — opt-in to continue receiving updates when TUF signatures have expired.
- Constructor variants:
  - `NewGRPCClient` — verified TUF, standard endpoint.
  - `NewUnverifiedGRPCClient` — skips TUF verification (used in internal tooling).
  - `NewUnverifiedMRFGRPCClient` — skips TUF, uses MRF HA endpoint.
- `Subscribe(product, cb)` / `SubscribeAll(product, listener)` / `SubscribeIgnoreExpiration` — register callbacks per product.
- `GetConfigs(product)` — synchronous snapshot of the current config map.
- `UpdateApplyStatus(cfgPath, ApplyStatus)` — report acknowledgement or error back to the repository.
- Option functions: `WithProducts`, `WithPollInterval`, `WithAgent`, `WithCluster`, `WithUpdater`, `WithDirectorRootOverride`, `WithoutTufVerification`.
- Listener helpers: `NewUpdateListener`, `NewListener`, `NewUpdateListenerIgnoreExpiration`.

Internally, `Client` delegates all TUF state to a `state.Repository` (from `pkg/remoteconfig/state`).

---

## Usage in the codebase

### Starting the service (core agent)

`CoreAgentService` is provided as an optional Fx component in `comp/remote-config/rcservice/rcserviceimpl`. It is started once during agent startup; the component exposes `GetConfigSubscription` and `ClientGetConfigs` gRPC methods to downstream clients.

### Subscribing from a sub-process

The typical pattern for a sub-process or component that wants RC updates:

```go
// Create a gRPC client connected to the core agent
c, err := client.NewGRPCClient(
    ipcAddress, port, authToken, tlsConfig,
    client.WithAgent("trace-agent", version.AgentVersion),
    client.WithProducts(state.ProductAPMSampling, state.ProductAgentConfig),
    client.WithPollInterval(5*time.Second),
)

// Register a callback for a specific product
c.Subscribe(state.ProductAPMSampling, func(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
    for path, cfg := range updates {
        // parse cfg.Config ([]byte)
        applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
    }
})

c.Start()
defer c.Close()
```

This pattern is used by `comp/trace/config/impl/remote.go`, `pkg/security/rconfig/policies.go`, `comp/remote-config/rcclient/rcclientimpl/rcclient.go`, and others.

### The rcclient Fx component

`comp/remote-config/rcclient/rcclientimpl` wraps a `client.Client` as an Fx component. Other components inject `types.RCListener` or `types.RCAgentTaskListener` values tagged with `group:"rCListener"` / `group:"rCAgentTaskListener"` — they are wired automatically by Fx.

---

## Notes for contributors

- **Adding a new product:** Add the constant to `pkg/remoteconfig/state/products.go` (`validProducts` map and `const` block). The `data/product.go` constants are used by the `service` layer; keep both in sync.
- **TUF root rotation:** Embedded roots in `meta/` must be updated when the backend rotates its TUF signing keys. The version is parsed at startup; a mismatch will cause `NewRepository` to fail.
- **BoltDB location:** The Uptane transactional store persists TUF metadata to a BoltDB file whose path comes from `uptane.Metadata.Path`. If the file is corrupted or the agent/API key changes, `NewCoreAgentClientWithRecreatedTransactionalStore` drops and recreates it.
- **Signature expiration:** Listeners that implement `ShouldIgnoreSignatureExpiration() → true` continue receiving updates even when TUF timestamps expire. Only use this for non-security-sensitive products.

## Cross-references

| Topic | See also |
|-------|----------|
| TUF state machine (`Repository`, `Update`, `ApplyStatus`) used by `client` | [`pkg/remoteconfig`](../remoteconfig.md) |
| fx component wrapping `client.Client`; handles `AGENT_CONFIG` / `AGENT_TASK` built-ins | [`comp/remote-config/rcclient`](../../comp/remote-config/rcclient.md) |
| fx component wrapping `service.CoreAgentService` | [`comp/remote-config/rcservice`](../../comp/remote-config/rcservice.md) |
| Main agent config system; `SourceRC` / `SourceFleetPolicies` are applied on top of it | [`pkg/config`](config.md) |
| Fleet daemon that creates a `client.Client` via `pkg/config/remote` | [`pkg/fleet`](../fleet/fleet.md) |

### Component interaction diagram

```
┌─────────────────────────────────────────────┐
│           core agent process                │
│                                             │
│  pkg/config/remote/service                  │
│  (CoreAgentService)                         │
│    ├─ api.HTTPClient → Datadog RC backend   │
│    ├─ uptane.CoreAgentClient (BoltDB)        │
│    └─ gRPC server  ──────────────────────── │─── IPC ──► trace-agent
│                                             │            system-probe
│  comp/remote-config/rcservice               │            security-agent
│  (fx wrapper, optional)                     │
│                                             │
│  comp/remote-config/rcclient                │
│  (fx component, uses client.Client)         │
│    ├─ built-in: AGENT_CONFIG log_level       │
│    ├─ built-in: AGENT_TASK dispatch          │
│    └─ fx-group: RCListener / TaskListener   │
│                                             │
│  pkg/fleet/daemon.remoteConfig              │
│  (also uses client.Client directly)         │
│    ├─ ProductUpdaterTask                    │
│    ├─ ProductUpdaterCatalogDD               │
│    └─ ProductInstallerConfig                │
└─────────────────────────────────────────────┘
```

Both `rcclientimpl` and the fleet daemon create their own `client.Client` instances connected to the same `rcservice` gRPC endpoint. The `client.Client` itself delegates all TUF verification to `pkg/remoteconfig/state.Repository`.
