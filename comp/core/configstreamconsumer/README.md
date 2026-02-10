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

```go
// 1. Add to FX dependencies
fx.Provide(configstreamconsumerimpl.NewComponent)

// 2. Provide parameters (SessionID from RAR registration)
fx.Provide(func() configstreamconsumerimpl.Params {
    return configstreamconsumerimpl.Params{
        ClientName:       "my-agent",
        CoreAgentAddress: "localhost:5001",
        SessionID:        rarSessionID,
    }
})
```

// 3. Inject consumer and use in your remote agent
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

## Requirements

- **Core Agent**: Must have `configstream` component enabled (Phase 0) and RAR enabled
- **RAR Registration**: Remote agent must register with RAR before subscribing to config stream
- **Authentication**: SessionID from RAR registration is required and passed via gRPC metadata
- **IPC Component**: mTLS certificates for secure gRPC communication

## Team

**Team**: agent-metric-pipelines agent-configuration
