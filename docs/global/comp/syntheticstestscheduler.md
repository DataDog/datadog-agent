# comp/syntheticstestscheduler

**Team:** synthetics-executing
**Package:** `github.com/DataDog/datadog-agent/comp/syntheticstestscheduler`

## Purpose

The `syntheticstestscheduler` component runs Datadog Synthetics network tests (TCP, UDP, ICMP) directly from the agent host. Test configurations are delivered via Remote Config (`state.ProductSyntheticsTest`). The scheduler maintains an in-memory table of scheduled tests, periodically enqueues them for execution, and forwards results to the Datadog backend via the Event Platform forwarder.

This allows Synthetics tests to be executed from private locations without a dedicated Synthetics private location runner, using the standard Datadog Agent as the execution host.

The component is disabled unless `synthetics.collector.enabled: true` is set.

## Key Elements

### Component interface

`comp/syntheticstestscheduler/def/component.go`

```go
type Component interface{}
```

The public interface is intentionally empty. The component self-registers its RC listener via `Provides.RCListener` and manages its own goroutine lifecycle.

### `Requires` / `Provides` (impl)

| Field | Type | Description |
|---|---|---|
| `Requires.EpForwarder` | `eventplatform.Component` | Send test results to the backend |
| `Requires.Traceroute` | `traceroute.Component` | Execute the underlying network path probe |
| `Requires.HostnameService` | `hostname.Component` | Annotate results with the agent's hostname |
| `Requires.Statsd` | `statsd.ClientInterface` | Emit internal telemetry (checks received, errors, etc.) |
| `Provides.RCListener` | `rctypes.ListenerProvider` | Registers `onConfigUpdate` for `state.ProductSyntheticsTest` |

### Test configuration: `SyntheticsTestConfig`

`comp/syntheticstestscheduler/common/data.go`

```go
type SyntheticsTestConfig struct {
    Version  int
    Type     string
    Config   struct {
        Assertions []Assertion
        Request    ConfigRequest  // UDPConfigRequest | TCPConfigRequest | ICMPConfigRequest
    }
    Interval int    // tick_every, in seconds
    OrgID    int
    PublicID string
    ResultID string // non-empty for on-demand tests
    RunType  string
}
```

`ConfigRequest` is a sealed interface with three concrete types (`UDPConfigRequest`, `TCPConfigRequest`, `ICMPConfigRequest`). Custom JSON unmarshalling uses the `subtype` field to select the concrete type.

### Assertions

Each test carries a list of `Assertion` objects evaluated after the probe completes. Supported types:

| Type | Metrics |
|---|---|
| `packetLossPercentage` | packet loss ratio |
| `latency` | avg / min / max RTT |
| `jitter` | avg jitter |
| `multiNetworkHop` | avg / min / max hop count |

Operators: `is`, `isNot`, `moreThan`, `moreThanOrEqual`, `lessThan`, `lessThanOrEqual`.

A test result is marked `"failed"` if any assertion is not satisfied, if the traceroute itself errors, or if 100% packet loss is observed and no explicit assertion expects it.

### Concurrency model

The scheduler has three concurrent goroutines:

1. **Flush loop** (`flushLoop`): Ticks at `synthetics.collector.flush_interval`. On each tick, iterates the running test table under a write lock, enqueues tests whose `nextRun` has passed into a buffered channel (`syntheticsTestProcessingChan`, capacity 100), and advances `nextRun` by the test's configured interval.

2. **Worker pool** (`runWorkers`): Starts `synthetics.collector.workers` goroutines. Each worker drains the processing channel and the on-demand channel (on-demand tests take priority). For each test it: converts the config to a `traceroute.Config`, calls `traceroute.Run`, evaluates assertions, and forwards the result via `epForwarder`.

3. **On-demand poller** (`onDemandPoller`): Polls `https://intake.synthetics.<site>/api/unstable/synthetics/agents/tests` every 2 seconds to retrieve tests that should run immediately (triggered from the Datadog UI). These are injected into `onDemandPoller.TestsChan` and consumed by workers with higher priority than scheduled tests.

