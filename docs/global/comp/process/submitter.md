> **TL;DR:** The component that serialises process-check payloads and routes them to the correct Datadog intake endpoint (process, real-time, or connections), managing submission queues, retries, and real-time mode negotiation with the runner.

# comp/process/submitter — Process Agent Payload Submitter Component

**Import path (interface):** `github.com/DataDog/datadog-agent/comp/process/submitter`
**Import path (implementation):** `github.com/DataDog/datadog-agent/comp/process/submitter/submitterimpl`
**Team:** container-experiences
**Importers:** `comp/process/runner/runnerimpl`, `comp/process/agent/agentimpl`

## Purpose

`comp/process/submitter` is the component that forwards collected process payloads to Datadog's process intake. After process checks (process list, container list, network connections, …) run and produce a `types.Payload`, the submitter serialises those payloads, routes them to the correct intake endpoint, and manages submission queues, retries, and real-time (RT) mode negotiation.

Wrapping `pkg/process/runner.CheckSubmitter` as an fx component lets the rest of the Process Agent depend on the submitter through a stable interface and makes it straightforward to swap in a mock during tests.

## Package layout

| Path | Role |
|---|---|
| `comp/process/submitter/` | `Component` interface (`component.go`), mock interface (`component_mock.go`) |
| `comp/process/submitter/submitterimpl/` | `Module()`, `newSubmitter` constructor, thin `submitterImpl` wrapper |
| `pkg/process/runner/submitter.go` | Core `CheckSubmitter` and `Submitter` interface |

## Key elements

### Key interfaces

```go
// Component embeds pkg/process/runner.Submitter.
type Component interface {
    processRunner.Submitter
}
```

`processRunner.Submitter` is defined in `pkg/process/runner`:

```go
type Submitter interface {
    Submit(start time.Time, name string, messages *types.Payload)
}
```

The implementation also exposes `Start() error` and `Stop()` which are called via fx lifecycle hooks (not part of the public interface, but present on `submitterImpl`).

### Key types

#### `types.Payload`

Defined in `comp/process/types`. Carries a slice of protobuf messages for a single check run along with sizing metadata used to enforce queue limits.

#### `types.RTResponse`

A channel value signalling whether real-time mode should be enabled or disabled for a given check. The submitter emits these after each successful HTTP response; the runner consumes them to adjust check frequency.

### Configuration and build flags

Queue sizing is governed by `process_config.process_queue_bytes` (configured on the forwarder side). Lifecycle hooks are registered only when `agent.Enabled()` returns true.

## fx wiring

`submitterimpl.Module()` provides two values:

| Output | Type | Description |
|---|---|---|
| `Submitter` | `submitter.Component` | The submitter itself |
| `RTResponseNotifier` | `<-chan types.RTResponse` | Channel for real-time mode changes |

Dependencies:

| Dependency | Type | Notes |
|---|---|---|
| `Config` | `config.Component` | Process agent configuration |
| `SysProbeConfig` | `sysprobeconfig.Component` | System-probe configuration (for NPM endpoints) |
| `Checks` | `[]types.CheckComponent` | All registered check components (grouped) |
| `Forwarders` | `forwarders.Component` | HTTP forwarders to process/NPM/RT intakes |
| `HostInfo` | `hostinfo.Component` | Hostname used in request headers |
| `Statsd` | `statsd.ClientInterface` | Internal metrics |

The submitter is started and stopped via fx lifecycle hooks only when `agent.Enabled(...)` returns true (i.e., when at least one check is enabled and the process agent is running as a standalone binary).

## Usage in the codebase

### Runner (`comp/process/runner/runnerimpl`)

The runner is the primary consumer. It receives `submitter.Component` and assigns it to `CheckRunner.Submitter`:

```go
c.Submitter = deps.Submitter
```

After each check run the runner calls `submitter.Submit(start, checkName, payload)`. The submitter enqueues the payload for the corresponding intake endpoint and fires off the HTTP transaction through the forwarder.

### Agent component (`comp/process/agent/agentimpl`)

`agentimpl` indirectly depends on the submitter through the runner. It uses `RTResponseNotifier` to detect when the backend asks for real-time mode and adjusts the check scheduling interval accordingly.

## Submission flow

```
Check produces types.Payload
    -> runner calls submitter.Submit(start, name, payload)
        -> CheckSubmitter serialises payload (protobuf)
        -> routes to correct submitFunc (process / RT / NPM / …)
        -> enqueues BytesPayload in a WeightedQueue
        -> forwarder drains queue, POSTs to intake
        -> response contains RT heartbeat
        -> RTResponse emitted on rtNotifierChan
```

## Related documentation

| Document | Relationship |
|---|---|
| [comp/process/runner](runner.md) | Primary consumer: assigns `submitter.Component` as `CheckRunner.Submitter` and receives the `<-chan types.RTResponse` channel to manage real-time scheduling |
| [comp/process/types](types.md) | Defines `Payload` (the unit the runner hands to `Submit`) and `RTResponse` (the signal the submitter emits after each intake response) |
| [comp/forwarder/connectionsforwarder](../../comp/forwarder/connectionsforwarder.md) | Dedicated forwarder for network connection payloads; the submitter maps `checks.ConnectionsCheckName` to `connectionsforwarder.SubmitConnectionChecks` via `comp/process/forwarders` |
| [pkg/process/runner](../../../global/pkg/process/runner.md) | Documents `CheckSubmitter` internals: `WeightedQueue` sizing, per-check `submitFunc` dispatch, request-ID generation, and drop-list configuration |
