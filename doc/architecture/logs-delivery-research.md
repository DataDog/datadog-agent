# Agent logs: endpoints, batching, HTTP delivery, and failover

## Purpose and evidence

This document explains the existing log delivery path for selective multi-org routing research. It does not propose a routing design, configuration contract, or development plan.

Research baseline: local revision `9d8ea371d02`, inspected on 2026-10-22. The research covered source code and selected tests. No tests ran.

The routing discussion notes are at this local path:

`/Users/ella.taira/Documents/Obsidian Vault/ella.taira/4q206/routing.md`

The notes separate routing mechanics from product configuration and structured-filter DSL work. This document keeps those boundaries.

## 1. The main mental model

The Agent processes individual messages, groups them into payloads, and distributes those payloads to network destinations.

```text
Log source
  → Tailer / decoder
  → Processor                   one log at a time
  → Batch strategy              groups and compresses logs
  → Shared sender / worker      distributes whole payloads
  → HTTP destination(s)         requests, retries, backoff
  → Auditor                     source checkpoints
```

This diagram shows the normal, non-serverless HTTP path. It omits diagnostic receivers and special transport paths.

A **pipeline** is a local processing lane, not an org or endpoint. Several pipelines can feed one shared sender. Each endpoint becomes a separate destination with its own HTTP delivery state.

The Agent connects its auditor as the sender sink. The provider constructs the sender and processing lanes. Each lane contains a processor and a strategy.

Source entry points:

- [Agent setup](../../comp/logs/agent/impl/agent_core_init.go): `SetupPipeline` and `buildPipelineProvider`.
- [Pipeline provider](../../comp/logs-library/pipeline/provider.go): `NewProvider`, `httpSender`, and `Start`.
- [Pipeline construction](../../comp/logs-library/pipeline/pipeline.go): `NewPipeline` and `getStrategy`.

The [logs README](../../pkg/logs/README.md) gives useful collection background. Some paths and ordering statements do not match this checkout. Use the current implementation as the authority.

## 2. Terms and data types

| Term | Meaning in this path |
| --- | --- |
| Source | A log collection definition and its metadata |
| Message | One log record, its content, and its metadata |
| Origin | Source identity, offsets, service, source name, and tags |
| Pipeline | A local processor and strategy lane |
| Strategy | Converts a message stream into payloads |
| Payload | Encoded batch bytes and metadata for the included messages |
| Endpoint | Destination settings, including address and credentials |
| Destination | Runtime network delivery object for an endpoint |
| Auditor | Tracks source progress and saves a restart registry |

A `Message` combines `MessageContent` and `MessageMetadata`. A `Payload` retains message metadata, encoded bytes, content encoding, and unencoded size.

The sender works with payloads. It does not expose a general JSON query interface over individual application logs.

Sources: [message types](../../pkg/logs/message/message.go) and [origin metadata](../../pkg/logs/message/origin.go).

## 3. Multiple endpoints and ordinary dual shipping

An `Endpoint` contains the host, port, API key, TLS choice, intake metadata, reliability, and retry settings. `Endpoints` contains the main endpoint and extra endpoints.

The HTTP factory constructs separate destinations from the reliable and unreliable endpoint lists. Each destination sends its own API key in the `DD-API-KEY` header.

Different org credentials can use the same intake host. An endpoint is not just a region address.

**Ordinary extra endpoints receive copies of the same encoded payload.** The normal sender does not apply a different JSON or tag filter for each endpoint.

```text
Payload containing logs A, B, C
  ├─→ primary endpoint: A, B, C
  └─→ extra endpoint:   A, B, C
```

Sources:

- [Endpoint definitions](../../comp/logs/agent/config/endpoints.go): `Endpoint`, `Endpoints`, and `loadHTTPAdditionalEndpoints`.
- [HTTP sender factory](../../comp/logs-library/sender/http/http_sender.go): `httpDestinationFactory`.

### Reliable and unreliable destinations

The main endpoint is reliable. Extra endpoints default to reliable unless their settings say otherwise. **Reliable does not mean every org must receive every log.**

For ordinary active destinations, the worker follows this policy:

