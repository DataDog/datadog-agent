> **TL;DR:** `pkg/trace/api` is the front-door of the trace agent — it hosts the HTTP and gRPC servers that accept spans from language tracers and OTel, rate-limits and decodes incoming payloads, and also proxies profiling, DogStatsD, EVP, and other auxiliary protocols to the Datadog backend.

# pkg/trace/api

## Purpose

`pkg/trace/api` is the front-door of the trace-agent. It hosts the HTTP and gRPC servers that accept spans from language tracers (Go, Python, Java, Ruby, …), converts them into an internal `Payload` struct, and sends them downstream for sampling, aggregation, and forwarding to the Datadog backend.

Beyond trace ingestion the package also proxies several auxiliary protocols (profiling, DogStatsD, OpenLineage, debugger, EVP) and exposes a `/info` discovery endpoint so tracers can auto-negotiate the API version.

## Key elements

### Core types

| Type | File | Description |
|------|------|-------------|
| `HTTPReceiver` | `api.go` | Central struct. Owns the `http.Server`, manages a semaphore (`recvsem`) to bound concurrent deserialisations, and holds output channels (`out chan *Payload`, `outV1 chan *PayloadV1`). |
| `OTLPReceiver` | `otlp.go` | Implements `ptraceotlp.GRPCServer`; accepts OpenTelemetry spans via HTTP or gRPC and converts them to `*Payload`. |
| `Payload` | `payload.go` | Wrapper around a `*pb.TracerPayload` (protobuf). Carries source metadata, container tags, and client-side hints (`ClientComputedTopLevel`, `ClientComputedStats`, `ClientDroppedP0s`). |
| `PayloadV1` | `payload.go` | Like `Payload` but wraps the newer `*idx.InternalTracerPayload` used by the v1.0 wire format. |
| `Endpoint` | `endpoints.go` | Descriptor for a single HTTP route: pattern, handler factory, optional `IsEnabled` predicate, and `TimeoutOverride`. |
| `StatsProcessor` | `api.go` | Interface — implementors receive pre-computed client stats (`/v0.6/stats`). |
| `IDProvider` | `container.go` | Resolves a container ID from request context or origin-detection headers. |

### Sub-packages

#### `apiutil`
Path: `pkg/trace/api/apiutil`

`LimitedReader` — an `io.ReadCloser` that enforces `MaxRequestBytes`. When the limit is hit it returns `ErrLimitedReaderLimitReached` rather than silently truncating, so callers can distinguish a full payload from an oversized one.

`SetupCoverageHandler` (build tag `e2ecoverage`) — wires `/coverage` for E2E test coverage collection.

#### `internal/header`
Path: `pkg/trace/api/internal/header`

Pure-constant package. Defines all `Datadog-*` / `X-Datadog-*` HTTP header names used by the intake protocol:

| Constant | Header |
|----------|--------|
| `TraceCount` | `X-Datadog-Trace-Count` |
| `Lang` / `LangVersion` / `LangInterpreter` | tracer language metadata |
| `ContainerID` | `Datadog-Container-ID` (deprecated) |
| `LocalData` | `Datadog-Entity-ID` (origin detection) |
| `ExternalData` | `Datadog-External-Env` (pod/container metadata) |
| `ComputedTopLevel` / `ComputedStats` | client-side pre-computation hints |
| `DroppedP0Traces` / `DroppedP0Spans` | client-side drop counters |
| `RatesPayloadVersion` | sampling-rate cache busting |
| `SendRealHTTPStatus` | opt-in 429 responses |
| `TracerObfuscationVersion` | avoid double-obfuscation |

#### `loader`
Path: `pkg/trace/api/loader`

Provides helpers for **zero-downtime socket handoff** when the trace-agent is managed by the `trace-loader` supervisor process.

