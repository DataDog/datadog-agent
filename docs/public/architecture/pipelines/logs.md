# Logs pipeline

-----

The logs-agent collects log data from files, containers, journald, the Windows Event Log, network sockets, and Agent integrations, then decodes, processes, batches, compresses, and ships it to the Datadog logs intake. It runs inside the core Agent process as the Fx component `comp/logs/agent`, gated on `logs_enabled: true`. The low-level pipeline machinery — processor, batching strategies, senders, HTTP/TCP destination clients, diagnostics, telemetry — is extracted into the standalone `comp/logs-library` module so it can be reused by the [event platform forwarder](event-platform.md), the serverless Agent, and OTel components.

The mental model has two halves:

1. **What to log** (scheduling plane): schedulers turn Autodiscovery `integration.Config`s and static config into `sources.LogSource` entries; launchers subscribe to the source store by type and spawn tailers.
1. **How logs flow** (data plane): each tailer feeds a decoder (framing, parsing, multiline handling), which writes into one of N parallel pipelines, each being processor → strategy (batching and compression) → shared sender workers → HTTP or TCP destinations. Delivered payloads are acknowledged to the auditor, which persists per-source offsets in `registry.json` so tailing resumes after a restart without loss or duplication.

```text
 scheduling plane                          data plane (one per tailer)      (N pipelines, default 4)
+--------------------+   LogSource   +---------+   +----------+          +-----------+  +----------+
| schedulers         |-------------->| launcher|-->| tailer   |--frames->| decoder   |->| processor|
| (AD, channel, ...) |  pub/sub by   | (file,  |   | (per file|          | framer/   |  | rules,   |
|                    |  source type  |  container, |  /conn/  |          | parser/   |  | render,  |
+--------------------+               |  listener,  |  journal)|          | multiline |  | encode   |
                                     |  journald,  +----------+          +-----------+  +----+-----+
                                     |  winevent,  |                                         |
                                     |  integration)                                         v
+---------------+   offsets    +---------+     acks      +---------------+  payloads  +----------+
| registry.json |<-------------| auditor |<--------------| destinations  |<-----------| strategy |
+---------------+   every 1s   +---------+  (reliable    | HTTP or TCP   |   sender   | batch/   |
                                            endpoints)   +---------------+   workers  | stream   |
                                                                                      +----------+
```

## Key packages

| Path | Purpose |
|---|---|
| [`pkg/logs/README.md`](<<<SRC>>>/pkg/logs/README.md) | In-repo architecture overview with ASCII diagrams |
| [`comp/logs/agent/impl/agent.go`](<<<SRC>>>/comp/logs/agent/impl/agent.go) | The logs-agent component: lifecycle, start/stop ordering, transport restart, stream-logs endpoint |
| [`comp/logs/agent/impl/agent_core_init.go`](<<<SRC>>>/comp/logs/agent/impl/agent_core_init.go) | `SetupPipeline`: builds the pipeline provider and registers all launchers |
| [`comp/logs/agent/config`](<<<SRC>>>/comp/logs/agent/config) | `LogsConfig` (per-source config), `Endpoints` construction, `processing_rules.go` |
| [`pkg/logs/sources`](<<<SRC>>>/pkg/logs/sources) | `LogSources` store: pub/sub of `LogSource` by type |
| [`pkg/logs/schedulers`](<<<SRC>>>/pkg/logs/schedulers) | Scheduler framework; [`ad/scheduler.go`](<<<SRC>>>/pkg/logs/schedulers/ad/scheduler.go) is the Autodiscovery scheduler |
| [`pkg/logs/launchers`](<<<SRC>>>/pkg/logs/launchers) | file, container, listener (TCP/UDP), journald, windowsevent, integration, channel launchers |
| [`pkg/logs/launchers/container/tailerfactory`](<<<SRC>>>/pkg/logs/launchers/container/tailerfactory) | Decides file vs Docker-socket vs kubelet-API tailing per container |
| [`pkg/logs/tailers`](<<<SRC>>>/pkg/logs/tailers) | file, container, journald, windowsevent, socket tailers |
| [`pkg/logs/internal/decoder`](<<<SRC>>>/pkg/logs/internal/decoder) | Decoder actor: framer → line parser → line handler (single-line, multiline, auto-multiline) |
| [`pkg/logs/internal/framer/framer.go`](<<<SRC>>>/pkg/logs/internal/framer/framer.go) | Framings: UTF-8/UTF-16/Shift-JIS newline, DockerStream, syslog (RFC 6587), datagram/stream |
| [`pkg/logs/internal/parsers`](<<<SRC>>>/pkg/logs/internal/parsers) | `dockerfile`, `dockerstream`, `kubernetes` (CRI), `encodedtext`, `integrations`, `syslog`, `noop` parsers |
| [`pkg/logs/message`](<<<SRC>>>/pkg/logs/message) | `Message` model (content states), `Origin`, `Payload` |
| [`comp/logs-library/pipeline`](<<<SRC>>>/comp/logs-library/pipeline) | `Provider` (N pipelines, round-robin, optional failover routers), `Pipeline` (processor + strategy) |
| [`comp/logs-library/processor`](<<<SRC>>>/comp/logs-library/processor) | Processing rules, rendering, MRF tagging, encoders (JSON/proto/raw) |
| [`comp/logs-library/sender`](<<<SRC>>>/comp/logs-library/sender) | Sender queues and workers, [`batch_strategy.go`](<<<SRC>>>/comp/logs-library/sender/batch_strategy.go), [`stream_strategy.go`](<<<SRC>>>/comp/logs-library/sender/stream_strategy.go) |
| [`comp/logs-library/client/http/destination.go`](<<<SRC>>>/comp/logs-library/client/http/destination.go) | HTTP destination: worker pool, exponential backoff, API-key refresh on 403 |
| [`comp/logs-library/client/tcp`](<<<SRC>>>/comp/logs-library/client/tcp) | TCP destination: connection manager (TLS, SOCKS5), API-key prefixer, delimiter |
| [`comp/logs/auditor/impl/auditor.go`](<<<SRC>>>/comp/logs/auditor/impl/auditor.go) | Registry auditor: offsets and fingerprints persisted to `registry.json` |
| [`comp/logs-library/diagnostic`](<<<SRC>>>/comp/logs-library/diagnostic) | `BufferedMessageReceiver` backing `agent stream-logs` |
| [`comp/logs-library/metrics`](<<<SRC>>>/comp/logs-library/metrics) | Expvars and Prometheus telemetry, pipeline capacity/utilization monitors |
| [`comp/logs/integrations`](<<<SRC>>>/comp/logs/integrations) | Component through which Go/Python checks submit integration logs (`send_log`) |

