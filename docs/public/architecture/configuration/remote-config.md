# Remote configuration

-----

Remote configuration (RC) lets the Datadog backend push configuration files to Agents and tracer SDKs at runtime: APM sampling rates, CWS security policies, fleet-managed Agent settings, dynamic instrumentation probes, and a few dozen other products. The core Agent runs a single RC *service* that polls the backend, cryptographically verifies everything it receives through a dual-repository Uptane/TUF scheme, and caches the verified state in a local BoltDB file. Every consumer — other Agent processes, components inside the core Agent itself, and external tracer SDKs — gets its configuration from that service over the authenticated gRPC API, never from the backend directly. This page covers the service, the client library, the security model, the product catalog, and the failure modes; how RC-delivered values slot into the config source hierarchy is covered in [the configuration system](overview.md).

```text
                        Datadog RC backend (https://config.<site>)
                          /api/v0.1/configurations  (protobuf)
                                      │ HTTPS poll (~1m, backend-steerable)
                                      ▼
   ┌──────────────────────── core agent ────────────────────────┐
   │  CoreAgentService (pkg/config/remote/service)               │
   │    │ Uptane/TUF verify (config repo × director repo)        │
   │    ▼                                                        │
   │  BoltDB <run_path>/remote-config.db (transactional store)   │
   │    │                                                        │
   │  AgentSecure gRPC :5001  ClientGetConfigs / …HA / Subscribe │
   └────┬───────────┬─────────────┬───────────────┬─────────────┘
        │ loopback  │ gRPC        │ gRPC          │ gRPC (bidi stream)
        ▼           ▼             ▼               ▼
   rcclient     trace-agent   security-agent/  system-probe
   (AGENT_CONFIG,  │          CWS (verified    dyninst (LIVE_DEBUGGING
    AGENT_TASK,…)  │          client, CWS_*)    subscriptions)
                   ▼
             /v0.7/config on :8126 (JSON)
                   ▼
             tracer SDKs (APM_SAMPLING, ASM_*, APM_TRACING, …)
```

## Key packages

| Path | Purpose |
|---|---|
| [`pkg/config/remote/service/service.go`](<<<SRC>>>/pkg/config/remote/service/service.go) | `CoreAgentService`: backend poll loop, `ClientGetConfigs` server logic, cache bypass, org-status poller |
| [`pkg/config/remote/service/clients.go`](<<<SRC>>>/pkg/config/remote/service/clients.go) | Active-client tracking (30s TTL) and the cache-bypass rate limiter |
| [`pkg/config/remote/service/tracer_predicates.go`](<<<SRC>>>/pkg/config/remote/service/tracer_predicates.go) | Agent-side predicate matching (service/env/language/version constraints) on director-target metadata |
| [`pkg/config/remote/service/subscriptions.go`](<<<SRC>>>/pkg/config/remote/service/subscriptions.go) | Streaming config subscriptions (`CreateConfigSubscription`) |
| [`pkg/config/remote/service/util.go`](<<<SRC>>>/pkg/config/remote/service/util.go) | RC key parsing, `LatestConfigsRequest` builder, backend-driven refresh interval |
| [`pkg/config/remote/uptane/client.go`](<<<SRC>>>/pkg/config/remote/uptane/client.go) | Uptane client: two go-tuf clients, `verifyUptane()` cross-check, `verifyOrg()` org checks |
| [`pkg/config/remote/uptane/transactional_store.go`](<<<SRC>>>/pkg/config/remote/uptane/transactional_store.go) | In-memory write buffer over BoltDB; whole updates commit or roll back atomically |
| [`pkg/config/remote/uptane/util.go`](<<<SRC>>>/pkg/config/remote/uptane/util.go) | BoltDB open/recreate logic and the `AgentMetadata` cache-invalidation record |
| [`pkg/config/remote/meta/meta.go`](<<<SRC>>>/pkg/config/remote/meta/meta.go) | TUF root keys embedded per site (prod, staging, gov) |
| [`pkg/config/remote/api/http.go`](<<<SRC>>>/pkg/config/remote/api/http.go) | HTTP client to the RC backend (`/configurations`, `/org`, `/status`) and auth headers |
| [`pkg/config/remote/client/client.go`](<<<SRC>>>/pkg/config/remote/client/client.go) | The reusable RC client: gRPC poll loop, listener API, optional client-side TUF verification |
| [`pkg/config/remote/data`](<<<SRC>>>/pkg/config/remote/data) | Config path parsing (`datadog/<org_id>/<product>/<config_id>/<file>`) and product types |
| [`pkg/remoteconfig/state`](<<<SRC>>>/pkg/remoteconfig/state) | Client-side TUF state, the product catalog ([`products.go`](<<<SRC>>>/pkg/remoteconfig/state/products.go)), `AGENT_CONFIG` layer merging, `ApplyStatus` types |
| [`comp/remote-config/rcservice/impl/rcservice.go`](<<<SRC>>>/comp/remote-config/rcservice/impl/rcservice.go) | Fx component wrapping `CoreAgentService`; returns `option.None` when RC is disabled |
| [`comp/remote-config/rcservicemrf/impl/rcservicemrf.go`](<<<SRC>>>/comp/remote-config/rcservicemrf/impl/rcservicemrf.go) | Second service instance for Multi-Region Failover |
| [`comp/remote-config/rcclient/impl/rcclient.go`](<<<SRC>>>/comp/remote-config/rcclient/impl/rcclient.go) | In-agent client: `AGENT_CONFIG`, `AGENT_TASK`, `AGENT_FAILOVER` callbacks and Fx listener groups |
| [`comp/api/grpcserver/impl-agent/server.go`](<<<SRC>>>/comp/api/grpcserver/impl-agent/server.go) | Exposes the RC RPCs on the AgentSecure gRPC server |
| [`cmd/trace-agent/config/remote/config.go`](<<<SRC>>>/cmd/trace-agent/config/remote/config.go) | Trace-agent `/v0.7/config` HTTP handler proxying tracer requests to the core-agent gRPC API |
| [`pkg/config/remote/flare/flare.go`](<<<SRC>>>/pkg/config/remote/flare/flare.go) | Embeds a scrubbed copy of the RC database in flares |