1. Try the reliable destinations first.
2. Continue through another reliable destination when one destination enters retry state.
3. Try to buffer copies for reliable destinations that did not accept the payload.
4. Try nonblocking sends to unreliable destinations.

A skipped reliable destination can lose copies when its buffer fills. Unreliable destinations do not advance the auditor.

Unreliable destinations do not independently sustain delivery when all ordinary reliable destinations remain stuck. MRF has special skip paths, so this policy is not an absolute delivery guarantee.

`DestinationSender.Send` reports acceptance into a destination queue, not completed HTTP delivery. Its retry-state reader can cancel a blocked queue send.

Sources: [worker distribution](../../comp/logs-library/sender/worker.go), especially `run`, and [destination queue handling](../../comp/logs-library/sender/destination_sender.go).

## 4. Processor, batcher, and HTTP transport

These components act at different levels.

### Processor: one message

The processor does these steps for messages that survive filtering:

1. Apply global and source processing rules.
2. Render the message.
3. Report the rendered message to diagnostic receivers.
4. Mark MRF eligibility when log failover is active.
5. Encode the outgoing record.
6. Send the message to the strategy.

The HTTP encoder creates a JSON envelope with message, service, source, tags, hostname, status, and timestamp fields.

Existing rules include content regex filtering, masking, truncation exclusion, and structured source remapping. These rules do not form a general endpoint-routing DSL.

Sources: [processor](../../comp/logs-library/processor/processor.go), especially `processMessage` and `applyProcessingRules`, and [JSON encoder](../../comp/logs-library/processor/json.go).

### Batch strategy: one payload

The batch strategy accepts encoded messages. It keeps separate `main` and `mrf` batches.

Each batch serializes encoded records into a JSON array and compresses the array as it grows. It retains message metadata separately for accounting and checkpoints.

A batch flushes after any of these events:

- The batch reaches its message-count or byte limit.
- The flush timer fires.
- An explicit flush arrives.
- Input closes during shutdown.

The byte limit counts encoded record bytes before compression, not compressed network bytes. A message that cannot fit in an empty batch drops here.

**Compression happens before endpoint distribution.** Destinations reuse the encoded payload rather than independently encoding each log.

Sources:

- [Batch strategy](../../comp/logs-library/sender/batch_strategy.go): `Start`, `getBatch`, and `flushAllBatches`.
- [Batch implementation](../../comp/logs-library/sender/batch.go): `processMessage`, `flushBuffer`, and `sendMessages`.
- [Message buffer](../../comp/logs-library/sender/message_buffer.go): `AddMessage` and size checks.
- [Array serializer](../../comp/logs-library/sender/serializer.go): `Serialize` and `Finish`.

### HTTP destination: network delivery

Each destination builds its URL, creates a POST request, adds headers, and sends the encoded payload. Version and track settings select the intake path.

Important headers include `DD-API-KEY`, `Content-Type`, and `Content-Encoding`. Protocol, origin, and timestamp headers also carry delivery metadata.

Each destination owns its HTTP client, request worker pool, retry state, and backoff policy.

| Result | Normal behavior for a reliable destination |
| --- | --- |
| Network failure | Retry with backoff |
| HTTP 429 or server error | Retry with backoff |
| HTTP 400, 401, 403, or 413 | Drop the payload |

The 403 path has a secret-refresh exception. If refresh succeeds, the destination retries.

The worker-pool code supports dynamic concurrency. The normal provider currently sets equal minimum and maximum concurrency, so this path uses fixed concurrency.

Legacy and serverless paths use different sender arrangements. Do not apply the normal-path concurrency description to every caller.

Sources:

- [HTTP destination](../../comp/logs-library/client/http/destination.go): `sendAndRetry`, `unconditionalSendWithContext`, `updateRetryState`, and `buildURL`.
- [HTTP worker pool](../../comp/logs-library/client/http/worker_pool.go): `run` and `resizeUnsafe`.
- [Provider concurrency selection](../../comp/logs-library/pipeline/provider.go): `httpSender`.

## 5. Multi-Region Failover: the existing selection path

