# Logs Pipeline: Architecture & Dependency Findings
**Commit**: 8ff8f30e10b  
**Analyst focuses**: Focus 1 (Architecture & Data Flow) + Focus 9 (External Dependencies & Integration Points)

---

## Areas Examined

- `pkg/logs/` — all launchers, tailers, sources, schedulers, message, decoder, framer, parsers
- `comp/logs-library/` — pipeline, processor, sender, client (HTTP + TCP)
- `comp/logs/auditor/` — registry implementation
- `comp/logs/agent/config/` — endpoints, config keys
- `pkg/config/setup/` — default values for all channel/buffer/timeout settings

---

## Focus 1 — Architecture & Data Flow

### Component Topology

```
Autodiscovery
    └─> AD Scheduler (pkg/logs/schedulers/ad)
            └─> LogSources / LogServices stores
                    └─> Launchers (one goroutine each)
                            file, container, journald, listener(TCP/UDP), channel, integration, windowsevent
                                └─> Tailers (one goroutine pair each: readForever + forwardMessages + decoder)
                                        └─> outputChan (chan *message.Message, size = logs_config.message_channel_size = 100)
                                                └─> Pipeline.InputChan (chan *message.Message, same size 100)
                                                        └─> Processor (applies processingRules, encoder)
                                                                └─> strategyInput (chan *message.Message, same size 100)
                                                                        └─> Strategy (batch or stream)
                                                                                └─> Sender.queues[n] (chan *message.Payload, size = logs_config.payload_channel_size = 10)
                                                                                        └─> Worker.run()
                                                                                                └─> DestinationSender.input (chan *message.Payload, bufferSize = payload_channel_size = 10)
                                                                                                        └─> Destination.run() [HTTP or TCP]
                                                                                                                └─> output (auditor.Channel(), chan *message.Payload, size = message_channel_size = 100)
                                                                                                                        └─> Auditor (updates registry.json on disk)
```

### Exact Channel Declarations

| Channel | Location | Buffer Size | Notes |
|---|---|---|---|
| `Pipeline.InputChan` | `comp/logs-library/pipeline/pipeline.go:62` | `logs_config.message_channel_size` (default 100) | Tailers write here |
| `strategyInput` (processor → strategy) | `pipeline.go:45` | same 100 | |
| `Sender.queues[n]` | `sender/sender.go:136` | `workersPerQueue` (= 1 in non-legacy HTTP) | Small, payloads are large |
| `DestinationSender.input` | `sender/destination_sender.go:34` | `bufferSize` = `logs_config.payload_channel_size` (default 10) | Per-destination buffer |
| `Auditor.inputChan` | `auditor/impl/auditor.go:166` | `logs_config.message_channel_size` (default 100) | Auditor side of output |
| `flushRequestChan` | `auditor.go:167` | unbuffered | Synchronous flush |

### Pipeline Count & Pinning

- Number of pipelines is configured by `logs_config.pipelines` (undiscovered default, typically 4).
- Each tailer calls `pipelineProvider.NextPipelineChan()` or `NextPipelineChanWithMonitor()` once, and the returned channel is fixed for the tailer's lifetime — inputs are pinned to a pipeline.
- Round-robin assignment via atomic counter (`currentPipelineIndex.Inc() % pipelinesLen`, `provider.go:314`).
- Optional pipeline failover: `logs_config.pipeline_failover.enabled` (default false). When enabled, inserts a router layer with `routerChannels` (size = `logs_config.pipeline_failover.router_channel_size`, default 5) and a `forwardWithFailover` goroutine per pipeline (`provider.go:350–366`).

### Message Flow End-to-End (File Tailer, HTTP)

