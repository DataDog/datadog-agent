# Forwarder and resilience

-----

The forwarder is the last hop inside the Agent before data leaves the host. It receives already-serialized payloads — it never inspects or re-encodes them — wraps each one in an `HTTPTransaction`, and POSTs it to one or more Datadog intake domains with retries, exponential backoff, per-endpoint circuit breaking, and optional spill-to-disk buffering. Everything upstream of it (the [aggregator and serializer](metrics/serialization.md), metadata providers, the process checks) delegates delivery, ordering under failure, and API key handling to this one subsystem.

The central implementation is the **default forwarder** in [`comp/forwarder/defaultforwarder`](<<<SRC>>>/comp/forwarder/defaultforwarder). Several thin wrappers instantiate additional copies of it for other products: the [orchestrator forwarder](<<<SRC>>>/comp/forwarder/orchestrator/impl/forwarder_orchestrator.go) (targets `https://orchestrator.<site>`, see [Orchestrator explorer](../containers/orchestrator.md)) and the [connections forwarder](<<<SRC>>>/comp/forwarder/connectionsforwarder/impl/connectionsforwarder.go) (NPM connections to `https://process.<site>`, see [Process and container pipeline](processes.md)). The [event platform forwarder](event-platform.md) is a separate implementation built on the logs sender machinery, and the [logs](logs.md) and [trace](traces.md) pipelines have their own senders too — this page covers the default forwarder plus the cross-product concerns it anchors: endpoint URL construction, API keys, dual shipping, proxies, FIPS, and Multi-Region Failover.

## Key packages and files

