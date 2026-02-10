# Config Stream Consumer Component

A shared Go library for remote agents to consume configuration streams from the core Datadog Agent.

## Purpose

This component eliminates the need for each remote agent (system-probe, trace-agent, process-agent, etc.) to implement their own:
- gRPC connection management and reconnection logic
- Config sequencing and ordering
- Snapshot gating for startup readiness
- Update notification mechanics
- Thread-safe config reading

## Key Features

- **Automatic Connection Management**: Handles gRPC connection lifecycle with automatic reconnection
- **Readiness Gating**: `WaitReady()` blocks until first config snapshot is received
- **model.Reader Interface**: Drop-in replacement for local config access
- **Ordered Updates**: Guarantees sequential application of config changes
- **Change Subscription**: Optional notifications for config mutations
- **Telemetry**: Built-in metrics for observability
- **Thread-Safe**: Concurrent read access with RWMutex protection

## Quick Start

You must supply **either** a fixed `SessionID` **or** a `SessionIDProvider` (e.g. from the remote agent component); the consumer uses the provider at connect time so RAR can register first.

```go
// 1. Add to FX dependencies
configstreamconsumerfx.Module()

// 2a. Fixed SessionID (when you already have it)
fx.Provide(func() configstreamconsumerimpl.Params {
    return configstreamconsumerimpl.Params{
        ClientName:       "my-agent",
        CoreAgentAddress: "localhost:5001",
        SessionID:        rarSessionID,
    }
})

// 2b. SessionIDProvider (recommended when using RAR: consumer waits for registration)
//     Provide SessionIDProvider from your remote agent impl and optional ConfigWriter (e.g. config.Component) to mirror
//     streamed config into the main config. When SessionIDProvider is nil the component is not created (e.g. RAR disabled).
```

When `Params` is nil or both `SessionID` and `SessionIDProvider` are empty, the component is not created (e.g. when RAR is disabled). Inject the consumer and call `WaitReady(ctx)` before starting your agent for blocking startup.

## Usage Patterns

The consumer supports two distinct startup patterns depending on your remote agent's requirements:

### Pattern 1: Block Until Config Ready (Recommended for Most Cases)

Use this when your agent **requires** configuration to be fully populated before starting its core functionality. This ensures deterministic startup with complete config.

```go
func run(consumer configstreamconsumer.Component) error {
    // Start the background config stream
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
    port := cfg.GetInt("my_port")
    enabled := cfg.GetBool("my_feature.enabled")
    
    // Start your agent with complete config
    return startAgent(port, enabled)
}
```

**Use this pattern when:**
- Your agent needs specific config values to initialize (ports, endpoints, feature flags)
- You want deterministic behavior across all environments
- Config-driven behavior is critical for correct operation
- This is the equivalent of setting `wait_for_config = true`

### Pattern 2: Start Immediately (Eventually Consistent Config)

Use this when your agent can start with default/empty config and gracefully handle config updates as they arrive.

```go
func run(consumer configstreamconsumer.Component) error {
    // Start the background config stream
    if err := consumer.Start(context.Background()); err != nil {
        return fmt.Errorf("failed to start config stream: %w", err)
    }
    
    // DON'T call WaitReady() - proceed immediately
    
    // Config may be empty initially, will be populated asynchronously
    cfg := consumer.Reader()
    
    // Subscribe to config changes if you need to react to updates
    changes, unsubscribe := consumer.Subscribe()
    defer unsubscribe()
    
    go func() {
        for change := range changes {
            log.Infof("Config changed: %s = %v", change.Key, change.NewValue)
            // Handle config updates dynamically
        }
    }()
    
    // Start your agent immediately (may use default values initially)
    return startAgent(cfg)
}
```

**Use this pattern when:**
- Your agent has sensible defaults and can operate without full config
- You want the fastest possible startup time
- Your agent can dynamically reconfigure itself when config arrives
- You're migrating from `configsync` and want to maintain existing behavior
- This is the equivalent of setting `wait_for_config = false` (default)

### Choosing the Right Pattern

| Consideration | Pattern 1 (Block) | Pattern 2 (Immediate) |
|--------------|-------------------|----------------------|
| Startup time | Slightly slower (waits for snapshot) | Fastest (no wait) |
| Config guarantees | Fully populated before start | Eventually consistent |
| Complexity | Simple, synchronous flow | Requires change handling |
| Error handling | Fails fast if config unavailable | Must handle missing config |
| Recommended for | New remote agents, config-critical apps | Legacy migrations, resilient apps |

**Default recommendation**: Use Pattern 1 (blocking) for new remote agents unless you have specific requirements for immediate startup.

## Documentation

- **Phase 1 Implementation**: `PHASE1.md` - Detailed implementation guide, architecture, and API reference
- **Producer Component**: `../configstream/README.md` - Core agent config streaming service (Phase 0)

## Testing

```bash
cd impl
go test -tags test -v
```

### Testing config streaming with system-probe

**Manual testing**

1. Start the core agent with RAR and config stream enabled (default in development).
2. Enable the remote agent registry for system-probe in system-probe config (e.g. `remote_agent_registry.enabled: true`) and set `cmd_host` / `cmd_port` to the core agent IPC address.
3. Start system-probe. With config streaming enabled, system-probe will:
   - Log: `Waiting for initial configuration from core agent...`
   - Block until the first config snapshot is received (up to 60s).
   - Then log: `Initial configuration received, starting system-probe` and continue.
4. If the core agent is not running or config stream is not ready, system-probe will exit with: `waiting for initial config snapshot: context deadline exceeded`.

**Unit tests**

The config stream consumer has unit tests in `impl/consumer_test.go` (build tag `test`). They cover blocking/non-blocking patterns, `WaitReady`, and snapshot/update ordering. They do not start the full system-probe process. To assert that system-probe blocks on `WaitReady` in integration, you would need an integration test that runs the system-probe FX graph with a mock config stream server and checks that startup does not complete until the mock sends a snapshot.

## Requirements

- **Core Agent**: Must have `configstream` component enabled (Phase 0) and RAR enabled
- **RAR Registration**: Remote agent must register with RAR before subscribing to config stream
- **Authentication**: SessionID is required and is passed via gRPC metadata. Supply either:
  - **SessionID**: a fixed string (e.g. from an earlier RAR registration), or
  - **SessionIDProvider**: an interface with `WaitSessionID(ctx) (string, error)` so the consumer obtains the session ID at connect time (e.g. from the remote agent component after RAR has registered).
- **ConfigWriter** (optional): If set, streamed snapshot/updates are written to the given `model.Writer` with `SourceLocalConfigProcess`, so the main config stays in sync (replacing configsync-style behavior).
- **IPC Component**: mTLS certificates for secure gRPC communication

## Team

**Team**: agent-metric-pipelines agent-configuration