1. **readForever** (`tailer.go:343`): polls file at `DefaultSleepDuration` (1s), reads 4096-byte chunks, pushes raw bytes to `decoder.InputChan` (unbuffered pipe inside decoder).
2. **Decoder** (`internal/decoder/decoder.go`): framing + parsing → emits `decoder.Message` on `OutputChan`.
3. **forwardMessages** (`tailer.go:392`): constructs `message.Message` with offset/fingerprint/tags, sends to `outputChan` (blocking, cancellable via `forwardContext`).
4. `outputChan` == `Pipeline.InputChan` (directly in non-failover mode).
5. **Processor** (`processor/processor.go`): applies processing rules (include/exclude regexes, masking, multiline), encodes (JSON/proto/raw), sends to `strategyInput`.
6. **BatchStrategy** (`sender/batch_strategy.go`): accumulates to batch, flushes on `batchWait` ticker (default 5s), max 1000 messages or 5 MB uncompressed.
7. **Sender/Worker** (`sender/worker.go`): distributes `Payload` to `DestinationSender.input`. Blocking on primary reliable destination. Falls back to `NonBlockingSend` for secondaries.
8. **DestinationSender** (`sender/destination_sender.go`): `Send()` blocks if destination retrying. When destination transitions to retrying, `cancelSendChan` unblocks the waiting caller.
9. **HTTP Destination** (`client/http/destination.go`): exponential backoff retry (`sendAndRetry`). Worker pool scales dynamically (EWMA of latency vs. target 150ms). Drops on 400/401/403/413. Retries on 5xx and network errors.
10. **Auditor** (`auditor/impl/auditor.go`): receives completed payloads on its input channel; updates in-memory registry; flushes registry.json to disk every 1s; clean up stale entries every 300s.

### Serialization / Compression Boundaries

- **HTTP path**: `BatchStrategy` accumulates unencoded messages; `batch.sendMessages()` compresses the whole batch (gzip or zstd, configurable per-endpoint). Default is no compression unless `logs_config.use_compression: true`.
- **TCP path**: `StreamStrategy` sends one message per payload; no compression.
- Encoding (JSON wrapping + hostname/service metadata) happens in `Processor` before strategy.
- Content-Encoding header set from `payload.Encoding` in `http/destination.go:363`.

---

## Focus 9 — External Dependencies & Integration Points

### 1. Files on Disk & Rotation

**Dependency**: `afero.File` (wraps `os.File`) in `pkg/logs/tailers/file/tailer.go:66`.

**Poll loop**: `tailer_nix.go:50` — reads 4096 bytes at a time, returns `io.EOF` on empty (no error). Sleeps `tailerSleepDuration` (default 1s, `launchers/file/launcher.go:38`).

**Rotation detection** (`tailer/file/rotate_nix.go:25`):
- Opens the path again, compares `os.SameFile()` (inode). Also checks size < offset (truncation).
- Called from file launcher's scan loop (period = `logs_config.file_scan_period`, typically 10s).
- On rotation: `StopAfterFileRotation()` (`tailer.go:308`) — starts a timer goroutine that waits `logs_config.close_timeout` (default 60s), then cancels `forwardContext` and stops the tailer.
- **DATA LOSS CONDITION** (`tailer.go:326`): if `forwardContext` is cancelled while `forwardMessages` is blocked on a full `outputChan`, messages are silently discarded (`case <-t.forwardContext.Done()`). Remaining unread bytes logged as `BytesMissed` metric.

**File open limit**: 200 (Linux) / 500 (other), `pkg/config/setup/common_settings.go:1795–1799`.

**Failure modes**:
- File deleted before open → `setup()` returns error; source marked error; tailer not started.
- File deleted during tailing → next `read()` gets OS error → tailer logs error and stops.
- `stat` fails during rotation check → treated as rotated (`rotate_nix.go:38`).

### 2. Docker API Socket

**Dependency**: Docker client in `pkg/logs/launchers/container/tailerfactory/socket.go:43`. Built tag: `//go:build docker`.

**How obtained**: `dockerutilPkg.GetDockerUtil()` — returns a singleton; fails if Docker socket unavailable.

**Failure handling** (`socket.go:52–57`): if `GetDockerUtil()` fails, returns error from `makeSocketTailer` → fallback to file-based tailing (whichTailer logic in `tailerfactory/whichtailer.go`).

**Read timeout**: `logs_config.docker_client_read_timeout` (in seconds). Passed to `tailers.NewDockerSocketTailer`.

**Failure modes**:
- Socket unavailable at startup → falls back to file tailing.
- Socket disappears mid-tailing → read on Docker log stream returns error → tailer goroutine exits; no automatic restart (launcher must detect and restart via scan loop).

### 3. Journald

**Dependency**: `go-systemd/v22/sdjournal` via `//go:build systemd` tag (`pkg/logs/tailers/journald/tailer.go:6`).

