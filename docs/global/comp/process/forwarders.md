# comp/process/forwarders

## Purpose

The `forwarders` component bundles the three HTTP forwarder instances that the process-agent uses to ship data to Datadog:

| Forwarder | Data type |
|-----------|-----------|
| process forwarder | Full process snapshots (`/api/v1/collector`) |
| real-time process forwarder | High-frequency process updates (`/api/v1/collector`) |
| connections forwarder | Network connection data (`/api/v1/connections`) |

The process and real-time forwarders share the same API endpoint configuration but are separate `defaultforwarder.Component` instances so their in-flight queues remain independent. The connections forwarder is a specialised `connectionsforwarder.Component` that is injected as a dependency rather than created here.

## Key elements

### `comp/process/forwarders` (definition)

| Symbol | Description |
|--------|-------------|
| `Component` | Interface with three accessors: `GetProcessForwarder()`, `GetRTProcessForwarder()`, `GetConnectionsForwarder()`. |

### `comp/process/forwarders/forwardersimpl`

| Symbol | Description |
|--------|-------------|
| `Module()` | Returns the fx `fxutil.Module` that registers `newForwarders` as a provider. |
| `dependencies` | fx input struct: `config.Component`, `log.Component`, `connectionsforwarder.Component`, `compdef.Lifecycle`, `secrets.Component`. |
| `forwardersComp` | Concrete implementation of `Component`; stores the three forwarder instances. |
| `newForwarders(deps)` | Constructor. Reads `process_config.process_queue_bytes` (falls back to `DefaultProcessQueueBytes`), resolves API endpoints via `endpoint.GetAPIEndpoints`, then creates two `defaultforwarder` instances and wires in the injected connections forwarder. |
| `createForwarder(deps, options)` | Calls `defaultforwarder.NewForwarder` with `DisableAPIKeyChecking=true`. |
| `createParams(config, log, queueBytes, endpoints)` | Builds `defaultforwarder.Options` from API endpoints, attaches the retry queue byte limit. |

#### Configuration reference

| Key | Default | Effect |
|-----|---------|--------|
| `process_config.process_queue_bytes` | `pkgconfigsetup.DefaultProcessQueueBytes` | Maximum total payload bytes held in each forwarder's retry queue. |

### `comp/process/forwarders/forwardersimpl/forwarders_mock.go`

Provides a mock implementation of `Component` for use in tests.

## Usage

### Registration

The component is part of the process bundle (`comp/process/bundle.go`). To include it in a new binary, add both `forwardersimpl.Module()` and `connectionsforwarderfx.Module()` to the fx app, since the connections forwarder is an injected dependency:

```go
// In an fx app:
forwardersimpl.Module()
connectionsforwarderfx.Module()  // must be included separately
```

### Consuming the forwarders

The primary consumer is `comp/process/submitter`, which receives `forwarders.Component` as a dependency and maps check names to the correct `Submit*` method at construction time:

```go
// pkg/process/runner/submitter.go
processFwd := forwarders.GetProcessForwarder()
rtProcessFwd := forwarders.GetRTProcessForwarder()
connFwd := forwarders.GetConnectionsForwarder()  // delegated to connectionsforwarder.Component
```

The submitter serialises check payloads into `transaction.BytesPayloads` and calls the corresponding forwarder method — for example `SubmitProcessChecks` for full snapshots and `SubmitRTProcessChecks` for high-frequency updates. Each forwarder maintains an independent in-memory retry queue bounded by `process_config.process_queue_bytes`. See [comp/process/submitter](submitter.md) for the full submission flow including `WeightedQueue` sizing and RT-mode negotiation.

### Independent retry queues

The process and real-time forwarders are separate `defaultforwarder.Component` instances backed by independent `domainForwarder` goroutines and retry queues. This means a burst of full process snapshots cannot crowd out real-time updates (and vice versa). Endpoints and API keys are shared (both resolved via `endpoint.GetAPIEndpoints`), but in-flight queues are separate. For the internal `DefaultForwarder` architecture (worker pool, circuit breaker, disk spill) see [comp/forwarder/defaultforwarder](../forwarder/defaultforwarder.md).

### Connections forwarder

`GetConnectionsForwarder()` returns the injected `connectionsforwarder.Component`. This component owns its own `DefaultForwarder` instance and posts payloads to `/api/v1/connections`. Because it is injected rather than created here, it can be independently provided in the system-probe process as well as the process-agent process. See [comp/forwarder/connectionsforwarder](../forwarder/connectionsforwarder.md) for lifecycle details and how `pkg/network/sender` uses it directly in system-probe.

## Related documentation

| Document | Relationship |
|---|---|
| [comp/process/submitter](submitter.md) | Primary consumer; receives this component and maps each check name to the correct `Submit*` call; documents `WeightedQueue` sizing and RT-mode negotiation |
| [comp/forwarder/defaultforwarder](../forwarder/defaultforwarder.md) | Underlying forwarder type used for the process and RT-process instances; documents worker pool, retry queue, and circuit-breaker behaviour |
| [comp/forwarder/connectionsforwarder](../forwarder/connectionsforwarder.md) | The connections forwarder injected into this component; independently provides `/api/v1/connections` delivery with its own lifecycle and queue |
