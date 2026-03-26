# comp/forwarder/defaultforwarder — Default Metric/Event Forwarder Component

**Import path:** `github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder`
**Team:** agent-metric-pipelines
**Importers:** ~39 packages

## Purpose

`comp/forwarder/defaultforwarder` sends serialized agent payloads (metrics series, sketches, service checks, events, host metadata, process checks, orchestrator payloads, etc.) to the Datadog backend over HTTPS.

It is the transport layer for everything that flows through the `pkg/aggregator` and `pkg/serializer` stack. Producers hand it a slice of pre-serialized byte payloads; the forwarder wraps each in an `HTTPTransaction`, fans the transaction out to every configured domain/API-key pair, and delivers it via a pool of concurrent workers. Failed transactions are queued in a retry store — in-memory by default, spilling to disk for the core agent when `forwarder_storage_max_size_in_bytes > 0`.

## Package layout

| Package | Role |
|---|---|
| `comp/forwarder/defaultforwarder` (root) | `Component`/`Forwarder`/`ForwarderV2` interfaces, `Module()`, `Params`, `Options`, `DefaultForwarder`, `NoopForwarder` |
| `endpoints/` | Named `transaction.Endpoint` constants for every intake route |
| `resolver/` | `DomainResolver` abstraction (single-domain, MRF, OPW/vector, local cluster-agent) |
| `transaction/` | `HTTPTransaction`, `BytesPayloads`, priority/kind types, disk-serialization format |
| `internal/retry/` | In-memory and on-disk retry queues, disk-usage limit, file removal policy |

## Component and Forwarder interfaces

`Component` embeds `Forwarder` directly:

```go
type Component interface {
    Forwarder
}
```

The `Forwarder` interface covers all payload types the agent can submit:

```go
type Forwarder interface {
    // Metrics
    SubmitV1Series(payload BytesPayloads, extra http.Header) error
    SubmitSeries(payload BytesPayloads, extra http.Header) error        // v2
    SubmitSketchSeries(payload BytesPayloads, extra http.Header) error
    SubmitV1CheckRuns(payload BytesPayloads, extra http.Header) error
    // Metadata / intake
    SubmitV1Intake(payload BytesPayloads, kind Kind, extra http.Header) error
    SubmitHostMetadata(payload BytesPayloads, extra http.Header) error
    SubmitAgentChecksMetadata(payload BytesPayloads, extra http.Header) error
    SubmitMetadata(payload BytesPayloads, extra http.Header) error
    // Process/container/orchestrator (return a response channel)
    SubmitProcessChecks(payload BytesPayloads, extra http.Header) (chan Response, error)
    SubmitProcessDiscoveryChecks(payload BytesPayloads, extra http.Header) (chan Response, error)
    SubmitRTProcessChecks(payload BytesPayloads, extra http.Header) (chan Response, error)
    SubmitContainerChecks(payload BytesPayloads, extra http.Header) (chan Response, error)
    SubmitRTContainerChecks(payload BytesPayloads, extra http.Header) (chan Response, error)
    SubmitConnectionChecks(payload BytesPayloads, extra http.Header) (chan Response, error)
    SubmitOrchestratorChecks(payload BytesPayloads, extra http.Header, payloadType int) error
    SubmitOrchestratorManifests(payload BytesPayloads, extra http.Header) error
    // Generic / low-level
    ForwarderV2  // GetDomainResolvers(), SubmitTransaction(*HTTPTransaction)
}
```

Process-like submitters return a `chan Response` so the caller can read back the HTTP status code and body from the backend.

`NoopForwarder` is a zero-value implementation (no sends, no errors) used when a process needs to satisfy the `Component` interface without actually forwarding.

## Internal architecture

```
Producer
  │  Submit*(BytesPayloads)
  ▼
DefaultForwarder              (one per process)
  │  createHTTPTransactions() — fans out to every domain/key pair
  ▼
domainForwarder               (one per configured domain)
  │  HighPrio queue ← new transactions
  │  LowPrio queue  ← retry transactions
  ▼
Worker(s)                     (forwarder_num_workers per domain, default 4)
  │  HTTP POST with exponential-backoff circuit breaker per endpoint
  ▼
blockedEndpoints              (shared by all workers in a domain)
```

When the retry queue exceeds `forwarder_retry_queue_payloads_max_size`, the oldest transactions are dropped. Transactions flagged `StorableOnDisk` (all except those containing API keys) are serialised to disk via protobuf (`HttpTransactionProto.proto`) and reloaded on the next agent start.

## Features bitmask

`Options.EnabledFeatures` gates certain behaviour. The `Features` type is a bitmask:

| Constant | Effect |
|---|---|
| `CoreFeatures` | Enables on-disk retry queue (`agentName = "core"`) and `QueueDurationCapacity` telemetry |
| `TraceFeatures` | Reserved for trace-agent |
| `ProcessFeatures` | Reserved for process-agent |
| `SysProbeFeatures` | Reserved for system-probe |