## Component wiring and startup

`NewComponent` in [`agent.go`](<<<SRC>>>/comp/logs/agent/impl/agent.go) builds the agent only when `logs_enabled` is true; otherwise it returns `option.None`. Dependencies include config, hostname, the auditor component, workloadmeta (optional), the tagger, log compression, secrets, and an Fx value group of `[]schedulers.Scheduler` tagged `log-agent-scheduler` — the Autodiscovery scheduler is contributed by [`comp/logs/adscheduler`](<<<SRC>>>/comp/logs/adscheduler).

The Fx `OnStart` hook runs four steps:

1. `buildEndpoints` performs an HTTP connectivity check (`CheckConnectivity`, 5-second timeout, POST of an empty JSON body to `agent-http-intake.logs.<site>`), then endpoint construction in [`config.go`](<<<SRC>>>/comp/logs/agent/config/config.go) decides HTTP versus TCP transport (see [Transport selection](#transport-selection-and-endpoints)).
1. `setupAgent` validates global `logs_config.processing_rules` and `logs_config.fingerprint_config`, then `SetupPipeline` ([`agent_core_init.go`](<<<SRC>>>/comp/logs/agent/impl/agent_core_init.go)) builds the destinations context, the diagnostic message receiver, the `pipeline.Provider`, and registers the launcher instances (file, listener, journald, windowsevent, container, integration).
1. `startPipeline` starts everything in dependency order: destinations context → auditor → pipeline provider → diagnostic receiver → launchers, then starts the schedulers.
1. If endpoints resolved to TCP (and TCP was not explicitly pinned via `force_use_tcp` or a SOCKS5 proxy), a background `smartHTTPRestart` goroutine re-checks HTTP connectivity with exponential backoff (capped by `logs_config.http_connectivity_retry_interval_max`, default 1 hour) and, on success, performs a partial pipeline restart ([`agent_restart.go`](<<<SRC>>>/comp/logs/agent/impl/agent_restart.go)): launchers and pipelines are stopped and rebuilt with HTTP endpoints while sources, schedulers, the tailer tracker, the auditor, and the diagnostic receiver survive — so tailers resume from registry offsets. If the restart fails it rolls back to TCP.

`stop()` reverses the order under a grace period (`logs_config.stop_grace_period`, default 30s); on timeout it force-closes destinations (draining without network writes), waits five more seconds, then dumps goroutines.

## Sources and schedulers

A `LogSource` ([`pkg/logs/sources/source.go`](<<<SRC>>>/pkg/logs/sources/source.go)) is a named `*config.LogsConfig` — type, path, service, source, tags, processing rules, multiline options — plus runtime status (messages, latency stats, and the info registry rendered in `agent status`). The `LogSources` store ([`sources.go`](<<<SRC>>>/pkg/logs/sources/sources.go)) is a pub/sub hub: launchers call `GetAddedForType`/`SubscribeForType`/`SubscribeAll` and receive adds and removes over channels.

Schedulers add and remove sources through the `SourceManager` interface in [`pkg/logs/schedulers`](<<<SRC>>>/pkg/logs/schedulers). The most important one is the AD scheduler ([`ad/scheduler.go`](<<<SRC>>>/pkg/logs/schedulers/ad/scheduler.go)), which registers an [Autodiscovery](../checks/autodiscovery.md) listener and, for each `integration.Config` carrying a logs section:

1. Parses the `logs_config` — YAML for the file provider, JSON for container labels, pod annotations, `process_log`, and remote-config providers — into one or more `LogsConfig` entries.
1. For container-attached configs, sets `cfg.Type` to the container runtime (`docker`, `containerd`, ...) and `cfg.Identifier` to the container ID; a config that explicitly requests a `file`/`tcp`/`udp`/`integration` type keeps its type and only gets the identifier.
1. Filters `process_log`-provider file sources that conflict with manually configured file paths, and honors container exclude lists via the workload filter.

`container_collect_all` is implemented inside Autodiscovery, not the logs code: [`container_collect_all.go`](<<<SRC>>>/comp/core/autodiscovery/common/utils/container_collect_all.go) appends an almost-empty logs config per container when `logs_config.container_collect_all: true`, dropped during resolution if another template already provides a logs config for that container. Catch-all sources are only scheduled after AD's first config scan so annotated configs win, but the interaction is inherently racy (documented in `pkg/logs/README.md`).

Remote Configuration can schedule log configs only when `remote_configuration.agent_integrations.allow_log_config_scheduling: true`. The legacy `Services` store ([`pkg/logs/service`](<<<SRC>>>/pkg/logs/service)) still exists but is being phased out per [`pkg/logs/schedulers/README.md`](<<<SRC>>>/pkg/logs/schedulers/README.md); the modern container launcher consumes sources only.

## Launchers and tailers

Launchers are registered in `addLauncherInstances` ([`agent_core_init.go`](<<<SRC>>>/comp/logs/agent/impl/agent_core_init.go)); each receives the source provider, the pipeline provider, the auditor registry, and the tailer tracker on `Start`.

| Launcher | Source types | Tailer granularity | Offset stored |
|---|---|---|---|
| [`file`](<<<SRC>>>/pkg/logs/launchers/file/launcher.go) | `file` | one per tailed file | byte offset (+ optional fingerprint) |
| [`container`](<<<SRC>>>/pkg/logs/launchers/container/launcher.go) | `docker`, `containerd`, `podman`, `cri-o` | one per container | byte offset (file) or timestamp (socket/API) |
| [`listener`](<<<SRC>>>/pkg/logs/launchers/listener) | `tcp`, `udp` | one per TCP connection / one per UDP port | none |
| [`journald`](<<<SRC>>>/pkg/logs/launchers/journald/launcher.go) | `journald` | one per journal config | journal cursor |
| [`windowsevent`](<<<SRC>>>/pkg/logs/launchers/windowsevent/launcher.go) | `windows_event` | one per channel/query | bookmark XML |
| [`integration`](<<<SRC>>>/pkg/logs/launchers/integration/launcher.go) | `integration` | delegates to file launcher | byte offset |
| [`channel`](<<<SRC>>>/pkg/logs/launchers/channel/launcher.go) | serverless channel | one per channel | none |

### File launcher

Every `logs_config.file_scan_period` (default 1s) a scan resolves wildcard paths via the [`fileprovider`](<<<SRC>>>/pkg/logs/launchers/file/provider/file_provider.go) (selection mode `logs_config.file_wildcard_selection_mode`: `by_name`, the default, reverse-lexicographical; or `by_modification_time`), capped at `logs_config.open_files_limit` (default 500; 200 on macOS). Hitting the limit silently leaves the remaining matches untailed, and `by_modification_time` churns tailers in rotation-heavy directories.

Rotation is detected per tailer: on Unix by inode change or file shrink ([`rotate_nix.go`](<<<SRC>>>/pkg/logs/tailers/file/rotate_nix.go)), on Windows heuristically by size and creation time ([`rotate_windows.go`](<<<SRC>>>/pkg/logs/tailers/file/rotate_windows.go)), or via fingerprint comparison when fingerprinting is enabled. On rotation a new tailer starts at the beginning of the new file while the old tailer drains for `logs_config.close_timeout` (default 60s). Starting-offset logic lives in [`position.go`](<<<SRC>>>/pkg/logs/launchers/file/position.go): a forced `start_position` beats the registry offset, and a stored fingerprint mismatch forces a restart from the beginning (the file at that path is a different file). For Kubernetes pod logs, `logs_config.validate_pod_container_id` (default true) cross-checks the container ID against the `/var/log/containers` symlink name to avoid tailing a recreated pod's file with a stale source.

File fingerprinting ([`fingerprint.go`](<<<SRC>>>/pkg/logs/tailers/file/fingerprint.go)) is configured globally by `logs_config.fingerprint_config` with per-source overrides: strategies `line_checksum` (hash of the first `count` lines), `byte_checksum` (hash of the first `count` bytes, default 1024), or `disabled` (the default), plus `count_to_skip`. Fingerprints stored in the registry make rotation-to-same-path detection reliable and invalidate stale offsets.

### Container launcher

Handles all container runtimes behind a single launcher (build tags `kubelet || docker`) and delegates the per-container transport decision to the [`tailerfactory`](<<<SRC>>>/pkg/logs/launchers/container/tailerfactory) — see [Container log collection modes](#container-log-collection-modes).

### Network listener

TCP sources spawn one [`stream tailer`](<<<SRC>>>/pkg/logs/tailers/socket/stream_tailer.go) per accepted connection; UDP sources use one [`datagram tailer`](<<<SRC>>>/pkg/logs/tailers/socket/datagram_tailer.go) per port. Read buffer size comes from `logs_config.frame_size` (default 9000). Sources with `format: syslog` use RFC 6587 syslog framing, which auto-detects octet counting versus non-transparent (LF/NUL-delimited) framing. Network sources have an empty registry identifier, so there is no offset tracking — after a restart they simply resume from live traffic.

### journald

Linux only, behind the `systemd` build tag; uses `go-systemd/sdjournal` (cgo). One tailer per journal config, identified as `journald:default` or `journald:<config_id>`. The offset is the journal cursor. Sources support include/exclude unit filters and a `path:` to read a specific journal directory. Messages are structured (`StateStructured`); `process_raw_message` controls whether processing rules see the full journald JSON or just the message content. The `dd-agent` user must be in the `systemd-journal` group to read `/var/log/journal`.

### Windows Event Log

The [`windowsevent tailer`](<<<SRC>>>/pkg/logs/tailers/windowsevent/tailer.go) uses the Win32 `EvtSubscribe` pull subscription via `pkg/util/winutil/eventlog`, rendering events into structured messages; the offset is a bookmark XML blob. Per-source config uses `channel_path` and `query`.

### Integration logs

Go and Python checks emit logs through the [`comp/logs/integrations`](<<<SRC>>>/comp/logs/integrations) component (for example Python's `send_log`). The [`integration launcher`](<<<SRC>>>/pkg/logs/launchers/integration/launcher.go) writes them to files under `<logs_config.run_path>/integrations/` (capped by `integrations_logs_files_max_size`, `integrations_logs_total_usage`, and `integrations_logs_disk_ratio`) and creates file sources so the file launcher tails them — integration logs ride the file pipeline.

## Container log collection modes

For each container source, [`whichtailer.go`](<<<SRC>>>/pkg/logs/launchers/container/tailerfactory/whichtailer.go) picks one of three tailing modes, and [`factory.go`](<<<SRC>>>/pkg/logs/launchers/container/tailerfactory/factory.go) falls back between file and socket on error (unreadable file → socket; no Docker socket → file):

| Mode | Mechanism | When |
|---|---|---|
| file | tails the runtime's log file on disk | Docker default (`docker_container_use_file`, default true); Kubernetes pods with `k8s_container_use_file: true` or — the usual containerd/CRI-O case — via fallback, since socket tailing only supports Docker |
| socket | streams from the Docker API over `/var/run/docker.sock` | preferred for Kubernetes pods by default (`k8s_container_use_file`, default false); Docker when a registry entry recorded socket tailing (sticky), or when file mode fails |
| API | streams from the kubelet `/containerLogs` API | `logs_config.k8s_container_use_kubelet_api: true` |

In file mode, [`file.go`](<<<SRC>>>/pkg/logs/launchers/container/tailerfactory/file.go) creates a child `LogSource` of type `file` pointing at the runtime log path — `/var/lib/docker/containers/<id>/<id>-json.log` (Windows: `c:\programdata\docker\containers\...`; override with `logs_config.docker_path_override`; Podman with `logs_config.use_podman_logs`: `.../storage/overlay-containers/<id>/userdata/ctr.log`) or the Kubernetes pod path `/var/log/pods/<ns>_<pod>_<uid>/<container>/*.log`. The child source is pushed back into `LogSources` where the file launcher picks it up; its source type (`docker` vs `kubernetes`) tells the decoder whether to use the Docker JSON-file parser or the CRI parser.

Socket mode uses [`NewDockerSocketTailer`](<<<SRC>>>/pkg/logs/launchers/container/tailerfactory/socket.go) (read timeout `logs_config.docker_client_read_timeout`, default 30s). API mode uses [`NewAPITailer`](<<<SRC>>>/pkg/logs/launchers/container/tailerfactory/api.go) (read timeout `logs_config.kubelet_api_client_read_timeout`, default 30s) and needs no hostPath mounts at all, which suits unprivileged setups — its offset is a timestamp replayed to the kubelet as a `sinceTime` query parameter rather than a byte offset into an on-disk file.

The mode choice is sticky through the registry: if a socket offset exists for a container, `whichtailer` keeps using the socket to avoid re-sending logs already shipped under the other mode's offset scheme. The choice between "log containers" and "log pods" is made by the `containersorpods.Chooser`, which waits up to `logs_config.container_runtime_waiting_timeout` (default 3s) for a runtime to appear. Container and pod lookups (log paths, Podman root, existence checks) come from [workloadmeta](../containers/workloadmeta.md).

## Decoding: framer, parsers, line handlers

Each tailer owns a [`decoder`](<<<SRC>>>/pkg/logs/internal/decoder/decoder.go) — an actor with `InputChan`/`OutputChan` of `*message.Message` — built from three stages:

1. The [`framer`](<<<SRC>>>/pkg/logs/internal/framer/framer.go) splits the byte stream into frames. Framings: `UTF8Newline` (also Latin-1), `UTF16BE/LENewline`, `SHIFTJISNewline` (chosen by the per-source `encoding:` setting), `DockerStream` (8-byte binary headers of Docker multiplexed streams), `SyslogFraming` (RFC 6587, auto-detects octet counting vs LF/NUL delimiting), `UTF8NewlineDatagram` (flush after every UDP datagram), `UTF8NewlineStream` (flush the remainder at connection close), and `NoFraming`. Frames longer than the source's `max_message_size_bytes` are split.
1. The line parser ([`line_parser.go`](<<<SRC>>>/pkg/logs/internal/decoder/line_parser.go)) wraps a format parser from [`pkg/logs/internal/parsers`](<<<SRC>>>/pkg/logs/internal/parsers): `dockerfile` (Docker json-file: extracts content, status from the stream name, timestamp), `dockerstream` (Docker socket format), `kubernetes` (CRI text format, reassembling `P` partial lines via `MultiLineParser` with flush timeout `logs_config.aggregation_timeout`, default 1000ms), `encodedtext` (UTF-16/Shift-JIS to UTF-8), `integrations`, `syslog` (RFC 5424/3164 attribute parsing), and `noop`. A parsed timestamp lands in `msg.ParsingExtra.Timestamp`, used for container since-offsets and optionally as the log timestamp (`logs_config.use_container_timestamp`).
1. The line handler aggregates lines into messages, selected in priority order by `buildLineHandler`:
    1. A per-source `multi_line` processing rule → regex [`RegexAggregator`](<<<SRC>>>/pkg/logs/internal/decoder/preprocessor/regex_aggregator.go) (combine lines until the next start-pattern match, truncate at `max_message_size_bytes`).
    1. `logs_config.force_auto_multi_line_detection_v1` → the [`LegacyAutoMultilineHandler`](<<<SRC>>>/pkg/logs/internal/decoder/legacy_auto_multiline_handler.go): samples `auto_multi_line_default_sample_size` (500) lines against a table of timestamp regexes plus `auto_multi_line_extra_patterns`, and switches permanently to a [`MultiLineHandler`](<<<SRC>>>/pkg/logs/internal/decoder/multiline_handler.go) once the match ratio passes `auto_multi_line_default_match_threshold` (0.48); the detected pattern survives file rotation.
    1. Auto-multiline v2 (`auto_multi_line_detection`, default true — the default path): the [`preprocessor`](<<<SRC>>>/pkg/logs/internal/decoder/preprocessor) tokenizes line prefixes, labels lines (JSON detection, datetime detection, a learned pattern table), and combines detected groups into one message (`CombiningAggregator`, with JSON aggregation via `auto_multi_line.enable_json_aggregation`, default true, and stack-trace aggregation via `auto_multi_line.stack_trace_parsers`, default `["go"]`). When detection is disabled, `auto_multi_line_detection_tagging` (default true) still labels lines and tags detected group starts without combining them (`DetectingAggregator`).
    1. Otherwise a pass-through single-line handler.

Multiline-combined and truncated messages get diagnostic tags when `tag_multi_line_logs`/`tag_truncated_logs` are enabled (both default true). The preprocessor also hosts the experimental adaptive sampler and noisy-log detection (`logs_config.experimental_adaptive_sampling.*`, `experimental_noisy_log_detection`), a pattern-clustering rate limiter per source.

### Message model

[`message.go`](<<<SRC>>>/pkg/logs/message/message.go) defines `Message = MessageContent + MessageMetadata`. Content moves through states: `StateUnstructured` (raw bytes from file/socket tailers) or `StateStructured` (journald/windowsevent maps) → the processor `Render()`s it to `StateRendered` → an encoder makes it `StateEncoded`. Metadata carries `Hostname`, `Origin` (identifier, offset, source, fingerprint, tags), `Status` (severity), `IngestionTimestamp`, and `RawDataLen` (raw bytes consumed, used for offset math). A `Payload` is a batch of metadata plus the encoded bytes.

Tags on a message are the union of the source config `tags:`, container tags from the [tagger](../containers/tagger.md) (fetched per message through `tag.Provider`, with an optional `logs_config.tagger_warmup_duration`), host tags baked in for `logs_config.expected_tags_duration` after startup, and processing tags (`auto_multiline`, `truncated`, ...).

## Pipelines: processor, strategy, sender, destinations

The [`pipeline.Provider`](<<<SRC>>>/comp/logs-library/pipeline/provider.go) owns `logs_config.pipelines` `Pipeline` instances (default: `min(GOMAXPROCS, 4)`). Each tailer gets a channel via `NextPipelineChan()` — round-robin assignment, but a given tailer sticks to one pipeline, preserving per-source ordering. Optional pipeline failover (`logs_config.pipeline_failover.enabled`, default false) inserts router goroutines that try the primary pipeline and fail over non-blocking to any other pipeline, trading strict per-source ordering for throughput under skew.

### Processor

One [`Processor`](<<<SRC>>>/comp/logs-library/processor/processor.go) per pipeline:

1. Applies processing rules — global (`logs_config.processing_rules`) plus per-source. Rule types ([`processing_rules.go`](<<<SRC>>>/comp/logs/agent/config/processing_rules.go)): `exclude_at_match` (drop on regex match), `include_at_match` (drop unless match), `mask_sequences` (regex replace with `replace_placeholder`), `exclude_truncated`, `remap_source`, and `multi_line` (valid per-source only; as a global rule the agent warns at startup and flags it in status).
1. Renders the message and mirrors a copy to the diagnostic `BufferedMessageReceiver` for `stream-logs`.
1. When multi-region failover is active (`multi_region_failover.failover_logs`, toggleable at runtime through [remote configuration](../configuration/remote-config.md)), tags messages `IsMRFAllow`, optionally filtered by `multi_region_failover.logs_service_allowlist`.
1. Encodes in place: JSON for HTTP (`{"message","status","timestamp","hostname","service","ddsource","ddtags"}`), protobuf for TCP with `dev_mode_use_proto` (default true), otherwise an RFC 5424-style syslog line carrying `[dd ddsource="..."][dd ddtags="..."]` structured data before the message content.

/// note
The Sensitive Data Scanner (SDS) integration was removed from the Agent (PR #44221); there is no `sds.*` configuration or libdatadog scanner in the logs pipeline anymore. Redaction is done exclusively by `processing_rules` (`mask_sequences`, `exclude_at_match`, ...).
///

### Strategy: batching and compression

For HTTP transport, [`batch_strategy.go`](<<<SRC>>>/comp/logs-library/sender/batch_strategy.go) accumulates messages into a `MessageBuffer` as a streaming-compressed JSON array ([`serializer.go`](<<<SRC>>>/comp/logs-library/sender/serializer.go) writes `[msg,msg,...]` through a zstd or gzip stream compressor from `comp/serializer/logscompression`). A batch flushes on message count (`batch_max_size`, 1000), content size (`batch_max_content_size`, 5 MB), a timer (`batch_wait`, default 5s, max 10), explicit flush (serverless), or shutdown. The strategy maintains two concurrent batches — `main` and `mrf` — so MRF-tagged messages ship in payloads flagged for the failover intake. A single message exceeding the content limit is dropped and counted in `logs_sender_batch_strategy.dropped_too_large`. For TCP transport, [`stream_strategy.go`](<<<SRC>>>/comp/logs-library/sender/stream_strategy.go) emits one payload per message with no compression.

### Sender, workers, and reliability

The [`Sender`](<<<SRC>>>/comp/logs-library/sender/sender.go) is shared across all pipelines. For HTTP it runs one queue and one worker backed by a destination-level concurrent worker pool ([`worker_pool.go`](<<<SRC>>>/comp/logs-library/client/http/worker_pool.go), adaptive between `#pipelines` and `#pipelines×10` workers based on measured send latency against a fixed 150 ms target); for TCP it runs one queue with `#pipelines` workers because the TCP destination is synchronous. `logs_config.disable_distributed_senders: true` restores the legacy per-pipeline senders.

Each payload handled by a [`worker`](<<<SRC>>>/comp/logs-library/sender/worker.go) must be accepted by at least one **reliable** destination — the main endpoint plus any `additional_endpoints` entry with `is_reliable: true` (the default). While all reliable destinations are retrying, the worker blocks, and that backpressure propagates all the way to the tailers: files keep growing unread, TCP sources apply socket backpressure, UDP drops. Reliable destinations that are currently failing receive best-effort buffered copies so they can catch up; **unreliable** destinations get non-blocking sends only and never acknowledge to the auditor. MRF destinations only receive payloads flagged MRF.

### HTTP destination

[`destination.go`](<<<SRC>>>/comp/logs-library/client/http/destination.go) POSTs to `https://agent-http-intake.logs.<site>/api/v2/logs` (the v2 events-platform intake; `/v1/input` when `use_v2_api: false`) with `DD-API-KEY`, `Content-Encoding` (zstd by default, gzip via `logs_config.compression_kind`), `DD-EVP-ORIGIN(-VERSION)`, and timestamp headers. HTTP/2 is negotiated via ALPN by default (`logs_config.http_protocol: auto`), with `logs_config.http_timeout` (10s) and the standard Agent proxy settings. Retry policy: exponential backoff (`sender_backoff_base` 1s, `sender_backoff_max` 120s, factor 2) on network errors and 5xx; permanent drop on 400/401/413. A 403 whose API key came from a [secrets backend](../configuration/secrets.md) triggers `secrets.Refresh()` and a retry instead of a drop, supporting API-key rotation without an agent restart. Retries surface in `agent status` through the `RetryCount` and `RetryTimeSpent` metrics.

### TCP destination

[`client/tcp`](<<<SRC>>>/comp/logs-library/client/tcp) prepends the API key to each frame ([`prefixer.go`](<<<SRC>>>/comp/logs-library/client/tcp/prefixer.go)), appends a newline delimiter (a length prefix for protobuf frames), and writes over a TLS connection managed by [`connection_manager.go`](<<<SRC>>>/comp/logs-library/client/tcp/connection_manager.go) (plaintext with `logs_no_ssl`, SOCKS5 via `logs_config.socks5_proxy_address`). The default endpoint is `agent-intake.logs.<site>:10516` (`logs_config.dd_port`; the EU intake uses 443). On write error the connection is torn down and the payload retried on a new connection, indefinitely.

### Transport selection and endpoints

`BuildEndpointsWithConfig` in [`config.go`](<<<SRC>>>/comp/logs/agent/config/config.go) chooses HTTP when any of: `force_use_http` is set, `logs_dd_url` is an `http(s)://` URL, an Observability Pipelines Worker is configured, or the startup connectivity check passed and neither `force_use_tcp` nor `socks5_proxy_address` is set. Otherwise it falls back to TCP — and the smart restart described above upgrades back to HTTP when connectivity returns.

`additional_endpoints` enables dual shipping and inherits compression, backoff, and API version from the main endpoint. An Observability Pipelines Worker (`observability_pipelines_worker.logs.url` + `.enabled`) replaces the main endpoint, or dual-ships as an additional endpoint (`.dual_ship`, unreliable unless `.dual_ship_reliable`). An MRF endpoint is appended when `multi_region_failover.enabled` is set, keyed with `multi_region_failover.api_key`.

/// warning
TCP fallback loses features: no compression, no batching (one message per frame), no MRF or events-platform headers, and markedly worse throughput. The agent logs a prominent warning when it happens.
///

## Auditor and the registry

The sender's sink is the [`auditor`](<<<SRC>>>/comp/logs/auditor/impl/auditor.go) channel (component `comp/logs/auditor`, with a no-op variant for clients that do not need it). After a payload is accepted by a reliable destination, the auditor updates in-memory registry entries keyed by origin identifier — `file:<path>`, `journald:<id>`, `eventlog:<channel>;<query>`, `docker:<containerID>` — each holding `{LastUpdated, Offset, TailingMode, IngestionTimestamp, Fingerprint}`. An empty identifier means no tracking (network sources).

The registry flushes to `<logs_config.run_path>/registry.json` every second as `{"Version": 2, "Registry": {...}}`, with migration readers for v0/v1 formats. Writes are atomic (temp file + rename) unless `logs_config.atomic_registry_write: false` — the default is non-atomic on ECS Fargate, where rename is not permitted on some mounts. Entries expire after `logs_config.auditor_ttl` (default 23h) unless kept alive by an active tailer. For dual shipping, an entry is only updated when the payload's `IngestionTimestamp` is newer, so the slower endpoint cannot rewind offsets. The auditor also registers a liveness handle via [`comp/logs-library/kubehealth`](<<<SRC>>>/comp/logs-library/kubehealth), so "logs-agent" appears in `/live` health checks.

In containers, the registry must live on a persistent volume (the standard images mount `/opt/datadog-agent/run`); otherwise every container restart re-tails from the configured `start_position` and can duplicate or lose logs.

## Configuration

The master switch is `logs_enabled: true` (`DD_LOGS_ENABLED`). Everything else lives under `logs_config.` (env `DD_LOGS_CONFIG_<KEY>`); defaults are declared in [`pkg/config/setup/common_settings.go`](<<<SRC>>>/pkg/config/setup/common_settings.go). The most architecturally significant knobs:

| Key (under `logs_config.`) | Default | Effect |
|---|---|---|
| `logs_dd_url`, `dd_port`, `use_port_443` | site-derived, 10516 | intake override; `http(s)://` URLs force HTTP transport |
| `force_use_http` / `force_use_tcp` | false | pin the transport, skipping the connectivity check |
| `use_compression`, `compression_kind` | true, `zstd` | HTTP payload compression (gzip level via `compression_level`) |
| `batch_wait`, `batch_max_size`, `batch_max_content_size` | 5s, 1000, 5 MB | batch flush triggers |
| `pipelines` | min(GOMAXPROCS, 4) | parallel processor/strategy instances |
| `pipeline_failover.enabled` | false | cross-pipeline failover, relaxing per-source ordering |
| `additional_endpoints` | — | dual shipping (`is_reliable` per entry) |
| `container_collect_all` | false | catch-all container source (commonly enabled by Helm/Operator) |
| `open_files_limit` | 500 (200 on macOS) | concurrent file tailers |
| `file_scan_period`, `file_wildcard_selection_mode` | 1s, `by_name` | wildcard scanning |
| `close_timeout` | 60s | drain window for rotated files |
| `k8s_container_use_kubelet_api`, `docker_container_use_file` | false, true | container tailing mode |
| `auto_multi_line_detection`, `auto_multi_line_detection_tagging` | true, true | auto-multiline v2 (default: detect and combine; tag-only when detection is disabled) |
| `processing_rules` | — | global redaction/filtering rules |
| `expected_tags_duration` | 0 | attach host tags to each log for a duration after startup |
| `run_path`, `auditor_ttl`, `atomic_registry_write` | run dir, 23h, true | registry behavior |
| `stop_grace_period` | 30s | shutdown drain budget |

Per-source configuration (`logs:` in an integration config, pod annotation `ad.datadoghq.com/<container>.logs`, or container label `com.datadoghq.ad.logs`) supports `type`, `path`, `port`, `service`, `source`, `tags`, `encoding`, `start_position`, `exclude_paths`, `identifier`, `log_processing_rules`, `multi_line`/`auto_multi_line*`, journald unit filters, and `channel_path`/`query` for Windows events.

## Deployment-mode differences

1. **Linux host**: all launchers available; journald requires the `systemd`-tagged build (standard agent binaries have it) and journal read permissions. Registry at `/opt/datadog-agent/run/registry.json`.
1. **Windows**: `windowsevent` replaces journald; file rotation detection is size/creation-time-based (no inodes), and `windows_open_file_timeout` governs how long rotated files stay open — a writer cannot delete a file the agent holds. Registry under `C:\ProgramData\Datadog\run`.
1. **Docker (containerized host agent)**: container launcher logs containers; needs `/var/lib/docker/containers` mounted for file mode (preferred) or `/var/run/docker.sock` for socket mode, plus a persistent mount for the registry.
1. **Kubernetes DaemonSet**: logs pods from `/var/log/pods` (hostPath) with the CRI or Docker parser; pod annotations drive per-container config through AD; `container_collect_all` typically enabled. `k8s_container_use_kubelet_api: true` streams from the kubelet instead, requiring no hostPath. The [Cluster Agent](../containers/cluster-agent.md) does not run a logs-agent — log collection is strictly node-local.
1. **ECS Fargate**: no file access to container logs, so API-style tailing over the runtime abstractions; `atomic_registry_write` defaults to false.
1. **Serverless (Lambda extension)**: a separate build ([`agent_serverless_init.go`](<<<SRC>>>/comp/logs/agent/impl/agent_serverless_init.go), `//go:build serverless`) with no AD and no auditor; only the channel and file launchers, a dedicated JSON encoder, no destination retries, and a synchronous `Flush()` at the end of each invocation that blocks until every payload reaches the destination.

The same `comp/logs-library` machinery also underpins the [event platform forwarder](event-platform.md) in every agent flavor: eventType-keyed "passthrough pipelines" (dbm-samples, network-devices-metadata, netflow, ...) each with their own config-key prefix, HTTP-only. (CWS events also reuse this machinery, but through their own dedicated sender rather than the event platform forwarder — see [Workload Protection](../ebpf/cws.md).) The general-purpose metrics [forwarder](forwarder.md) is a separate, unrelated implementation.

## Ports and IPC

| Direction | Endpoint | Notes |
|---|---|---|
| outbound | `agent-http-intake.logs.<site>:443` (HTTPS) | default transport, zstd-compressed batches |
| outbound | `agent-intake.logs.<site>:10516` (TCP+TLS) | fallback transport (EU: 443); optional SOCKS5 proxy |
| outbound | OPW / MRF intakes | when configured |
| inbound | arbitrary TCP/UDP ports | one per `type: tcp`/`type: udp` source |
| local | `POST /agent/stream-logs` on `cmd_port` (default 5001) | IPC-authenticated, used by `agent stream-logs` |
| local reads | `/var/run/docker.sock`, kubelet `:10250`, journald, Windows Event Log API | tailing inputs |

The tagger, workloadmeta, and AD are consumed in-process in the core Agent; see [inter-process communication](../processes/ipc.md) for the general IPC model.

## Troubleshooting surfaces

`agent stream-logs` ([`command.go`](<<<SRC>>>/cmd/agent/subcommands/streamlogs/command.go)) POSTs to the `/agent/stream-logs` IPC endpoint, which enables the [`BufferedMessageReceiver`](<<<SRC>>>/comp/logs-library/diagnostic/message_receiver.go); every processed message (post-redaction, rendered) is then mirrored to the HTTP response, with optional filters by name, type, source, or service. Only one streamer can be active at a time. The [`comp/logs/streamlogs`](<<<SRC>>>/comp/logs/streamlogs/impl/streamlogs.go) component additionally lets remote-config tasks and [flares](../operations/flare.md) capture a fixed duration of stream-logs output to a file.

The "Logs Agent" section of `agent status` shows per-source status and errors, bytes read, average latency, invalid-processing-rule warnings, and the current transport (also reported to inventory metadata as `logs_transport`). Telemetry from [`comp/logs-library/metrics`](<<<SRC>>>/comp/logs-library/metrics) includes expvars (`LogsDecoded/Processed/Sent`, `DestinationErrors`, `BytesSent`, `RetryCount`, ...) and Prometheus series (`logs_sender.*`, `logs_client_http_destination.*`, capacity/utilization from the `PipelineMonitor`). See [status, health, and telemetry](../operations/introspection.md) for the general introspection stack.

## Gotchas

1. **Ordering is per-source, not global**: a tailer sticks to one pipeline, but the HTTP worker pool sends payloads concurrently, so wire ordering across sources is not guaranteed; `pipeline_failover.enabled` relaxes even per-source ordering. The auditor guards offsets with `IngestionTimestamp` comparisons.
1. **Backpressure is by design**: when all reliable destinations are down, channels fill and tailers stop reading rather than dropping data. Unreliable additional endpoints never block.
1. **Registry stickiness**: switching a container between file and socket tailing is deliberately sticky through the registry to avoid duplicating logs; a stored socket offset keeps socket mode.
1. **Auto-multiline v2 aggregates by default** — `auto_multi_line_detection` defaults to true, so detected multiline groups are combined into single messages; setting it to false falls back to tag-only labeling (`auto_multi_line_detection_tagging`), and the legacy v1 handler remains selectable via `force_auto_multi_line_detection_v1`.
1. **A global `multi_line` processing rule is invalid** — it only works per-source; the agent warns and surfaces it in status.
1. **`expected_tags_duration` confusion**: host tags are baked into each log only for the configured duration after startup, a common source of "tags disappeared after ten minutes" reports.
1. **Fingerprint mismatch resets the offset**: if the file at a registered path no longer matches its stored fingerprint, tailing restarts from the beginning — correct for replaced files, surprising if fingerprint config changes.
1. **403 handling depends on the key's origin**: secrets-backend keys trigger a refresh and retry; plain-config 403s (and 400/401/413) drop the payload permanently, counted in `logs_client_http_destination` telemetry.
