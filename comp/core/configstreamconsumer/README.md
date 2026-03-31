# Config Stream Consumer Component

A shared Go library for remote agents (system-probe, trace-agent, process-agent, etc.) to consume configuration streams from the core Datadog Agent. It provides gRPC connection management, snapshot gating, and ordered config application, writing received settings directly into the agent's `config.Component`.

## Overview

- **Real-time config**: Receive full snapshot then incremental updates from the core agent over gRPC.
- **RAR-gated**: Only registered remote agents can subscribe; session ID is required (fixed or via `SessionIDProvider`).
- **Readiness gating**: `WaitReady(ctx)` blocks until the first config snapshot is received.
- **Single source of truth**: Streamed config is written into `config.Component` via `model.Writer`. Callers read config and subscribe to changes through `config.Component` directly — not through this component.
- **Ordered updates**: Sequential application by sequence ID; stale updates dropped, discontinuities trigger resync.
- **Restart safety**: `lastSeqID` is never reset on reconnect. If the core agent restarts and its sequence counter resets, the consumer logs an error and refuses the new snapshot until the sub-process itself restarts.
- **Telemetry**: Metrics for time-to-first-snapshot, reconnects, sequence ID, and dropped updates.

## Architecture

Producer (core agent) and consumer (remote agents) communicate over the same gRPC contract:

```
┌─────────────────────────┐          ┌─────────────────────────┐
│   Core Agent Process    │          │  Remote Agent Process   │
│                         │          │  (e.g. system-probe)    │
│  ┌──────────────────┐   │          │  ┌──────────────────┐   │
│  │  configstream    │   │  gRPC    │  │ configstream-    │   │
│  │  (producer)      │◄──┼──────────┼─►│ consumer         │   │
│  │                  │   │  stream  │  │                  │   │
│  └──────────────────┘   │          │  └──────────────────┘   │
└─────────────────────────┘          └─────────────────────────┘
```

**Flow:**

1. Remote agent registers with RAR and obtains `session_id` (or supplies it via `SessionIDProvider`).
2. Consumer connects to core agent and calls `StreamConfigEvents` with `session_id` in gRPC metadata.
3. Core agent validates the session and sends an initial snapshot, then streams incremental updates.
4. Consumer applies snapshot/updates in order and writes them into `config.Component` via `model.Writer`.

See `../configstream/README.md` for the producer side and the gRPC/protobuf contract.

## Quick Start

Supply **either** a fixed `SessionID` **or** a `SessionIDProvider` (e.g. from the remote agent component). The consumer uses the provider at connect time so RAR can register first.

**Blocking startup**: `OnStart` blocks until the first config snapshot is received, so all other components and the binary's `run` function see a fully-populated config without any extra synchronization. Set `Params.ReadyTimeout` to control how long to wait (default: 60s); exceeding it aborts FX startup.

**`Params` must be non-nil**: if a binary includes this component, it must provide `*Params`. Returning nil `Params` is an error — binaries that do not need config streaming should not include this component.

**`model.Writer`** (`config.Component`) must be provided in `Requires` as a mandatory FX dependency. Do not pass it via `Params`.

```go
// 1. Add configstreamconsumerfx.Module() to the binary's FX options.
configstreamconsumerfx.Module()

// 2. Provide the SessionIDProvider from the remote agent component (it will block until RAR registration completes).
fx.Provide(func(ra remoteagent.Component) configstreamconsumerimpl.SessionIDProvider {
    if ra == nil {
        return nil
    }
    if p, ok := ra.(configstreamconsumerimpl.SessionIDProvider); ok {
        return p
    }
    return nil
})

// 3. Provide *Params only when configstream is enabled.
//    config.Component satisfies model.Writer and is injected into Requires automatically by FX.
fx.Provide(func(c config.Component, deps struct {
    fx.In
    SessionProvider configstreamconsumerimpl.SessionIDProvider `optional:"true"`
}) (*configstreamconsumerimpl.Params, error) {
    if !c.GetBool("remote_agent.configstream.enabled") {
        // Do not return nil — do not include this component when streaming is disabled.
        return nil, errors.New("configstream is disabled; do not include configstreamconsumerfx.Module()")
    }
    host := c.GetString("cmd_host")
    port := c.GetInt("cmd_port")
    if port <= 0 {
        port = 5001
    }
    return &configstreamconsumerimpl.Params{
        ClientName:        "my-agent",
        CoreAgentAddress:  net.JoinHostPort(host, strconv.Itoa(port)),
        SessionIDProvider: deps.SessionProvider,
    }, nil
})
```


## Requirements

- **Core agent**: `configstream` component (`remote_agent.configstream.enabled: true`) and RAR enabled (`remote_agent.registry.enabled: true`).
- **RAR**: Remote agent must register with RAR before subscribing; pass `session_id` via gRPC metadata (supply fixed `SessionID` or `SessionIDProvider` with `WaitSessionID(ctx) (string, error)`).
- **IPC**: mTLS and auth token for gRPC (same as other core-agent IPC).
- **ConfigWriter**: `config.Component` (implements `model.Writer`) must be provided as a mandatory FX dependency in `Requires`. Streamed snapshot/updates are written with `SourceLocalConfigProcess`.

## Telemetry

| Metric | Type | Description |
|--------|------|-------------|
| `configstream_consumer.time_to_first_snapshot_seconds` | Gauge | Time to receive first snapshot |
| `configstream_consumer.reconnect_count` | Counter | Stream reconnections |
| `configstream_consumer.last_sequence_id` | Gauge | Last received config sequence ID |
| `configstream_consumer.dropped_stale_updates` | Counter | Stale updates dropped |

## Testing

### Manual testing with system-probe

1. Start the core agent with RAR and config stream enabled.
2. Set `cmd_host` / `cmd_port` in the config used by system-probe.
3. Start system-probe. You should see:
   - `Waiting for initial configuration from core agent...`
   - After snapshot: `Initial configuration received from core agent. Starting system-probe.`
4. If the core agent is down or the stream never sends a snapshot, system-probe exits with: `waiting for initial config snapshot: context deadline exceeded`.

## Troubleshooting

- **session_id required in metadata**
  Ensure the remote agent registers with RAR first and that the consumer is given either a fixed `SessionID` or a `SessionIDProvider` that returns the session ID.

- **WaitReady timeout**
  Core agent must be running, config stream enabled, and RAR returning a valid session. Check core agent logs for config stream and RAR errors.

- **"core agent may have restarted" error in logs**
  The consumer received a snapshot with a lower sequence ID than its last-known value, indicating the core agent restarted. Restart the sub-process to accept the new configuration.

## Related documentation

- **Producer**: `../configstream/README.md` — core agent config streaming service and gRPC contract.
- **Test client**: `cmd/config-stream-client/README.md` — standalone client for end-to-end testing.

**Team**: agent-configuration
