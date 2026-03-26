# pkg/util/json

## Purpose

`pkg/util/json` provides small, focused helpers for JSON encoding that are not covered by
the standard library or the `json-iterator` library used elsewhere in the agent. The package
has three distinct utilities: a recursive map traversal function, a CLI output formatter,
and a low-allocation streaming JSON builder.

## Key elements

### `GetNestedValue`

```go
func GetNestedValue(inputMap map[string]interface{}, keys ...string) interface{}
```

Drills into a nested `map[string]interface{}` tree using a variadic key path. Returns `nil`
if any key is absent or if an intermediate value is not a map. Useful when working with
decoded JSON blobs whose structure is only partially known.

### `PrintJSON`

```go
func PrintJSON(w io.Writer, rawJSON any, prettyPrintJSON bool, removeEmpty bool, searchTerm string) error
```

Marshals an arbitrary Go value (or a `json.RawMessage`) to an `io.Writer`. Options:

- `prettyPrintJSON` — uses `json.MarshalIndent` with 2-space indentation.
- `removeEmpty` — recursively strips empty-string fields from maps and slices before
  marshaling. Empty maps and arrays are preserved since they may be part of the API contract.
- `searchTerm` — if non-empty, checks that the top-level `"Entities"` map is non-empty and
  returns an error if not, producing a user-friendly "no entities found" message.

Used by CLI sub-commands that display structured agent data to the terminal.

### `RawObjectWriter`

```go
type RawObjectWriter struct { ... }
```

A thin wrapper around a `*jsoniter.Stream` that tracks nesting depth (up to 8 levels) and
automatically inserts commas between fields and array elements. It is designed for building
JSON objects incrementally without intermediate allocations.

**Construction**

```go
func NewRawObjectWriter(stream *jsoniter.Stream) *RawObjectWriter
```

**Methods**

| Method | Description |
|---|---|
| `StartObject() error` | Writes `{` and pushes a new scope |
| `FinishObject() error` | Writes `}` and pops the scope |
| `StartArrayField(fieldName string) error` | Writes `"fieldName":[` and pushes a scope |
| `FinishArrayField() error` | Writes `]` and pops the scope |
| `AddStringField(fieldName, value string, policy EmptyPolicy)` | Writes a string key/value pair; respects `EmptyPolicy` |
| `AddInt64Field(fieldName string, value int64)` | Writes an int64 key/value pair |
| `AddStringValue(value string)` | Writes a bare string (for use inside an array) |
| `Flush() error` | Flushes the underlying `jsoniter.Stream` |

**`EmptyPolicy`**

```go
const (
    OmitEmpty  EmptyPolicy = iota // skip field if value is ""
    AllowEmpty                    // always write the field
)
```

## Usage

### CLI sub-commands

`cmd/agent/subcommands/configcheck` and `pkg/cli/subcommands/workloadlist` use `PrintJSON`
to format and print check configuration and workload metadata to stdout, with optional
pretty-printing and empty-field removal.

`comp/core/tagger/api/getlist.go` and `comp/core/workloadmeta/collectors/internal/cloudfoundry/vm/cf_vm.go`
also use `PrintJSON` to render tagger entity lists and Cloud Foundry VM metadata.

### Serializer — service checks and metrics

`pkg/serializer/internal/metrics/service_checks.go` uses `RawObjectWriter` to serialize
service check payloads directly into a `jsoniter.Stream` without constructing intermediate
Go structs. This avoids reflection overhead on the hot metrics serialization path. The
serializer's `JSONPayloadBuilder` (in `internal/stream/`) drives the outer streaming loop
while individual marshalers such as `ServiceChecks` use `RawObjectWriter` for the inner
field-by-field encoding. See [`pkg/serializer`](../serializer.md) for how these two layers
fit together.

`GetNestedValue` appears in metadata and config-check code that parses JSON responses from
the agent's internal HTTP API.

`comp/host-profiler/collector/impl/converters` also calls `GetNestedValue` to navigate
nested JSON maps produced when deserializing host profiler payloads.

### When to prefer `GetNestedValue` vs `pkg/util/jsonquery`

| Scenario | Recommended tool |
|---|---|
| One or two known key segments, plain Go map | `GetNestedValue` — zero dependencies, no compile overhead |
| jq-style expressions, dynamic queries, YAML inputs | [`pkg/util/jsonquery.RunSingleOutput`](jsonquery.md) |

`GetNestedValue` is intentionally minimal: it only traverses `map[string]interface{}` trees
and returns `nil` on any type mismatch. For richer extraction (array index access, conditional
filters, type coercions) use `pkg/util/jsonquery`.

## Cross-references

| Document | Relationship |
|---|---|
| [`pkg/serializer`](../serializer.md) | Consumes `RawObjectWriter` in `service_checks.go` for low-allocation service-check serialization on the hot metrics path |
| [`pkg/util/jsonquery`](jsonquery.md) | Higher-level jq query engine built on `gojq`; use when `GetNestedValue`'s flat key traversal is insufficient |
