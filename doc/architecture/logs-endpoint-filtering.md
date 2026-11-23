# Normal-Mode Logs Endpoint Filtering and Routing

This document describes the normal-mode Logs endpoint filtering implementation built on top of the injected delivery group routing mechanism (PR #59049).

## Scope

Endpoint-specific selection for normal-mode HTTP JSON Logs delivery. The initial driving policy is:

- **US1 (primary, reliable):** unfiltered, mandatory delivery.
- **US5 (additional, unreliable):** filtered (for example NOTICE-and-higher), best-effort delivery.

Filtering and reliability are independent: selectors can be attached to any reliable or unreliable endpoint, including the primary and MRF endpoints.

## Pipeline Stages

1. **Admission (`EndpointFilteredProvider.admit`):** Reserves bounded source order and capacity in `routing.Progress` (`CompletionRequireAny`) before processing lanes can reorder records.
2. **Common processing & metadata preparation (`processor.NewForEndpointFiltering`):** Applies common processing rules (exclude, include, mask, remap), resolves `tag_filters`, and evaluates MRF allowlist classification (`msg.IsMRFAllow`). A common drop removes every copy and completes source progress locally.
3. **Pre-render endpoint selection (`routing.EndpointFilterPolicy.Evaluate`):** Evaluates compiled per-endpoint selectors against a read-only `routing.LogView` while structured JSON fields and unfiltered local Agent tags (`Origin.Tags()`) remain accessible. Outgoing `tag_filters` (`Origin.TransportTags()`) still apply during subsequent JSON encoding.
4. **Single render & JSON encode:** Records matching at least one endpoint are rendered and JSON-encoded once. Records matching zero endpoints skip rendering and encoding completely.
5. **Layout B batching & dispatch:** Each configured endpoint maintains its own independent batcher (`sender.NewBatchStrategyWithTerminalLoss`) and HTTP destination so primary batches are never split by sparse secondary matches.

## Selection Edge Cases and Completion Rules

| Case | Delivery behavior | Source progress / checkpoint |
| --- | --- | --- |
| Common processing drop | No delivery to any endpoint | Completed locally (`CompleteLocal`) |
| 0 endpoints match | Skip render/encode; no delivery | Completed locally (`Excluded: true`) |
| Optional-only (0 reliable, >=1 optional match) | Render/encode once; non-blocking handoff to matched optional queues | Completed locally immediately after non-blocking handoff |
| >=1 reliable endpoints match | Try all matching reliable endpoints; block only if all matching reliable queues are full. Optional matches get non-blocking best-effort copies. | Satisfied when any one matching reliable endpoint completes (`CompletionRequireAny`); optional acknowledgments never advance pending reliable records. |
| Selector evaluation error (reliable endpoint) | Fail-open (include endpoint) + error metric & rate-limited warning | Follows normal reliable completion (never holds checkpoint on eval error) |
| Selector evaluation error (optional endpoint) | Fail-closed (exclude endpoint) + error metric & rate-limited warning | Unaffected |

## Isolation, Backpressure, and MRF

- **Optional failure isolation:** Optional (unreliable) endpoints use `shouldRetry=false`, bounded non-blocking message and payload queues, and a no-op sink. Slowness, HTTP errors, or queue saturation on an optional endpoint drop best-effort copies with telemetry (`logs_endpoint_filter.optional_messages_dropped`, `logs_endpoint_filter.optional_payloads_dropped`) without blocking reliable delivery or advancing checkpoints.
- **Primary backpressure:** When all matching reliable endpoints for a record are stalled (for example US1 returning 503), admission pauses at `routing.SourcePendingLimit` (64 records per source) and source checkpoints stay held until recovery.
- **MRF interaction:** MRF endpoints (`Endpoint.IsMRF == true`) are eligible only when `msg.IsMRFAllow` is true (and their selector, if configured, matches). When failover is active, MRF delivery satisfies `CompletionRequireAny` for allowed services so source checkpoints can progress during a primary outage.

## OTTL Integration Stub Boundary

OTTL expression compilation and YAML selector syntax are intentionally stubbed until OTTL filtering (PR #59688) lands:

- `routing.EndpointSelector` (`Match(routing.LogView) (bool, error)`)
- `routing.SelectorCompiler` (`Compile(expr string) (routing.EndpointSelector, error)`)
- `routing.StubSelectorCompiler` (returns `nil` for empty expressions and `routing.ErrSelectorCompilerNotConfigured` for non-empty expressions)
- `comp/logs/agent/impl` keeps `pipeline.NewProvider` when no endpoint selectors are configured (zero overhead on the default path) and switches to `pipeline.NewEndpointFilteredProvider` when selectors are present.

## Verified Limits

- `routing.MaxFilterEndpoints = 64`: inline `uint64` bitmask per message (zero heap allocations on error-free evaluation).
- `routing.SourcePendingLimit = 64`: max pending records retained per checkpointed source.
- `routedProgressCapacity = 4096`: aggregate pending record budget across all sources.
- `optionalStreamCapacity = 128`: bounded per-optional-endpoint non-blocking message queue.
