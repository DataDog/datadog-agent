# Telemetry Metrics Reference - Logs Sender

This document maps the logs sender metrics to the current implementation in
`pkg/logs`. The important caveat is that the "shared" counters are not fully
normalized across HTTP, gRPC, and TCP: both the bytes counted and the point
where the counters are incremented differ by transport.

For benchmarking, assume "clean run" means:

- no retries
- no send failures
- no drops
- fixed gRPC stream lifetime

Without those conditions, some counters stop being comparable.

---

## 0. Architecture Overview

### 0.1 SMP Benchmarking Setup

There are three processes in an SMP experiment. Lading plays two roles:
load **generator** (writes log files) and **blackhole** (receives what the
agent sends). The agent sits in the middle.

```text
┌─────────────────────────────────────────────────────────────────────────┐
│                         LADING PROCESS                                  │
│                                                                         │
│  ┌───────────────┐                            ┌───────────────────────┐ │
│  │  Generator     │    log files on disk       │  Blackhole(s)         │ │
│  │  (file_gen)    │ ──────────────────────┐    │                       │ │
│  │                │                       │    │  ┌─────────────────┐  │ │
│  │  Writes logs   │                       │    │  │ gRPC blackhole  │  │ │
│  │  to /smp-shared│                       │    │  │ :9092           │  │ │
│  └───────────────┘                       │    │  └─────────────────┘  │ │
│         │                                │    │  ┌─────────────────┐  │ │
│         │ bytes_written                  │    │  │ HTTP blackhole  │  │ │
│         │ missed_bytes                   │    │  │ :19092          │  │ │
│         ▼                                │    │  └─────────────────┘  │ │
│  ┌───────────────┐                       │    └───────────────────────┘ │
│  │ Prometheus     │                       │              ▲               │
│  │ metrics export │                       │              │               │
│  └───────────────┘                       │              │               │
└─────────────────────────────────────────────────────────────────────────┘
                                            │              │
                               ┌────────────┘              │
                               │                           │
                               ▼                           │
┌──────────────────────────────────────────────────────────┘──────────────┐
│                      DATADOG AGENT PROCESS                              │
│                                                                         │
│  ┌─────────────┐   ┌──────────────┐   ┌──────────────┐   ┌──────────┐ │
│  │ File Tailer  │──▶│ Log Pipeline │──▶│ Batch Strategy│──▶│ Sender   │ │
│  │              │   │ (decode,     │   │ (serialize,  │   │ (HTTP or │ │
│  │ Reads files  │   │  process,    │   │  compress)   │   │  gRPC)   │ │
│  │ from disk    │   │  enrich)     │   │              │   │          │─┼──▶ to blackhole
│  └─────────────┘   └──────────────┘   └──────────────┘   └──────────┘ │
│         │                  │                  │                │        │
│         │           LogsProcessed      UnencodedSize     LogsSent      │
│         │           LogsDecoded        EncodedBytesSent  BytesSent     │
│         │                              (+ gRPC-specific)               │
│         │                                                              │
│  ┌──────────────────────────────────────────────────────────────┐      │
│  │ /telemetry  (Prometheus endpoint, port 5000)                 │      │
│  │ All agent telemetry counters/gauges exposed here             │      │
│  └──────────────────────────────────────────────────────────────┘      │
└────────────────────────────────────────────────────────────────────────┘
                               │
                               │  Lading scrapes :5000/telemetry
                               ▼
┌────────────────────────────────────────────────────────────────────────┐
│                      SMP CAPTURE PIPELINE                              │
│                                                                        │
│  Lading collects:                                                      │
│  1. Its own generator metrics     (bytes_written, missed_bytes, ...)   │
│  2. Its own blackhole metrics     (bytes_received, batches_received)   │
│  3. Agent telemetry via Prometheus (datadog__logs__sent, ...)          │
│                                                                        │
│  All become: single_machine_performance.regression_detector.capture.*  │
└────────────────────────────────────────────────────────────────────────┘
```

### 0.2 Where Each Metric Is Measured

This diagram shows exactly where in the data flow each metric is captured.
Read left-to-right as bytes flowing from generation to receipt.

