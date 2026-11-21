# Logs Agent Endpoint Filtering and Routing

## Overview

This document describes endpoint-specific selection and routing for normal-mode HTTP JSON logs delivery, stacked on the injected delivery group routing mechanism (PR #59049).

**OTTL status (important):** Customer-configurable OTTL endpoint filter expressions (such as configuring a `NOTICE`-and-higher filter for US5 in `datadog.yaml`) are **not yet shipped**. Upstream OTTL integration (PR #59688) and final YAML selector syntax are still pending. This implementation provides the full endpoint-routing delivery pipeline, read-only pre-encoding record view, and a rejecting `StubCompiler` boundary (`routing.ErrOTTLNotImplemented` for any non-empty expression; empty selectors default to match-all).

## Activation and Configuration

```yaml
logs_config:
  use_http: true            # Required when additional_endpoints are configured
  routing:
    enabled: false          # Default false: legacy pipeline.Provider unchanged
    max_groups: 4           # Maximum routed endpoints/groups (hard cap: 32)
```

- When `logs_config.routing.enabled` is `false` (default), the legacy `pipeline.Provider` is used with zero behavioral or performance change, including existing Multi-Region Failover (MRF) behavior.
- When `logs_config.routing.enabled` is `true`, normal Agent startup constructs `pipeline.RoutedProvider` in endpoint-policy mode over the resolved HTTP `main` and `additional_endpoints` configuration (`main` is `"main"`, additional endpoints are `"additional-0"`, `"additional-1"`, ...). With no selectors configured, all endpoints match all records with reliable-set semantics.

### Unsupported Combinations

When `logs_config.routing.enabled` is `true`, Agent startup fails explicitly (no silent fallback) if combined with:
- TCP transport (`!UseHTTP`, including `force_use_tcp`, `socks5_proxy_address`, or `additional_endpoints` without `use_http: true` / `force_use_http: true`)
- Protobuf transport (`UseProto`)
- Multi-Region Failover (`multi_region_failover.enabled: true` or any `IsMRF` endpoint)
- Legacy distributed senders (`logs_config.disable_distributed_senders: true`)
- Serverless mode

## Delivery and Completion Semantics

1. **Common processing precedes selection:** Global and source `processing_rules` (`exclude_at_match`, `include_at_match`, `mask_sequences`, `exclude_truncated`, `remap_source`) run once before endpoint selection. A common drop removes all copies and completes locally; common masks apply to every surviving copy.
2. **Pre-encoding evaluation (`routing.RecordView`):** Selectors run after common processing and before rendering/JSON encoding, with read-only access to structured JSON attributes, raw content, unfiltered local tags (`Origin.Tags()`, including tags stripped from outgoing `ddtags` by export `tag_filters`), `Source()` (including `remap_source`), `Service()`, `SourceCategory()`, `Status()`, and `Hostname()`.
3. **Render and encode once, batch with Layout B:** Surviving records are rendered and JSON-encoded once, then fanned out to per-endpoint `batchStrategy` streams with real per-stream compression. Primary/unfiltered endpoints build full batches without splitting on sparse secondary matches; filtered secondary endpoints compress their subsets separately.
4. **Reliable-set OR with explicit terminal-loss distinction:**
   - The Agent attempts delivery to every matching reliable endpoint.
   - **Any one** actual HTTP delivery success (2xx) satisfies the record's reliable-set obligation and releases source progress.
   - Explicit permanent rejections (e.g. HTTP 4xx) or local batch encoding/oversized loss remove only that endpoint; source progress completes via local-loss policy only after **all** matching reliable endpoints terminally lose the record.
   - Retries (5xx/network errors), credential waits, canceled sends, and saturated queues remain pending and never complete a record.
   - Unreliable/optional endpoint deliveries never satisfy a pending reliable obligation.
   - Multi-reliable fanout retains unadmitted sibling copies in a bounded per-source per-endpoint FIFO backlog so primary batching continues unblocked while at least one matching reliable endpoint can accept the record; a record matching only a saturated endpoint backpressures its dispatch lane. Completed sibling copies behind an unfinished head are compacted to keep retention strictly bounded.
5. **Optional endpoint isolation:**
   - Optional (unreliable) admission runs before potentially blocking reliable admission and stays strictly non-blocking end-to-end via bounded queues and `OptionalHTTPSender`.
   - Slowness, HTTP errors, or queue saturation on an unreliable endpoint drops optional copies with telemetry (`datadog.logs_routing.optional_stream_dropped`, `datadog.logs_routing.optional_payloads_dropped`) without blocking healthy reliable delivery or checkpoints.
   - A reliable primary outage may backpressure both streams at `SourcePendingLimit` (64 pending records per source).
6. **Edge cases:**
   - **No recipients:** Explicit intentional local exclusion; skips render/encode and completes locally (respecting contiguous source-prefix ordering).
   - **Optional-only recipients:** Renders/encodes once, offers non-blockingly to matching optional queues, then completes locally immediately.
   - **Evaluation errors:** Reliable selector errors hold the source checkpoint as an unresolved barrier (no partial selection or fallback). Optional selector errors suppress only that optional copy with rate-limited diagnostics (never logging record content or credentials).

## Source Isolation and Verified Limits

- **File launcher (`pkg/logs/launchers/file`):** Adopts `SourceInput`, `RetireSource`, and `GuardRegistry` via a narrow optional provider interface. Each file tailer gets a dedicated input channel and monotonic generation across creation and rotation, giving per-file isolation (a blocked file pauses at 64 pending records while unrelated files continue).
- **Other launchers:** Remain on bounded shared ingress with per-identifier generation tracking and `GuardRegistry` protection; shared ingress does not yet provide cross-source head-of-line isolation.
- **Bounds:**
  - `routing.MaxEndpoints = 32` (further capped by `logs_config.routing.max_groups`, default 4)
  - `routing.SourcePendingLimit = 64` pending records per checkpointed source
  - Aggregate progress capacity: `4096` records
  - Bounded optional stream/sender queues and aggregate HTTP concurrency cap (`64` split across endpoints)
  - Base injected-group API (`routing.Group`, `routing.Definitions`, `CompleteGroup`) retains its independent per-group AND semantics unchanged.