**Journal object**: passed in from `pkg/logs/launchers/journald/launcher.go:151` via `journal_factory.go`.

**Poll**: `tail()` calls `journal.Next()` in a loop, waits `defaultWaitDuration` (1s) between polls when no new entries.

**Failure modes**:
- `journal.AddMatch` / `journal.SeekHead` fail → `Start()` returns error; source marked error.
- Journal rotated externally → `Next()` should still work as journald manages the cursor.
- No explicit reconnect logic if journal connection drops; `tail()` loop will return on error.

### 4. TCP/UDP Listener

**Dependency**: local OS socket, bound to configured port.

**Listener**: `pkg/logs/launchers/listener/tcp.go`. Each accepted connection spawns a stream tailer goroutine.

**Failure modes**:
- Port already in use → `net.Listen` fails; error logged; source marked error; no retry.
- Client disconnects → stream tailer's `Read` returns EOF → tailer exits cleanly.

### 5. Datadog HTTP Intake

**Client construction** (`client/http/destination.go:449–480`):
- `httpClientFactory` wraps `http.Client` with `Timeout = logs_config.http_timeout` (default 10s).
- Transport: `httputils.CreateHTTPTransport` with HTTP/2 or HTTP/1 depending on `logs_config.http_protocol` (default auto = H2).
- `ResetClient` wraps it: connection reset logic if `ConnectionResetInterval > 0` (default 0 = disabled).

**URL construction** (`http/destination.go:485–507`):
- EPv2 with TrackType: `https://<host>/api/v2/<tracktype>`
- EPv1 fallback: `https://<host>/v1/input`
- Default intake host: `agent-intake.logs.datadoghq.com` (set in `config.go:setupFipsLogsConfig` or similar).

**Retry policy** (`client/http/destination.go:263–320`):
- Network errors → `RetryableError` → `updateRetryState(err, isRetrying)` → signals `isRetrying <- true` to `DestinationSender` via buffered channel.
- `DestinationSender.Send()` unblocks when `isRetrying` transitions to true (`cancelSendChan` fires).
- Exponential backoff: factor=2, base=1s, max=120s, recovery every 2 errors (`DefaultLogsSenderBackoffFactor/Base/Max`, `config.go:123-133`).
- HTTP 400, 401, 403 (unless API key refresh), 413 → non-retryable `errClient` → logs dropped, counted in `DestinationLogsDropped`.
- HTTP 403 with secrets backend → triggers `secrets.Refresh()` then retries.
- HTTP 5xx → `RetryableError` → infinite retry with backoff.
- `context.Canceled` (shutdown) → stops without retry.

**Concurrency model** (`client/http/worker_pool.go`):
- Dynamic worker pool, EWMA of send latency. Target latency 150ms.
- min=`numberOfPipelines`, max=`numberOfPipelines × 10` (for non-legacy, non-serverless).
- On retryable error: `shouldBackoff=true` → reset virtual latency to 0 and drop to minWorkers.

**TCP Intake**:
- `client/tcp/connection_manager.go:58`: blocks until connection succeeds (no limit on retries).
- Backoff: exponential, max 2^7 seconds (~128s), with jitter (`backoff()`, `connection_manager.go:187`).
- SSL: TLS handshake with 20s timeout.
- Failure: `Write` error → closes conn, signals retry, loops to reconnect.

### 6. Auditor Registry (Local Disk)

**Dependency**: `registry.json` on local filesystem at `logs_config.run_path`.

**Write cadence**: flush every 1s (`defaultFlushPeriod`, `auditor.go:26`); also on Stop and on-demand Flush.

**Failure modes**:
- Permission error on write → logged once (fileError sync.Once), continues in-memory tracking.
- Disk full → `flushRegistry` returns error, logged, in-memory state preserved.
- Missing on startup → starts fresh (no error), tailers resume from defaults (`recoverRegistry`, `auditor.go:336`).
- Corrupt on startup → JSON unmarshal fails → starts fresh; offset history lost → potential re-reads / duplicates.
- Read after restart → offset restored; tailer replays from that offset; at-least-once delivery guarantee.

### 7. Autodiscovery Integration

**Dependency**: Datadog AD system (Kubernetes API server, Docker API, local config).

