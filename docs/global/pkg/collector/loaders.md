# pkg/collector/loaders

## Purpose

The `loaders` package is a registry that collects all available check loaders at startup and returns them in priority order. A *loader* knows how to turn an integration configuration (a YAML `instance` block) into a runnable `check.Check`. Different loader implementations handle different check runtimes (Go, Python, shared libraries, etc.).

The package separates *registration* (which happens at `init` time, in each loader's own package) from *instantiation* (which is deferred until the catalog is first requested, after all components — including the Python interpreter — are initialized).

## Key elements

### Interfaces

```go
// check.Loader — defined in pkg/collector/check/loader.go
type Loader interface {
    Name() string
    Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data, instanceIndex int) (Check, error)
}
```

Every loader must implement `Name()` (a string identifier used when a config specifies `loader:` explicitly) and `Load()` (produces a configured `Check` or returns an error).

`check.ErrSkipCheckInstance` may be returned by `Load()` to signal a deliberate, non-error refusal (e.g. a Go check rejecting a config intended for its Python counterpart). The scheduler handles this sentinel differently from an actual error — it does not log it unless all loaders return it.

### Types in this package

| Type | Description |
|------|-------------|
| `LoaderFactory` | `func(sender.SenderManager, option.Option[integrations.Component], tagger.Component, workloadfilter.Component) (check.Loader, int, error)` — a factory that defers loader construction and also returns a priority integer. |

### Functions

| Function | Description |
|----------|-------------|
| `RegisterLoader(factory LoaderFactory)` | Appends a factory to the global `factoryCatalog`. Called from `init()` functions in loader packages. |
| `LoaderCatalog(senderManager, logReceiver, tagger, filter) []check.Loader` | Builds and returns the ordered loader slice. Each factory is called once; any that fail are skipped with an `Info` log. Loaders with a lower priority integer run first. The result is memoized via `sync.Once`. |

### Priority values used by known loaders

| Loader | Priority | Build tag |
|--------|----------|-----------|
| Python (`pkg/collector/python`) | 20 | `python` |
| Go / core checks (`pkg/collector/corechecks`) | 30 (10 if `prioritize_go_check_loader` is set) | none |
| Shared library (`pkg/collector/sharedlibrary`) | 40 | `sharedlibrarycheck` |

Lower numbers run earlier. When a config is loaded, the `CheckScheduler` tries each loader in order and stops at the first successful `Load()`.

## Known loader implementations

| Package | Loader name | Build tag | Description |
|---------|-------------|-----------|-------------|
| `pkg/collector/python` | `python` | `python` | Loads Python-based integration checks via the `rtloader` CGo bridge. Registers itself in its `init()`. |
| `pkg/collector/corechecks` | `core` | none | Loads Go checks registered in the `corechecks` catalog via `corechecks.RegisterCheck(name, factory)`. |
| `pkg/collector/sharedlibrary/sharedlibraryimpl` | — | `sharedlibrarycheck` | Loads checks from shared `.so` / `.dll` libraries. Registered by an explicit `InitSharedLibraryChecksLoader()` call (not from `init()`). Experimental. |

JMX checks are a special case: they are filtered out before any loader is invoked (see `check.IsJMXInstance`), because they are managed by an external JMX fetch process, not by a loader.

## Usage

### Registering a new loader

Add an `init()` function (or an explicit registration call) in your loader package:

```go
func init() {
    factory := func(
        sm sender.SenderManager,
        lr option.Option[integrations.Component],
        t tagger.Component,
        f workloadfilter.Component,
    ) (check.Loader, int, error) {
        l, err := NewMyLoader(sm)
        return l, 25, err // priority 25 = runs before Go checks, after Python
    }
    loaders.RegisterLoader(factory)
}
```

Ensure the package is imported (directly or via a blank import) in your binary's build graph so the `init()` runs.

### Consuming the catalog

The catalog is consumed by `pkg/collector.InitCheckScheduler`:

```go
for _, loader := range loaders.LoaderCatalog(senderManager, logReceiver, tagger, filterStore) {
    checkScheduler.addLoader(loader)
}
```

`LoaderCatalog` is safe to call multiple times; construction happens exactly once. Pass the same arguments on every call or the `sync.Once` will use whichever arguments it was first called with.

### Overriding the loader for a specific check

A config file can force a specific loader by name:

```yaml
init_config:
  loader: core   # or "python"

instances:
  - loader: python   # per-instance override
    # ...
```

The `CheckScheduler` skips loaders whose `Name()` does not match the specified name. If no loader with that name succeeds the check is not loaded.