MRF connects destination setup, activation, message selection, separate batching, and destination checks.

### Setup and activation are different

| Setting | Role |
| --- | --- |
| `multi_region_failover.enabled` | Creates the MRF infrastructure |
| `multi_region_failover.failover_logs` | Activates log forwarding through that infrastructure |
| `multi_region_failover.logs_service_allowlist` | Selects services while log failover is active |

Endpoint construction adds a secondary endpoint when the MRF setting is true. That endpoint has its own API key and `IsMRF` flag.

**HTTP errors do not automatically activate MRF in this inspected logs path.** They control retries and backoff.

The remote-config client subscribes to `AGENT_FAILOVER` updates. Its callback applies failover flags and allowlists through runtime settings. Runtime settings also support other configuration sources.

The Agent code shows how it consumes the activation signal. This research did not inspect the backend policy that produces that signal.

Sources:

- [Endpoint construction](../../comp/logs/agent/config/config.go): the HTTP MRF endpoint block.
- [Remote-config client](../../comp/remote-config/rcclient/impl/rcclient.go): `mrfUpdateCallback` and runtime-setting application.
- [Failover payload](../../comp/remote-config/rcclient/impl/agent_failover.go): remote-config field types.
- [Runtime settings](../../cmd/agent/subcommands/run/internal/settings/runtime_settings_multi_region_failover.go): configuration reads and writes.

### The processor selects messages by Agent service

While log failover is active:

- An empty effective allowlist selects all services.
- A nonempty allowlist selects exact matches against `msg.Origin.Service()`.

The processor sets `IsMRFAllow` on selected messages. This flag is internal delivery metadata, **not a normal Datadog tag string**.

An allowlist alone does not activate failover.

Source: [processor](../../comp/logs-library/processor/processor.go), especially `updateFailoverConfig` and `filterMRFMessages`.

### The batcher separates eligibility classes

The strategy puts selected messages in an `mrf` batch and other messages in a `main` batch. Each payload contains only one eligibility class.

`Payload.IsMRF()` checks the first message because the batcher maintains that invariant. Mixing eligibility classes in a payload would break that assumption.

Sources: [batch selection](../../comp/logs-library/sender/batch_strategy.go) and [payload eligibility](../../pkg/logs/message/message.go), especially `Payload.IsMRF`.

### The worker checks the destination

The worker skips non-MRF payloads for MRF destinations. The MRF destination sender also checks the enable and activation flags.

A disabled MRF sender can report queue-send success without sending the payload. This special path prevents a disabled secondary destination from blocking normal delivery.

**The primary destination still receives both message classes.** MRF adds selected forwarding to the secondary region instead of removing those messages from primary delivery.

For failover active with an allowlist containing `payments`:

```text
payments logs → primary endpoint + MRF endpoint
other logs    → primary endpoint
```

This is selective secondary forwarding. It is not a general JSON or tag routing engine.

Sources: [worker](../../comp/logs-library/sender/worker.go), especially its `IsMRF` checks, and [destination sender](../../comp/logs-library/sender/destination_sender.go), especially `canSend`.

## 6. Metadata and JSON are different concepts

MRF reads the Agent service field. `Origin.Service` prefers the configured source service, then the service stored on the origin.

The selector does not search arbitrary application JSON for a `service` property. A service value inside application content is not automatically the same value.

Likewise, a JSON HTTP envelope does not prove the Agent parsed the application log into searchable attributes.

The message model distinguishes unstructured, structured, rendered, and encoded states. Some inputs provide structured content. This support is not a general JSON filtering DSL.

Sources: [service resolution](../../pkg/logs/message/origin.go), [message states](../../pkg/logs/message/message.go), and [HTTP JSON envelope](../../comp/logs-library/processor/json.go).

## 7. Auditor, retries, and source checkpoints

Reliable destinations send completed payloads to the auditor channel. Unreliable destinations use a drain channel.

The auditor reads payload message metadata, updates source offsets, and saves its registry. It does not maintain a separate checkpoint for every org.

**Completed does not always mean successful intake delivery.** The HTTP destination also forwards payloads after permanent errors, so those drops can advance source checkpoints.