## fx wiring

`defaultforwarder.Module(params)` is typically included through the forwarder bundle:

```go
// cmd/agent/subcommands/run/command.go
forwarder.Bundle(defaultforwarder.NewParams(
    defaultforwarder.WithFeatures(defaultforwarder.CoreFeatures),
))
```

The constructor `newForwarder` resolves endpoints from config (`utils.GetMultipleEndpoints`), optionally wires an Observability Pipelines Worker (OPW/vector) redirect for metrics, subscribes each `DomainResolver` to live API key updates, and registers `Start`/`Stop` hooks on the fx lifecycle.

It also provides a `status.InformationProvider` so forwarder state (queue depth, worker count, blocked endpoints) appears in `agent status`.

### Params

```go
// Typical core-agent usage
defaultforwarder.NewParams(
    defaultforwarder.WithFeatures(defaultforwarder.CoreFeatures),
)

// Disable API key validation (CI, unit tests)
defaultforwarder.NewParams(defaultforwarder.WithDisableAPIKeyChecking())

// Use domain resolvers directly (e.g. configsync)
defaultforwarder.NewParams(defaultforwarder.WithResolvers())
```

`ModulWithOptionTMP(option fx.Option)` is a temporary variant used by configsync that injects a pre-built `Params` via an fx option instead of `fx.Supply`.

### Mock and Noop modules

```go
defaultforwarder.MockModule()  // real DefaultForwarder, no domain resolvers
defaultforwarder.NoopModule()  // provides NoopForwarder (all methods are no-ops)
```

## Key configuration knobs

| Key | Default | Description |
|---|---|---|
| `forwarder_num_workers` | 4 | Concurrent HTTP workers per domain |
| `forwarder_retry_queue_payloads_max_size` | 15 MB | Max in-memory retry queue size |
| `forwarder_storage_max_size_in_bytes` | 0 (disabled) | Disk retry quota (core agent only) |
| `forwarder_storage_path` | `<run_path>/transactions_to_retry` | On-disk retry directory |
| `forwarder_stop_timeout` | 2 s | Grace period to drain queue on Stop |
| `forwarder_connection_reset_interval` | 0 | Periodically reset HTTP connections (0 = disabled) |
| `forwarder_backoff_factor` / `_base` / `_max` | 2 / 2 / 64 | Exponential backoff parameters |
| `forwarder_apikey_validation_interval` | 60 min | How often to revalidate API keys |

## Usage patterns

**Submitting metrics from the serializer:**

```go
type deps struct {
    fx.In
    Forwarder defaultforwarder.Component
}

func (s *mySerializer) flush(payload []byte) error {
    return s.Forwarder.SubmitSeries(
        transaction.BytesPayloads{{Bytes: &payload}},
        nil, // no extra headers
    )
}
```

**Submitting a process-check payload and reading the response:**

```go
responses, err := fwd.SubmitProcessChecks(payloads, extraHeaders)
if err != nil { return err }
for r := range responses {
    // r.StatusCode, r.Body, r.Err
}
```

**Low-level submission (e.g. from the trace agent):**

```go
t := transaction.NewHTTPTransaction()
t.Domain = "https://app.datadoghq.com"
t.Endpoint = endpoints.SeriesEndpoint
t.Payload = ...
fwd.SubmitTransaction(t)
```

## Related components

| Component | Relationship |
|---|---|
| [`comp/aggregator/demultiplexer`](../../comp/aggregator/demultiplexer.md) | Constructs the serializer and passes this component as its primary forwarder; wires it in the fx graph |
| [`comp/forwarder/eventplatform`](eventplatform.md) | Parallel forwarder for structured event-platform payloads; independent HTTP pipelines, separate from this component |
| [`comp/forwarder/orchestrator`](orchestrator.md) | Optional forwarder wrapping a `DefaultForwarder` dedicated to Kubernetes orchestrator payloads; exposes the same `Forwarder` interface |
| [`comp/forwarder/connectionsforwarder`](connectionsforwarder.md) | Wraps a `DefaultForwarder` instance dedicated to network connection check payloads (CNM/USM) |
| [`pkg/serializer`](../../pkg/serializer.md) | Sits directly above this component: serializes metrics into `transaction.BytesPayloads` then calls `Submit*` methods |
| [`comp/serializer/metricscompression`](../serializer/metricscompression.md) | Provides the `Compressor` used by the serializer before payloads reach this forwarder |

## Key consumers

- `pkg/serializer` — flushes metric series, sketches, service checks, and events
- `pkg/aggregator` (via `comp/aggregator/demultiplexer`) — wired in at `AgentDemultiplexer` construction
- `cmd/agent`, `cmd/dogstatsd`, `cmd/cluster-agent`, `cmd/cluster-agent-cloudfoundry`, `cmd/otel-agent` — agent processes that start a forwarder
- `comp/snmptraps/senderhelper` — submits SNMP trap metrics
- `comp/ndmtmp/forwarder` — NDM temporary forwarder path
