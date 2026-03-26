> **TL;DR:** The scheduling backbone for all periodic metadata collection — it spawns one goroutine per registered `MetadataProvider` callback, drives each at the interval the callback returns, and gracefully drains them on shutdown.

# comp/metadata/runner — Metadata Collection Runner

**Import path:** `github.com/DataDog/datadog-agent/comp/metadata/runner`
**Team:** agent-configuration
**Importers:** ~20 packages

## Purpose

`comp/metadata/runner` is the scheduling backbone for all periodic metadata collection in the agent. It manages a pool of goroutines — one per registered metadata provider — and drives each provider at the interval the provider itself returns. When the agent starts, the runner launches all registered providers; when the agent stops, it gracefully waits for in-flight collections to finish before exiting.

Without the runner, no metadata payload (host, inventory-agent, inventory-host, inventory-checks, etc.) would be sent periodically to the Datadog backend.

## Package layout

| Package | Role |
|---|---|
| `comp/metadata/runner` | Component interface (`Component`, empty marker) |
| `comp/metadata/runner/runnerimpl` | Implementation (`runnerImpl`), `Provider` type, `NewProvider` helper, fx `Module()` |

## Key elements

### Key interfaces

```go
type Component interface{}
```

The `Component` interface is intentionally empty. Its fx type exists solely to give the fx dependency graph something to depend on so the runner's lifecycle hooks (start/stop) are guaranteed to execute. Consumers never call methods on it directly.

### Key types

#### `runnerimpl.MetadataProvider`

```go
type MetadataProvider func(context.Context) time.Duration
```

The callback signature every metadata provider must implement. It is called once per collection cycle. It must:
1. Collect and send its metadata payload.
2. Return the `time.Duration` the runner should wait before calling it again.

#### `runnerimpl.Provider`

```go
type Provider struct {
    fx.Out
    Callback MetadataProvider `group:"metadata_provider"`
}
```

An fx value type. Wrap your `MetadataProvider` function in a `Provider` so fx can inject it into the runner's provider group.

### Key functions

#### `runnerimpl.NewProvider`

```go
func NewProvider(callback MetadataProvider) Provider
```

Convenience constructor. Pass `nil` as callback to register a no-op provider (used by inventory payloads when the `enable_metadata_collection` flag is false).

### Configuration and build flags

| Key | Default | Description |
|---|---|---|
| `enable_metadata_collection` | `true` | Master switch; set to `false` to disable all periodic metadata |
| `metadata_provider_stop_timeout` | `2s` | Maximum time to wait for an in-flight provider to finish on stop |

## fx wiring

The component is provided by `runnerimpl.Module()`, which is included in `metadata.Bundle()`. It collects all values tagged `group:"metadata_provider"` and spawns one goroutine per provider on `OnStart`.

```go
// Register a new provider from another component's constructor:
return provides{
    Comp:     myComp,
    Provider: runnerimpl.NewProvider(myComp.collect), // registered automatically
}
```

The runner itself is enabled by the `enable_metadata_collection` configuration key (default: `true`). When disabled, no goroutines are started and a warning is logged.

## Lifecycle

```
fx OnStart  →  runner.start()  →  one goroutine per provider (handleProvider)
fx OnStop   →  runner.stop()   →  closes stopChan, waits for WaitGroup
```

Each provider goroutine follows this loop:

1. Call the provider function in a goroutine, capturing the returned interval via a channel.
2. Wait for either the interval (then loop) or `stopChan` (then drain and return).
3. On stop: give the running collection up to `metadata_provider_stop_timeout` seconds to complete, then forcibly exit.

This design prevents payload corruption on shutdown (the serializer is not called mid-write) while still bounding the shutdown delay.

## Configuration

| Key | Default | Description |
|---|---|---|
| `enable_metadata_collection` | `true` | Master switch; set to `false` to disable all periodic metadata |
| `metadata_provider_stop_timeout` | `2s` | Maximum time to wait for an in-flight provider to finish on stop |

## Usage

### Implementing a new metadata provider

Any component that wants to participate in periodic metadata collection should:

1. Implement a `func(ctx context.Context) time.Duration` method.
2. Return a `runnerimpl.Provider` as an fx output from its constructor.

```go
import "github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"

type provides struct {
    fx.Out
    Comp     mypackage.Component
    Provider runnerimpl.Provider
}

func newMyComponent(deps dependencies) provides {
    c := &myComponent{...}
    return provides{
        Comp:     c,
        Provider: runnerimpl.NewProvider(c.collect),
    }
}

func (c *myComponent) collect(ctx context.Context) time.Duration {
    // build and send payload
    return 10 * time.Minute // next collection in 10 minutes
}
```

### Depending on the runner

Components that must ensure the runner is active (e.g., to guarantee metadata is sent before a command exits) can inject `runner.Component`:

```go
type dependencies struct {
    fx.In
    Runner runner.Component
}
```

## Related packages

| Package | Relationship |
|---------|-------------|
| [`comp/metadata/host`](host.md) | Registers a `runnerimpl.Provider` that drives the v5 host metadata collection on an exponential-backoff schedule (5 min → 30 min). The runner is the only caller of the `host` component's `collect` callback. |
| [`comp/metadata/inventoryagent`](inventoryagent.md) | Registers a `runnerimpl.Provider` via `util.InventoryPayload.MetadataProvider()`. The runner calls it at up to `inventories_max_interval`; the component can also schedule an early flush via `Refresh()` when configuration changes, subject to `inventories_min_interval`. |
| [`comp/metadata/inventorychecks`](inventorychecks.md) | Same pattern as `inventoryagent`: registers a `runnerimpl.Provider` and exposes a `Refresh()` that respects `inventories_min_interval`. The runner calls it periodically; the component self-triggers when checks are added or removed. |
| [`comp/metadata/inventoryhost`](inventoryhost.md) | Registers a `runnerimpl.Provider` via `util.InventoryPayload.MetadataProvider()`. Entirely data-driven at collection time (no `Set` caching). The runner drives the only path through which host hardware metadata reaches the backend. |
| [`comp/metadata/clusterchecks`](clusterchecks.md) | The cluster-agent-only metadata component also registers a `runnerimpl.Provider` so cluster check state is periodically flushed. Its `Provides` struct emits a `runnerimpl.Provider` the same way as the other inventory components. |
| [`comp/forwarder/defaultforwarder`](../forwarder/defaultforwarder.md) | The runner itself does not call the forwarder directly. Each provider callback (e.g. `inventoryagent.collect`) calls a `serializer.MetricSerializer`, which in turn calls `defaultforwarder.SubmitMetadata` / `SubmitAgentChecksMetadata` to deliver the payload. The runner's role is purely scheduling. |
