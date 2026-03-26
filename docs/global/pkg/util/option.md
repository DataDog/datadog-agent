# pkg/util/option

## Purpose

`pkg/util/option` provides a generic optional-value type for Go. It expresses "a value that may or may not be present" explicitly, avoiding the ambiguity that arises when using nil pointers or zero values for the same purpose. With 145 importers it is widely used across both the component layer (`comp/`) and pkg packages.

The package is especially important in the agent's dependency-injection graph: optional components are exposed as `option.Option[T]` values so that consumers can work gracefully when a component is absent, rather than depending on a type that is never provided.

**Related packages:**
- [`pkg/util/fxutil`](../util/fxutil.md) — provides `ProvideOptional[T]()` which wraps an already-provided `T` in an `option.Option[T]` for the fx graph
- [`comp/core/config`](../../comp/core/config.md) — some optional configuration providers are exposed as `option.Option` values

## Key elements

### `Option[T any]`

The core type. A struct with two fields — a value of type `T` and a boolean `set` flag. The zero value represents "no value" (i.e. `None`).

```go
type Option[T any] struct {
    value T
    set   bool
}
```

Because `Option[T]` is a value type (not a pointer), the zero value `Option[T]{}` is always safe to use as "absent" without allocation. The pointer-returning constructors (`NewPtr`, `NonePtr`) exist for the rare cases where an `*Option[T]` is needed.

### Constructors

| Function | Returns | Meaning |
|---|---|---|
| `New[T](value T) Option[T]` | set option | A present value |
| `NewPtr[T](value T) *Option[T]` | pointer to set option | Same, as a pointer |
| `None[T]() Option[T]` | unset option | No value |
| `NonePtr[T]() *Option[T]` | pointer to unset option | No value, as a pointer |

### Methods on `*Option[T]`

| Method | Description |
|---|---|
| `Get() (T, bool)` | Returns the value and `true` if set; `(zero, false)` if not |
| `Set(value T)` | Sets the value |
| `Reset()` | Clears the value (marks as unset) |
| `SetIfNone(value T)` | Sets the value only if not already set |
| `SetOptionIfNone(option Option[T])` | Copies another option in only if not already set |
| `UnmarshalYAML(func(interface{}) error) error` | YAML deserialization support |

Note that `Get`, `Set`, `Reset`, `SetIfNone`, and `SetOptionIfNone` are pointer-receiver methods. Calling them on a non-addressable `Option[T]` value (e.g. a function return value that has not been assigned to a variable) requires taking the address first.

`UnmarshalYAML` sets the option to `None` on unmarshal error, rather than leaving it in a partially initialised state.

### Free functions

**`MapOption[T1, T2 any](optional Option[T1], fct func(T1) T2) Option[T2]`**
Applies a transform function to a set option and returns the result wrapped in a new option. Returns `None` if the input is unset. Equivalent to `Option.map` in functional languages.

## Usage

### Optional component dependencies in fx

The most common use of `option.Option` in the codebase is to model components that are not always included in every binary. A module registers a component normally *and* provides an `option.Option` wrapper of the same type using `fxutil.ProvideOptional`:

```go
// comp/remote-config/rcclient/rcclientimpl/rcclient.go
func Module() fxutil.Module {
    return fxutil.Component(
        fx.Provide(newRemoteConfigClient),
        fxutil.ProvideOptional[rcclient.Component](),
    )
}
```

`fxutil.ProvideOptional[T]()` is a helper in `pkg/util/fxutil` that registers a provider equivalent to:
```go
fx.Provide(func(c T) option.Option[T] { return option.New(c) })
```

Consumers that depend on the component only when available declare:
```go
type Requires struct {
    compdef.In
    RCClient option.Option[rcclient.Component]
}
```
And check at runtime:
```go
if client, ok := reqs.RCClient.Get(); ok {
    // use client
}
```

When the module is not included in a binary (its `Module()` is not in the fx options), `option.Option[rcclient.Component]` is simply not provided. If no provider for the `option.Option` type is registered, fx will leave the field at its zero value — which for `Option[T]` is `None`. This works because fx treats `option.Option[T]` as a plain struct, not an interface.

### Optional fields in configuration structs

`Option[T]` is used where a config field that is absent should be distinguishable from a field set to a zero value. The `UnmarshalYAML` method means it works directly with the standard YAML unmarshaler.

### Conditional component construction

Components that may or may not be built (e.g. based on build tags or feature flags) return `option.None[T]()` when disabled and `option.New[T](c)` when enabled:

```go
// comp/collector/collector/collectorimpl/collector.go
if !enabled {
    return option.None[collector.Component]()
}
return option.New[collector.Component](c)
```

### `MapOption` for chained transforms

```go
name := option.MapOption(hostnameOpt, func(h hostname.Component) string {
    return h.Get()
})
```
Returns `option.None[string]()` if `hostnameOpt` is unset, otherwise returns `option.New(h.Get())`.

### Merging options with priority

`SetIfNone` and `SetOptionIfNone` implement a "first wins" merge: call them in order from highest to lowest priority, and only the first set value takes effect.

```go
var result option.Option[string]
result.SetOptionIfNone(fromFlag)    // CLI flag has highest priority
result.SetOptionIfNone(fromEnv)     // env var if flag was absent
result.SetIfNone("default")         // fallback default
```

## Pitfalls

- **`Get` is a pointer-receiver method.** You cannot call `myFunc().Get()` if `myFunc()` returns an `Option[T]` by value — assign to a local variable first: `opt := myFunc(); v, ok := opt.Get()`.
- **`None[T]()` and `Option[T]{}` are equivalent.** The zero value is already a valid `None`. Avoid using `*Option[T]` when `Option[T]` suffices, to prevent confusion between a nil pointer and an unset option.
- **YAML unmarshalling on error resets to `None`.** If the YAML value is present but cannot be decoded into `T`, the option becomes `None` and the error is returned. Callers should always check the error from their YAML decoder.
