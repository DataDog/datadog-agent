> **TL;DR:** Provides a conditional `DefaultForwarder` dedicated to Kubernetes/ECS orchestrator metadata, absent by default so that non-container builds never link the Kubernetes client-go dependency.

# comp/forwarder/orchestrator — Orchestrator Forwarder Component

**Import path:** `github.com/DataDog/datadog-agent/comp/forwarder/orchestrator`
**Team:** kubernetes-experiences
**Importers:** ~17 packages

## Purpose

`comp/forwarder/orchestrator` provides a conditional `defaultforwarder.Forwarder` dedicated to sending Kubernetes/ECS orchestrator metadata to the Datadog Orchestrator Explorer intake. Because orchestrator collection is only meaningful when the agent runs in a Kubernetes or ECS environment with `orchestrator_explorer.enabled: true`, the component is designed to be absent (rather than a no-op) when those conditions are not met.

Consumers call `Get()` and check the boolean return before forwarding. This approach ensures that binaries built without the `orchestrator` build tag (e.g. the standard Agent on non-container hosts) never link in the multi-megabyte Kubernetes client-go dependency.

## Package layout

| Package | Role |
|---|---|
| `comp/forwarder/orchestrator` (root) | `Component` type alias for `orchestratorinterface.Component` |
| `orchestratorinterface/` | `Component` interface definition (`Get()` method) |
| `orchestratorimpl/` | `Module()`, `Params`, and two build-tag-gated constructors |

## Key Elements

### Key interfaces

## Component interface

```go
// Component is the component type. Get() returns the forwarder only if it is enabled.
type Component interface {
    Get() (defaultforwarder.Forwarder, bool)
}
```

`Component` is implemented as an `option.Option[defaultforwarder.Forwarder]`. When the forwarder is not needed, the option is empty and `Get()` returns `(nil, false)`.

### Key types

## Build-tag gating

Two implementations of `newOrchestratorForwarder` exist, selected at compile time:

| Build tag | File | Behavior |
|---|---|---|
| `orchestrator` | `forwarder_orchestrator.go` | Full implementation; reads orchestrator config, resolves endpoints, creates and starts a `DefaultForwarder` |
| `!orchestrator` (default) | `forwarder_no_orchestrator.go` | Always returns an empty or noop option; avoids pulling in Kubernetes dependencies |

The `orchestrator` tag is set when building the cluster-agent or cluster-checks-runner.

### Key functions

## Params

```go
// NewDefaultParams — real forwarder enabled when orchestrator environment is detected
orchestratorimpl.NewDefaultParams()

// NewDisabledParams — forwarder never instantiated; Get() always returns (nil, false)
orchestratorimpl.NewDisabledParams()

// NewNoopParams — NoopForwarder wired in (useful for testing or non-forwarding processes)
orchestratorimpl.NewNoopParams()
```

`NewDefaultParams` still gates on `orchestrator_explorer.enabled` and the current environment (`IsKubernetes()`, `IsECS()`, `IsECSFargate()`, `IsECSManagedInstances()`). If either check fails, the component returns empty.

### Configuration and build flags

## fx wiring

```go
// Cluster-agent / cluster-checks-runner startup:
orchestratorimpl.Module(orchestratorimpl.NewDefaultParams())

// Non-orchestrator processes (e.g. standard Agent):
orchestratorimpl.Module(orchestratorimpl.NewDisabledParams())

// Tests:
orchestratorimpl.MockModule() // provides NoopForwarder wrapped in option.New
```

### Dependencies injected by fx (orchestrator build)

`newOrchestratorForwarder` requires:
- `log.Component` — logs config load errors
- `config.Component` — reads `orchestrator_explorer.enabled`
- `secrets.Component` — passed to forwarder options for secret resolution
- `tagger.Component` — fetches global tags for orchestrator config construction
- `fx.Lifecycle` — registers `Start`/`Stop` hooks on the inner `DefaultForwarder`

## Usage patterns

**Checking availability before forwarding:**

```go
type deps struct {
    fx.In
    OrchestratorFwd orchestrator.Component
}

func (s *mySerializer) sendMetadata(payloads []types.ProcessMessageBody) error {
    fwd, ok := s.OrchestratorFwd.Get()
    if !ok {
        return errors.New("orchestrator forwarder is not setup")
    }
    return fwd.SubmitOrchestratorChecks(payloads, extraHeaders, payloadType)
}
```

Always guard with `Get()` before calling forwarder methods. The pattern is identical to `comp/forwarder/eventplatform`.

## Key consumers

- `pkg/serializer.Serializer` — `SendOrchestratorMetadata` and `SendOrchestratorManifests` call `Get()` and forward serialized protobuf payloads via `SubmitOrchestratorChecks` / `SubmitOrchestratorManifests`
- `pkg/aggregator.AgentDemultiplexer` — receives the component at construction time and passes it to the serializer
- `comp/aggregator/demultiplexer/demultiplexerimpl` — wires the component into the demultiplexer fx graph for the main agent and cluster-agent
- `comp/otelcol/otlp/components/exporter/serializerexporter` — passes the component to `NewSerializer` for OTel-based metric export
