# Event platform forwarder

-----

The event platform forwarder (EvP forwarder, [`comp/forwarder/eventplatform`](<<<SRC>>>/comp/forwarder/eventplatform)) ships "track"-based products — database monitoring, network device monitoring, NetFlow, network paths, container lifecycle/images, SBOM, synthetics, and more — to per-product intake hostnames such as `dbm-metrics-intake.<site>`. It is deliberately **not** built on the [default forwarder](forwarder.md): instead it reuses the [logs pipeline](logs.md) sender machinery from [`comp/logs-library`](<<<SRC>>>/comp/logs-library), giving each product an independent passthrough pipeline with its own batching, compression, and endpoint configuration, all derived from a config-key prefix.

The trade is explicit: products get the logs stack's HTTP v2 intake semantics (track type in the URL path, `DD-EVP-ORIGIN` headers, per-endpoint concurrency) but none of the metrics forwarder's durability machinery — there is no transaction retry queue and no on-disk buffering. A pipeline whose input channel fills up drops events.

## Key packages and files

| Path | Purpose |
|---|---|
| [`comp/forwarder/eventplatform/def/component.go`](<<<SRC>>>/comp/forwarder/eventplatform/def/component.go) | Event type constants and the `Forwarder` interface (`SendEventPlatformEvent`, `SendEventPlatformEventBlocking`, `Purge`) |
| [`comp/forwarder/eventplatform/impl/epforwarder.go`](<<<SRC>>>/comp/forwarder/eventplatform/impl/epforwarder.go) | `defaultEventPlatformForwarder`, `passthroughPipeline` construction, `Diagnose()`, the noop variant |
| [`comp/forwarder/eventplatform/impl/pipelines_dbm.go`](<<<SRC>>>/comp/forwarder/eventplatform/impl/pipelines_dbm.go) and sibling `pipelines_*.go` files | One `passthroughPipelineDesc` per track: config prefix, hostname prefix, track type, batching defaults |
| [`comp/forwarder/eventplatform/fx/fx.go`](<<<SRC>>>/comp/forwarder/eventplatform/fx/fx.go), [`def/params.go`](<<<SRC>>>/comp/forwarder/eventplatform/def/params.go) | Fx wiring; `Params` selects the real or noop implementation |
| [`comp/forwarder/eventplatformreceiver/`](<<<SRC>>>/comp/forwarder/eventplatformreceiver) | Debug tap: every message is mirrored to it, powering `agent stream-event-platform` ([`cmd/agent/subcommands/streamep`](<<<SRC>>>/cmd/agent/subcommands/streamep/command.go)) |
| [`comp/logs-library/sender/http/http_sender.go`](<<<SRC>>>/comp/logs-library/sender/http/http_sender.go) | The HTTP sender each pipeline instantiates |
| [`comp/logs-library/sender/batch_strategy.go`](<<<SRC>>>/comp/logs-library/sender/batch_strategy.go), [`stream_strategy.go`](<<<SRC>>>/comp/logs-library/sender/stream_strategy.go) | Batching (JSON tracks) vs streaming (protobuf tracks) |
| [`comp/logs-library/client/http/destination.go`](<<<SRC>>>/comp/logs-library/client/http/destination.go) | Builds the final URL (`<prefix>/api/v2/<track>`) and intake headers |
| [`comp/logs/agent/config/config.go`](<<<SRC>>>/comp/logs/agent/config/config.go), [`config_keys.go`](<<<SRC>>>/comp/logs/agent/config/config_keys.go) | `BuildHTTPEndpointsWithCompressionOverride` and the per-prefix config-key machinery shared with the logs agent |

## How it works

`newDefaultEventPlatformForwarder` ([`impl/epforwarder.go`](<<<SRC>>>/comp/forwarder/eventplatform/impl/epforwarder.go)) iterates over every `passthroughPipelineDesc` returned by `getPassthroughPipelines()` and builds one `passthroughPipeline` per event type:

```text
 product code (DBM check, NetFlow server, SBOM collector, ...)
        |
        v  SendEventPlatformEvent(msg, eventType)     [mirrored to eventplatformreceiver]
   in chan *message.Message        (capacity: input_chan_size, per-track default)
        |
        v
   sender.Strategy                 batch (JSON tracks) or stream (protobuf tracks)
        |                          + compression
        v
   httpsender  --> destinations    POST https://<hostname-prefix><site>/api/v2/<track>
                                   (main + additional_endpoints + MRF endpoint)
```

Each pipeline builds its endpoints with `config.BuildHTTPEndpointsWithCompressionOverride`, passing the descriptor's `endpointsConfigPrefix` (for example `database_monitoring.samples.`) and `hostnameEndpointPrefix` (for example `dbm-metrics-intake.`). Because this is the exact machinery the logs agent uses, **every EvP product automatically supports the full per-prefix config surface**: `<prefix>logs_dd_url` (host:port override), `<prefix>additional_endpoints`, `<prefix>use_compression` / `compression_kind` / `compression_level`, `<prefix>batch_wait`, `<prefix>batch_max_concurrent_send`, `<prefix>batch_max_content_size`, `<prefix>batch_max_size`, `<prefix>input_chan_size`, plus API key rotation callbacks and the [Multi-Region Failover](forwarder.md#multi-region-failover-mrf) endpoint (`<hostname-prefix><site>` becomes `<hostname-prefix>logs.mrf.<mrf site>`, gated at runtime by `failover_logs` in [`destination_sender.go`](<<<SRC>>>/comp/logs-library/sender/destination_sender.go)).

