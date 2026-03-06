# Config Stream Consumer Component

A shared Go library for remote agents (system-probe, trace-agent, process-agent, etc.) to consume configuration streams from the core Datadog Agent. It provides gRPC connection management, snapshot gating, ordered config application, and a `model.Reader`-compatible API so agents do not need to implement their own config-stream plumbing.

## Overview

- **Real-time config**: Receive full snapshot then incremental updates from the core agent over gRPC.
- **RAR-gated**: Only registered remote agents can subscribe; session ID is required (fixed or via `SessionIDProvider`).
- **Readiness gating**: `WaitReady(ctx)` blocks until the first config snapshot is received.
- **model.Reader**: Drop-in config access with `GetString`, `GetInt`, `GetBool`, etc., and optional `ConfigWriter` to mirror streamed config into the main config.
- **Ordered updates**: Sequential application by sequence ID; stale updates dropped, discontinuities trigger resync.
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
4. Consumer applies snapshot/updates in order and exposes them via `Reader()` and optional `ConfigWriter`.

See `../configstream/README.md` for the producer side and the gRPC/protobuf contract.

## Quick Start

Supply **either** a fixed `SessionID` **or** a `SessionIDProvider` (e.g. from the remote agent component). The consumer uses the provider at connect time so RAR can register first. 

**Blocking vs non-blocking startup** is determined in your run function: call `WaitReady(ctx)` to block until the first snapshot, or omit it to start immediately (see [Usage patterns](#usage-patterns)).

**Params must be provided as `*Params`** so FX injects into the consumer's optional `*Params` field. When `Params` is nil or both `SessionID` and `SessionIDProvider` are empty, the component is not created (e.g. when RAR is disabled).

```go
// 1. Add to FX dependencies
configstreamconsumerfx.Module()

// 2a. Fixed SessionID (when you already have it, e.g. from an earlier RAR registration)
fx.Provide(func(c config.Component) *configstreamconsumerimpl.Params {
    return &configstreamconsumerimpl.Params{
        ClientName:       "my-agent",
        CoreAgentAddress: net.JoinHostPort(c.GetString("cmd_host"), strconv.Itoa(c.GetInt("cmd_port"))),
        SessionID:        rarSessionID,
        ConfigWriter:     c, // optional: mirror streamed config into main config
    }
})

// 2b. SessionIDProvider (recommended when using RAR: consumer waits for registration at connect time)
fx.Provide(func(ra remoteagent.Component) configstreamconsumerimpl.SessionIDProvider {
    if ra == nil {
        return nil
    }
    if p, ok := ra.(configstreamconsumerimpl.SessionIDProvider); ok {
        return p
    }
    return nil
})
fx.Provide(func(c config.Component, deps struct {
    fx.In
    SessionProvider configstreamconsumerimpl.SessionIDProvider `optional:"true"`
}) *configstreamconsumerimpl.Params {
    host := c.GetString("cmd_host")
    port := c.GetInt("cmd_port")
    if port <= 0 {
        port = 5001
    }
    return &configstreamconsumerimpl.Params{
        ClientName:        "my-agent",
        CoreAgentAddress:  net.JoinHostPort(host, strconv.Itoa(port)),
        SessionIDProvider: deps.SessionProvider,
        ConfigWriter:      c, // optional: mirror streamed config into main config
    }
})
```

Use **2a** when you already have a session ID; use **2b** when the remote agent component implements `WaitSessionID(ctx) (string, error)` and you want the consumer to obtain the session ID at connect time (after RAR has registered). In your run function, inject the consumer and call `WaitReady(ctx)` for blocking startup, or start without it for non-blocking (eventually consistent) startup.

## Usage Patterns

### Pattern 1: Block until config ready (recommended)

Use when the agent needs the config to be fully populated before starting:

```go
func run(consumer configstreamconsumer.Component) error {
    if err := consumer.Start(context.Background()); err != nil {
        return fmt.Errorf("failed to start config stream: %w", err)
    }
    // Block until first config snapshot is received
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    if err := consumer.WaitReady(ctx); err != nil {
        return fmt.Errorf("config not ready: %w", err)
    }
    // Config is guaranteed to be fully populated here
    cfg := consumer.Reader()
    // Start agent with complete config
    return startAgent(cfg.GetInt("my_port"), cfg.GetBool("my_feature.enabled"))
}
```

### Pattern 2: Start immediately (eventually consistent)

Use when the agent can start with defaults and react to config changes:

```go
func run(consumer configstreamconsumer.Component) error {
    if err := consumer.Start(context.Background()); err != nil {
        return fmt.Errorf("failed to start config stream: %w", err)
    }
    // Do not call WaitReady(); config may be empty initially.
    changes, unsubscribe := consumer.Subscribe()
    defer unsubscribe()
    go func() {
        for change := range changes {
            log.Infof("Config changed: %s = %v", change.Key, change.NewValue)
            // Handle config updates dynamically
        }
    }()
    // Start your agent immediately (may use default values initially)
    return startAgent(consumer.Reader())
}
```

## Requirements

- **Core agent**: `configstream` component and RAR enabled (`remote_agent_registry.enabled: true`).
- **RAR**: Remote agent must register with RAR before subscribing; pass `session_id` via gRPC metadata (supply fixed `SessionID` or `SessionIDProvider` with `WaitSessionID(ctx) (string, error)`).
- **IPC**: mTLS and auth token for gRPC (same as other core-agent IPC).
- **ConfigWriter** (optional): If set, streamed snapshot/updates are written with `SourceLocalConfigProcess`, keeping main config in sync.

## Telemetry

| Metric | Type | Description |
|--------|------|-------------|
| `configstream_consumer.time_to_first_snapshot_seconds` | Gauge | Time to receive first snapshot |
| `configstream_consumer.reconnect_count` | Counter | Stream reconnections |
| `configstream_consumer.last_sequence_id` | Gauge | Last received config sequence ID |
| `configstream_consumer.dropped_stale_updates` | Counter | Stale updates dropped |
| `configstream_consumer.buffer_overflow_disconnects` | Counter | Disconnects due to subscriber buffer overflow |

## Testing

### Unit tests

```bash
cd comp/core/configstreamconsumer/impl
go test -tags test -v
```
Covers: snapshot and ordered updates, stale/dropped updates, `WaitReady` blocking, change subscription, reader API, discontinuity handling.

### Manual testing with system-probe

1. Start the core agent with RAR and config stream enabled.
2. Set `remote_agent_registry.enabled: true` and `cmd_host` / `cmd_port` in the config used by system-probe.
3. Start system-probe. You should see:
   - `Waiting for initial configuration from core agent...`
   - After snapshot: `Initial configuration received from core agent. Starting system-probe.`
4. If the core agent is down or the stream never sends a snapshot, system-probe exits with: `waiting for initial config snapshot: context deadline exceeded`.

## Troubleshooting

- **Config streaming not in use**  
  Log shows `(remote_agent_registry.enabled=true)` but no wait: ensure the consumer receives non-nil `*Params` (FX provider must return `*configstreamconsumerimpl.Params`, not `Params`).

- **session_id required in metadata**  
  Ensure the remote agent registers with RAR first and that the consumer is given either a fixed `SessionID` or a `SessionIDProvider` that returns the session ID.

- **WaitReady timeout**  
  Core agent must be running, config stream enabled, and RAR returning a valid session. Check core agent logs for config stream and RAR errors.

## Related documentation

- **Producer**: `../configstream/README.md` — core agent config streaming service and gRPC contract.
- **Test client**: `cmd/config-stream-client/README.md` — standalone client for end-to-end testing.

**Team**: agent-metric-pipelines agent-configuration