```text
  LADING GENERATOR              AGENT                           LADING BLACKHOLE
  ════════════════     ═══════════════════════                  ══════════════════

  Log file on disk     Tailer    Pipeline   BatchStrategy  Sender       Blackhole
       │                 │          │            │            │              │
       │                 │          │            │            │              │
 bytes_written ●         │          │            │            │              │
       │                 │          │            │            │              │
       ├─── read ───────▶│          │            │            │              │
       │                 │          │            │            │              │
       │            (strip \n,      │            │            │              │
       │             trim ws)       │            │            │              │
       │                 │          │            │            │              │
       │                 ├────▶ LogsDecoded      │            │              │
       │                 │     LogsProcessed     │            │              │
       │                 │          │            │            │              │
       │                 │          ├───────────▶│            │              │
       │                 │          │            │            │              │
       │                 │          │     UnencodedSize ●     │              │
       │                 │          │    (= sum of RawDataLen │              │
       │                 │          │     for gRPC, or JSON   │              │
       │                 │          │     body size for HTTP)  │              │
       │                 │          │            │            │              │
       │                 │          │     serialize + compress │              │
       │                 │          │            │            │              │
       │                 │          │     payload.Encoded ●    │              │
       │                 │          │    (compressed bytes)    │              │
       │                 │          │            │            │              │
       │                 │          │            ├───────────▶│              │
       │                 │          │            │            │              │
       │                 │          │            │     ┌──────┤              │
       │                 │          │            │     │ HTTP: send request  │
       │                 │          │            │     │ gRPC: stream.Send() │
       │                 │          │            │     └──────┤              │
       │                 │          │            │            ├── on wire ──▶│
       │                 │          │            │            │              │
       │                 │          │            │  ┌── on success/ack ──┐  │
       │                 │          │            │  │                    │  │
       │                 │          │            │  │  LogsSent ●        │  │
       │                 │          │            │  │  BytesSent ●       │  │
       │                 │          │            │  │  EncodedBytesSent ●│  │
       │                 │          │            │  │                    │  │
       │                 │          │            │  │  (gRPC-specific):  │  │
       │                 │          │            │  │  worker.bytes_sent ●  │
       │                 │          │            │  │  pattern_logs ●    │  │
       │                 │          │            │  └────────────────────┘  │
       │                 │          │            │            │              │
       │                 │          │            │            │     bytes_received ●
       │                 │          │            │            │     batches_received ●
       │                 │          │            │            │     data_items_received ●
       │                 │          │            │            │     decoded_bytes_received ●
       │                 │          │            │            │       (HTTP only)
       │                 │          │            │            │              │

  ● = metric measurement point
```

### 0.3 gRPC Path vs HTTP Path

The agent uses different batch strategies and senders depending on transport.
Here is how bytes transform through each path:

```text
  ┌────────────────────────── gRPC PATH ──────────────────────────────┐
  │                                                                    │
  │  Raw log line  ("2025-01-01 ERROR something broke")                │
  │       │                                                            │
  │       ▼                                                            │
  │  MessageTranslator (tokenize, pattern-match, create Datum proto)   │
  │       │                                                            │
  │       │  ● pattern_logs / pattern_logs_bytes                       │
  │       │    (proto.Size of each Datum, before delta encoding)       │
  │       ▼                                                            │
  │  Batch Strategy (collect Datums into DatumSequence)                │
  │       │                                                            │
  │       │  UnencodedSize = sum(RawDataLen)                           │
  │       │  (raw log content only — no proto overhead)                │
  │       ▼                                                            │
  │  proto.Marshal(DatumSequence)                                      │
  │       │                                                            │
  │       │  [no metric for this size today]                           │
  │       ▼                                                            │
  │  compress(serialized) → payload.Encoded                            │
  │       │                                                            │
  │       │  ● logs.encoded_bytes_sent  (on ack)                       │
  │       ▼                                                            │
  │  Wrap as StatefulBatch { batch_id, data: payload.Encoded }         │
  │       │                                                            │
  │       │  ● logs_sender_grpc_worker.bytes_sent (proto.Size)         │
  │       ▼                                                            │
  │  stream.Send(StatefulBatch)  ──────▶  gRPC blackhole               │
  │                                        ● bytes_received            │
  │                                          (batch.encoded_len())     │
  │                                        ● batches_received          │
  │                                        ● data_items_received       │
  └────────────────────────────────────────────────────────────────────┘

  ┌────────────────────────── HTTP PATH ──────────────────────────────┐
  │                                                                    │
  │  Raw log line  ("2025-01-01 ERROR something broke")                │
  │       │                                                            │
  │       ▼                                                            │
  │  Batch Strategy (write JSON array into compressor)                 │
  │       │                                                            │
  │       │  UnencodedSize = writerCounter.getWrittenBytes()           │
  │       │  (JSON body written into compressor, includes framing)     │
  │       ▼                                                            │
  │  compressor.Close() → payload.Encoded                              │
  │       │                                                            │
  │       │  ● logs.encoded_bytes_sent  (on send attempt)              │
  │       │  ● logs.bytes_sent          (on send attempt)              │
  │       ▼                                                            │
  │  HTTP POST  ───────────────────────▶  HTTP blackhole               │
  │                                        ● bytes_received            │
  │                                          (body.len(), compressed)  │
  │                                        ● decoded_bytes_received    │
  │                                          (after decompression)     │
  │                                        ● requests_received         │
  └────────────────────────────────────────────────────────────────────┘
```

### 0.4 What "Bytes" Means at Each Stage

```text
  Stage              gRPC "bytes"                     HTTP "bytes"
  ─────              ────────────                     ────────────

  bytes_written      Bytes lading wrote to disk        Same
  (lading)           (log lines + newlines)

  UnencodedSize      sum(RawDataLen) per message       JSON body before
  (agent)            = raw log content only             compression
                     NO proto overhead                  = includes JSON
                     NO newlines                        framing [{},{}]
                                                        NO newlines

  payload.Encoded    compress(proto.Marshal(            compress(JSON body)
  (agent)            DatumSequence))

  worker.bytes_sent  proto.Size(StatefulBatch)          N/A
  (agent, gRPC only) = Encoded + batch_id wrapper

  bytes_received     batch.encoded_len()                body.len()
  (lading blackhole) ≈ proto size of StatefulBatch      = compressed HTTP body
                     (data field retains compressed      = what arrived on wire
                      DatumSequence; ~2-6 byte
                      envelope overhead vs HTTP)

  decoded_bytes      N/A (does not exist)               body after decompression
  _received
  (lading blackhole)
```