Requests carry the v2 intake headers set in [`destination.go`](<<<SRC>>>/comp/logs-library/client/http/destination.go): the track type is part of the URL path (`/api/v2/<track>`), `DD-EVP-ORIGIN: agent`, and `DD-EVP-ORIGIN-VERSION: <agent version>`. Pipeline descriptors override the hardcoded logs defaults where the product needs it — DBM raises `batch_max_concurrent_send` from 0 to 10 and the input channel to 500 to absorb 4k events/s bursts; NetFlow uses a 10,000-message input channel.

Strategy selection: JSON tracks use `sender.NewBatchStrategy` (accumulate up to `batch_max_size` messages or `batch_max_content_size` bytes, flush every `batch_wait` seconds, then compress); protobuf tracks (`container-lifecycle`, `container-images`, `container-sbom`, `genresources`, `agentdiscovery`) and `event-management` (which sets `useStreamStrategy`) use `sender.NewStreamStrategy`, which sends each message as its own request without batching.

### Sending API

`SendEventPlatformEvent(msg, eventType)` is non-blocking: if the pipeline's input channel is full it drops the message and returns an error naming the channel capacity. `SendEventPlatformEventBlocking` blocks instead — callers that cannot afford loss (and can afford backpressure) use it. Both first mirror the message to the [`eventplatformreceiver`](<<<SRC>>>/comp/forwarder/eventplatformreceiver), so `agent stream-event-platform` can live-tail any track for debugging. `Purge()` drains every channel and returns the drained messages; the one-shot `agent check` command uses it (through the aggregator's `GetEventPlatformEvents`) to print the events a check produced.

The component interface wraps the forwarder in an option (`Component.Get() (Forwarder, bool)`) — in binaries that don't ship EvP data, `Get` returns `false`. A noop variant (`newNoopEventPlatformForwarder`) keeps the pipelines but strips the senders, so events accumulate in channels without leaving the process; it is used where sending must be disabled while check code still runs.

`Diagnose()` builds throwaway endpoints for every pipeline and tests connectivity, producing the per-track rows in `agent diagnose` (see [Diagnostics](../operations/diagnostics.md)). It also detects FQDN-vs-PQDN failures when `convert_dd_site_fqdn.enabled` is set.

## Track reference

Default batching values below come from the descriptor defaults; anything can be overridden under the config prefix. "Stream" content type means protobuf messages sent one per request.

| Event type | Config prefix | Default intake host | Track | Encoding / strategy | Input chan |
|---|---|---|---|---|---|
| `dbm-samples` | `database_monitoring.samples.` | `dbm-metrics-intake.<site>` | `databasequery` | JSON, batch | 500 |
| `dbm-metrics` | `database_monitoring.metrics.` | `dbm-metrics-intake.<site>` | `dbmmetrics` | JSON, batch | 500 |
| `dbm-activity` | `database_monitoring.activity.` | `dbm-metrics-intake.<site>` | `dbmactivity` | JSON, batch | 500 |
| `dbm-metadata` | `database_monitoring.metrics.` (shared) | `dbm-metrics-intake.<site>` | `dbmmetadata` | JSON, batch | 500 |
| `dbm-health` | `database_monitoring.metrics.` (shared) | `dbm-metrics-intake.<site>` | `dbmhealth` | JSON, batch | 500 |
| `dbm-column-statistics` | `database_monitoring.metrics.` (shared) | `dbm-metrics-intake.<site>` | `dbmcolumnstatistics` | JSON, batch | 500 |
| `network-devices-metadata` | `network_devices.metadata.` | `ndm-intake.<site>` | `ndm` | JSON, batch | 100 |
| `network-devices-snmp-traps` | `network_devices.snmp_traps.forwarder.` | `snmp-traps-intake.<site>` | `ndmtraps` | JSON, batch | 100 |
| `network-devices-netflow` | `network_devices.netflow.forwarder.` | `ndmflow-intake.<site>` | `ndmflow` | JSON, batch | 10000 |
| `ndmconfig` | `network_devices.config_management.forwarder.` | `ndm-intake.<site>` | `ndmconfig` | JSON, batch | 100 |
| `network-path` | `network_path.forwarder.` | `netpath-intake.<site>` | `netpath` | JSON, batch | 100 |
| `container-lifecycle` | `container_lifecycle.` | `contlcycle-intake.<site>` | `contlcycle` | Protobuf, stream | 100 |
| `container-images` | `container_image.` | `contimage-intake.<site>` | `contimage` | Protobuf, stream | 100 |
| `container-sbom` | `sbom.` | `sbom-intake.<site>` | `sbom` | Protobuf, stream | 1000 |
| `kube-actions` | `kubeactions.forwarder.` | `kubeops-intake.<site>` | `kubeactions` | JSON, batch | 100 |
| `genresources` | `genresources.` | `resources-intake.<site>` | `genresources` | Protobuf, stream | 100 |
| `synthetics` | `synthetics.forwarder.` | `http-synthetics.<site>` | `synthetics` | JSON, batch | 100 |
| `event-management` | `event_management.forwarder.` | `event-management-intake.<site>` | `events` | JSON, stream (forced) | 100 |
| `data-streams-message` | `data_streams.forwarder.` | `trace.agent.<site>` | `data_streams_messages` | JSON, batch | 100 |
| `do-query-results` | `data_observability.forwarder.` | `data-obs-intake.<site>` | `query-actions` | JSON, batch | 500 |
| `software-inventory` | `software_inventory.forwarder.` | `softinv-intake.<site>` | `softinv` | JSON, batch | 100 |
| `agentdiscovery` | `config_files_discovery.forwarder.` | `agentdiscovery-intake.<site>` | `agentdiscovery` | Protobuf, stream | 100 |