| Function | Description |
|----------|-------------|
| `GetListenerFromFD(fdStr, name)` | Converts an already-open TCP fd (passed as `DD_APM_NET_RECEIVER_FD`) into a `net.Listener`. |
| `GetConnFromFD(fdStr, name)` | Converts an already-open TCP fd (passed as `DD_APM_NET_RECEIVER_CLIENT_FD`) into a `net.Conn`. Used to replay an in-flight initial client connection. |
| `GetTCPListener(addr)` | Normal `net.Listen("tcp", addr)` wrapper. |
| `NewListenerInitialConn(ln, conn)` | Wraps a listener so it serves `conn` as the very first accepted connection before delegating to `ln`. |

### Endpoints

The `endpoints` slice registers all routes on `HTTPReceiver`. Key entries:

| Pattern | Purpose |
|---------|---------|
| `/v0.3/traces` … `/v0.7/traces`, `/v1.0/traces` | Trace ingestion (msgpack or protobuf, version-negotiated) |
| `/v0.6/stats` | Client-computed APM stats |
| `/evp_proxy/v1/` … `/evp_proxy/v4/` | EVP (Event Platform) proxy to Datadog intake |
| `/profiling/v1/input` | Continuous profiling proxy |
| `/telemetry/proxy/` | Instrumentation Telemetry proxy |
| `/debugger/v1/input`, `/debugger/v2/input` | Remote Configuration / Dynamic Instrumentation |
| `/dogstatsd/v2/proxy` | DogStatsD over HTTP proxy |
| `/tracer_flare/v1` | Tracer flare (support diagnostics) |
| `/openlineage/api/v1/lineage` | OpenLineage proxy |
| `/info` | Discovery: returns supported endpoints and agent version |

Additional endpoints can be registered at startup with `AttachEndpoint(e Endpoint)`.

### Important functions

| Function | Description |
|----------|-------------|
| `NewHTTPReceiver(conf, dynConf, out, outV1, statsProcessor, …)` | Creates the receiver. Sizes the decode semaphore to `conf.Decoders` (default: `GOMAXPROCS/2`). |
| `(*HTTPReceiver).Start()` | Binds TCP, UDS, or Windows named-pipe listeners (one or more); starts the HTTP server. Falls back gracefully if file-descriptor handoff fails. |
| `(*HTTPReceiver).Stop()` | Drains in-flight requests and shuts down listeners. |
| `AttachEndpoint(e Endpoint)` | Registers an extra endpoint before `Start()`. Not thread-safe. |

## Usage

### Startup sequence

```go
recv := api.NewHTTPReceiver(conf, dynConf, outChan, outV1Chan, statsProc, telemetry, statsdClient, timing)
recv.Start()
// ... later:
recv.Stop()
```

Payloads appear on `outChan` as `*api.Payload` and on `outV1Chan` as `*api.PayloadV1`. The trace-agent's main pipeline reads these channels and hands them to the sampler / concentrator / writer.

### Extending with a new endpoint

```go
api.AttachEndpoint(api.Endpoint{
    Pattern: "/my/custom/endpoint",
    Handler: func(r *api.HTTPReceiver) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { /* ... */ })
    },
    IsEnabled: func(cfg *config.AgentConfig) bool { return cfg.MyFeatureEnabled },
})
```

### Transport and listener configuration

The receiver supports three listener types, controlled by `config.AgentConfig`:

| Field | Transport |
|-------|-----------|
| `ReceiverPort > 0` | TCP (default: `127.0.0.1:8126`) |
| `ReceiverSocket != ""` | Unix domain socket |
| `WindowsPipeName != ""` | Windows named pipe |

All three can be active simultaneously. If `DD_APM_NET_RECEIVER_FD` is set, the TCP socket is inherited from the loader process instead of being bound fresh.

### Per-endpoint timeouts

The default request timeout is `conf.ReceiverTimeout` (5 s fallback). Use `Endpoint.TimeoutOverride` to set a different timeout for long-running handlers (e.g. profiling uses `conf.ProfilingProxy.ReceiverTimeout`, EVP proxy uses 30 s).