---

## 1. Shared Agent-Level Metrics

Defined in `pkg/logs/metrics/metrics.go`.

### 1.1 `logs.sent`

| Field | Value |
|-------|-------|
| expvar | `LogsSent` |
| telemetry | `TlmLogsSent` |

What it means | Depends on transport/path; see table below

| Transport / path | When incremented | Value added | What it really means |
|------------------|------------------|-------------|----------------------|
| HTTP destination (`client/http/destination.go`) | After `unconditionalSend()` returns `nil` | `payload.Count()` | Successfully sent messages on the normal HTTP path |
| HTTP sync destination (`client/http/sync_destination.go`) | After every `unconditionalSend()` call, even if it returned an error | `payload.Count()` | Attempted messages on the serverless/sync HTTP path |
| gRPC (`sender/grpc/stream_worker.go`) | When the matching `BatchStatus` ack is received for a regular batch (`batch_id > 0`) | `payload.Count()` | Acked messages |
| TCP (`client/tcp/destination.go`) | After `conn.Write(frame)` succeeds | `1` | Successfully written payloads; in the normal TCP path each payload contains exactly one message because TCP uses `streamStrategy` |

### 1.2 `logs.bytes_sent`

| Field | Value |
|-------|-------|
| expvar | `BytesSent` |
| telemetry | `TlmBytesSent` (tag: `source`) |
| What it means | `Payload.UnencodedSize`; the meaning of that field depends on transport |

| Transport / path | When incremented | How `Payload.UnencodedSize` is computed | What it really measures |
|------------------|------------------|----------------------------------------|--------------------------|
| HTTP | At the start of `unconditionalSend()`, before the request is sent; each retry adds again | `writerCounter.getWrittenBytes()` in `sender/batch.go` | JSON request body size before compression; excludes HTTP headers |
| gRPC | On ack | `sum(msgMeta.RawDataLen)` in `sender/grpc/batch_strategy.go` | Original message bytes from metadata before protobuf serialization; not the protobuf size |
| TCP | After `conn.Write(frame)` succeeds | `len(msg.GetContent())` in `sender/stream_strategy.go` | Single-message content bytes before API-key prefixing and delimiter framing |

Notes:

- HTTP is attempt-based. Retries double-count `logs.bytes_sent`.
- gRPC is ack-based.
- TCP is success-on-write based.

### 1.3 `logs.encoded_bytes_sent`

| Field | Value |
|-------|-------|
| expvar | `EncodedBytesSent` |
| telemetry | `TlmEncodedBytesSent` (tags: `source`, `compression_kind`) |
| What it means | `len(payload.Encoded)`; this is encoded application-payload bytes, not true on-wire bytes |

| Transport / path | When incremented | What `payload.Encoded` contains | `compression_kind` tag | What is excluded |
|------------------|------------------|---------------------------------|------------------------|------------------|
| HTTP | At the start of `unconditionalSend()`, before the request is sent; each retry adds again | Compressed JSON request body | Actual configured compression kind, or `none` | HTTP headers and transport framing |
| gRPC | On ack for regular batches only | `compress(proto.Marshal(DatumSequence))` | Hardcoded to `grpc` | `StatefulBatch` wrapper, snapshot batch `0`, gRPC framing, HTTP/2 framing |
| TCP | After `conn.Write(frame)` succeeds | Raw message content (TCP uses no compression in the normal path) | `none` | API-key prefix and delimiter framing |

Note: the gRPC `compression_kind` tag is hardcoded to `"grpc"` (the transport
name), not the actual compression algorithm (`zstd`). This is a naming oddity
in the current implementation — the tag does not match the semantics used by
HTTP where it is the real algorithm name.

This metric is the best shared size metric currently available, but it is not a
wire-byte metric.

### 1.4 `logs.pre_compression_bytes_sent`

| Field | Value |
|-------|-------|
| expvar | `PreCompressionBytesSent` |
| telemetry | `TlmPreCompressionBytesSent` (tag: `source`) |
| What it means | `len(proto.Marshal(DatumSequence))` before compression |

| Transport / path | When incremented | What it measures | What is excluded |
|------------------|------------------|------------------|------------------|
| gRPC | On ack for regular batches only | Serialized `DatumSequence` bytes before `compress(serialized)` | `StatefulBatch` wrapper, snapshot batch `0`, gRPC framing, HTTP/2 framing |

This metric is currently gRPC-only. It exists to separate "stateful encoding
produced X bytes of protobuf" from "compression reduced that protobuf to Y
bytes". Use `logs.encoded_bytes_sent / logs.pre_compression_bytes_sent` when
you want the true gRPC compression ratio for regular batches.

### 1.5 Other shared metrics

