# pkg/security/reporter — CWS event reporter

## Purpose

`pkg/security/reporter` wires the Datadog **logs pipeline** into CWS so that security events (rule matches, anomaly detections, etc.) can be shipped to the Datadog intake as log-like messages. It creates a `logs/pipeline.Provider`, obtains a pipeline channel from it, and returns a `RuntimeReporter` that the rest of CWS uses to send raw byte payloads to the intake without needing to know about the logs subsystem internals.

The package has no build tag and is used on both Linux and Windows wherever `security-agent` runs.

## Key elements

### Types

| Type | File | Description |
|------|------|-------------|
| `RuntimeReporter` | `reporter.go` | Implements `seccommon.RawReporter`. Holds a `*sources.LogSource`, a hostname string, and a `chan *message.Message` obtained from the pipeline provider. |

`RuntimeReporter` satisfies `seccommon.RawReporter` (defined in `pkg/security/common`):

```go
type RawReporter interface {
    ReportRaw(content []byte, service string, timestamp time.Time, tags ...string)
}
```

### Constructor

```go
func NewCWSReporter(
    hostname string,
    stopper startstop.Stopper,
    endpoints *logsconfig.Endpoints,
    context *client.DestinationsContext,
    compression compression.Component,
) (seccommon.RawReporter, error)
```

Internally delegates to `newReporter` with fixed source name `"runtime-security-agent"` and source type `"runtime-security"`.

`newReporter` sets up:
- A `pipeline.Provider` with 4 pipelines and a `NoopSink` (events are not stored locally, only forwarded).
- A `sources.LogSource` tagged with the provided source name and type.
- Registers the provider with `stopper` so it is shut down cleanly with the rest of the agent.

### Sending events

```go
func (r *RuntimeReporter) ReportRaw(content []byte, service string, timestamp time.Time, tags ...string)
```

Wraps `content` in a `message.Message` with `StatusInfo`, attaches origin tags and service, sets the hostname, and pushes it onto `logChan`. The log pipeline serializes, compresses, and HTTP-POSTs it to the intake endpoint.

## Usage

`NewCWSReporter` is called in `pkg/security/agent/start.go` during security-agent startup, once per intake track:

```go
// Runtime events (CWS detections, anomalies)
endpoints, ctx, _ := common.NewLogContextRuntime(useSecRuntimeTrack)
runtimeReporter, _ := reporter.NewCWSReporter(hostname, stopper, endpoints, ctx, compression)

// Remediation / sec-info events
secInfoEndpoints, secInfoCtx, _ := common.NewLogContextSecInfo()
secInfoReporter, _ := reporter.NewCWSReporter(hostname, stopper, secInfoEndpoints, secInfoCtx, compression)

agent.Start(runtimeReporter, endpoints, secInfoReporter, secInfoEndpoints)
```

Inside the module, `pkg/security/module/msg_sender.go` also calls `NewCWSReporter` to create a reporter for the system-probe side (when `direct_send_from_system_probe` is enabled), using `seccommon.RawReporter` as the interface throughout.

The intake endpoints and destinations context (`logsconfig.Endpoints`, `client.DestinationsContext`) are built separately by `pkg/security/common` (see `common.NewLogContextRuntime`, `common.NewLogContextSecInfo`), keeping endpoint configuration out of this package.

### Two reporter instances

The security-agent always creates **two** `RuntimeReporter` instances with separate `pipeline.Provider` configurations:

| Reporter field | Log context helper | Track | Purpose |
|---|---|---|---|
| `reporter` | `common.NewLogContextRuntime` | `SecRuntime` or `Logs` | Main CWS detections, anomaly events |
| `secInfoReporter` | `common.NewLogContextSecInfo` | `SecInfo` | Remediation and security-info events |

`RuntimeSecurityAgent.DispatchEvent` routes each incoming `SecurityEventMessage` to the correct reporter by inspecting `evt.Track == string(common.SecInfo)`. See [agent.md](agent.md) for the routing logic and [common.md](common.md) for the track-type constants.

### Pipeline configuration

`newReporter` creates its `pipeline.Provider` with:
- **4 parallel pipelines** — provides throughput without head-of-line blocking.
- **`NoopSink`** — events are forwarded to the intake only, not stored locally.
- **`StaticHostnameService`** (from `pkg/security/common`) — injects the agent hostname into log messages without a full hostname resolution component.
- **`NoopStatusProvider`** (from `pkg/security/common`) — satisfies the pipeline's `StatusProvider` interface without side effects.

The reporter is registered with `stopper` so `pipeline.Provider.Stop()` is called during graceful agent shutdown. See [pkg/logs/pipeline.md](../../pkg/logs/pipeline.md) for full pipeline internals.

### direct_send_from_system_probe mode

When `runtime_security_config.direct_send_from_system_probe` is `true`, `StartRuntimeSecurity` in `pkg/security/agent` returns early and the security-agent does **not** create reporters. Instead, `pkg/security/module/msg_sender.go` (inside system-probe) calls `NewCWSReporter` directly and forwards events without going through the gRPC event stream. This mode bypasses `RuntimeSecurityAgent` entirely.

## Related documentation

| Doc | Description |
|-----|-------------|
| [agent.md](agent.md) | `StartRuntimeSecurity` calls `NewCWSReporter` twice and passes both reporters to `RuntimeSecurityAgent.Start`; `DispatchEvent` routes events to the correct reporter. |
| [common.md](common.md) | Provides `RawReporter` interface, `NewLogContextRuntime`, `NewLogContextSecInfo`, `StaticHostnameService`, `NoopStatusProvider`, and the `SecInfo` track constant consumed here. |
| [../../pkg/logs/pipeline.md](../../pkg/logs/pipeline.md) | The `pipeline.Provider` (4 pipelines, `NoopSink`) created inside `newReporter`; describes encoder/strategy selection and transport lifecycle. |
| [security.md](security.md) | Top-level CWS event flow: `RuntimeReporter` is the final hop before the Datadog intake in the `RuntimeSecurityAgent → RawReporter → Datadog backend` path. |
