# Create a component

This tutorial builds a compression component with two implementations. Read the [component framework overview](../../architecture/components/index.md) and follow the [component guidelines](../../guidelines/components.md) as you work through it.

The component compresses a payload before sending it to the Datadog backend.

Since there are multiple ways to compress data, this component provides two implementations of the same interface:

* The [ZSTD](https://en.wikipedia.org/wiki/Zstd) data compression algorithm
* The [ZIP](https://en.wikipedia.org/wiki/ZIP_(file_format)) data compression algorithm

The component separates its interface, implementations, Fx wrappers, and mock into packages. See the [file hierarchy](../../guidelines/components.md#file-hierarchy) for the conventions and [package separation](../../architecture/components/index.md#package-separation) for the rationale.

## Bootstrapping components

You can use the [command](../../setup/required.md#tooling) `dda inv components.new-component comp/<COMPONENT_NAME>` to generate a scaffold for your new component.

Document the public API according to the [documentation guidelines](../../guidelines/components.md#documentation).

### The def folder

The `def` folder contains your interface and ALL public types needed by the users of your component.

In the example of a compression component, the def folder looks like this:

/// tab | :octicons-file-code-16: comp/compression/def/component.go
```go
// Package compression contains all public type and interfaces for the compression component
package compression

// team: <your team>

// Component describes the interface implemented by all compression implementations.
type Component interface {
    // Compress compresses the input data.
    Compress([]byte) ([]byte, error)

    // Decompress decompresses the input data.
    Decompress([]byte) ([]byte, error)
}
```
///

Keep the interface small and follow the [interface guidelines](../../guidelines/components.md#ownership-and-interfaces) when choosing public types.

### The impl folders

The `impl` folder is where the component implementation is written. The details of component implementation are up to the developer. The only requirements are that the package name follows the pattern `<COMPONENT_NAME>impl` for the regular implementation or `<IMPL_NAME>impl` for the alternative implementation, and that there is a public instantiation function called `NewComponent`.

/// tab | :octicons-file-code-16: comp/compression/impl-zstd/compressor.go
```go
package zstdimpl

// NewComponent returns a new ZSTD implementation for the compression component
func NewComponent(reqs Requires) Provides {
    ....
}
```
///

To require input arguments to the `NewComponent` instantiation function, use a special struct named `Requires`. The instantiation function returns a special stuct named `Provides`. This internal nomenclature is used to handle the different component dependencies using Fx groups.

In this example, the compression component must access the configuration component and the log component. To express this, define a `Requires` struct with two fields. The name of the fields is irrelevant, but the type must be the concrete type of interface that you require.

/// tab | :octicons-file-code-16: comp/compression/impl-zstd/compressor.go
```go
package zstdimpl

import (
    "fmt"

    config "github.com/DataDog/datadog-agent/comp/core/config/def"
    log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// Here, list all components and other types known by Fx that you need.
// To be used in `fx` folders, type and field need to be public.
//
// In this example, you need config and log components.
type Requires struct {
    Conf config.Component
    Log  log.Component
}
```
///

/// info | Using other components
If you want to use another component within your own, add it to the `Requires` struct, and `Fx` will give it to you at initialization. Be careful of circular dependencies.
///

For the output of the component, populate the `Provides` struct with the return values.

/// tab | :octicons-file-code-16: comp/compression/impl-zstd/compressor.go
```go
package zstdimpl

import (
    // Always import the component def folder, so that you can return a 'compression.Component' type.
    compression "github.com/DataDog/datadog-agent/comp/compression/def"
)

// Here, list all the types your component is going to return. You can return as many types as you want; all of them are available through Fx in other components.
// To be used in `fx` folders, type and field need to be public.
//
// In this example, only the compression component is returned.
type Provides struct {
    Comp compression.Component
}
```
///

All together, the component code looks like the following:

/// tab | :octicons-file-code-16: comp/compression/impl-zstd/compressor.go
```go
package zstdimpl

import (
    "fmt"

    compression "github.com/DataDog/datadog-agent/comp/compression/def"
    config "github.com/DataDog/datadog-agent/comp/core/config/def"
    log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

type Requires struct {
    Conf config.Component
    Log  log.Component
}

type Provides struct {
    Comp compression.Component
}

// The actual type implementing the 'Component' interface. This type MUST be private, you need the guarantee that
// components can only be used through their respective interfaces.
type compressor struct {
    // Keep a ref on the config and log components, so that you can use them in the 'compressor' methods
    conf config.Component
    log  log.Component

    // any other field you might need
}

// NewComponent returns a new ZSTD implementation for the compression component
func NewComponent(reqs Requires) Provides {
    // Here, do whatever is needed to build a ZSTD compression comp.

    // And create your component
    comp := &compressor{
        conf: reqs.Conf,
        log:  reqs.Log,
    }

    return Provides{
        comp: comp,
    }
}

//
// You then need to implement all methods from your 'compression.Component' interface
//

// Compress compresses the input data using ZSTD
func (c *compressor) Compress(data []byte) ([]byte, error) {
    c.log.Debug("compressing a buffer with ZSTD")

    // [...]
    return compressData, nil
}

// Decompress decompresses the input data using ZSTD.
func (c *compressor) Decompress(data []byte) ([]byte, error) {
    c.log.Debug("decompressing a buffer with ZSTD")

    // [...]
    return compressData, nil
}
```
///

The constructor can return either a `Provides`, if it is infallible, or `(Provides, error)`, if it could fail. In the latter case, a non-nil error results in the Agent crashing at startup with a message containing the error.

Each implementation follows the same pattern.

### The fx folders

The `fx` folder must be the only folder importing and referencing Fx. It's meant to be a simple wrapper. Its only goal is to allow dependency injection with Fx for your component.

All `fx.go` files must define a `func Module() fxutil.Module` function. The helpers contained in `fxutil` handle all the logic. Most `fx/fx.go` file should look the same as this:

/// tab | :octicons-file-code-16: comp/compression/fx-zstd/fx.go
```go
package fx

import (
    "github.com/DataDog/datadog-agent/pkg/util/fxutil"

    // You must import the implementation you are exposing through FX
    compressionimpl "github.com/DataDog/datadog-agent/comp/compression/impl-zstd"
)

// Module specifies the compression module.
func Module() fxutil.Module {
    return fxutil.Component(
        // ProvideComponentConstructor will automatically detect the 'Requires' and 'Provides' structs
        // of your constructor function and map them to FX.
        fxutil.ProvideComponentConstructor(
            compressionimpl.NewComponent,
        )
    )
}
```
///

/// info | Optional dependencies
To create an optional wrapper type for your component, you can use the helper function `fxutil.ProvideOptional`. This generic function requires the type of the component interface, and will automatically make a conversion function `optional.Option` for that component.

See [optional dependencies](../../how-to/components/optional-dependencies.md#optional-component) for how to consume the wrapper.
///

For the ZIP implementation, create the same file in `fx-zip` folder. In most cases, your component has a single implementation. If so, you have only one `impl` and `fx` folder.

For components that can be absent from a binary, see [optional dependencies](../../how-to/components/optional-dependencies.md).

### The mock folder

Add a mock that implements the compression interface and follows the [mock conventions](../../guidelines/components.md#mocks).

In the following example, your mock has no dependencies and returns the same string every time.

/// tab | :octicons-file-code-16: comp/compression/mock/mock.go
```go
//go:build test

package mock

import (
    "testing"

    compression "github.com/DataDog/datadog-agent/comp/compression/def"
)

type Provides struct {
    Comp compression.Component
}

type mock struct {}

// New returns a mock compressor
func New(*testing.T) Provides {
    return Provides{
        comp: &mock{},
    }
}

// Compress compresses the input data using ZSTD
func (c *mock) Compress(data []byte) ([]byte, error) {
    return []byte("compressed"), nil
}

// Decompress decompresses the input data using ZSTD.
func (c *compressor) Decompress(data []byte) ([]byte, error) {
    return []byte("decompressed"), nil
}
```
///

### Go modules

If this component will be used outside the Agent repository, follow the [module boundaries](../../guidelines/components.md#go-modules) and the [nested module how-to](../../how-to/go/modules.md) to export the required packages.

## Final state

In the end, a classic component folder should look like:

```
comp/<COMPONENT_NAME>/
├── def
│   └── component.go
├── fx
│   └── fx.go
├── impl
│   └── component.go
└── mock
    └── mock.go

4 directories, 4 files
```

The example compression component, which has two implementations, looks like:

```
comp/core/compression/
├── def
│   └── component.go
├── fx-zip
│   └── fx.go
├── fx-zstd
│   └── fx.go
├── impl-zip
│   └── component.go
├── impl-zstd
│   └── component.go
└── mock
    └── mock.go

6 directories, 6 files
```

## Next steps

Continue with [testing this component](testing.md), then see how to [use components in a binary](../../how-to/components/using-components.md). Check the [component guidelines](../../guidelines/components.md) when reviewing the completed implementation.