Products riding these tracks include database monitoring checks (DBM tracks), the [network device monitoring servers](ndm.md) (`ndm`, `ndmtraps`, `ndmflow`, `ndmconfig`, `netpath`), workloadmeta-driven container lifecycle and image collectors, the SBOM collector (see [Compliance and SBOM](../ebpf/compliance.md)), synthetics private locations, and data streams monitoring. Note that Workload Protection events do **not** use the EvP forwarder — [CWS](../ebpf/cws.md) has its own logs-style sender with its own endpoint config — even though both are built on the same `comp/logs-library` machinery.

## Differences from the default forwarder

| | Default forwarder | Event platform forwarder |
|---|---|---|
| Input | Pre-serialized payload bytes | Individual `*message.Message` items |
| Batching / compression | Upstream (serializer) | In the pipeline (batch strategy + compressor) |
| Retry on failure | Transaction retry queue, exponential backoff, circuit breaker | In-flight payload retried per destination with exponential backoff (permanent 4xx errors drop); no transaction queue |
| Buffering during outage | Up to 15 MiB memory + optional on-disk `.retry` files | Input channel only (drops when full) |
| Endpoint config | `dd_url` + `additional_endpoints` (global) | Per-track prefix: `<prefix>{logs_dd_url,dd_url,additional_endpoints,...}` |
| URL shape | Fixed routes (`/api/v2/series`, ...) on a version-prefixed domain | `/api/v2/<track>` on a per-product hostname |
| API key auth | `DD-Api-Key` resolved per transaction via `DomainResolver` | Logs endpoint API key (with rotation callbacks) |
| MRF | `isMRF` domain forwarder gated by `failover_metrics` | MRF endpoint appended by the logs endpoint builder, gated by `failover_logs` |

## Deployment-mode notes

1. **Fargate (ECS)**: the `data-streams-message` pipeline attaches an `X-Datadog-Additional-Tags` header including the ECS `task_arn` fetched from the ECS metadata v2 endpoint (`getECSFargateTaskARN` in [`impl/epforwarder.go`](<<<SRC>>>/comp/forwarder/eventplatform/impl/epforwarder.go)).
1. **One-shot `agent check` / `cluster-agent check` runs** ([`pkg/cli/subcommands/check`](<<<SRC>>>/pkg/cli/subcommands/check/command.go)) use the noop forwarder: pipelines exist, senders don't, and the buffered events are printed with the check output.
1. **FIPS proxy mode** (`fips.enabled`): DBM, NDM metadata, SNMP traps, and NetFlow prefixes are rewritten to dedicated localhost ports of the FIPS proxy — see the port map in [Forwarder and resilience](forwarder.md#proxies-tls-and-fips).

## Gotchas

1. **No durability**: unlike the metrics forwarder there is no retry queue and no disk spill. While intake is down the sender blocks, retrying its in-flight payloads with backoff; the pipeline backs up until the input channel fills, after which `SendEventPlatformEvent` drops events with an error suggesting to raise `batch_max_concurrent_send`.
1. `dbm-metadata`, `dbm-health`, and `dbm-column-statistics` deliberately share the `database_monitoring.metrics.` config prefix — overriding the metrics URL moves all four tracks.
1. Protobuf tracks ignore the batch settings entirely; the stream strategy sends one message per request.
1. The channel-capacity error message recommends increasing `batch_max_concurrent_send` (send parallelism), but for bursty producers raising `<prefix>input_chan_size` is often the more direct fix.
1. `Diagnose()` skips `event-management`, `do-query-results`, and `kube-actions` because those intakes reject the empty probe payload — absence from `agent diagnose` output does not mean the pipeline is missing.
1. Every message passes through the `eventplatformreceiver` tap even when no one is streaming; the tap is cheap but it means `agent stream-event-platform` sees events *before* batching, not what was actually delivered.
