# pkg/trace/payload

## Purpose

Defines common interfaces and types shared across tracer payload processing. It acts as a thin abstraction layer that other trace packages import to avoid circular dependencies — it is only permitted to import `pkg/proto/pbgo/trace` and nothing else.

## Key elements

### `TracerPayloadModifier` (interface, `modifier.go`)

```go
type TracerPayloadModifier interface {
    Modify(*pb.TracerPayload)
}
```

Allows tracer implementations to mutate a `TracerPayload` early in the agent's processing pipeline (before filtering, sampling, and stats computation). Implementations plug into `agent.Agent` via the `TracerPayloadModifier` field, which is type-aliased to `payload.TracerPayloadModifier`.

## Usage

`pkg/trace/agent` re-exports this interface as a type alias so callers never need to import `pkg/trace/payload` directly:

```go
// in pkg/trace/agent/agent.go
type TracerPayloadModifier = payload.TracerPayloadModifier
```

The agent calls `Modify` once per incoming `TracerPayload`, before any per-trace processing:

```go
if a.TracerPayloadModifier != nil {
    a.TracerPayloadModifier.Modify(p.TracerPayload)
}
```

Typical use cases include injecting container tags, enriching spans with infrastructure metadata, or applying environment-specific overrides at the payload level rather than span-by-span.