### RC update handling

When RC pushes a new `state.ProductSyntheticsTest` configuration:
- New tests are added to the running table with `nextRun = now`.
- Existing tests whose `version` changed have their `nextRun` reset to `now` to run immediately.
- Tests no longer present in the RC update are removed.

An empty RC update is not a special-case reset (unlike `filterlist`); tests are simply removed if not in the new set.

### Configuration keys

| Key | Default | Description |
|---|---|---|
| `synthetics.collector.enabled` | `false` | Enable the component |
| `synthetics.collector.workers` | — | Number of worker goroutines |
| `synthetics.collector.flush_interval` | — | How often to check and enqueue due tests |
| `site` | `datadoghq.com` | Datadog site (used by the on-demand poller endpoint) |
| `api_key` | — | Used to authenticate on-demand poll requests |

## Usage

The component is wired into the agent binary in `cmd/agent/subcommands/run/command.go`:

```go
import syntheticsTestsfx "github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/fx"
// ...
syntheticsTestsfx.Module()
```

No other component holds a reference to `syntheticstestscheduler.Component`. Consumers interact with it only through Remote Config (to deliver test configurations) and through the Event Platform (to receive test results).

Test results are sent as `eventplatform.EventTypeSynthetics` messages containing a serialized `common.TestResult`, which embeds the full `NetworkPath` traceroute payload alongside assertion results, timing, and network statistics.

### End-to-end flow

```
Remote Config backend
  │  pushes state.ProductSyntheticsTest configs
  ▼
comp/remote-config/rcclient
  │  invokes onConfigUpdate (registered via Provides.RCListener)
  ▼
comp/syntheticstestscheduler (running table updated: add / update / remove tests)

  flushLoop (every synthetics.collector.flush_interval)
  │  enqueues due tests into syntheticsTestProcessingChan (capacity 100)
  ▼
worker goroutines (×synthetics.collector.workers)
  │  convert SyntheticsTestConfig → traceroute.Config
  │  comp/networkpath/traceroute.Run(ctx, cfg) → payload.NetworkPath
  │  evaluate Assertion list (packetLossPercentage, latency, jitter, multiNetworkHop)
  │  serialise common.TestResult (NetworkPath + assertion outcomes + timing)
  ▼
comp/forwarder/eventplatform
  │  SendEventPlatformEvent(msg, EventTypeSynthetics)
  │  BatchStrategy → http-synthetics.
  ▼
Datadog Synthetics backend

  onDemandPoller (every 2 s)
  │  GET https://intake.synthetics.<site>/api/unstable/synthetics/agents/tests
  │  injects on-demand tests into onDemandPoller.TestsChan
  │  workers drain on-demand channel with higher priority than scheduled channel
```

On-demand tests (triggered from the Datadog UI) bypass the flush loop entirely and are picked up by workers ahead of normally scheduled tests.

## Related components and packages

| Component / Package | Relationship |
|---|---|
| [`comp/networkpath/traceroute`](networkpath/traceroute.md) | The network probe primitive used to execute each synthetic test. Workers call `traceroute.Run` with a `config.Config` derived from `SyntheticsTestConfig` (protocol, destination, port, max TTL). The scheduler uses `impl-local` so probes run in-process inside the core agent. |
| [`comp/remote-config/rcclient`](remote-config/rcclient.md) | Delivers `state.ProductSyntheticsTest` configuration updates to the scheduler. `comp/syntheticstestscheduler` returns a `types.ListenerProvider` in its `Provides.RCListener` field; `rcclient` collects all group members and invokes `onConfigUpdate` on every RC poll. Unlike `filterlist`, an empty RC update simply removes all tests rather than falling back to a local configuration. |
| [`comp/forwarder/eventplatform`](forwarder/eventplatform.md) | Receives test results as `EventTypeSynthetics` messages routed to the `http-synthetics.` pipeline. The component calls `epForwarder.SendEventPlatformEvent` (non-blocking) after each test execution. Workers also emit internal telemetry (checks received, errors) via the injected `statsd.ClientInterface`. |
