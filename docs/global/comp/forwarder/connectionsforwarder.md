> **TL;DR:** Provides a dedicated `DefaultForwarder` for network connection check payloads (CNM/USM), isolated from other process-agent forwarding so the connections pipeline can be independently started, stopped, and configured.

# comp/forwarder/connectionsforwarder — Connections Forwarder Component

**Import path:** `github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def`
**Team:** container-experiences
**Importers:** ~11 packages

## Purpose

`comp/forwarder/connectionsforwarder` provides a dedicated `DefaultForwarder` for submitting network connection check payloads (CNM/USM data) from the system-probe to the Datadog backend. It is separate from the process-agent's general-purpose forwarder so that the connections pipeline can be independently configured, started, and stopped without interfering with other process check forwarding.

The component owns the full lifecycle of its inner `DefaultForwarder` (start on `OnStart`, stop on `OnStop`) and exposes a single method matching the `defaultforwarder.Forwarder` interface subset relevant to connections data.

## Package layout

| Package | Role |
|---|---|
| `comp/forwarder/connectionsforwarder/def` | `Component` interface |
| `impl/` | `NewComponent` constructor, `createParams` helper |
| `fx/` | `Module()` wiring; also provides the component as optional via `ProvideOptional` |
| `mock/` | `Mock()` — returns a `defaultforwarder.MockedForwarder` for tests |

## Key Elements

### Key interfaces

## Component interface

```go
type Component interface {
    SubmitConnectionChecks(payload transaction.BytesPayloads, extra http.Header) (chan defaultforwarder.Response, error)
}
```

The concrete implementation delegates directly to `defaultforwarder.DefaultForwarder.SubmitConnectionChecks`, which posts each `BytesPayload` to the `/api/v1/connections` endpoint with retry and queue support.

### Key types

## Configuration

`NewComponent` reads the following config keys to construct the inner forwarder:

| Key | Effect |
|---|---|
| `process_config.process_queue_bytes` | Max in-flight retry queue size in bytes (default: `DefaultProcessQueueBytes`) |

Endpoints are resolved via `pkg/process/runner/endpoint.GetAPIEndpoints`, which reads `process_config.process_dd_url` / `process_config.endpoints`. API key checking is disabled on the forwarder options (`DisableAPIKeyChecking: true`) because key validation is handled upstream.

### Key functions

## fx wiring

```go
// system-probe or process-agent startup:
connectionsforwarderfx.Module()

// Tests:
connectionsforwarder.Mock(t)
```

`Module()` also calls `fxutil.ProvideOptional`, so dependents can inject `optional.Option[connectionsforwarder.Component]` and handle the case where the component is absent (e.g. when running without network collection enabled).

### Dependencies injected by fx

`NewComponent` requires:
- `compdef.Lifecycle` — registers `Start`/`Stop` hooks on the inner `DefaultForwarder`
- `config.Component` — reads queue size and endpoint configuration
- `log.Component` — passed to the forwarder for internal logging
- `secrets.Component` — used for secret resolution in forwarder options

### Configuration and build flags

## Usage patterns

**Injecting and using the component:**

```go
type deps struct {
    fx.In
    Forwarder connectionsforwarder.Component
}

func (s *sender) send(payload transaction.BytesPayloads, headers http.Header) error {
    responses, err := s.Forwarder.SubmitConnectionChecks(payload, headers)
    if err != nil {
        return err
    }
    // optionally drain responses channel for error reporting
    return nil
}
```

**Via `comp/process/forwarders`:**

The process-agent uses `comp/process/forwarders` as an aggregator of all process-related forwarders. `forwardersimpl.GetConnectionsForwarder()` returns this component, and `pkg/process/runner/submitter` maps `checks.ConnectionsCheckName` directly to `forwarders.GetConnectionsForwarder().SubmitConnectionChecks`.

## Key consumers

- `pkg/network/sender` (system-probe) — the `Sender` struct holds a `connectionsforwarder.Component` as its `Forwarder` field and calls `SubmitConnectionChecks` with serialized connection data from `system-probe/api`
- `comp/process/forwarders/forwardersimpl` — aggregates this component alongside the process and RT-process forwarders and exposes it via `GetConnectionsForwarder()`
- `pkg/process/runner/submitter` — maps the `connections` check name to `SubmitConnectionChecks` in the submitter dispatch table
- `pkg/system-probe/api/module/common` — references the component when building the module-level API that serves connection data to the sender