## The core service

The service is created by the [`rcservice`](<<<SRC>>>/comp/remote-config/rcservice/impl/rcservice.go) component during core-agent startup. If `IsRemoteConfigEnabled` ([`pkg/config/utils/miscellaneous.go`](<<<SRC>>>/pkg/config/utils/miscellaneous.go)) returns false, the component resolves to `option.None` — no service exists, the gRPC RPCs answer `codes.Unimplemented`, and any startup failure reason is exported through the `remoteConfigStartup` expvar for the status page. Note that on GovCloud and FIPS builds (`IsFed`), RC is off unless `remote_configuration.enabled` is *explicitly* set.

`NewService` in [`service.go`](<<<SRC>>>/pkg/config/remote/service/service.go) then:

1. Resolves credentials: the main `api_key` (overridable with `remote_configuration.api_key`), an optional legacy RC key (`remote_configuration.key`, a `DDRCM_`-prefixed base32-encoded msgpack blob carrying an app key, datacenter, and org ID — parsed in [`util.go`](<<<SRC>>>/pkg/config/remote/service/util.go)), and an optional private-action-runner JWT. These become the `DD-Api-Key`, `DD-Application-Key`, and `DD-PAR-JWT` headers sent by [`api/http.go`](<<<SRC>>>/pkg/config/remote/api/http.go).
1. Resolves the backend URL: `https://config.<site>` by default, overridable with `remote_configuration.rc_dd_url`.
1. Opens the BoltDB cache at `<run_path>/remote-config.db`. `openCacheDB` in [`uptane/util.go`](<<<SRC>>>/pkg/config/remote/uptane/util.go) stores an `AgentMetadata{Version, APIKeyHash, URL}` record; if the agent version, the SHA-256 of the API key, or the URL differs from what is stored, the entire database is deleted and recreated (a deliberate cache-poisoning defense). The bbolt open uses a 1s lock timeout, producing the well-known `rc db is locked` error when two agents share a `run_path`.
1. Builds an exponential backoff policy for poll errors, capped at `remote_configuration.max_backoff_interval` (default 2m, clamped to 2–5m).

`Start()` launches two goroutines: the config poll loop, and an org-status poller that hits `/api/v0.1/status` every minute to log whether RC is enabled for the org and whether the API key carries the `Remote Configuration Read` permission (both are also exported as expvars under `remoteConfigStatus`).

### The poll loop

Each `refresh()` cycle:

1. Collects the set of *active clients* — every downstream client that called `ClientGetConfigs` within the last `remote_configuration.clients.ttl_seconds` (default 30s, [`clients.go`](<<<SRC>>>/pkg/config/remote/service/clients.go)) — and derives the union of products they requested. The backend only sends files for products someone is actually asking for.
1. Builds a protobuf `LatestConfigsRequest` (`buildLatestConfigsRequest` in [`util.go`](<<<SRC>>>/pkg/config/remote/service/util.go)): hostname, agent UUID and version, host tags, the current TUF versions the agent holds, the active clients (including their per-config `ApplyStatus` reports), an opaque `BackendClientState` blob echoed from the previous response, and the org UUID. On ECS Fargate the tags getter explicitly appends the `task_arn` tag from the tagger because it is not part of host tags — RC predicate targeting on Fargate depends on it ([`rcservice.go`](<<<SRC>>>/comp/remote-config/rcservice/impl/rcservice.go)).
1. POSTs to `/api/v0.1/configurations`. A 401 becomes a descriptive `ErrUnauthorized`; other 4XX errors hint at a proxy mangling the request; 503/504 responses are logged at WARN and only escalate to ERROR after 5 consecutive failures.
1. Feeds the response to `uptane.CoreAgentClient.Update`, which verifies and commits it (next section).
1. Adjusts the next poll delay. The default interval is 1 minute, but unless the user explicitly set `remote_configuration.refresh_interval` (minimum 5s), the backend can steer it between 1s and 1m via the `agent_refresh_interval` field in the director targets custom metadata. Errors add exponential backoff.

## Security model: Uptane over TUF

RC's trust model is borrowed from [Uptane](https://uptane.org/), the automotive software-update framework: two independent TUF repositories must agree before the agent accepts anything.

1. The **config repository** is global and signed by Datadog. It contains every published config file across all orgs, under top-level and delegated targets roles.
1. The **director repository** is org-scoped. Its `targets.json` selects the subset of files intended for *this* org and agent, and is what clients ultimately trust for content hashes.

Root keys for both repositories are embedded in the agent binary per site ([`pkg/config/remote/meta`](<<<SRC>>>/pkg/config/remote/meta/meta.go): `prod`, `staging` for `datad0g.com`, `gov` for `ddog-gov.com`; overridable for testing with `remote_configuration.config_root` and `director_root`). Root rotation is supported — all historical roots are stored, and downstream clients receive intermediate roots so they can walk the chain.

`Update` in [`uptane/client.go`](<<<SRC>>>/pkg/config/remote/uptane/client.go) stores the received target files and TUF metadata, then runs two verification passes before anything becomes visible:

1. `verifyOrg()`: the snapshot's custom `org_uuid` must match the org UUID the agent fetched from `/api/v0.1/org` (stored in BoltDB keyed by config-root version, so a root rotation can rescue an agent locked out by a bad stored UUID). If a legacy RC key is configured, the `<org_id>` embedded in every `datadog/<org_id>/...` target path must also match the key's org ID. Paths under `employee/` are exempt from the org-ID check — these are Datadog-employee-signed configs such as default CWS policies.
1. `verifyUptane()`: every file listed in director targets must exist in the config repository with identical length and hashes, and the actual file contents must match those hashes. This is the core Uptane property: the director can only *select* files that Datadog actually signed into the config repository, so a compromised director alone cannot inject content, and the config repository alone cannot target a specific org.

All writes during an update are buffered in memory by the [`transactionalStore`](<<<SRC>>>/pkg/config/remote/uptane/transactional_store.go) and committed to BoltDB in a single transaction only if verification passes; on failure the buffer is rolled back and the previous verified state remains live.

Downstream, the raw director metadata travels with every response, so clients can independently re-verify: tracer SDKs and CWS do full TUF verification starting from the embedded director root, while intra-agent consumers use `NewUnverifiedGRPCClient` and trust the authenticated local gRPC channel instead (the service already verified everything).

## Serving clients: the gRPC API

The RC RPCs live on the **AgentSecure** gRPC service — localhost `cmd_port` (default 5001), TLS, per-RPC bearer token — described in [inter-process communication](../processes/ipc.md). [`comp/api/grpcserver/impl-agent/server.go`](<<<SRC>>>/comp/api/grpcserver/impl-agent/server.go) forwards `ClientGetConfigs`, `ClientGetConfigsHA`, `GetConfigState`, `GetConfigStateHA`, `ResetConfigState`, and `CreateConfigSubscription` to the service.