| Metric | Type | Current coverage |
|--------|------|------------------|
| `logs.network_errors` | counter | Updated by HTTP and TCP. gRPC does not currently feed this metric; use `logs_sender_grpc_worker.stream_errors` instead. |
| `logs.dropped` | counter (tag: `destination`) | Updated by HTTP and TCP. gRPC does not currently populate it. |
| `logs.retry_count` | counter | Updated by the HTTP retry loop only. |
| `logs.sender_latency` | histogram (ms) | Updated by HTTP only. gRPC has no equivalent today. |
| `logs.bytes_missed` | counter | Tailer-side metric; transport-independent. |

---

## 2. gRPC Pipeline-Level Metrics

Defined in `pkg/logs/sender/grpc/state_telemetry.go`.
Updated in `pkg/logs/sender/grpc/mock_state.go`.

All byte counters in this section use `proto.Size(datum)` at translation time.
For log datums, that happens before batch-level delta encoding in
`pkg/logs/sender/grpc/batch_strategy.go`.

### State gauge

| Metric | Type | Tags | Description |
|--------|------|------|-------------|
| `logs_sender_grpc.state_size_bytes` | gauge | `pipeline` | Running ledger that adds `proto.Size(PatternDefine/DictEntryDefine)` and subtracts `proto.Size(PatternDelete/DictEntryDelete)` as datums are emitted. This is not an exact measurement of the live snapshot size or Go heap usage. |

### Pattern counters

| Metric | Type | Tags | Description |
|--------|------|------|-------------|
| `logs_sender_grpc.patterns_added` | counter | `pipeline` | Number of `PatternDefine` datums emitted |
| `logs_sender_grpc.pattern_bytes_added` | counter | `pipeline` | `proto.Size()` of emitted `PatternDefine` datums |
| `logs_sender_grpc.patterns_removed` | counter | `pipeline` | Number of `PatternDelete` datums emitted |
| `logs_sender_grpc.pattern_bytes_removed` | counter | `pipeline` | `proto.Size()` of emitted `PatternDelete` datums |

### Dictionary counters

| Metric | Type | Tags | Description |
|--------|------|------|-------------|
| `logs_sender_grpc.tokens_added` | counter | `pipeline` | Number of `DictEntryDefine` datums emitted |
| `logs_sender_grpc.token_bytes_added` | counter | `pipeline` | `proto.Size()` of emitted `DictEntryDefine` datums |
| `logs_sender_grpc.tokens_removed` | counter | `pipeline` | Number of `DictEntryDelete` datums emitted |
| `logs_sender_grpc.token_bytes_removed` | counter | `pipeline` | `proto.Size()` of emitted `DictEntryDelete` datums |

### Log counters

| Metric | Type | Tags | Description |
|--------|------|------|-------------|
| `logs_sender_grpc.pattern_logs` | counter | `pipeline` | Number of structured logs processed by `MessageTranslator` |
| `logs_sender_grpc.pattern_logs_bytes` | counter | `pipeline` | `proto.Size()` of each `StructuredLog` datum before batch-level delta encoding mutates timestamp/pattern/tags |
| `logs_sender_grpc.raw_logs` | counter | `pipeline` | Dead code in the current implementation; `sendRawLog()` is never called |
| `logs_sender_grpc.raw_logs_bytes` | counter | `pipeline` | Dead code in the current implementation; `sendRawLog()` is never called |

---

## 3. gRPC Worker-Level Metrics

Defined in `pkg/logs/sender/grpc/state_telemetry.go`.
Updated in `pkg/logs/sender/grpc/stream_worker.go` and
`pkg/logs/sender/grpc/inflight.go`.

| Metric | Type | Tags | Description | Where updated |
|--------|------|------|-------------|---------------|
| `logs_sender_grpc_worker.streams_opened` | counter | `worker` | Incremented on every async stream creation attempt, before connection readiness and `LogsStream()` succeed | `asyncCreateNewStream()` |
| `logs_sender_grpc_worker.stream_errors` | counter | `worker`, `reason` | Incremented on stream creation failures, protocol invariants, and send/recv failures. See full reason list below. | `stream_worker.go` |
| `logs_sender_grpc_worker.bytes_sent` | counter | `worker` | `proto.Size(batch)` after `stream.Send(batch)` succeeds. This is the protobuf size of `StatefulBatch{batch_id,data}`; it includes the batch wrapper and snapshot batch `0`, but still excludes gRPC and HTTP/2 framing. | `senderLoop()` |
| `logs_sender_grpc_worker.bytes_dropped` | counter | `worker` | `len(payload.Encoded)` for regular payloads whose ack was received but whose auditor handoff was dropped because the output channel was full | `handleBatchAck()` |
| `logs_sender_grpc_worker.inflight_bytes` | gauge | `worker` | Sum of `len(payload.Encoded)` for regular payloads currently held in the inflight tracker (both sent-and-unacked and buffered-but-unsent) | `inflight.go` |

### `stream_errors` reason values

All possible `reason` tag values as of the current implementation:

| Reason | Trigger | Source |
|--------|---------|--------|
| `stream_creation_failed` | `ensureConnectionReady()` or `LogsStream()` failed during async stream creation | `asyncCreateNewStream()` |
| `received_ack_but_no_sent_payloads` | Server sent an ack but the inflight tracker has no sent payloads to match | `handleBatchAck()` |
| `batch_id_mismatch` | Server acked a batch ID that does not match the next expected ID | `handleBatchAck()` |
| `send_err_<CODE>` | `stream.Send()` failed; `<CODE>` is the gRPC status code (e.g. `UNAVAILABLE`, `CANCELLED`) | `senderLoop()` |
| `server_eof` | `stream.Recv()` returned `io.EOF` (server closed its send side) | receiver goroutine |
| `recv_error_<CODE>` | `stream.Recv()` returned a gRPC status error; `<CODE>` is the status code | receiver goroutine |
| `transport_error` | `stream.Recv()` returned a non-gRPC-status, non-EOF error | receiver goroutine |
| `irrecoverable_error` | An error classified as irrecoverable (e.g. auth failure) | `handleIrrecoverableError()` |

### Batch strategy metric

Defined in `pkg/logs/sender/grpc/batch_strategy.go`, not in `state_telemetry.go`:

| Metric | Type | Tags | Description |
|--------|------|------|-------------|
| `logs_sender_grpc_batch_strategy.dropped_too_large` | counter | `pipeline` | Payloads dropped because a single message exceeds the batch content size limit even after flushing |

### `bytes_sent` vs `encoded_bytes_sent`

These two metrics are close, but not the same:

```text
regular payload
  DatumSequence
    -> proto.Marshal(datumSeq)                  [no metric today]
    -> compress(serialized) = payload.Encoded
         -> logs.encoded_bytes_sent             [on ack; regular batches only]
         -> logs_sender_grpc_worker.inflight_bytes
    -> wrap as StatefulBatch{batch_id, data}
         -> proto.Size(batch)
              -> logs_sender_grpc_worker.bytes_sent
                 [after stream.Send(); includes wrapper, and also snapshot batch 0]
```

Use `logs_sender_grpc_worker.bytes_sent` when you want the gRPC-only byte metric
that is closest to what the worker hands to `stream.Send()`.

### Pipeline utilization and capacity metrics

The gRPC batch strategy and sender participate in the `PipelineMonitor` system
used by all logs pipeline components, defined in `pkg/logs/metrics/`.

**Utilization ratio** (from `UtilizationMonitor`):

| Metric | Type | Tags | Description |
|--------|------|------|-------------|
| `logs_component_utilization.ratio` | gauge | `name`, `instance` | EWMA of the fraction of wall-clock time a component spent doing work (0.0–1.0). Only the gRPC **batch strategy** creates a `UtilizationMonitor` (via `MakeUtilizationMonitor("strategy", instanceID)`). The gRPC sender and stream workers do **not** create one — there is no sender-side ratio for gRPC today. |

**Capacity gauges** (from `CapacityMonitor`):

| Metric | Type | Tags | Description |
|--------|------|------|-------------|
| `logs_component_utilization.items` | gauge | `name`, `instance` | EWMA of `ingress - egress` message count for a component. Updated on each `ReportComponentIngress` / `ReportComponentEgress` call, sampled once per second. |
| `logs_component_utilization.bytes` | gauge | `name`, `instance` | EWMA of `ingress - egress` bytes for a component. Bytes are `Payload.Size()` which returns `sum(MessageMetadata.RawDataLen)` — raw log content bytes, not encoded or compressed bytes. |

The gRPC batch strategy reports egress under `name=strategy` with the pipeline's
`instanceID`, and simultaneously reports ingress for the sender component under
`name=sender` with fixed `instance="0"` (`SenderTlmInstanceID` in
`pipeline_monitor.go`). Because of this fixed instance, the sender capacity
gauge does not distinguish between pipelines.

---

## 4. Cross-Transport Comparability

### First rule: only compare clean runs

HTTP byte counters are attempt-based and include retries. gRPC byte counters are
ack-based. TCP byte counters are success-on-write based. If retries or send
errors occur, cross-transport byte comparisons become misleading immediately.

### `logs.bytes_sent` is not comparable across transports

- HTTP measures JSON body bytes before compression.
- gRPC measures `RawDataLen` from message metadata, not protobuf bytes.
- TCP measures per-message content bytes before prefix/delimiter framing.

These numbers answer different questions.

### `logs.encoded_bytes_sent` is the best shared size metric, but it is still not true wire bytes

- HTTP: compressed JSON body only
- gRPC: compressed `DatumSequence` only
- TCP: message content only

Missing from that metric:

- HTTP headers
- TCP API-key prefix and delimiter bytes
- gRPC `StatefulBatch` wrapper
- gRPC snapshot batch `0`
- gRPC/HTTP2 framing

### Common baseline: Lading `bytes_written`

Lading's `bytes_written` is a Prometheus counter incremented immediately after
`write_all()` to disk in the file generator (both `logrotate` and
`traditional`). It counts the exact bytes written to the log files that the
agent tails. For `static_timestamped` variants, Lading parses the leading
timestamp token, strips it from the emitted output, and writes only the message
body plus newline delimiters. If `emit_placeholder: true` is enabled, empty
seconds are represented by a single placeholder newline, and that byte is also
included in `bytes_written`.

If you inspect Lading Prometheus output or `smp-playground` capture files
directly, the raw metric name is `bytes_written`.

For HTTP-vs-gRPC benchmarking, the best currently available normalized size
metric is:

```text
encoded_payload_efficiency = logs.encoded_bytes_sent / lading.bytes_written
```

