# pkg/proto/utils

## Purpose

`pkg/proto/utils` provides a reflection-based helper for shallow-copying protobuf message
values. Its primary function, `ProtoCopier`, was the original mechanism for copying `Span`
and `TracerPayload` structs in the trace pipeline. It is now superseded in those hot paths by
hand-written `ShallowCopy()` methods, but remains available for other protobuf types where the
reflection overhead is acceptable.

## Key elements

**`ProtoCopier(v interface{}) func(v interface{}) interface{}`**

Accepts a pointer to a protobuf value and returns a reusable copy function for that type.
At construction time it uses `reflect` to discover all exported fields of the pointed-to
struct that have a corresponding `Get<FieldName>()` method (no-arg, single-return, matching
field type). These `(field index, method index)` pairs are captured in a closure.

When the returned function is called with a value of the same type, it:
1. Allocates a new zero-value struct of the same type.
2. Calls each captured getter on the source value and assigns the result to the corresponding
   field on the destination.
3. Returns a pointer to the new struct as `interface{}`.

This is a **shallow copy**: map and slice fields (e.g. `Meta`, `Metrics`) are copied by
reference, not deep-cloned.

**Panic behaviour** — The returned function panics if called with a value whose type does not
match the type passed to `ProtoCopier`. The error message is
`"ProtoCopier dst <type> != src <type>"`.

## Usage

```go
copier := utils.ProtoCopier((*MyProtoMessage)(nil))
// ...
cloned := copier(original).(*MyProtoMessage)
```

### Why it is no longer used for `Span`

`pkg/proto/pbgo/trace.Span.ShallowCopy()` replaced `ProtoCopier` for span copies because
reflection has measurable overhead on a code path that runs for every span processed. The
hand-written method is validated at startup by an `init` function that calls `ProtoCopier`
once and checks the field set matches, ensuring `ShallowCopy` stays in sync with the struct
without carrying per-copy reflection cost. The same pattern is used for `TracerPayload` in
`pkg/proto/pbgo/trace/tracer_payload_utils.go`.

`ProtoCopier` remains useful for protobuf types that are not on hot paths and where
maintaining a hand-written copy method would be more error-prone than the small reflection
cost.