**Path**: AD Scheduler (`comp/logs/adscheduler`) subscribes to AD events and calls `LogSources.AddSource()` / `RemoveSource()`, which feeds into launcher subscription channels (`SubscribeForType`).

**Failure modes**:
- AD integration config references unavailable container → launcher tries to start tailer; fails; source marked error; retried on next scan.
- AD config change while tailer running → launcher receives on `removedSources` channel, stops tailer, receives `addedSources` for new config, starts new tailer.

---

## Key Configuration Defaults Summary

| Setting | Default | Effect |
|---|---|---|
| `logs_config.message_channel_size` | 100 | Pipeline.InputChan buffer size |
| `logs_config.payload_channel_size` | 10 | DestinationSender.input buffer size |
| `logs_config.batch_wait` | 5s | HTTP batch flush interval |
| `logs_config.batch_max_size` | 1000 | Max messages per HTTP batch |
| `logs_config.batch_max_content_size` | 5MB | Max bytes per HTTP batch |
| `logs_config.http_timeout` | 10s | HTTP client request timeout |
| `logs_config.close_timeout` | 60s | Time after rotation before tailer forced-stop |
| `logs_config.open_files_limit` | 200/500 | Max simultaneously tailed files |
| `logs_config.sender_backoff_max` | 120s | Max HTTP retry backoff |
| `logs_config.connection_reset_interval` | 0 (disabled) | TCP connection reset interval |
| `logs_config.pipeline_failover.router_channel_size` | 5 | Inter-pipeline router buffer (only if failover enabled) |

---

## Data Drop Points (Enumerated)

1. **Rotation close timeout** (`tailer.go:327`): if pipeline is backed up and `close_timeout` expires, `forwardContext` cancelled, remaining messages in decoder discarded. Metric: `BytesMissed`.
2. **NonBlockingSend failure for secondary destinations** (`worker.go:160, 175`): if `DestinationSender.input` is full, payload is silently dropped. Metrics: `logs_sender.payloads_dropped`, `logs_sender.messages_dropped`.
3. **HTTP non-retryable response** (`http/destination.go:413–419`): 400/401/403/413 → payload dropped. Metric: `DestinationLogsDropped`, `logs_client_http_destination.payloads_dropped`.
4. **Message too large for batch** (`sender/batch.go:117`): single message > `batch_max_content_size` → dropped with warning. Metric: `logs_sender_batch_strategy.dropped_too_large`.
5. **Stream encoding failure** (`stream_strategy.go:43`): compression error → payload dropped, strategy goroutine returns (silent shutdown!).
6. **Worker NonBlockingSend to secondary when primary is stuck** (`worker.go:160`): already covered by #2.

---

## Assumptions & Open Questions

1. **Number of pipelines**: The default value for `logs_config.pipelines` was not found in this grep pass. Likely set elsewhere (possibly agent entrypoint). Needs verification.
2. **File tailer decoder InputChan buffer**: The decoder's internal `InputChan` buffer size was not confirmed; `decoder.go` was not fully read. If unbuffered, `readForever` blocks on full pipeline immediately.
3. **DestinationsContext lifetime**: `Start()` / `Stop()` on `DestinationsContext` control the context passed to all HTTP requests. On agent shutdown, `Stop()` cancels all in-flight HTTP requests. The ordering of `Stop()` vs. draining pipeline channels is not fully verified.
4. **Worker pool scaling on shutdown**: when `Stop()` is called on the worker, it sends to `done` channel and waits for `finished`. In-flight HTTP sends (in worker pool goroutines) are not interrupted — they run to completion or until the HTTP client context (from `DestinationsContext`) is cancelled.
5. **TCP destination reconnect**: `NewConnection` retries indefinitely, blocking the goroutine. If intake is permanently unavailable, the TCP destination blocks the entire `inputChan`, which backs up through the pipeline all the way to the file tailer.
6. **Adaptive sampling (preprocessor/sampler.go)**: not examined in detail. May be an additional drop point for high-frequency log sources.
7. **MRF (Multi-Region Failover)** destinations exist (`IsMRF`, `isMRFAllow` on messages): these are dropped unless `multi_region_failover.enabled` + `multi_region_failover.failover_logs` are set. Not tested in default config.
