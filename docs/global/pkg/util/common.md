# pkg/util/common

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/common`

## Purpose

`pkg/util/common` is a small grab-bag of shared utilities that do not belong to a more specific package. It provides:

- A **singleton main context** for the agent process lifecycle.
- A **string set** type backed by a `map[string]struct{}`.
- **Reflection-based struct serialisation** to `map[string]interface{}`.
- A **slice transform** helper.
- A **math rounding** helper.

The package is intentionally kept small. If a helper grows or gains dependencies it should be split into its own package.

## Key Elements

### `GetMainCtxCancel() (context.Context, context.CancelFunc)` (`context.go`)

Returns the process-wide root `context.Context` and its `CancelFunc`. The context is created exactly once (via `sync.Once`) on the first call; all subsequent calls return the same pair. Cancelling this context is the canonical way for a component to initiate a graceful shutdown of the entire agent.

Components that need to react to agent shutdown should derive their own sub-contexts from this root rather than storing it directly.

Callers include `cmd/agent/subcommands/run/command.go`, `comp/core/tagger/impl/`, `comp/core/workloadmeta/impl/`, and several other top-level components.

### `StringSet` (`common.go`)

```go
type StringSet map[string]struct{}
```

A simple, unordered, deduplicated set of strings.

- `NewStringSet(initItems ...string) StringSet` — creates and populates a set.
- `(StringSet).Add(item string)` — adds an element (idempotent).
- `(StringSet).GetAll() []string` — returns all elements in an unspecified order.

### `StructToMap(obj interface{}) map[string]interface{}` (`common.go`)

Converts a struct to a `map[string]interface{}` using reflection. Key names come from the `json` struct tag if present, or the field name otherwise. Fields tagged with `json:"-"` and unexported fields are skipped. Nested structs are converted recursively. Slices, arrays, and maps are also handled. Returns an empty map for non-struct inputs.

### `GetSliceOfStringMap(slice []interface{}) ([]map[string]string, error)` (`common.go`)

Converts a `[]interface{}` (as returned by YAML/config parsers) where each element is `map[interface{}]interface{}` into `[]map[string]string`. Returns an error if any element has an unexpected type.

### `StringSliceTransform(values []string, fct func(string) []byte) [][]byte` (`slice.go`)

Maps a function over a `[]string`, returning a `[][]byte`. Useful when converting string configuration values to byte slices for lower-level processing.

### `ToPowerOf2(x int) int` (`math.go`)

Rounds `x` to the nearest power of two using `log2` and `round`. Used when sizing buffers or ring-queues that must have power-of-two capacity.

## Usage

### Agent shutdown via main context

```go
import "github.com/DataDog/datadog-agent/pkg/util/common"

func runAgent() {
    ctx, cancel := common.GetMainCtxCancel()
    defer cancel()

    // Pass ctx to all components; cancelling it triggers graceful shutdown.
    startComponents(ctx)
    <-ctx.Done()
}
```

### Using `StringSet` for deduplication

```go
seen := common.NewStringSet()
for _, tag := range incomingTags {
    seen.Add(tag)
}
uniqueTags := seen.GetAll()
```

### Serialising a struct for a generic endpoint

```go
payload := common.StructToMap(myStruct)
// payload is map[string]interface{} keyed by json tag names
jsonBytes, _ := json.Marshal(payload)
```

---

## Related packages

| Package / component | Relationship |
|---------------------|--------------|
| [`pkg/util/fxutil`](../util/fxutil.md) | `GetMainCtxCancel` and `fxutil.Run`/`fxutil.OneShot` are complementary lifecycle primitives. `fxutil.Run` builds the fx application graph and blocks until `app.Done()` signals shutdown; the main context from `GetMainCtxCancel` provides an independent, process-wide cancellation signal that components can react to directly without going through the fx shutdowner. Components wired via fx should prefer deriving a sub-context from the injected `context.Context` rather than calling `GetMainCtxCancel` directly. |
| [`pkg/config/setup`](../../pkg/config/setup.md) | `GetSliceOfStringMap` is used to decode `[]map[string]string` config values from the YAML-unmarshalled `[]interface{}` that the config layer returns for list-of-map keys (e.g. `config_providers`). `StructToMap` is used in the config settings API endpoint (`comp/api/api/apiimpl/internal/config/endpoint.go`) to serialise runtime config values. |
| [`comp/core/workloadmeta`](../../comp/core/workloadmeta.md) | `comp/core/workloadmeta/impl` calls `common.GetMainCtxCancel()` to obtain the root context for its internal collector goroutines. This ensures workloadmeta's collectors are cancelled when the agent initiates a graceful shutdown. |
