> **TL;DR:** `pkg/logs/client` defines the `Destination` abstraction for log transport and provides two concrete implementations — an async HTTP batching client and a persistent TCP client — along with the `DestinationsContext` that coordinates graceful shutdown across all active connections.

# pkg/logs/client

## Purpose

Defines the shared abstractions for log transport destinations and provides two concrete implementations: an HTTP batching client (`http/`) and a persistent TCP client (`tcp/`). The package also manages the lifecycle context that coordinates graceful shutdown across all destinations.

## Key Elements

### Key interfaces

#### Base package (`pkg/logs/client`)

| Symbol | Description |
|---|---|
| `Destination` interface | Common contract for all transport destinations. Has `Start(input, output, isRetrying chan)`, `Target() string`, `IsMRF() bool`, and `Metadata()`. |
| `Destinations` | Groups a set of `Destination` into `Reliable` and `Unreliable` slices. Used by the sender/pipeline layer to fan out payloads. |
| `DestinationsContext` | Wraps a `context.Context` with `Start()`/`Stop()` lifecycle. Cancelling it unblocks in-flight destinations so the pipeline can shut down cleanly. |
| `DestinationMetadata` | Carries telemetry labels (`TelemetryName`, `MonitorTag`, `EvpCategory`) for a destination. Created with `NewDestinationMetadata(componentName, instanceID, kind, endpointID, evpCategory)`. Use `NewNoopDestinationMetadata()` for destinations that should not emit telemetry (e.g., test helpers, TCP). |
| `RetryableError` | Wraps an error to signal that the send can be retried (network errors, HTTP 5xx). Non-retryable errors (HTTP 4xx) are returned as plain errors and cause payload drops. |

### Key types

#### HTTP sub-package (`pkg/logs/client/http`)

| Symbol | Description |
|---|---|
| `Destination` | Full-featured async HTTP destination. Reads payloads from `input`, sends POST requests concurrently, retries on retryable errors with exponential backoff, and forwards payloads to `output` when done. |
| `NewDestination(endpoint, contentType, ctx, shouldRetry, destMeta, cfg, minConcurrency, maxConcurrency, pipelineMonitor, instanceID)` | Primary constructor. `shouldRetry` should be `false` for serverless. |
| `SyncDestination` | Simplified single-worker HTTP destination used in serverless mode. Does not retry; relies on the serverless flush strategy. |
| `workerPool` | Internal: dynamically scales the number of concurrent HTTP senders between `minWorkers` and `maxWorkers` using an EWMA of observed latency. Target latency is 150 ms by default. Scales up when virtual latency exceeds the target, and backs off to `minWorkers` on retryable errors. |
| `CheckConnectivity(endpoint, cfg)` | Sends an empty JSON payload with a 5 s timeout to validate connectivity at agent startup. Returns a `config.HTTPConnectivity` bool. |
| `CheckConnectivityDiagnose(endpoint, cfg)` | Like `CheckConnectivity` but uses the configured `logs_config.http_timeout` and returns the URL + error for diagnostic reporting. |
| Content type constants | `TextContentType`, `JSONContentType`, `ProtobufContentType`. |

HTTP requests include the following headers: `DD-API-KEY`, `Content-Type`, `User-Agent`, `DD-PROTOCOL`, `DD-EVP-ORIGIN`, `dd-message-timestamp`, `dd-current-timestamp`, and any `ExtraHTTPHeaders` from the endpoint config.

**Retry/backoff behavior:** 4xx responses (except `413`) are permanent drops. Network errors and 5xx responses are wrapped in `RetryableError` and retried with exponential backoff governed by `endpoint.BackoffFactor/Base/Max/RecoveryInterval/RecoveryReset`.

#### TCP sub-package (`pkg/logs/client/tcp`)

| Symbol | Description |
|---|---|
| `Destination` | Persistent TCP destination. Maintains a long-lived `net.Conn`, reconnects automatically on write failure (when `shouldRetry` is true), and resets the connection periodically if `endpoint.ConnectionResetInterval` is set. |
| `NewDestination(endpoint, useProto, ctx, shouldRetry, status)` | Constructor. `useProto=true` uses length-prefix framing; `false` uses newline framing. |
| `ConnectionManager` | Handles dial (plain TCP or SOCKS5 proxy), optional TLS handshake, exponential reconnect backoff (capped at ~2 min), and server-close detection. Updates `statusinterface.Status` with a global warning on persistent connection errors. |
| `Delimiter` interface | Two implementations: `lengthPrefixDelimiter` (4-byte big-endian length header) and `lineBreakDelimiter` (appends `\n`). Created via `NewDelimiter(useProto bool)`. |
| `prefixer` | Prepends the API key and a space to each payload before framing. |
| `CheckConnectivityDiagnose(endpoint, timeoutSeconds)` | Attempts a TCP dial and returns the address + error for diagnostic purposes. |

### Key functions

#### `CheckConnectivity` / `CheckConnectivityDiagnose`

| Function | Package | Description |
|---|---|---|
| `CheckConnectivity(endpoint, cfg)` | `http/` | Sends an empty JSON payload with a 5 s timeout to validate connectivity at agent startup. Returns a `config.HTTPConnectivity` bool. |
| `CheckConnectivityDiagnose(endpoint, cfg)` | `http/` | Like `CheckConnectivity` but uses the configured `logs_config.http_timeout` and returns the URL + error for diagnostic reporting. |
| `CheckConnectivityDiagnose(endpoint, timeoutSeconds)` | `tcp/` | Attempts a TCP dial and returns the address + error for diagnostic purposes. |

### Configuration and build flags

| Config key | Description |
|---|---|
| `endpoint.BackoffFactor` / `BackoffBase` / `BackoffMax` | HTTP exponential retry backoff parameters. |
| `endpoint.RecoveryInterval` / `RecoveryReset` | HTTP retry-state recovery settings. |
| `endpoint.ConnectionResetInterval` | TCP: if set, forces a periodic reconnect of the TCP connection. |
| `endpoint.ExtraHTTPHeaders` | Additional HTTP headers sent with every log request. |
| `logs_config.http_timeout` | HTTP request timeout used by `CheckConnectivityDiagnose`. |

## Usage

`pkg/logs/client` is used primarily by:

- **`pkg/logs/pipeline`** and **`pkg/logs/sender`** — construct `Destinations` objects, call `Destination.Start()` to wire payload channels through the transport layer, and use `DestinationsContext` to coordinate shutdown.
- **`comp/logs/agent/agentimpl`** — calls `CheckConnectivity` during agent startup to validate the HTTP endpoint before beginning log collection.
- **`comp/forwarder/eventplatform`** — creates HTTP destinations for the event platform forwarder using the same `Destination` interface.
- **`pkg/security/reporter`** and **`pkg/compliance/reporter`** — use HTTP destinations to ship security and compliance events.

Typical construction pattern:

```go
ctx := client.NewDestinationsContext()
ctx.Start()
defer ctx.Stop()

dest := http.NewDestination(endpoint, http.JSONContentType, ctx,
    true, client.NewDestinationMetadata(...), cfg, 1, 4, monitor, "0")

stop := dest.Start(inputChan, outputChan, isRetryingChan)
// close inputChan to stop; wait on stop channel for clean shutdown
```