Retries, destination buffers, and source progress are related but separate mechanisms. Do not interpret a source checkpoint as proof of delivery to every endpoint.

Sources:

- [Worker output channels](../../comp/logs-library/sender/worker.go): `run`.
- [HTTP completion](../../comp/logs-library/client/http/destination.go): `sendAndRetry`.
- [Auditor](../../comp/logs/auditor/impl/auditor.go): `run`, `updateRegistry`, and `flushRegistry`.

## 8. Three different failover mechanisms

| Mechanism | What changes | What it does not mean |
| --- | --- | --- |
| Multi-Region Failover | Secondary destination activation and message eligibility | Automatic activation after any HTTP failure |
| Pipeline failover | Local processing lane when another lane blocks | Selection of an org or region |
| HTTP/TCP fallback | Transport after connectivity checks | Regional message selection |

Pipeline failover tries another local pipeline with a nonblocking send. If all lanes block, it applies backpressure through the primary lane.

Do not assume strict intake ordering from the high-level pipeline diagram. Concurrent HTTP requests and optional pipeline failover make that diagram insufficient proof.

Sources:

- [Local pipeline failover](../../comp/logs-library/pipeline/provider.go): `NextPipelineChan`, `forwardWithFailover`, and `trySendToPipeline`.
- [Transport selection](../../comp/logs/agent/impl/agent_core_init.go): `buildEndpoints` and connectivity checks.
- [Agent lifecycle](../../comp/logs/agent/impl/agent.go): `start` and its HTTP recovery path.

## 9. Tests as examples of behavior

I inspected these unit tests but did not execute them.

| Question | Test and source |
| --- | --- |
| Which services qualify for MRF? | `TestFilterMRFMessages` in [processor tests](../../comp/logs-library/processor/processor_test.go) |
| Why do MRF messages need separate batches? | `TestBatchStrategyMRFRouting` in [batch strategy tests](../../comp/logs-library/sender/batch_strategy_test.go) |
| How does MRF activation change sending? | `TestDestinationSenderDisabledToEnabled` in [destination sender tests](../../comp/logs-library/sender/destination_sender_test.go) |
| What happens when one reliable destination fails? | `TestSenderReliableContinuseWhenOneFails` in [worker tests](../../comp/logs-library/sender/worker_test.go) |
| Can an unreliable destination continue alone? | `TestSenderUnreliableStopsWhenMainFails` in [worker tests](../../comp/logs-library/sender/worker_test.go) |

The remote-config E2E suite also contains [MRF service-allowlist cases](../../test/new-e2e/tests/remote-config/mrf_logs_service_allowlist_test.go). Its assertions compare main-region delivery with selected failover-region delivery. I did not run that suite.

## 10. Foldspace and research limits

A bounded search found no Foldspace references in tracked content or filenames. It did not cover external repositories, untracked files, or Git history.

The notes identify Foldspace compatibility as a concern. This checkout alone does not establish its architecture or its relationship to this sender.

Other limits:

- I inspected tests but did not execute them.
- I did not inspect deployed settings or backend MRF activation policy.
- This document covers the normal Agent HTTP path, with selected comparisons to other paths.
- No general JSON or tag routing contract follows from the existing MRF mechanism.

## 11. Reading order and questions

This is a study sequence, not an implementation plan.

1. Read `Message`, `Payload`, and `Origin`.
2. Follow one message through the processor and JSON encoder.
3. Follow the encoded record into a batch and compressed payload.
4. Follow the payload through the worker and destination sender.
5. Examine HTTP completion, retries, and auditor updates.
6. Repeat the trace with MRF eligibility set.

Useful questions during review:

- Where does one log become part of a batch?
- Which components can still inspect individual log content?
- Which components operate only on payloads and metadata?
- What does queue acceptance prove?
- What does an auditor checkpoint prove?
- Why must MRF eligibility stay uniform within a payload?
- Why does the primary endpoint still receive MRF-selected logs?
- Which service value does the MRF selector actually read?

**The main takeaway:** ordinary multi-endpoint shipping copies payloads. Existing MRF selection depends on message classification before batching.