| Path | Purpose |
|---|---|
| [`comp/forwarder/defaultforwarder/README.md`](<<<SRC>>>/comp/forwarder/defaultforwarder/README.md) | In-tree overview of `DefaultForwarder`, `domainForwarder`, `Worker`, `blockedEndpoints`, and transactions |
| [`def/component.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/def/component.go) | `Forwarder` interface (`SubmitSeries`, `SubmitSketchSeries`, `SubmitProcessChecks`, ...) and the `Features` bitmask |
| [`def/params.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/def/params.go) | Fx `Params`: `WithResolvers()`, `WithDisableAPIKeyChecking()`, `WithFeatures(...)` |
| [`fx/fx.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/fx/fx.go), [`impl/forwarder.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/forwarder.go) | Fx wiring; `createOptions` derives `Options` from `utils.GetMultipleEndpoints(config)` |
| [`impl/default_forwarder.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/default_forwarder.go) | `DefaultForwarder`: one `domainForwarder` per domain, transaction fan-out, all `Submit*` methods |
| [`impl/domain_forwarder.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/domain_forwarder.go) | Per-domain worker pool, high/low priority channels, 5 s retry ticker, MRF gating |
| [`impl/worker.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/worker.go) | `Worker`: consumes the queues, per-worker request semaphore, stop semantics |
| [`impl/blocked_endpoints.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/blocked_endpoints.go) | Per-endpoint circuit breaker with exponential backoff (policy from [`pkg/util/backoff`](<<<SRC>>>/pkg/util/backoff)) |
| [`impl/forwarder_health.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/forwarder_health.go) | Periodic API key validation against `https://api.<site>/api/v1/validate` |
| [`impl/shared_connection.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/shared_connection.go) | One `*http.Client` shared by all workers of a domain, swappable for connection resets |
| [`impl/otel_sync_forwarder.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/otel_sync_forwarder.go) | `OTelSyncForwarder`: synchronous, error-propagating variant for the [OTel exporter](../otel/otlp-ingest.md) |
| [`transaction/transaction.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/transaction/transaction.go) | `HTTPTransaction` and its HTTP status handling (`internalProcess`) |
| [`endpoints/endpoints.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/endpoints/endpoints.go) | Registry of `transaction.Endpoint` routes: `/api/v2/series`, `/api/beta/sketches`, `/api/v1/collector`, ... |
| [`resolver/domain_resolver.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/resolver/domain_resolver.go) | `DomainResolver`: API key set, config-update hooks, per-endpoint overrides, MRF flag |
| [`internal/retry/`](<<<SRC>>>/comp/forwarder/defaultforwarder/internal/retry) (+ [`README.md`](<<<SRC>>>/comp/forwarder/defaultforwarder/internal/retry/README.md)) | In-memory retry queue, on-disk queue, protobuf serializer, disk usage limits, capacity telemetry |
| [`pkg/config/utils/endpoints.go`](<<<SRC>>>/pkg/config/utils/endpoints.go) | `GetMainEndpoint`, `GetMultipleEndpoints`, `AddAgentVersionToDomain`, MRF endpoint helpers |
| [`pkg/config/setup/common_settings.go`](<<<SRC>>>/pkg/config/setup/common_settings.go) | All `forwarder_*` defaults |
| [`pkg/config/setup/multi_region_failover_settings.go`](<<<SRC>>>/pkg/config/setup/multi_region_failover_settings.go) | All `multi_region_failover.*` settings |
| [`pkg/util/http/transport.go`](<<<SRC>>>/pkg/util/http/transport.go) | `CreateHTTPTransport`: TLS knobs, proxy resolution, `no_proxy` semantics |
| [`pkg/fips/`](<<<SRC>>>/pkg/fips) | FIPS build/runtime detection (`Enabled()`, `BuiltForFIPS()`) |

## Architecture: DefaultForwarder, domainForwarder, Worker

```text
 serializer / metadata / process checks
                 |
                 v  Submit*(payloads)
        +------------------+
        | DefaultForwarder |  1 HTTPTransaction per payload x domain x API key
        +------------------+
           |            |
           v            v
  +----------------+  +----------------+
  | domainForwarder|  | domainForwarder|   one per intake domain
  |  app.<site>    |  |  additional or |   (additional_endpoints, MRF, ...)
  |                |  |  app.mrf.<site>|
  +----------------+  +----------------+
   | highPrio | lowPrio (retries)
   v          v
  +--------+ +--------+
  | Worker | | Worker |   forwarder_num_workers per domain
  +--------+ +--------+
   |  up to forwarder_max_concurrent_requests in-flight each
   v
  blockedEndpoints check --> shared http.Client --> POST https://<domain>/<route>
   |                                                    |
   |  failure: backoff + requeue                        v
   +----> retry queue (memory, spills to .retry files on disk)
```

### Component creation and wiring

Binaries opt in through the Fx bundle ([`comp/forwarder/bundle.go`](<<<SRC>>>/comp/forwarder/bundle.go)). The core agent passes `WithFeatures(CoreFeatures)` ([`cmd/agent/subcommands/run/command.go`](<<<SRC>>>/cmd/agent/subcommands/run/command.go)); the [Cluster Agent](../containers/cluster-agent.md) passes `WithResolvers()` plus `WithDisableAPIKeyChecking()` ([`cmd/cluster-agent/subcommands/start/command.go`](<<<SRC>>>/cmd/cluster-agent/subcommands/start/command.go)).

`createOptions` calls `utils.GetMultipleEndpoints(config)`, which produces the full domain-to-API-keys map: the main infra URL (from `dd_url`/`site`, default `https://app.datadoghq.com`) keyed to `api_key`, every entry of `additional_endpoints`, and — when `multi_region_failover.enabled` is set — the MRF infra URL flagged as `IsMRF`. If `observability_pipelines_worker.metrics.enabled` (legacy `vector.metrics.enabled`) points at a URL, the main-domain resolver becomes a `MultiDomainResolver` that diverts series and sketch endpoints to the OPW/Vector URL while everything else still goes to Datadog.

`NewDefaultForwarder` then builds one `domainForwarder` per domain. For Datadog-owned domains it first applies `utils.AddAgentVersionToDomain`, rewriting `app.<site>` to `<major>-<minor>-<patch>-app.agent.<site>` — the regexp `ddURLRegexp` only matches Datadog sites (including `app.mrf.<site>`), so custom proxy URLs are never rewritten. Domains with no usable API key are dropped with an error.

### Transactions and fan-out

`createAdvancedHTTPTransactions` ([`impl/default_forwarder.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/default_forwarder.go)) creates one `HTTPTransaction` per payload × domain × API-key index. A transaction stores an `APIKeyIndex` and a reference to its `Resolver` rather than the key itself; the `DD-Api-Key` header (or `Authorization: Bearer` for local resolvers) is set inside `Resolver.Authorize` at send time, so a rotated key is picked up even by transactions that have been sitting in the retry queue or on disk. Standard headers include `DD-Agent-Version`, `User-Agent: datadog-agent/<version>`, and `Allow-Arbitrary-Tag-Value: true` when `allow_arbitrary_tags` is set.

Each transaction carries a `Kind` (Series, Sketches, ServiceChecks, Events, CheckRuns, Metadata, Process), a `Priority` (Normal or High — metadata is high priority), and a `Destination` (`AllRegions`, `PrimaryOnly`, `SecondaryOnly`, or `LocalOnly`) used for MRF and autoscaling routing. Metadata payloads that embed the API key in the body are created with `storableOnDisk=false` so they never touch the disk queue. Process-style submissions (`SubmitProcessChecks` and friends) return a `chan Response` fed by per-transaction completion handlers with a 30 s aggregate timeout, and `SubmitV1IntakeDirect` bypasses the worker queues entirely for synchronous shutdown telemetry.

### Queues, workers, and the retry loop

Each `domainForwarder` owns two buffered channels: `highPrio` for new transactions (`forwarder_high_prio_buffer_size`, default 100) and `lowPrio` for retries (`forwarder_low_prio_buffer_size`, default 100). If `highPrio` is full, the transaction goes straight into the retry queue rather than blocking the caller (counted by the `HighPriorityQueueFull` transactions expvar). Workers prefer `highPrio` over `lowPrio` via a double `select` in [`Worker.Start`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/worker.go).

Each worker holds a weighted semaphore of size `forwarder_max_concurrent_requests` (default 10) and runs each HTTP request in its own goroutine, so effective concurrency per domain is `forwarder_num_workers × forwarder_max_concurrent_requests`, not just the worker count. Every 5 s (`flushInterval`, hardcoded in [`impl/domain_forwarder.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/domain_forwarder.go)) `retryTransactions` drains the retry queue, sorts it with [`transaction.SortByCreatedTimeAndPriority`](<<<SRC>>>/comp/forwarder/defaultforwarder/transaction/sort_by_created_time_and_priority.go) (high priority first, then newest first), and re-enqueues into `lowPrio` — unless the target endpoint is currently blocked, in which case the transaction goes back into the retry queue, possibly spilling to disk or evicting older entries.

All workers of a domain share a single `http.Client` through [`SharedConnection`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/shared_connection.go); `forwarder_connection_reset_interval` (0 = disabled) periodically swaps it to force fresh connections, `forwarder_http_protocol` selects `http1` or `auto` (HTTP/2 negotiation), and the client timeout is `forwarder_timeout` (20 s). On shutdown, `forwarder_stop_timeout` (2 s) bounds a final purge and `forwarder_stop_wait_for_inflight` (false) chooses between awaiting or cancelling in-flight requests; if disk storage is enabled, the remaining retry queue is flushed to disk.

### Circuit breaker and backoff

[`blockedEndpoints`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/blocked_endpoints.go) is a circuit breaker keyed by full target (domain + route), shared by all workers of a domain. It has three states: `unblocked`, `blocked` (a backoff deadline is active; all sends are skipped), and `halfBlocked` (the deadline passed; `isBlockForSend` lets exactly one probe transaction through). A successful probe decrements the error count — stepping back through progressively shorter block windows until unblocked (or clearing entirely if `forwarder_recovery_reset` is true) — while a failed probe re-blocks with a longer window.

The backoff durations come from the exponential policy in [`pkg/util/backoff`](<<<SRC>>>/pkg/util/backoff), parameterized by `forwarder_backoff_factor` (2), `forwarder_backoff_base` (2), `forwarder_backoff_max` (64 s), and `forwarder_recovery_interval` (2). Values below the supported minimums are clamped with a warning.

### HTTP status semantics

`internalProcess` in [`transaction/transaction.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/transaction/transaction.go) decides the fate of each response:

| Response | Action |
|---|---|
| 2xx | Success; success telemetry, every Nth transaction logged (`logging_frequency`) |
| 400, 413 | **Dropped permanently** — the payload is malformed or too large, retrying cannot help |
| 403 | Triggers a throttled [secrets](../configuration/secrets.md) refresh (`secret_refresh_on_api_key_failure_interval`); retried if a refresh actually ran, dropped otherwise |
| 404 | **Retried forever, by design** — the payload may target a route that a proxy or secondary region does not implement yet |
| Any other >400 | Retried with backoff |
| Network error / timeout | Retried with backoff |
| Cancelled context | Silently discarded (shutdown path) |

The intake `Date` response header is also parsed into an NTP-style clock-offset expvar (`corechecks_net_ntp_intake_time_offset`, [`transaction/intake_offset.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/transaction/intake_offset.go)) — useful when diagnosing "payload too old" intake drops.

## Retry queue and on-disk storage

Each domain has a [`retry.TransactionRetryQueue`](<<<SRC>>>/comp/forwarder/defaultforwarder/internal/retry/transaction_retry_queue.go) capped at `forwarder_retry_queue_payloads_max_size` (15 MiB of payload bytes; the deprecated `forwarder_retry_queue_max_size` transaction count is converted at ~2 MiB per transaction). On overflow, if disk storage is enabled the oldest/lowest-priority `forwarder_flush_to_disk_mem_ratio` (0.5) fraction of the queue is serialized into a new `.retry` file; otherwise the oldest transactions are dropped and counted in `transactions.dropped` telemetry.

Disk storage is enabled by setting `forwarder_storage_max_size_in_bytes` > 0 and works **only in the core agent process** — other processes log "feature is unavailable for this process" and silently keep memory-only behavior. Files land under `forwarder_storage_path` (default `${run_path}/transactions_to_retry`, so `/opt/datadog-agent/run/...` on Linux and `C:\ProgramData\Datadog\run\...` on Windows) in a per-domain hash folder. Reads happen memory-first, then newest file first, and files are read and written whole.

The serialized format ([`http_transactions_serializer.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/internal/retry/http_transactions_serializer.go), protobuf `HttpTransactionProtoCollection`) has two deliberate security properties: the **domain is not stored** (a tampered file cannot redirect API-key-bearing requests to another host — a file only replays against the domain folder it was written in), and **API keys are replaced by placeholders** (`\xfeAPI_KEY\xfe<index>\xfe`) in headers, URL, and payload, resolved back through the live resolver on deserialization — so rotated keys apply to replayed transactions too. [`tools/retry_file_dump`](<<<SRC>>>/tools/retry_file_dump/README.md) can decode `.retry` files for debugging.

Two janitors bound disk usage: [`DiskUsageLimit`](<<<SRC>>>/comp/forwarder/defaultforwarder/internal/retry/disk_usage_limit.go) caps usable space at min(`forwarder_storage_max_size_in_bytes`, free space minus a reserve keeping total disk usage below `forwarder_storage_max_disk_ratio` = 0.80), deleting the **oldest** files when full; [`FileRemovalPolicy`](<<<SRC>>>/comp/forwarder/defaultforwarder/internal/retry/file_removal_policy.go) removes files older than `forwarder_outdated_file_in_days` (10) and folders for unknown domains at startup. [`QueueDurationCapacity`](<<<SRC>>>/comp/forwarder/defaultforwarder/internal/retry/queue_duration_capacity.go) tracks ingress bytes/sec over a 15-minute sliding window and emits `datadog.agent.retry_queue_duration.capacity_secs` — "how many seconds of outage can this Agent buffer".

## Domains, API keys, and dual shipping

A [`DomainResolver`](<<<SRC>>>/comp/forwarder/defaultforwarder/resolver/domain_resolver.go) owns the API key list for one domain and registers `config.OnUpdate` hooks: a change to the scalar `api_key` calls `UpdateAPIKey(old, new)` in place, and changes to `additional_endpoints` reload the whole map — this is how secret-backend rotations and remote `api_key` updates propagate to queued and on-disk transactions without a restart. Keys are sanitized (whitespace trimmed) by [`pkg/config/utils/api_key.go`](<<<SRC>>>/pkg/config/utils/api_key.go).

`additional_endpoints` (a map of URL to a list of API keys) dual-ships **all** default-forwarder traffic: every domain × API key pair receives every payload.

/// warning
Each additional domain/API key pair multiplies egress bandwidth and Datadog billing. Two API keys on one domain means every metric is ingested twice.
///

[`forwarderHealth`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/forwarder_health.go) registers the `forwarder` readiness probe and validates every key against `<api domain>/api/v1/validate` at startup and every `forwarder_apikey_validation_interval` (60 min). The API domain is derived by regexp — any Datadog-owned domain maps to `https://api.<site>`; custom domains are validated as-is. The check **fails open**: if validation merely errors (endpoint unreachable), keys are assumed valid to avoid marking whole fleets unhealthy during an intake outage; only explicit 403s on all keys mark the forwarder unhealthy. Key statuses surface in the expvar `forwarder.APIKeyStatus` and on the status page.

In Kubernetes with `autoscaling.failover.enabled` and `cluster_agent.enabled`, a `LocalDomainResolver` is added pointing at the Cluster Agent (HTTPS, bearer-token auth from the cluster-agent token, one dedicated worker); it only accepts series transactions marked `LocalOnly` and powers [local autoscaling failover](../containers/autoscaling.md), not Datadog intake.

## Proxies, TLS, and FIPS

[`CreateHTTPTransport`](<<<SRC>>>/pkg/util/http/transport.go) builds every outbound transport. Proxy resolution order is `proxy.http`/`proxy.https`/`proxy.no_proxy` in the config (env `DD_PROXY_*` beats the standard `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY`, case-insensitive). `no_proxy` uses legacy exact-host matching unless `no_proxy_nonexact_match: true`; while it is false the transport logs deprecation warnings whenever the legacy and standard behaviors would diverge. `use_proxy_for_cloud_metadata: false` (the default) appends the cloud metadata IPs (169.254.169.254, 100.100.100.200) to `no_proxy`. TLS is governed by `skip_ssl_validation`, `min_tls_version` (default TLS 1.2), `tls_handshake_timeout`, and `sslkeylogfile` for debugging.

FIPS comes in two mutually exclusive shapes:

1. The **FIPS agent flavor** (`datadog-fips-agent`, built with the `goexperiment.systemcrypto` and `requirefips` tags — see [Binaries and flavors](../processes/binaries.md)) uses Go's native FIPS-140 crypto; [`pkg/fips`](<<<SRC>>>/pkg/fips) reports it via `Enabled()`/`BuiltForFIPS()`. When this flavor is detected, `setupFipsEndpoints` ignores `fips.enabled` entirely — no URL rewriting happens.
1. The legacy **FIPS proxy** mode (`fips.enabled: true` on a non-FIPS binary) rewrites every product URL to `http(s)://<fips.local_address>:<fips.port_range_start + n>` where a local HAProxy terminates FIPS TLS. The port offsets are fixed in [`setupFipsEndpoints`](<<<SRC>>>/pkg/config/setup/config.go): +1 metrics, +2 traces, +3 profiles, +4 processes, +5 logs, +6 DBM metrics/metadata/activity, +7 DBM samples, +8 NDM metadata, +9 SNMP traps, +10 instrumentation telemetry, +12 orchestrator, +13 runtime security, +14 compliance, +15 NetFlow (default range 9804–9818, with 9803 for HAProxy stats).

/// warning
`fips.enabled: true` silently discards all user proxy configuration and unsets the `HTTP_PROXY`/`HTTPS_PROXY` environment variables — everything must flow through the local FIPS proxy.
///

## Multi-Region Failover (MRF)

MRF keeps a standby Datadog org in a second region warm and lets Datadog (or the operator) flip live traffic to it during a regional outage. Configuration lives under `multi_region_failover.*` ([`multi_region_failover_settings.go`](<<<SRC>>>/pkg/config/setup/multi_region_failover_settings.go)): `enabled`, `site`, `dd_url`, `api_key`, and the per-product gates `failover_metrics`, `failover_logs`, `failover_apm`.

Endpoint construction ([`pkg/config/utils/endpoints.go`](<<<SRC>>>/pkg/config/utils/endpoints.go)): infra traffic goes to `https://app.mrf.<mrf site>`, logs and event-platform hosts get a `logs.mrf.` infix (`agent-http-intake.logs.mrf.<site>`), and APM uses `https://trace.agent.mrf.<site>` (appended in [`comp/trace/config/impl/setup.go`](<<<SRC>>>/comp/trace/config/impl/setup.go)).

Per-product gating happens at different layers:

1. **Metrics**: the MRF domain gets its own `domainForwarder` with `isMRF=true`; `shouldSendHTTPTransaction` drops every non-Metadata transaction while `failover_metrics` is off. **Metadata is always dual-shipped** so the failover org stays populated with hosts and tags. The [serializer](metrics/serialization.md) additionally builds a dedicated pipeline for the MRF resolver filtered by `multi_region_failover.metric_allowlist` ([`pkg/serializer/metrics.go`](<<<SRC>>>/pkg/serializer/metrics.go)) — an unset or empty allowlist forwards **all** metrics; a non-empty list restricts to the listed names.
1. **Logs**: the MRF endpoint is appended as an `IsMRF` endpoint and [`destination_sender.go`](<<<SRC>>>/comp/logs-library/sender/destination_sender.go) gates each send on `failover_logs` at runtime; while disabled, sends to it are treated as successful no-ops so the pipeline never stalls.
1. **APM**: the trace-agent gates its MRF endpoint on `failover_apm` via its remote-config handler.

The `failover_*` flags are designed to be flipped remotely: a dedicated remote-config client subscribes to the `AGENT_FAILOVER` product and `mrfUpdateCallback` in [`rcclient.go`](<<<SRC>>>/comp/remote-config/rcclient/impl/rcclient.go) applies the values at runtime (source `SourceRC`; the payload is parsed in [`agent_failover.go`](<<<SRC>>>/comp/remote-config/rcclient/impl/agent_failover.go)), falling back to file values when the RC config is withdrawn — see [Remote configuration](../configuration/remote-config.md). `agent diagnose` includes explicit failover rows for MRF domains ([`pkg/diagnose/connectivity/inventoryendpoint.go`](<<<SRC>>>/pkg/diagnose/connectivity/inventoryendpoint.go)).

/// note
Despite the similar name, the "HA Agent" feature (`ha_agent.enabled`, active/standby election for NDM checks) is unrelated to MRF.
///

## Intake endpoint reference

All defaults derive from `site` (default `datadoghq.com`). `BuildURLWithPrefix` may append a trailing dot (FQDN) for well-known Datadog sites when `convert_dd_site_fqdn.enabled` to skip resolv.conf search-domain lookups. Datadog-owned infra URLs additionally receive the agent-version prefix at forwarder level (`7-x-y-app.agent.<site>`).

| Product | Default host | Override key | Routes / notes |
|---|---|---|---|
| Metrics, service checks, events, metadata | `https://app.<site>` (sent as `https://<v>-app.agent.<site>`) | `dd_url` (`DD_DD_URL` beats `DD_URL`) | `/api/v2/series`, `/api/beta/sketches`, `/api/v1/check_run`, `/intake/`, `/api/v1/metadata`; v3: `/api/intake/metrics/v3/series` |
| MRF infra | `https://app.mrf.<mrf site>` | `multi_region_failover.dd_url` | Same routes, gated by `failover_metrics` |
| Logs (HTTP, default) | `agent-http-intake.logs.<site>` | `logs_config.logs_dd_url` (host:port or full URL) | See [Logs pipeline](logs.md); serverless uses `http-intake.logs.` |
| Logs (TCP, legacy) | `agent-intake.logs.<site>:10516` (EU: 443) | `logs_config.logs_dd_url`, `logs_config.use_port_443` | SOCKS5 via `logs_config.socks5_proxy_address` |
| APM traces | `https://trace.agent.<site>` | `apm_config.apm_dd_url` | See [Trace pipeline](traces.md) |
| APM profiling | `intake.profile.<site>` | `apm_config.profiling_dd_url` | Proxied through the trace-agent |
| APM instrumentation telemetry | `https://instrumentation-telemetry-intake.<site>` | `apm_config.telemetry.dd_url` | Proxied through the trace-agent |
| Processes and containers | `https://process.<site>` | `process_config.process_dd_url` | `/api/v1/collector`, `/api/v1/container`, `/api/v1/connections`, `/api/v1/discovery` |
| Orchestrator explorer | `https://orchestrator.<site>` | `orchestrator_explorer.orchestrator_dd_url` | `/api/v2/orch`, `/api/v2/orchmanif` |
| Event platform tracks | `dbm-metrics-intake.<site>`, `ndm-intake.<site>`, `ndmflow-intake.<site>`, `snmp-traps-intake.<site>`, `netpath-intake.<site>`, `contlcycle-intake.<site>`, `contimage-intake.<site>`, `sbom-intake.<site>`, `resources-intake.<site>`, `http-synthetics.<site>`, `event-management-intake.<site>`, `data-obs-intake.<site>`, `softinv-intake.<site>`, `agentdiscovery-intake.<site>`, `kubeops-intake.<site>` | `<config prefix>.logs_dd_url` / `.additional_endpoints` | See [Event platform forwarder](event-platform.md) for the full track table |
| Remote configuration | `https://config.<site>` | `remote_configuration.rc_dd_url` | See [Remote configuration](../configuration/remote-config.md) |
| API (key validation) | `https://api.<site>` | — | `/api/v1/validate` |
| Install/package repos | `https://install.datadoghq.com`, `https://{apt,yum,keys}.datadoghq.com` | `installer.registry.url` | Used by [Fleet and the installer](../deployment/fleet.md) |
| OPW / Vector | user-supplied | `observability_pipelines_worker.{metrics,logs}.{enabled,url}` (legacy `vector.*`) | Metrics: resolver-level diversion; logs: replace or dual-ship |

Note that `dd_url` only overrides metrics/infra traffic; every other product has its own override key. Version-prefixed hostnames mean the intake FQDN changes on every Agent upgrade — relevant for strict egress allowlists — while custom proxy URLs are never version-prefixed.

## Configuration summary

Defaults live in the `forwarder()` function of [`pkg/config/setup/common_settings.go`](<<<SRC>>>/pkg/config/setup/common_settings.go):

| Group | Keys (defaults) |
|---|---|
| Workers / HTTP | `forwarder_num_workers` (1), `forwarder_max_concurrent_requests` (10), `forwarder_timeout` (20 s), `forwarder_http_protocol` (`auto`), `forwarder_connection_reset_interval` (0 = off) |
| Backoff / circuit breaker | `forwarder_backoff_factor` (2), `forwarder_backoff_base` (2), `forwarder_backoff_max` (64 s), `forwarder_recovery_interval` (2), `forwarder_recovery_reset` (false) |
| Memory retry queue | `forwarder_retry_queue_payloads_max_size` (15 MiB), `forwarder_high_prio_buffer_size` / `forwarder_low_prio_buffer_size` / `forwarder_requeue_buffer_size` (100 each) |
| Disk retry queue | `forwarder_storage_max_size_in_bytes` (0 = off), `forwarder_storage_path` (`${run_path}/transactions_to_retry`), `forwarder_flush_to_disk_mem_ratio` (0.5), `forwarder_storage_max_disk_ratio` (0.80), `forwarder_outdated_file_in_days` (10) |
| Shutdown | `forwarder_stop_timeout` (2 s), `forwarder_stop_wait_for_inflight` (false) |
| Health | `forwarder_apikey_validation_interval` (60 min) |
| Dual shipping | `additional_endpoints`, `observability_pipelines_worker.*` |
| Proxy / TLS | `proxy.*`, `no_proxy_nonexact_match`, `skip_ssl_validation`, `min_tls_version`, `use_proxy_for_cloud_metadata` |
| FIPS proxy | `fips.enabled` (false), `fips.local_address`, `fips.port_range_start` (9803), `fips.https`, `fips.tls_verify` |
| MRF | `multi_region_failover.*` |

## Deployment-mode differences

1. **Host install (core agent)**: the only process shape that can enable on-disk retry storage.
1. **Process agent, security agent, and others**: reuse `DefaultForwarder` without `CoreFeatures`, so no disk queue; process and NPM traffic targets `https://process.<site>` with its own queue sizing (`process_config.process_queue_bytes`).
1. **Cluster Agent**: forwarder created with `WithResolvers()` and API key checking disabled; the orchestrator forwarder only activates in Kubernetes/ECS environments and when `orchestrator_explorer.enabled` is set.
1. **Kubernetes node agent with autoscaling failover**: adds the local cluster-agent domain forwarder (bearer token, `LocalOnly` series only).
1. **FIPS environments**: choose the FIPS flavor (native Go FIPS crypto, no URL rewriting) or the legacy FIPS proxy (`fips.enabled`, localhost port map) — never both.

## Observability and ports

The forwarder makes outbound HTTPS (443) connections only and owns no listening sockets (legacy logs TCP uses 10516). Its state is visible through the `Forwarder` section of `agent status` ([`impl/status.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/status.go)), expvars under `forwarder.*` (transaction counts by domain/status, `APIKeyStatus`), internal telemetry metrics `transactions.*` ([`impl/telemetry.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/telemetry.go)), the `forwarder` readiness probe, and the connectivity rows of `agent diagnose` — see [Status, health, and telemetry](../operations/introspection.md) and [Diagnostics](../operations/diagnostics.md).

## Gotchas

1. **404s are retried forever** by design; only 400/413 (and unrecoverable 403s) drop payloads. A misconfigured `dd_url` pointing at a host that returns 404 grows the retry queue indefinitely.
1. The recurring log "The forwarder is still retrying Transaction" means the previous retry pass was still running when the next 5 s tick fired — the retry queue is so large that sorting and re-enqueueing exceeds the flush interval. The remedy the log itself suggests is *lowering* `forwarder_retry_queue_payloads_max_size`, not raising it.
1. Disk persistence silently no-ops in every process except the core agent, even with `forwarder_storage_max_size_in_bytes` set.
1. The API key health check fails open on network errors; an Agent can look healthy while its key was revoked, until the validation endpoint answers 403.
1. Effective per-domain concurrency is `forwarder_num_workers × forwarder_max_concurrent_requests` — tune the semaphore, not just workers.
1. `.retry` files deliberately omit the target domain and mask API keys with indexed placeholders; you cannot replay them against another domain, and stale keys resolve through the resolver's *current* key list.
1. An empty `multi_region_failover.metric_allowlist` allows **all** metrics during failover; the filter only restricts when the list is non-empty.
1. Intake hostnames are version-prefixed (`7-x-y-app.agent.<site>`) and change on every upgrade; egress allowlists should use wildcards.