`ClientGetConfigs` (in [`service.go`](<<<SRC>>>/pkg/config/remote/service/service.go)) is the workhorse. The request is validated strictly: a client must identify as exactly one of `is_agent`, `is_tracer`, or `is_updater`, and must send its TUF root version, product list, per-config apply status, and the metadata of files it already caches. The handler then:

1. Computes the director roots the client is missing since its reported root version.
1. Evaluates **tracer predicates** ([`tracer_predicates.go`](<<<SRC>>>/pkg/config/remote/service/tracer_predicates.go)) on each director target's custom metadata against the client's attributes — service, environment, language, app and tracer version (with semver constraints), runtime ID, client ID — plus a per-file `expires` timestamp. This is how one org-wide director target set fans out differently to each tracer process; coarse targeting happens server-side in the director, fine targeting happens here.
1. Diffs the matched files against the client's `cached_target_files` and returns only the missing contents, along with the raw director `targets.json` and the matched config paths.

Two behaviors are worth knowing:

1. **New-client cache bypass.** When a client ID that has not been seen within the TTL polls, the handler nudges the poll loop to fetch from the backend immediately, so a freshly started tracer gets its configs in seconds instead of waiting out the poll interval. This is bounded by a fixed-window rate limiter (`remote_configuration.clients.cache_bypass_limit`, default 5 per window, accepted range 1–10) and a 2s block TTL; excess clients simply get whatever is cached. Skips and timeouts are counted in internal telemetry ([`rctelemetryreporter`](<<<SRC>>>/comp/remote-config/rctelemetryreporter/impl/rctelemetryreporter.go)).
1. **Signature-expiration flush.** If the stored director `timestamp.json` has expired — meaning the backend has been unreachable for a long time — the response carries `ConfigStatus_CONFIG_STATUS_EXPIRED` and no configs. Clients react by flushing their cached state, a fail-safe so stale security policies or sampling rates do not persist forever. Listeners can opt out per-subscription (`SubscribeIgnoreExpiration`), which security-relevant products should not do.

`CreateConfigSubscription` is a bidirectional streaming variant used by the system-probe dynamic-instrumentation module ([`pkg/dyninst/procsubscribe/remote_config.go`](<<<SRC>>>/pkg/dyninst/procsubscribe/remote_config.go), products `LIVE_DEBUGGING` and `LIVE_DEBUGGING_SYMBOL_DB`): the subscriber tracks tracer runtime IDs, and whenever a tracked tracer polls, the files matched for it are pushed onto the stream ([`subscriptions.go`](<<<SRC>>>/pkg/config/remote/service/subscriptions.go); limits: 2 concurrent subscriptions, 16K tracked runtime IDs).

## The client library

Consumers do not speak gRPC by hand; they use `client.Client` from [`pkg/config/remote/client/client.go`](<<<SRC>>>/pkg/config/remote/client/client.go):

1. It polls `ClientGetConfigs` on an interval (5s for most in-agent clients, 1s for CWS), with backoff on errors — and at a fast 1s cadence until the very first success, because some products block startup on an initial state.
1. It maintains a `state.Repository` ([`pkg/remoteconfig/state/repository.go`](<<<SRC>>>/pkg/remoteconfig/state/repository.go)). `NewGRPCClient` builds a *verified* repository that re-runs TUF verification from the embedded director root; `NewUnverifiedGRPCClient` skips it for intra-agent use.
1. `Subscribe(product, callback)` registers a listener. The callback receives a `map[path]state.RawConfig` and an apply-state function used to report `ApplyStatus{Acknowledged|Error|Unacknowledged}` per config path; those statuses ride back to the backend in the next `LatestConfigsRequest` and power the per-host config status shown in the Datadog UI. Always report status — a config stuck at `Unacknowledged` is invisible pain for whoever ships that product.
1. On `codes.Unimplemented` — RC disabled in the core agent — the client **permanently stops** polling. Enabling RC later requires restarting dependent processes.

The maximum gRPC message size for RC responses is 110 MB, matching the backend limit.

## Products and consumers

The full product catalog lives in [`pkg/remoteconfig/state/products.go`](<<<SRC>>>/pkg/remoteconfig/state/products.go). The main consumers, by process:

| Process | Products | Consumer |
|---|---|---|
| core agent | `AGENT_CONFIG` | [`rcclient`](<<<SRC>>>/comp/remote-config/rcclient/impl/rcclient.go): merges layered fleet configs (`state.MergeRCAgentConfig`) and applies `log_level` through [runtime settings](runtime-settings.md) with source `SourceRC` |
| core agent | `AGENT_TASK` | [flare](../operations/flare.md) on demand, NDM device scans; tasks deduplicated by UUID |
| core agent | `AGENT_FAILOVER` | Multi-Region Failover toggles (metrics/logs/APM failover and the metrics allowlist), via the separate MRF client |
| core agent | `AGENT_INTEGRATIONS`, `DSM_KAFKA_ACTIONS`, `DO_QUERY_ACTIONS` | RC-scheduled integrations and actions ([autodiscovery](../checks/autodiscovery.md) providers; only `AGENT_INTEGRATIONS` is gated by `remote_configuration.agent_integrations.enabled` plus allow/block lists) |
| core agent | `AGENT_REMOTE_FLAGS`, `NDM_DEVICE_PROFILES_CUSTOM`, `BTF_DD` | [`comp/core/remoteflags`](<<<SRC>>>/comp/core/remoteflags), SNMP profile provider, eBPF BTF loader ([`pkg/ebpf/rc_btf.go`](<<<SRC>>>/pkg/ebpf/rc_btf.go)) |
| trace-agent | `APM_SAMPLING`, `AGENT_CONFIG`, `APM_SEMANTIC_CORE_DD`, `AGENT_FAILOVER` | [`pkg/trace/remoteconfighandler`](<<<SRC>>>/pkg/trace/remoteconfighandler/remote_config_handler.go): sampler target TPS, trace-agent log level, APM failover — see [trace pipeline](../pipelines/traces.md) |
| trace-agent (proxy) | `ASM`, `ASM_DD`, `ASM_DATA`, `ASM_FEATURES`, `APM_TRACING`, `LIVE_DEBUGGING`, … | Not consumed by the agent — proxied verbatim to tracer SDKs via `/v0.7/config` |
| security-agent / system-probe | `CWS_DD`, `CWS_CUSTOM`, `CWS_REMEDIATION` | TUF-verified client in [`pkg/security/rconfig/policies.go`](<<<SRC>>>/pkg/security/rconfig/policies.go), 5s debounce before policy reload — see [Workload Protection](../ebpf/cws.md) |
| system-probe | `LIVE_DEBUGGING`, `LIVE_DEBUGGING_SYMBOL_DB` | dyninst streaming subscriptions |
| cluster agent | `APM_TRACING`, `CONTAINER_AUTOSCALING_*`, `CLUSTER_AUTOSCALING_VALUES`, `K8S_ACTIONS`, `K8S_INJECTION_DD` | [admission controller](../containers/admission-controller.md) instrumentation patcher ([`rc_provider.go`](<<<SRC>>>/pkg/clusteragent/admission/patch/rc_provider.go)), [autoscaling](../containers/autoscaling.md), kube actions |
| installer daemon | `UPDATER_CATALOG_DD`, `INSTALLER_CONFIG`, `UPDATER_TASK` | [`pkg/fleet/daemon/remote_config.go`](<<<SRC>>>/pkg/fleet/daemon/remote_config.go): package catalog, fleet config policies, remote upgrade tasks — see [Fleet automation](../deployment/fleet.md) |
| private action runner | `AP_RUNNER_KEYS` | Action-platform signing keys, authenticating with a `DD-PAR-JWT` |

Components inside the core agent have two ways to subscribe: contribute an `RCListener` to the Fx value group defined in [`rcclient/types`](<<<SRC>>>/comp/remote-config/rcclient/types/types.go) (a `map[product]callback` collected by the rcclient component), or call `Subscribe` directly on a client they construct. Adding a new product requires declaring it in `products.go` first — the client validates product names.

### AGENT_CONFIG layering

`AGENT_CONFIG` is the Fleet Automation "change settings remotely" product and the only one with merge semantics: the backend ships several config *layers* plus an order file (`configuration_order`), and `MergeRCAgentConfig` in [`agent_config.go`](<<<SRC>>>/pkg/remoteconfig/state/agent_config.go) folds them by priority, honoring only non-empty fields per layer. Currently only `log_level` is supported end to end. The merged value is applied through the settings component with source `SourceRC`, which sits below CLI overrides in the [config source hierarchy](overview.md) — RC cannot override a log level set on the command line, and deleting the RC config falls back to the previous source.