Interpretation: "for every byte generated by the load generator, how many bytes
ended up in the sender's encoded payload objects?"

That is comparable across transports in clean runs. It is not exact wire
efficiency.

**Caveat: `lading.bytes_written` > `logs.bytes_sent` even before encoding.**
The agent's file tailer strips newlines and trims whitespace from each line
before setting `RawDataLen = len(content)`. So `logs.bytes_sent` is
systematically lower than `lading.bytes_written` by at least 1 byte per log
line (the `\n`), plus any leading/trailing whitespace. This gap is consistent
across transports, so the `encoded_payload_efficiency` ratio is still valid for
A/B comparison, but the absolute value will always be below 1.0 even with zero
compression.

### Lading `bytes_received` is comparable across transports

Unlike agent-side metrics, Lading's `bytes_received` measures the same
conceptual thing for both HTTP and gRPC: the size of the compressed
application payload, excluding transport framing. The gRPC blackhole's
`batch.encoded_len()` retains the compressed `DatumSequence` in the `data`
field unchanged (it is a `bytes` proto field, not decompressed by tonic),
adding only ~2–6 bytes of protobuf envelope overhead per batch. See §7.4
for the full code-level verification.

If you see a large ratio difference between:

```text
bytes_received (gRPC) / bytes_written   vs   bytes_received (HTTP) / bytes_written
```

the cause is the difference in **payload content** (stateful protocol
overhead from pattern/dict management datums), not a measurement mismatch.

### gRPC-only transport-side metric

If you want the gRPC metric that is closest to actual bytes handed to
`stream.Send()`, use:

```text
grpc_stream_efficiency = logs_sender_grpc_worker.bytes_sent / lading.bytes_written
```

This includes snapshot batches and the `StatefulBatch` wrapper, but still not
gRPC/HTTP2 framing.

---

## 5. Known Issues and Missing Metrics

### Known issues

- `logs_sender_grpc.raw_logs` and `logs_sender_grpc.raw_logs_bytes` stay at zero
  because `sendRawLog()` is not used.
- `logs_sender_grpc.state_size_bytes` is not an exact live-state size metric:
  pattern updates add the new `PatternDefine` size without subtracting the old
  definition, and deletes subtract the delete-datum size rather than the size of
  the removed definition.
- `logs_sender_grpc.pattern_logs_bytes` is measured before delta encoding, so it
  overstates what may actually be marshaled into a batch.
- `logs.encoded_bytes_sent` does not include gRPC snapshot batch `0`, so it
  understates total gRPC bytes when streams rotate.

### Commented-out metrics in gRPC sender

In `stream_worker.go`, `handleBatchAck()` has commented-out code for
`DestinationLogsDropped` and `LogsDropped` when the auditor channel is full.
These are marked TODO and not currently tracked, meaning gRPC drops are only
visible through `logs_sender_grpc_worker.bytes_dropped` (bytes, not log count).

### `RawDataLen` does not reflect bytes on disk

The framer sets `rawDataLen` to include the newline (matching bytes on disk),
but the file tailer creates a new `Message` via `NewMessageWithParsingExtra()`
which sets `RawDataLen = len(content)` — the trimmed content length. The
decoder's `rawDataLen` (which tracks disk bytes) is discarded. This means
`logs.bytes_sent` for gRPC (and `Payload.UnencodedSize` generally) undercounts
relative to actual disk I/O by at least the newline byte per line.

### Missing metrics

| Gap | Description | Impact |
|-----|-------------|--------|
| Pre-compression payload size outside gRPC | `logs.pre_compression_bytes_sent` now covers regular gRPC batches, but there is still no equivalent for HTTP/TCP or for gRPC snapshot batch `0` | Cannot compute the same "true compression ratio" across all transports from shared metrics |
| True on-wire bytes | No metric includes HTTP headers, TCP prefix/delimiter bytes, or gRPC/HTTP2 framing | Cannot compute exact network utilization from existing counters |
| Time-to-ack latency | No metric tracks elapsed time from `stream.Send()` to the matching `BatchStatus` ack | Cannot measure gRPC round-trip / intake processing latency |
| Tokenizer processing latency | No metric tracks time spent in `MessageTranslator` tokenization/patterning | Must rely on profiling for CPU analysis |
| Exact live state size | No metric exposes the actual serialized snapshot size or actual in-memory footprint of the pattern/tag state | `state_size_bytes` should only be used as an approximate state-churn ledger |
| gRPC log drop count | `DestinationLogsDropped` and `LogsDropped` are commented out in gRPC; only `bytes_dropped` is tracked | Cannot count individual logs lost on gRPC auditor-channel-full drops |

---

## 6. Recommended Metrics for Benchmarking Stateful Encoding

If the goal is "should we use stateful encoding, based on byte savings and CPU
cost?", use these metrics:

