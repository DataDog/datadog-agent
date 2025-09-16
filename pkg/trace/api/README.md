# Datadog Agent - Trace API package

This package implements the HTTP and gRPC endpoints used by the Datadog Trace Agent to receive, proxy, and forward data from tracers and other APM clients. It also contains helpers for request handling, transport wrappers, and several specialized proxy endpoints.

## Top-level components

- **`api.go`**: Core HTTP receiver (`HTTPReceiver`) and request handling. Builds the HTTP mux, registers endpoints, applies per-endpoint timeouts, and wires telemetry and stats processing. Also includes utilities for buffering request bodies and exposes `GetHandler` for retrieving registered handlers.
- **`endpoints.go`**: Declarative list of HTTP endpoints exposed by the trace-agent (paths, handlers, enablement conditions, and timeout overrides). Uses versioning from `version.go`.
- **`version.go`**: Documents and enumerates API versions (`v0.3`, `v0.4`, `v0.5`, `v0.6`, `v0.7`) and their payload formats/semantics.
- **`listener.go`**: Instrumented `net.Listener` wrappers:
  - `measuredListener`: emits accept/timedout/error metrics and enforces max concurrent connections.
  - `rateLimitedListener`: connection rate limiter with periodic lease refresh and status metrics.
- **`responses.go`**: Common HTTP responses and error helpers used by handlers (e.g., decoding/format errors, rate-by-service response with `Datadog-Rates-Payload-Version`).
- **`payload.go`**: Defines the `Payload` structure (wrapping `pb.TracerPayload`) with helper methods to access/modify trace chunks.
- **`receiver.go`**: Defines the transport-agnostic `Receiver` interface implemented by HTTP and non-HTTP receivers. Adds lifecycle methods (`Start`, `Stop`), `BuildHandlers`, `UpdateAPIKey`, language/metrics accessors, and `GetHandler(pattern)`.
- **`bypass_receiver.go`**: Implements `BypassReceiver`, a minimal receiver that exposes no HTTP endpoints. Allows programmatic ingestion via `SubmitTraces`/`SubmitStats`, building `Payload`s from raw bytes and forwarding them to the processing pipeline.
- **`processing.go`**: Transport-agnostic helpers for decoding tracer and client-stats payloads from bytes, computing rate-by-service responses, updating tag stats, and constructing `Payload`s. Used by both HTTP and bypass receivers.

## Data intake (Traces, Stats, OTLP)

- **`otlp.go`**: OpenTelemetry receiver (`OTLPReceiver`) implementing OTLP gRPC v1 ingestion. Translates incoming `ptraceotlp.ExportRequest` to internal `Payload`s, applies sampling logic, extracts tags/headers, and forwards to processing.
- **`pipeline_stats.go`**: Reverse proxy for pipeline stats ingestion (`/v0.1/pipeline_stats`). Forwards requests to one or more configured intake endpoints, sets container and additional tags, and emits debug metrics.
- Tests: `pipeline_stats_test.go`

## Proxy endpoints

- **`profiles.go`**: Reverse proxy for profiler payloads to profile intake (`/profiling/v1/input`), supporting additional endpoints and rich error handling with timeouts/body-read handling.
- **`debugger.go`**: Reverse proxy for Dynamic Instrumentation (Debugger) logs and diagnostics (`/debugger/v1/input`, `/debugger/v1/diagnostics`, `/debugger/v2/input`). Adds request IDs and aggregated tags; supports additional endpoints.
- **`symdb.go`**: Reverse proxy for Symbol Database ingestion (`/symdb/v1/input`). Mirrors debugger proxy logic and headers.
- **`openlineage.go`**: Reverse proxy for OpenLineage events (`/openlineage/api/v1/lineage`). Supports API versioning via query string and multiple endpoints.
- **`evp_proxy.go`**: Event Platform proxy (`/evp_proxy/v{1..4}/`). Strictly validates subdomain/path/query; forwards a constrained header set, injects Datadog headers (API/app keys per endpoint), container tags, hostname/env, and enforces request timeouts and maximum payload sizes.
- **`dogstatsd.go`**: Simple HTTP-to-UDP proxy for DogStatsD payloads (`/dogstatsd/v1|v2/proxy`). Requires DogStatsD to be enabled; does not guarantee delivery.
- **`tracer_flare.go`**: Reverse proxy for tracer flare uploads (`/tracer_flare/v1`). Builds site-specific destination and sets API key.