## Not one service but four

The same `CoreAgentService` machinery runs in up to four places in a fleet, each with its own poll loop and its own BoltDB:

| Instance | Database | Notes |
|---|---|---|
| core agent | `<run_path>/remote-config.db` | Serves every local process over gRPC; the core agent's own rcclient dials the loopback gRPC endpoint rather than consuming in-process |
| core agent, MRF | `<run_path>/remote-config-ha.db` | [`rcservicemrf`](<<<SRC>>>/comp/remote-config/rcservicemrf/impl/rcservicemrf.go), pointed at the failover datacenter ([`multi_region_failover.*` settings](<<<SRC>>>/pkg/config/setup/multi_region_failover_settings.go)); served via `ClientGetConfigsHA` |
| cluster agent | DCA `run_path` | Own backend poll ([`command.go`](<<<SRC>>>/cmd/cluster-agent/subcommands/start/command.go)); consumed *in-process* via `rcclient.NewClient(rcService, …)`, no gRPC hop |
| installer daemon | `remote-config-installer.db` | Own service ([`run.go`](<<<SRC>>>/cmd/installer/subcommands/daemon/run.go)); its client identifies as `is_updater` and reports installed packages/experiments |

The loopback-vs-in-process distinction matters for failure analysis: the core agent's own RC consumption depends on the API server being up and on auth-token consistency, while the cluster agent's and installer's do not.

## Configuration

Defaults are set in [`common_settings.go`](<<<SRC>>>/pkg/config/setup/common_settings.go); env vars are `DD_` plus the upper-cased, underscored path.

| Setting | Default | Effect |
|---|---|---|
| `remote_configuration.enabled` | `true` | Master switch; effectively `false` on GovCloud/FIPS unless explicitly configured |
| `remote_configuration.key` | — | Legacy `DDRCM_…` RC key; enables org-ID path verification and the `DD-Application-Key` header |
| `remote_configuration.api_key`, `.rc_dd_url` | — | Override the API key / backend URL (`https://config.<site>`) |
| `remote_configuration.refresh_interval` | `0s` | `0s` means backend-driven (1s–1m); an explicit value (min 5s) disables backend steering |
| `remote_configuration.max_backoff_interval` | `2m` | Error backoff cap, clamped 2–5m |
| `remote_configuration.clients.ttl_seconds` | `30s` | Active-client window; values outside 5–60s fall back to the default |
| `remote_configuration.clients.cache_bypass_limit` | `5` | New-client backend fetches per window; values outside 1–10 fall back to the default |
| `remote_configuration.apm_sampling.enabled` | `true` | Gates `APM_SAMPLING` in the trace-agent |
| `remote_configuration.agent_config.enabled` | `true` | Gates the trace-agent's `AGENT_CONFIG` subscription; inherits an explicitly set `apm_sampling.enabled` for backward compatibility |
| `remote_configuration.apm_semantics.enabled` | `false` | Gates `APM_SEMANTIC_CORE_DD` (opt-in) |
| `remote_configuration.agent_integrations.enabled` | `false` | Gates RC-scheduled integrations, with `allow_list` / `block_list` / `allow_log_config_scheduling` |
| `remote_configuration.config_root`, `.director_root` | — | TUF root overrides for testing/self-hosted backends |
| `remote_configuration.no_tls`, `.no_tls_validation` | `false` | TLS is mandatory otherwise; `skip_ssl_validation` alone is rejected for RC |
| `remote_configuration.no_websocket_echo` | `false` | Disables the [`rcprotocoltest`](<<<SRC>>>/comp/remote-config/rcprotocoltest/impl) WebSocket connectivity canary (`/api/v0.2/echo-test`, no config data) |
| `runtime_security_config.remote_configuration.enabled` | `true` | Gates the CWS RC client ([`system_probe_cws.go`](<<<SRC>>>/pkg/config/setup/system_probe_cws.go)) |
| `run_path` | platform-specific | Location of `remote-config.db` |

## IPC and ports

