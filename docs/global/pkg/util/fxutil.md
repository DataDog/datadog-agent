> **TL;DR:** Thin wrapper over Uber's fx dependency injection framework that standardises how the Datadog Agent starts, stops, and tests its fx applications, providing component registration conventions, lifecycle adapters, and a comprehensive set of test helpers.

# pkg/util/fxutil

## Purpose

`pkg/util/fxutil` provides a thin wrapper layer on top of [Uber's fx](https://github.com/uber-go/fx) dependency injection framework. It standardises how the Datadog Agent starts, stops, and tests its fx applications, and gives component authors a set of conventions for registering components into the fx graph without coupling implementation code directly to fx types.

Most of the agent's binaries and components use this package instead of calling fx directly. With 388 importers it is one of the most widely used packages in the repo.

**Related packages:**
- [`pkg/util/option`](../util/option.md) — provides `option.Option[T]`, used by `ProvideOptional` to model optional component dependencies
- [`comp/core/config`](../../comp/core/config.md) — wraps the global config and is typically the first module passed to `Run`/`OneShot`
- [`comp/core/log`](../../comp/core/log.md) — the standard logger component, wired via `FxAgentBase` and `LogParams`

## Key elements

### Key types

**`Module`** / **`BundleOptions`** — thin structs that embed `fx.Option` and also carry an `Options []fx.Option` field for introspection (used by `TestBundle`). Returned by `Component` and `Bundle` respectively.

**`NoDependencies`** — convenience struct (`fx.In` embedded, no fields) for use as the type parameter in `Test` when no resolved dependencies are needed.

### Key functions

#### Application entrypoints

**`Run(opts ...fx.Option) error`**
Builds and starts an fx application intended for long-running daemons (e.g. the trace-agent or system-probe). It blocks until the app signals shutdown (via `app.Done()`), then stops cleanly. Returns errors instead of calling `os.Exit`. Automatically prepends `FxAgentBase()` and `TemporaryAppTimeouts()` to every call. When the test override `fxAppTestOverride` is set (e.g. by `TestRun`), the real app is not constructed — this allows testing the dependency graph without launching the agent.

**`OneShot(oneShotFunc interface{}, opts ...fx.Option) error`**
Like `Run`, but for commands that should complete and exit (e.g. `agent status`). The app starts all components, then calls `oneShotFunc` with its arguments resolved by fx (via `delayedFxInvocation`), then shuts down immediately. `oneShotFunc` must return `error` or nothing. All lifecycle `OnStart` hooks run to completion before the one-shot function executes.

### Component and bundle registration

**`Component(opts ...fx.Option) Module`**
Wraps `fx.Module` and auto-derives the component name from the caller's file path (`comp/<bundle>/<component>/`). Every component's `fx.go` file returns a `Module` from this call. Panics if called from outside a `comp/` path.

**`Bundle(opts ...fx.Option) BundleOptions`**
Like `Component` but for bundle-level `bundle.go` files (`comp/<bundle>/`). Panics if called from outside a `comp/<bundle>` path.

### fx-agnostic component constructors

**`ProvideComponentConstructor(compCtorFunc interface{}) fx.Option`**
Bridges the gap between plain component constructors (which use `comp/def.In` / `comp/def.Out` marker structs) and fx's `fx.In` / `fx.Out` system. This lets implementation packages stay import-free from the `go.uber.org/fx` package. At runtime the function uses reflection to build fx-aware wrapper types and registers them with `fx.Provide`.

The constructor must have 0 or 1 arguments and 1 or 2 return values. The argument (if present) must be a struct embedding `compdef.In`; the first return value must be a struct embedding `compdef.Out`. An optional second return value of type `error` is supported. Non-exported fields cause a compile-time error.

Example pattern (from `comp/dogstatsd/statsd/fx/fx.go`):
```go
func Module() fxutil.Module {
    return fxutil.Component(
        fxutil.ProvideComponentConstructor(statsdimpl.NewComponent),
    )
}
```

The constructor lives in the `impl` package and its argument / return structs embed `compdef.In` / `compdef.Out` instead of `fx.In` / `fx.Out`.

### Optional components in the fx graph

**`ProvideOptional[T any]() fx.Option`**
Registers a provider equivalent to:
```go
fx.Provide(func(c T) option.Option[T] { return option.New(c) })
```
Used when a component is optional for some binaries — callers depend on `option.Option[MyComponent]` and check presence at runtime, rather than making the component a hard dependency. See [`pkg/util/option`](../util/option.md) for the full `Option[T]` API.

### fx group utilities

**`GetAndFilterGroup[S ~[]E, E any](group S) S`**
Removes nil / zero values from an fx value-group slice. Components that are conditionally disabled may return `nil` into a group; every consumer of that group should call this function before iterating, to avoid nil pointer dereferences.

### Base options

**`FxAgentBase() fx.Option`**
Returns the set of options that every Agent fx application must include:
- An adapter from `compdef.Lifecycle` to `fx.Lifecycle` (`newFxLifecycleAdapter`)
- An adapter from `compdef.Shutdowner` to `fx.Shutdowner` (`newFxShutdownerAdapter`)
- The default fx logging configuration (`logging.DefaultFxLoggingOption()`)

Called automatically by `Run` and `OneShot`. Both adapters exist so that implementation packages can declare lifecycle/shutdown dependencies without importing `go.uber.org/fx` directly.

**`FxLifecycleAdapter() fx.Option`**
Exposed for callers that construct a partial fx graph and only need the lifecycle adapter.

### Timeouts

**`TemporaryAppTimeouts() fx.Option`**
Sets start and stop timeouts to 5 minutes (overridable via `DD_FX_START_TIMEOUT_SECONDS` / `DD_FX_STOP_TIMEOUT_SECONDS`). Applied automatically by `Run` and `OneShot`. The long default is intentional — the agent has historically had no timeout, and reducing it too aggressively causes flaky test failures. Note that the OS service manager (upstart, Windows SCM) may impose a shorter external timeout.

### Error handling

**`UnwrapIfErrArgumentsFailed(err error) error`**
Strips fx's `errArgumentsFailed` wrapper to surface the real underlying error message. Used internally by `Run` and `OneShot`.

### Testing helpers (build tag: `test | functionaltests | stresstests`)

| Function | Purpose |
|---|---|
| `Test[T any](t, opts...) T` | Starts a test app, resolves dependencies into `T` (must embed `fx.In`), stops on `t.Cleanup`. The most common test helper. Supplies `testing.TB` into the graph so mocks can register cleanup hooks. |
| `TestApp[T any](opts...) (*fx.App, T, error)` | Like `Test` but returns the raw app for manual lifecycle control. Does not register automatic cleanup. |
| `TestStart(t, opts, assertFn, fn)` | Starts an app and calls `assertFn` — allows asserting on expected startup failures. `fn` is never called; it is only used to set up the dependency type via reflection. |
| `TestRun(t, f)` | Validates that a function which calls `fxutil.Run` provides a valid, dependency-satisfying app graph (does not actually start the app). |
| `TestOneShotSubcommand(t, cmds, args, expectedFn, verifyFn)` | Validates a cobra subcommand that calls `fxutil.OneShot` — checks the correct function is invoked and all types are provided, then runs `verifyFn` for assertions. |
| `TestOneShot(t, fct)` | Validates that a function calling `fxutil.OneShot` has all dependencies satisfied, without executing the one-shot action. |
| `TestBundle(t, bundle, extraOpts...)` | Validates that every component registered inside a `BundleOptions` can be instantiated with `fx.ValidateApp`. Introspects `module.Options` to discover all provided types. |
| `TestRunWithApp(opts...)` | Starts the real app (same logic as `Run`) and returns it — useful for testing graceful shutdown. |

### Configuration and build flags

| Environment variable | Effect |
|---|---|
| `DD_FX_START_TIMEOUT_SECONDS` | Overrides the default 5-minute fx app start timeout |
| `DD_FX_STOP_TIMEOUT_SECONDS` | Overrides the default 5-minute fx app stop timeout |

Testing helpers are activated by the build tags: `test`, `functionaltests`, or `stresstests`.

### Internal: `delayedFxInvocation`

`OneShot` and the test helpers need to call a user-provided function *after* all lifecycle hooks have completed, but fx's `fx.Invoke` fires during app construction. `delayedFxInvocation` solves this by registering a capturing `fx.Invoke` that records the arguments at construction time, then calling the real function after `app.Start` returns.

## Usage

**Long-running daemons** call `fxutil.Run` from their cobra `RunE`, assembling components via their module functions:

```go
// cmd/trace-agent/subcommands/run/command.go
return fxutil.Run(
    fx.Provide(func() context.Context { return ctx }),
    coreconfig.Module(),
    logtracefx.Module(),
    statsdFx.Module(),
    trace.Bundle(),
    // ...
)
```

**Short-lived commands** use `fxutil.OneShot`, passing a function that receives whatever components it needs:

```go
fxutil.OneShot(func(cfg config.Component, log log.Component) error {
    // print status and return
}, coreconfig.Module(), logtracefx.Module())
```

**Component `fx` packages** use `fxutil.Component` + `fxutil.ProvideComponentConstructor`:

```go
// comp/dogstatsd/statsd/fx/fx.go
func Module() fxutil.Module {
    return fxutil.Component(
        fxutil.ProvideComponentConstructor(statsdimpl.NewComponent),
    )
}
```

**Optional components** are exposed with `ProvideOptional`:

```go
// comp/remote-config/rcclient/rcclientimpl/rcclient.go
func Module() fxutil.Module {
    return fxutil.Component(
        fx.Provide(newRemoteConfigClient),
        fxutil.ProvideOptional[rcclient.Component](),
    )
}
```
Consumers then declare a dependency on `option.Option[rcclient.Component]` and check `Get()` at runtime.

**Unit tests** for components call `fxutil.Test`:

```go
func TestMyComponent(t *testing.T) {
    c := fxutil.Test[mycomp.Component](t, fx.Options(
        mycomp.Module(),
        core.MockBundle(),
    ))
    // use c
}
```

**Tests for cobra commands** built on `fxutil.OneShot` use `fxutil.TestOneShotSubcommand` to assert that the correct function and correct option values are wired up, without actually running the command.

## Pitfalls

- **`Component` / `Bundle` must be called from the right path.** Both functions derive names from `runtime.Caller(2)` and panic if the call site is not inside `comp/<bundle>/<comp>/` or `comp/<bundle>/`.
- **`ProvideComponentConstructor` validates at registration time.** If the constructor has non-exported fields, wrong embed types, or wrong arity, it returns an `fx.Error` that surfaces as an app construction failure — not a compile error.
- **`fxAppTestOverride` is a package-level variable.** Test helpers set and clear it around each test. Do not call `fxutil.Run` / `fxutil.OneShot` concurrently in tests that use these helpers.
- **`TestBundle` skips fx value-group members.** Fields tagged with `group:"..."` in an `fx.Out` struct are not validated by `TestBundle`, because they cannot be satisfied by a bare `fx.Invoke`.
