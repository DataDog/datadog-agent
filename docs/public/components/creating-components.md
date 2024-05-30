# Creating a Component

This page explains how to create components in detail.

This page uses the example of creating a compression component. This component compresses a payload before sending it to the Datadog backend.

Since there are multiple ways to compress data, this component provides two implementations of the same interface:

* The [ZSTD](https://en.wikipedia.org/wiki/Zstd) data compression algorithm
* The [ZIP](https://en.wikipedia.org/wiki/ZIP_(file_format)) data compression algorithm

A component contains multiple folders and Go packages. Developers split a component into packages to isolate the interface from the implementations and improve code sharing. Declaring the interface in a separate package from the implementation allows you to import the interface without importing all of the implementations.

## File hierarchy

All components are located in the `comp` folder at the top of the Agent repo.

The file hierarchy is as follows:

```
comp /
  <bundle name> /        <-- Optional
    <comp name> /
      def /              <-- The folder containing the component interface and ALL its public types.
      impl /             <-- The only or primary implementation of the component.
      impl-<alternate> / <-- An alternate implementation.
      impl-none /        <-- Optional. A noop implementation.
      fx /               <-- All fx related logic for the primary implementation, if any.
      fx-<alternate> /   <-- All fx related logic for a specific implementation.
      mock /             <-- The mock implementation of the component to ease testing.
```

<!-- TODO: where do we put the `optional.NoneOption` ? Into its own fx-none folder ? -->

To note:

* If your component has only one implementation, it should live in the `impl` folder.
* If your component has several implementations instead of a single implementation, you have multiple `impl-<version>` folders instead of an `impl` folder.
For example, your compression component has `impl-zstd` and `impl-zip` folders, but not an `impl` folder.
* If your component needs to offer a dummy/empty version, it should live in the `impl-none` folder.

### Why all those files ?

This file hierarchy aims to solve a few problems:

* Component users should only interact with the `def` folders and never care about which implementation was loaded in the
  main function.
* Go module support: when you create a Go module, all sub folders are pulled into the module. Thus, you need different folders for each implementation, the definition, and fx. This way, an external repository can pull a specific implementation and definition without having to import everything else.
* You have one `fx` folder per implementation, to allow binaries to import/link against a single folder.
* A main function that imports a component should be able to select a specific implementation without compiling with the others.
  For example: the ZSTD library should not be included at compile time when the ZIP version is used.


## Bootstrapping components

You can use the [invoke](../setup.md#invoke) task `inv components.new-component comp/<component>` to generate a scaffold for your new component.

Every public variable, function, struct, and interface of your component **must** be documented. Refer to the [Documentation](#documentation) section below for details.

### The def folder

The `def` folder contains your interface and ALL public types needed by the users of your component.

In the example of a compression component, the def folder looks like this:

=== ":octicons-file-code-16: comp/compression/def/component.go"
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

All component interfaces must be called `Component`, so all imports have the form `def.Component`.

You can see that the interface only exposes the bare minimum. You should aim at having the smallest possible interface
for your component.

When defining a component interface, avoid using structs or interfaces from third-party dependencies.

!!! warning "Interface using a third-party dependency"
    ```
    package def

    import "github.com/prometheus/client_golang/prometheus"

    // team: agent-shared-components

    // Component is the component type.
    type Component interface {
        // RegisterCollector Registers a Collector with the prometheus registry
        RegisterCollector(c prometheus.Collector)
    }
    ```

In the example above, every user of the `telemetry` component would have to import `github.com/prometheus/client_golang/prometheus` no matter which implementation they use.

In general, be mindful of using external types in the public interface of your component. For example, it would make sense to use Docker types in a `docker` component, but not in a `container` component.

<!-- Also note that there is no `Start`/`Stop` method. Anything related to lifecycle will be handle
internally by each component (more on this [here TODO]()). -->

<!-- TODO: write the lifecycle part and update the link above -->

### The impl folders

The `impl` folder is where the component implementation is written. The details of component implementation are up to the developer.
The only requirement is that there is a public instantiation function called `NewComponent`.

=== ":octicons-file-code-16: comp/compression/impl-zstd/compressor.go"
    ```go
    package implzstd

    // NewComponent returns a new ZSTD implementation for the compression component
    func NewComponent(reqs Requires) Provides {
        ....
    }
    ```

To require input arguments to the `NewComponent` instantiation function, use a special struct named `Requires`.
The instantiation function returns a special stuct named `Provides`. This internal nomenclature is used
to handle the different component dependencies using Fx groups.

In this example, the compression component must access the configuration component and the log component. To express this, define a `Requires` struct with two fields. The name of the fields is irrelevant, but the type must be the concrete type of interface that you require.

=== ":octicons-file-code-16: comp/compression/impl-zstd/compressor.go"
    ```go
    package implzstd

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

!!! Info "Using other components"
    If you want to use another component within your own, add it to the `Requires` struct, and `Fx` will give it to
    you at initialization. Be careful of cycling dependencies.

For the output of the component, populate the `Provides` struct with the return values.
=== ":octicons-file-code-16: comp/compression/impl-zstd/compressor.go"
    ```go
    package implzstd

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

All together, the component code looks like the following:

=== ":octicons-file-code-16: comp/compression/impl-zstd/compressor.go"
    ```go
    package implzstd

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

The constructor can return either a `Provides`, if it is infallible, or `(Provides, error)`, if it could fail. In the
latter case, a non-nil error results in the Agent crashing at startup with a message containing the error.

Each implementation follows the same pattern.

### The fx folders

The `fx` folder must be the only folder importing and referencing Fx. It's meant to be a simple wrapper. Its only goal is to allow
dependency injection with Fx for your component.

All `fx.go` files must define a `func Module() fxutil.Module` function. The helpers contained in `fxutil` handle all
the logic. Most `fx/fx.go` file should look the same as this:

=== ":octicons-file-code-16: comp/compression/fx-zstd/fx.go"
    ```go
    package fxzstd

    import (
        "github.com/DataDog/datadog-agent/pkg/util/fxutil"

        // You must import the implementation you are exposing through FX
        implzstd "github.com/DataDog/datadog-agent/comp/compression/impl-zstd"
    )

    // Module specifies the compression module.
    func Module() fxutil.Module {
        return fxutil.Component(
            // ProvideComponentConstructor will automatically detect the 'Requires' and 'Provides' structs
            // of your constructor function and map them to FX.
            fxutil.ProvideComponentConstructor(
                implzstd.NewComponent,
            )
        )
    }
    ```

!!! Info "Optional dependencies"
    Creating a conversion function to `optional.Option` is done automatically by `ProvideComponentConstructor`.
    This means that you can depend on any `comp` as an `optional.Option[comp]` if needed.

    More on this in the [FAQ](faq.md#optional-component).

For the ZIP implementation, create the same file in `fx-zip` folder. In most cases, your component has a
single implementation. If so, you have only one `impl` and `fx` folder.

#### `fx-none`

Some parts of the codebase might have optional dependencies on your components (see [FAQ](faq.md#optional-component)).

If it's the case, you need to provide a fx wrapper called `fx-none` to avoid duplicating the use of `optional.NewNoneOption[def.Component]()` in all our binaries

=== ":octicons-file-code-16: comp/compression/fx-none/fx.go"
    ```go
    import (
        compression "github.com/DataDog/datadog-agent/comp/compression/def"
    )

    func Module() fxutil.Module {
        return fxutil.Component(
            fx.Provide(func() optional.Option[compression.Component] {
                return optional.NewNoneOption[compression.Component]()
            }))
    }
    ```

### The mock folder

To support testing, components MUST provide a mock implementation (unless your component has no public method in its
interface).

Your mock must implement the `Component` interface of the `def` folder but can expose more methods if needed. All mock
constructors must take a `*testing.T` as parameter.

In the following example, your mock has no dependencies and returns the same string every time.

=== ":octicons-file-code-16: comp/compression/mock/mock.go"
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

### Go module

Go modules are not mandatory, but if you want to allow your component to be used outside the `datadog-agent` repository, create Go modules in the following places:

* In the `impl`/`impl-*` folder that you want to expose (you can only expose some implementations).
* In the `def` folder to expose the interface
* In the `mock` folder to expose the mock

Never add a Go module to the component folder (for example,`comp/compression`) or any `fx` folders.

## Final state

In the end, a classic component folder should look like:

```
comp/<component>/
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

This can seem like a lot for a single compression component, but this design answers the exponentially increasing complexity
of the Agent ecosystem. Your component needs to behave correctly with many binaries composed of unique and shared
components, outside repositories that want to pull only specific features, and everything in between.

!!! Info "Important"
    No components know how or where they will be used and MUST, therefore, respect all the rules above. It's a very
    common pattern for teams to work only on their use cases, thinking their code will not be used anywhere else. But
    customers want common behavior between all Datadog products (Agent, serverless, Agentless, Helm, Operator, etc.).

    A key idea behind the component is to produce shareable and reusable code.

## General consideration about designing components

Your component must:

* Be thread safe.
* Any public methods should be able to be used as soon as your constructor is called. It's OK if some do nothing or
  drop data as long as the Agent lifecycle is still in its init phase (see [lifecycle section for more | TODO]()).
* Be clearly documented (see section below).
* Be tested.

### Documentation

The documentation (both package-level and method-level) should include everything a user of the component needs to know.
In particular, the documentation must address any assumptions that might lead to panic if violated by the user.

Detailed documentation of how to avoid bugs in using a component is an indicator of excessive complexity and should be
treated as a bug. Simplifying the usage will improve the robustness of the Agent.

Documentation should include:

* Precise information on when each method may be called. Can methods be called concurrently?
* Precise information about data ownership of passed values and returned values. Users can assume that any mutable value
  returned by a component will not be modified by the user or the component after it is returned. Similarly, any mutable
  value passed to a component will not be later modified, whether by the component or the caller. Any deviation from these
  defaults should be documented.

    !!! note
        It can be surprisingly hard to avoid mutating data -- for example, `append(..)` surprisingly mutates its first
        argument. It is also hard to detect these bugs, as they are often intermittent, cause silent data corruption, or
        introduce rare data races. Where performance is not an issue, prefer to copy mutable input and outputs to avoid
        any potential bugs.

* Precise information about goroutines and blocking. Users can assume that methods do not block indefinitely, so
  blocking methods should be documented as such. Methods that invoke callbacks should be clear about how the callback is
  invoked, and what it might do. For example, document whether the callback can block, and whether it might be called
  concurrently with other code.
* Precise information about channels. Is the channel buffered? What happens if the channel is not read from quickly
  enough, or if reading stops? Can the channel be closed by the sender, and if so, what does that mean?