## Telemetry and discovery

- **`telemetry.go`**: Asynchronous Telemetry Forwarder and handler (`/telemetry/proxy`). Buffers and forwards requests in the background with backpressure (limits on inflight bytes/requests), enriches headers (container/host/env/install/cloud), and handles multiple endpoints.
- **`info.go`**: `/info` handler returns discovery JSON including enabled endpoints, feature flags, effective runtime config, peer tags, and obfuscation version. Also returns a `Datadog-Container-Tags-Hash` header derived from service-origin tags.

## Platform-specific, server, and container context

- **`container_linux.go`**: Linux-only helpers:
  - `connContext` injects UNIX socket peer credentials into request context.
  - `IDProvider` and `NewIDProvider`: resolve container ID from headers and cgroups (v1/v2) using origin detection; fallback behaviors documented.
- **`pipe.go` / `pipe_off.go`**: Windows-only `listenPipe` for named pipes and a non-Windows stub.
- **`debug_server.go` / `debug_server_serverless.go`**: In-process TLS debug server exposing `/debug/*` (pprof, expvar, coverage), with serverless no-op build.

## Transports and utilities

- **`transports.go`**: HTTP `RoundTripper` wrappers:
  - `measuringTransport`: emits request count/timing metrics.
  - `forwardingTransport`: fan-out to main and additional endpoints; returns main response.
  - `newMeasuringForwardingTransport`: combines forwarding and measuring.
- **`apiutil/limited_reader.go`**: `LimitedReader` that caps reads and surfaces `ErrLimitedReaderLimitReached` to enforce payload limits.
- **`apiutil/coverage.go`, `coverage_off.go`**: Debug HTTP coverage endpoints wiring.
- **`internal/header/headers.go`**: Canonical HTTP header names used across trace agent and intakes (language metadata, process/container IDs, computed flags, sampling rates versioning, etc.).

## Handler registration and timeouts

- Endpoints are assembled in `api.go` via `buildMux()` using definitions from `endpoints.go`. Some endpoints override default timeouts (e.g., profiling, EVP proxy). `replyWithVersion` middleware adds agent version/commit headers and handles ETag-hash for `/info`.
- `HTTPReceiver` exposes `GetHandler(pattern)` to retrieve a built handler (useful for composition/testing). All handlers enforce request size limits via `apiutil.LimitedReader` and apply per-handler timeouts.
- Trace ingestion honors transport headers such as `Datadog-Trace-Count`, `_dd.computed_top_level`, `_dd.profiling.computed.stats`, `_dd.p.dm|sampling_rate_v1`, and `_dd.tracer.obfuscation.version`. Oversized payloads respond with 413 and are accounted in tag stats.

## Tests

A comprehensive set of tests validate behavior for endpoints, transports, and proxies:
- `*_test.go` files across the directory (e.g., `api_test.go`, `otlp_test.go`, `profiles_test.go`, `telemetry_test.go`, `openlineage_test.go`, `evp_proxy_test.go`, `pipeline_stats_test.go`, `responses_test.go`, `listener_test.go`, `container_linux_test.go`, etc.).

## Notes

- Many proxies support multiple endpoints; the first is treated as the main endpoint whose response is returned to the client; others are best-effort and their responses are discarded after draining.
- Container context enrichment uses `IDProvider` to compute container IDs and tags for request augmentation.
- Error paths strive to be explicit and measurable via statsd metrics across transports and handlers. Decoding/timeout/payload-too-large cases use consistent HTTP status codes and `datadog.trace_agent.receiver.*` metrics.