| Goal | Metric / formula | Why |
|------|------------------|-----|
| Cross-transport byte benchmark (HTTP vs gRPC) | `logs.encoded_bytes_sent / lading.bytes_written` | Best shared encoded-payload metric available today. Absolute ratio is always < 1.0 because newlines and trimmed whitespace are in `bytes_written` but not in agent metrics. |
| gRPC-only transport cost | `logs_sender_grpc_worker.bytes_sent / lading.bytes_written` | Includes snapshot overhead and `StatefulBatch` wrapper. Same denominator caveat as above. |
| gRPC-only compression ratio | `logs.encoded_bytes_sent / logs.pre_compression_bytes_sent` | Isolates compression from stateful encoding by comparing compressed `DatumSequence` bytes to the serialized protobuf before compression. |
| Log throughput | `logs.sent` rate | Useful, but watch the path-specific semantics above |
| Combined encoding + compression ratio within one transport | `logs.encoded_bytes_sent / logs.bytes_sent` | Compare only within the same transport and only in clean runs |
| State churn | `logs_sender_grpc.patterns_added`, `logs_sender_grpc.tokens_added` rates | Explains whether the encoder is stabilizing or constantly redefining state |
| Inflight / backpressure | `logs_sender_grpc_worker.inflight_bytes`, `logs_sender_grpc_worker.stream_errors`, `logs_sender_grpc_worker.bytes_dropped` | Tells you whether byte numbers are being distorted by transport problems |

Do not use these as the primary benchmark signal for stateful encoding:

- `logs.bytes_sent`: transport-dependent semantics are too different
- `logs_sender_grpc.pattern_logs_bytes`: measured before delta encoding
- `logs_sender_grpc.state_size_bytes`: not an exact live-state or memory metric

If you need to decide whether the feature is worth its CPU cost, pair the byte
metric above with CPU profiles. There is no current metric that isolates "stateful
encoding savings before compression" from "compression savings".

---

## 7. Lading-Side Metrics (SMP Blackhole + Generator)

These metrics come from Lading, not the agent. They are independent
measurements taken by the load generator and blackhole sinks. In SMP
experiments they surface as
`single_machine_performance.regression_detector.capture.<metric>`.

### 7.1 Generator metrics (input side)

From the `file_gen` generators (`logrotate`, `logrotate_fs`, `traditional`):

| Metric | Type | Description | Generator variant |
|--------|------|-------------|-------------------|
| `bytes_written` | counter | Bytes written to log files on disk, measured immediately after `write_all()`. For `static_timestamped` variants, the leading timestamp token is stripped from output; the counter includes message body bytes plus newline delimiters. If `emit_placeholder: true`, placeholder newlines are also counted. | All `file_gen` variants |
| `bytes_read` | counter | Bytes read from the FUSE filesystem by the agent (i.e. the agent pulled this many bytes). Only available with `logrotate_fs`. | `logrotate_fs` only |
| `missed_bytes` | counter (tag: `group_id`) | Bytes in log files that were deleted before the agent read them (log data lost to rotation). | `logrotate_fs` only |

`bytes_written` is the common baseline denominator for all efficiency
calculations. See section 6.

### 7.2 HTTP blackhole metrics (output side)

From `lading/src/blackhole/http.rs`. Used when the agent sends via HTTP
(e.g. the secondary HTTP blackhole in `stateful_logs_comp` experiments).

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `bytes_received` | counter | `component`, `component_name` | Raw compressed HTTP body size (`body.len()` before decompression). This is the closest to actual on-wire payload bytes, excluding HTTP headers. |
| `total_bytes_received` | counter | (none) | Aggregated `bytes_received` across all blackhole instances. No labels. |
| `decoded_bytes_received` | counter | `component`, `component_name` | Decompressed HTTP body size. Available because the HTTP blackhole runs `codec::decode()` on the body. Useful for computing compression ratio. |
| `requests_received` | counter | `component`, `component_name` | Number of HTTP requests received. |

Compression ratio from Lading's perspective (HTTP only):

```text
lading_http_compression_ratio = bytes_received / decoded_bytes_received
```

### 7.3 gRPC blackhole metrics (output side)

From `lading/src/blackhole/datadog_stateful_logs.rs`. Used when the agent
sends via stateful gRPC.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `bytes_received` | counter | `protocol`, `component`, `component_name` | `batch.encoded_len()` — the protobuf serialized size of the `StatefulBatch` message, re-computed by prost after tonic deserialization. The `data` field (`bytes` type) retains the application-compressed `DatumSequence` unchanged; `encoded_len()` = compressed payload size + ~2–6 bytes protobuf envelope. Excludes gRPC/HTTP2 framing overhead. |
| `total_bytes_received` | counter | (none) | Aggregated `bytes_received` across all blackhole instances. No labels. |
| `batches_received` | counter | `protocol`, `component`, `component_name` | Number of `StatefulBatch` messages received, including snapshot batch 0. |
| `data_items_received` | counter | `protocol`, `component`, `component_name` | Total Datum items across all batches. Only counted for batches with `data.len() > 0`. Includes log datums, state-change datums (PatternDefine, DictEntryDefine, etc.), and any other Datum types in the batch. |
| `streams_received` | counter | `protocol`, `component`, `component_name` | Number of gRPC bidirectional streams opened by the agent. |
| `stream_errors` | counter | `protocol`, `component`, `component_name` | Errors encountered while processing streams (receive errors or failed ack sends). |

### 7.4 gRPC vs HTTP blackhole `bytes_received` — comparable with caveats

Both blackholes measure `bytes_received` as the application-payload size
**excluding transport framing**, and both numbers reflect **compressed** data.
The measurement mechanisms differ slightly, but the resulting values are
close to each other in kind.

