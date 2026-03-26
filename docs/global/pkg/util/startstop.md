> **TL;DR:** Provides lightweight `Startable`/`Stoppable` interfaces and serial/parallel collection helpers to manage the lifecycle of agent component groups.

# pkg/util/startstop

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/startstop`

## Purpose

`startstop` provides lightweight interfaces and concrete implementations for
managing the lifecycle of agent components. It lets a caller start or stop a
group of components as a unit, either in series or in parallel, without having
to iterate manually or handle concurrency.

The package is deliberately small: it defines two orthogonal concerns
(`Startable`/`Stoppable`), groups them where needed, and provides constructors
for the common collection patterns.

## Key elements

### Key interfaces

| Interface | Method | Description |
|-----------|--------|-------------|
| `Startable` | `Start()` | A component that can be started. |
| `Stoppable` | `Stop()` | A component that can be stopped. |
| `StartStoppable` | `Start()` + `Stop()` | Convenience combination of both. |
| `Starter` | `Startable` + `Add(...Startable)` | A group of `Startable` objects that can be started together. |
| `Stopper` | `Stoppable` + `Add(...Stoppable)` | A group of `Stoppable` objects that can be stopped together. |

### Key functions

| Function | Description |
|----------|-------------|
| `NewStarter(components ...Startable) Starter` | Returns a serial starter. `Start()` calls each component's `Start()` in insertion order. |
| `NewSerialStopper(components ...Stoppable) Stopper` | Returns a serial stopper. `Stop()` calls each component's `Stop()` in insertion order. |
| `NewParallelStopper(components ...Stoppable) Stopper` | Returns a parallel stopper. `Stop()` calls every component's `Stop()` concurrently and waits for all of them to return. |

All constructors accept an optional variadic list of components, equivalent to
calling `Add()` for each one after construction.

### The `Add` method

Both `Starter` and `Stopper` expose `Add(components ...)` so components can be
accumulated before the collective `Start()` or `Stop()` is triggered. This
allows conditional registration:

```go
stopper := startstop.NewSerialStopper()
stopper.Add(componentA)
if featureEnabled {
    stopper.Add(componentB)
}
// ... later:
stopper.Stop()
```

## Usage

### Starting components in series

The `NewStarter` constructor is used when startup order matters — for example,
the logs agent starts its pipeline components in a fixed sequence so that each
stage is ready before the next one feeds it data:

```go
// From comp/logs/agent/agentimpl/agent.go
starter := startstop.NewStarter(
    a.destinationsCtx,
    a.auditor,
    a.pipelineProvider,
    a.diagnosticMessageReceiver,
    a.launchers,
)
starter.Start()
```

### Stopping components in series

`NewSerialStopper` is the mirror of `NewStarter`. It is commonly used where
teardown order is important (e.g., drain a pipeline before closing its
destination):

```go
// From comp/logs/agent/agentimpl/agent.go
stopper := startstop.NewSerialStopper(components...)
stopper.Stop()
```

### Stopping components in parallel

`NewParallelStopper` is preferred when components are independent and stopping
them concurrently is safe, shaving wall-clock time during agent shutdown:

```go
// From pkg/logs/launchers/file/launcher.go
stopper := startstop.NewParallelStopper()
stopper.Add(tailer1, tailer2, tailer3)
stopper.Stop() // all three Stop() calls run concurrently
```

### Where this pattern appears

`startstop` is used extensively in the logs pipeline (`pkg/logs/`), the
security module (`pkg/security/`), the OTel logs pipeline
(`comp/otelcol/logsagentpipeline/`), and agent subcommand entrypoints
(`cmd/cluster-agent/`, `cmd/security-agent/`).

## Relationship to other lifecycle patterns

### Component framework (fx)

Modern agent components rely on the Fx lifecycle hooks (`fx.Hook{OnStart, OnStop}`) provided by
`go.uber.org/fx`. `startstop` predates this and is used in packages that have not yet migrated
to the component framework, or where a lightweight imperative API is preferred over dependency
injection. The two approaches are composable: an Fx `OnStart` hook can call
`starter.Start()` on a group assembled with this package.

### pkg/util/subscriptions

[`pkg/util/subscriptions`](subscriptions.md) is the complement to `startstop` for **event
propagation** between components. Where `startstop` handles ordered lifecycle management,
`subscriptions` provides a type-safe, Fx-integrated pub/sub channel so that one component can
notify another without a direct dependency. The two are often used together: a component
registers a `Receiver` at construction time and starts draining it inside its `Start()` method.

### Security agent shutdown

`pkg/security/agent.RuntimeSecurityAgent` implements `Stop()` and is registered with a stopper
via `stopper.Add(agent)` (see [`pkg/security/agent`](../security/agent.md)). This is the
canonical pattern: the agent's `Stop()` cancels the gRPC stream context, waits for goroutines,
and closes client connections. The caller's `SerialStopper` or `ParallelStopper` ensures this
runs as part of the broader shutdown sequence.

### Logs pipeline ordering constraint

The logs pipeline in `comp/logs/agent` uses `NewStarter` to enforce a strict startup order
(destinations context → auditor → pipeline provider → message receiver → launchers) because each
stage must be ready before the next one feeds it data. On shutdown, `NewSerialStopper` tears down
the same components in reverse. See [`pkg/logs`](../logs/logs.md) for the full pipeline
architecture.

## Cross-references

| Topic | See also |
|-------|----------|
| Fx-integrated pub/sub for inter-component event notification | [`pkg/util/subscriptions`](subscriptions.md) |
| Logs pipeline component lifecycle that uses `NewStarter` / `NewSerialStopper` | [`pkg/logs`](../logs/logs.md) |
| Security agent `Stop()` registered via `stopper.Add` | [`pkg/security/agent`](../security/agent.md) |
| CWS module and security sub-system lifecycle | [`pkg/security`](../security/security.md) |