| Direction | Endpoint | Protocol |
|---|---|---|
| outbound | `https://config.<site>/api/v0.1/configurations`, `/org`, `/status` | HTTPS, protobuf `LatestConfigsRequest/Response` |
| outbound | `https://config.<site>/api/v0.2/echo-test[-alpn]` | WebSocket connectivity test (ALPN `dd-rc-v1`) |
| inbound | `localhost:<cmd_port=5001>` `datadog.api.v1.AgentSecure` | gRPC over TLS with bearer token: `ClientGetConfigs[HA]`, `GetConfigState[HA]`, `ResetConfigState`, `CreateConfigSubscription` |
| inbound (trace-agent) | `POST /v0.7/config` on the trace receiver (default 8126, also UDS/named pipe) | JSON mirror of the gRPC messages, for tracer SDKs |

The trace-agent handler ([`config.go`](<<<SRC>>>/cmd/trace-agent/config/remote/config.go)) does more than proxy: it attaches container tags resolved from the request's origin and normalizes the tracer's service/env before forwarding to the core agent, then relays the response verbatim. The gRPC dialer also supports vsock addresses for VM-isolated setups ([`agent_client.go`](<<<SRC>>>/pkg/util/grpc/agent_client.go)).

## Failure modes and opt-out

1. **RC disabled** (`remote_configuration.enabled: false`): no service component is built; AgentSecure RPCs return `Unimplemented`; downstream clients shut down permanently; `agent status` shows the reason.
1. **Org not enabled or key unauthorized**: the org-status poller reports it; refresh errors are downgraded from ERROR to quieter levels after 5 occurrences so an unentitled agent does not spam logs. The API key needs the `Remote Configuration Read` permission.
1. **Backend unreachable**: exponential backoff up to the cap; previously verified configs keep being served until the director `timestamp.json` expires, after which clients receive `CONFIG_STATUS_EXPIRED` and flush.
1. **Verification failure**: the transactional store rolls the whole update back; the previous verified state stays live.
1. **State reset**: an agent upgrade, API-key change, or URL change wipes the database automatically; the hidden `agent remote-config reset` command ([`command.go`](<<<SRC>>>/cmd/agent/subcommands/remoteconfig/command.go)) calls `ResetConfigState` to recreate it at runtime, and plain `agent remote-config` dumps the current TUF state via `GetConfigState`.

For debugging, the status page section ([`rcstatus`](<<<SRC>>>/comp/remote-config/rcstatus/impl/status.go)) surfaces the `remoteConfigStatus` and `remoteConfigStartup` expvars (org enabled, key scoped, last error, startup failure reason), and flares embed a scrubbed copy of the RC database — target-file *contents are replaced by SHA-256 hashes* ([`flare.go`](<<<SRC>>>/pkg/config/remote/flare/flare.go)), so expect metadata, not payloads. See [status, health, and telemetry](../operations/introspection.md) and [flare](../operations/flare.md).

## Gotchas

1. **The RC database does not survive upgrades or credential changes.** Any change to agent version, API key, or RC URL recreates `remote-config.db` from scratch; the next poll refetches everything.
1. **Two agents sharing a `run_path`** fail with `rc db is locked. Please check if another instance of the agent is running and using the same run_path parameter` after a 1s bbolt lock timeout.
1. **`Unimplemented` kills clients forever.** A client that observes RC disabled stops polling permanently; turning RC on requires restarting the consuming processes.
1. **Director target versions have 1-second resolution**, so the poll loop enforces a floor between backend updates to avoid a second update in the same second being dropped as a TUF no-op.
1. **The refresh interval belongs to the backend by default.** Setting `remote_configuration.refresh_interval` explicitly both fixes the interval and disables the backend's `agent_refresh_interval` steering — remember this when debugging "why is my agent polling every 10 seconds".
1. **`AGENT_CONFIG` merging only honors non-empty fields per layer.** A new field added to `ConfigContent` must replicate the non-empty guard in [`agent_config.go`](<<<SRC>>>/pkg/remoteconfig/state/agent_config.go) or higher-priority empty layers will silently clear lower-priority values.
1. **The trace-agent applies `AGENT_CONFIG` log level through its local debug server**, so the feature silently degrades when `apm_config.debug.port` is `0`.
1. **`employee/` config paths bypass the org-ID check by design** — they carry Datadog-signed defaults (for example CWS policies); the org UUID check still applies via the snapshot metadata.
1. **A burst of new tracers does not stampede the backend**: only `cache_bypass_limit` bypasses per window; the rest wait up to 2s and get the cache. If a fresh tracer takes one poll interval to receive configs, check the `remoteconfig.cache_bypass_ratelimiter_skip` telemetry.