**How each blackhole counts bytes (verified from lading source):**

- **HTTP** (`lading/src/blackhole/http.rs`): calls `body.len()` on the raw
  HTTP entity body collected by hyper. This is the compressed payload the
  agent sent via `bytes.NewReader(payload.Encoded)`.

- **gRPC** (`lading/src/blackhole/datadog_stateful_logs.rs`): calls
  `batch.encoded_len()` (prost `Message::encoded_len()`) on the deserialized
  `StatefulBatch` struct. Because `StatefulBatch.data` is a `bytes` field in
  the proto schema, tonic stores the application-compressed `DatumSequence`
  blob verbatim as a `Vec<u8>`. Re-computing `encoded_len()` yields
  `varint(batch_id tag+value) + varint(data tag+length) + len(compressed_data)`,
  which is within a few bytes of the original protobuf wire size (deterministic
  encoding). The compressed payload inside `data` is **not** decompressed by
  tonic.

| Aspect | HTTP blackhole | gRPC blackhole |
|--------|---------------|----------------|
| What `bytes_received` measures | `body.len()` — compressed HTTP body bytes | `batch.encoded_len()` — protobuf serialized size of `StatefulBatch` (re-computed after deserialization, but `data` field retains compressed bytes) |
| Contains compressed payload? | Yes — the entire body is compressed | Yes — `StatefulBatch.data` holds `compress(proto.Marshal(DatumSequence))` unchanged |
| Includes transport framing? | No (HTTP headers excluded) | No (gRPC 5-byte length-prefix and HTTP/2 framing excluded) |
| Has `decoded_bytes_received`? | Yes | **No** — this is a gap |
| Counting unit | Per HTTP request | Per `StatefulBatch` message (including snapshot batch 0) |
| Small overhead included | None beyond the compressed body | ~2–6 bytes for the `batch_id` varint field + length prefix of `data` field |

**Bottom line:** the two `bytes_received` values **are** comparable for
measuring how much compressed payload the blackhole received. The gRPC
number includes a small protobuf envelope overhead (~2–6 bytes per batch)
that the HTTP number does not, but this is negligible at typical batch sizes.

If you see a large difference (e.g. 2×) between HTTP and gRPC
`bytes_received`, the cause is **not** a compressed-vs-uncompressed
measurement mismatch. It is the difference in what the agent puts inside
the payload:

- HTTP baseline sends compressed JSON arrays of raw log lines.
- gRPC stateful sends compressed `DatumSequence` containing structured logs
  **plus** pattern management datums (PatternDefine, PatternDelete,
  DictEntryDefine, DictEntryDelete). This protocol overhead is the primary
  driver of size differences.

### 7.5 Useful Lading metrics for benchmarking

| Goal | Metric / formula | Why |
|------|------------------|-----|
| Total log data generated | `bytes_written` | Common input baseline for all experiments |
| Data lost to rotation | `missed_bytes` | Non-zero means the agent fell behind; invalidates efficiency metrics |
| gRPC output volume (server-side) | `bytes_received` (gRPC blackhole) | Independent measurement of what the blackhole received; `encoded_len()` of each `StatefulBatch` which contains the application-compressed `DatumSequence` in its `data` field plus ~2–6 bytes of protobuf envelope |
| HTTP output volume (server-side) | `bytes_received` (HTTP blackhole) | Independent measurement of compressed HTTP body received |
| HTTP compression ratio (server-side) | `bytes_received / decoded_bytes_received` (HTTP blackhole) | Only available for HTTP; gRPC blackhole has no decompressed metric |
| gRPC batch rate | `batches_received` rate | Batch frequency; useful for spotting batch-size tuning issues |
| gRPC datum throughput | `data_items_received` rate | How many datums per second the blackhole is processing; includes both log and state-change datums |
| gRPC stream lifecycle | `streams_received`, `stream_errors` | Stream rotation frequency and error rate, from the server's perspective |
| End-to-end efficiency (Lading-only, gRPC) | `bytes_received (gRPC) / bytes_written` | What fraction of generated bytes ended up as payload at the server. Comparable to the HTTP version — both measure compressed payload bytes with negligible envelope difference (see §7.4). Differences reflect payload content (stateful protocol overhead), not measurement methodology. |
| End-to-end efficiency (Lading-only, HTTP) | `bytes_received (HTTP) / bytes_written` | What fraction of generated bytes ended up as compressed HTTP payload at the server. |

### 7.6 Missing Lading-side metrics for stateful encoding

| Gap | Description | Impact |
|-----|-------------|--------|
| `decoded_bytes_received` for gRPC | The gRPC blackhole does not decompress and re-measure the `StatefulBatch.data` payload | Cannot compute server-side compression ratio for gRPC from Lading alone |
| Datum-type breakdown | `data_items_received` counts all datum types together | Cannot distinguish log datums from state-change datums (pattern/dict define/delete) in Lading metrics; must use agent-side `logs_sender_grpc.pattern_logs` and related counters |
| Snapshot size | No metric for the size of snapshot batch 0 specifically | Cannot measure snapshot overhead from Lading; must infer from `batches_received` where `batch_id=0` |
