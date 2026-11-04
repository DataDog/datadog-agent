# Use optional component dependencies

Use this guide when an existing component must work in binaries that omit one of its dependencies. It assumes the consumer already has a plain constructor and an Fx wrapper; follow the [creation tutorial](../../tutorials/components/creating-components.md) if you need to create them first.

The fragments below use the tutorial's compression interface. Adapt them to your existing consumer and register its constructor through its usual wrapper.

## Choose between a no-op component and absence

Choose a no-op implementation when consumers should keep calling the interface while its operations do nothing. Choose an absent option when consumers should detect that the component is unavailable and take their own fallback path.

Check what an existing wrapper provides before selecting it. The <<<repo("comp/core/ipc/fx-none/fx.go", "IPC `fx-none` wrapper")>>> supplies a no-op component and a present option. The <<<repo("comp/anomalydetection/recorder/fx-noop/fx_noop.go", "recorder `fx-noop` wrapper")>>> supplies an absent option. See the [Fx explanation](../../architecture/components/fx.md#optional-values) for the difference.

## Declare the optional dependency

Add these imports to the consumer's implementation:

```go
import (
    compression "github.com/DataDog/datadog-agent/comp/compression/def"
    "github.com/DataDog/datadog-agent/pkg/util/option"
)
```

Add an exported field to its existing `Requires` struct:

```go
Compression option.Option[compression.Component]
```

Store that value on the consumer if it will be used after construction. Keep the consumer's existing constructor adapter; it handles this field just like other dependency types.

## Handle absence in the consumer

Call `Get()` before using the dependency and implement the absent case. This helper prepares bytes using compression when available. Add it to the consumer's implementation, add `"bytes"` to its imports, and call it from the consumer's existing method with the optional value and payload.

```go
func preparePayload(compressionOption option.Option[compression.Component], data []byte) ([]byte, error) {
    if compressor, found := compressionOption.Get(); found {
        return compressor.Compress(data)
    }
    return bytes.Clone(data), nil
}
```

This fallback returns a copy of the original data. A sender must also communicate the selected encoding to its receiver.

## Provide a present implementation

In the dependency's Fx wrapper, register its implementation constructor and an optional conversion. The following expression belongs inside `Module`, with `compression`, `compressionimpl`, and `fxutil` imported as in the [tutorial's wrapper](../../tutorials/components/creating-components.md#register-the-component-with-fx):

```go
return fxutil.Component(
    fxutil.ProvideComponentConstructor(compressionimpl.NewComponent),
    fxutil.ProvideOptional[compression.Component](),
)
```

The component scaffold includes `ProvideOptional` by default; check for it before adding the registration to an existing wrapper. The application must also register the configuration and logging dependencies of the compression implementation.

## Provide an absent implementation

For binaries that omit the dependency, register a provider returning `option.None[T]()` in the selected Fx wrapper, following the <<<repo("comp/anomalydetection/recorder/fx-noop/fx_noop.go", "recorder example")>>>.

For the compression example, add `"go.uber.org/fx"`, `"github.com/DataDog/datadog-agent/pkg/util/fxutil"`, and the compression and option imports shown above. Use this body for the wrapper's `func Module() fxutil.Module`:

```go
return fxutil.Component(
    fx.Provide(func() option.Option[compression.Component] {
        return option.None[compression.Component]()
    }),
)
```

This provider needs no compression implementation or configuration and logging dependencies. It cannot satisfy a consumer that requires `compression.Component` directly.

## Register and verify the selected provider

Register the consumer's wrapper and exactly one provider of its optional dependency in the [application](using-components.md). Select the present wrapper when compression is available or the absent wrapper when the consumer should use its fallback. Request the consumer through an entry point such as `fx.Invoke` so Fx constructs it.

Check both configurations:

- With the present wrapper, confirm that the consumer calls compression and handles any returned error.
- With the absent wrapper, confirm that the consumer takes the fallback path and returns a copy of the input.

For direct constructor tests, import the [tutorial's mock](../../tutorials/components/creating-components.md#add-a-mock-for-consumers) from `github.com/DataDog/datadog-agent/comp/compression/mock` as `compressionmock`. Pass `option.New[compression.Component](compressionmock.New(t).Comp)` or `option.None[compression.Component]()` in the consumer's `Requires` value. With this mock, expect the prepared payload to equal `[]byte("compressed")`; with absence, expect a copy of the input. Also exercise application assembly with each wrapper to check provider registration.

If assembly fails, check these cases:

| Failure | Correction |
| --- | --- |
| Missing `option.Option[compression.Component]` provider (`missing type:` or `missing types:`). | Register either the optional conversion or an absent-option provider. |
| Missing `compression.Component` provider (`missing type:` or `missing types:`). | Register its implementation alongside the optional conversion. `ProvideOptional` requires an existing component. |
| Duplicate optional provider (`already provided by`). | Register only the selected wrapper and remove any repeated conversion. |
