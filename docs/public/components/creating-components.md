# Creating a Component

This page explains how to create components in detail.

Throughout this page we're going to create a compression component. The component compresses the payload before sending it to the Datadog backend.

Since there are multiple ways to compress data, we are going to provide two implementations of the same component interface: 

* The [ZSTD](https://en.wikipedia.org/wiki/Zstd) data compression algorithm
* The [ZIP](https://en.wikipedia.org/wiki/ZIP_(file_format)) data compression algorithm

A Component is organize into multiple folders and Go packages. The main reason to separate a component into separate package is to separate the interface from the implementations and to improve code sharing. Having the interface declare in a separate package from the implementation allow to import the interface without having to import the different implementations. 

## File hierarchy

All components are located in the `comp` folder at the top of the Agent repo.

The file hierarchy is the following:

```
comp /
  <bundle name> /        <-- Optional
    <comp name> /
      def /              <-- The folder containing the component interface and ALL its public types.
      impl /             <-- The only or primary implementation of the component.
      impl-<alternate> / <-- An alternate implementation.
      impl-none /        <-- Optional. A none implementation.
      fx /               <-- All fx related logic for the primary implementation, if any.
      fx-<alternate> /   <-- All fx related logic for a specific implementation.
      mock /             <-- The mock interface of the component to ease testing.
```

TODO: where do we put the `optional.NoneOption` ? Into its own fx-none folder ?

To note:

* If your component has only one implementation it should live in the `impl` folder.
* If your component don't have a single implementation but several versions you should have no `impl` folder but
  multiple `impl-<version>` folders. For example a component in charge of compressing data will have `impl-zstd` and `impl-zip` but not `impl` folder.
* If your component needs to offer a dummy/empty version it should live in the `impl-none` folder.

### Why all those files ?

This file hierarchy aimed at solving a few problems:

* Component users should only interact with the `def` folders and never care about which implementation was loaded in the
  main.
* We want to support Go modules. When creating a Go module, any sub folders will be pulled into the module. For this
  reason we need different folders for each implementation, the definition and fx. This way, external repository can pull a
  specific implementation and definition without having to import the rest.
* We have one `fx` folder per implementation to allow binaries to import/link against a single one.
* A main that imports a component should be able to select a specific implementation without compiling with the others.
  For example: the ZSTD library should not be included at compile time when the ZIP version is used.


**Go package naming convention**:

All implementations must use the package name: `<component name>impl`.

For example, a compression component with 2 implementation would use `package compressionimpl` in both
`comp/<bundle>/compression/impl-zstd` and `comp/<bundle>/compression/impl-zip` folders.

## Bootstraping Components

You can use the [invoke](../setup.md#invoke) task `inv components.new-component comp/<component>` to generate a scaffold for your new component.

Every public variable, function, struct, and interface of your component **must** be documented. Please refer to the [Documentation](#documentation) section below for details.

### The def folder

The def folder will contain your interface and ALL public types needed by the users of your component.

For our compression example it will look like this:

=== ":octicons-file-code-16: comp/compression/def/component.go"
    ```go
    // Package compressiondef contains all public type and interfaces for the compression component
    package compressiondef

    // team: <your team>

    // Component describes the interface implemented by all compression implementations.
    type Component interface {
        // Compress compresses the input data.
        Compress([]byte) ([]byte, error)

        // Decompress decompresses the input data.
        Decompress([]byte) ([]byte, error)
    }
    ```

All component interfaces must be called `Component` so all imports would be similar to `compressiondef.Component`.

You can see that our interface only exposes the bare minimum. You should aim at having the smallest possible interface
for your component. 

When defining a component interface we try to use struct or interface from third-party dependencies. Ensuring that using the component interface doesn't require importing any thrid-party dependency.

!!! warning "Interface using a third-party dependency"
    ```
    package telemetrydef

    import "github.com/prometheus/client_golang/prometheus"

    // team: agent-shared-components

    // Component is the component type.
    type Component interface {
        // RegisterCollector Registers a Collector with the prometheus registry
        RegisterCollector(c prometheus.Collector)
    }
    ```

For the example above every user of the telemetry component would have to import `github.com/prometheus/client_golang/prometheus`. To avoid those cases is best to design your interface mindfully.

TODO: write alternative solution

Also note that there is no `Start`/`Stop` method. Anything related to lifecycle will be handle
internally by each component (more on this [here TODO]()).

TODO: write the lifecycle part and update the link above

### The impl folders

The `impl` folder is where the component implementation is written. The component implementation details is up to the developer to design. 
The only requirement is that there is a public instantiation function called `NewComponent`.

=== ":octicons-file-code-16: comp/compression/fx-zstd/component.go"
    ```go
    package zstdimpl

    import (
        // We always import the component def folder to be able to return a 'def.Component' type.
        "github.com/DataDog/datadog-agent/comp/compression/def"
    )

    // NewComponent returns a new ZSTD implementation for the compression component
    func NewComponent(){
        ....
    }
    ```

To access an argument in the  `NewComponent` function we use a special struct name `Requires`. And the return value is a special stuct named `Provides`. Those are internal nomenclature used to handle the different component depencies using `FX`

For our component we are going to need to access the configuration component and the log component. To express that in our component we are going to define a `Requires` struct with two fields. The name is irrelevant but the type must by a concrete type. 

=== ":octicons-file-code-16: comp/compression/fx-zstd/component.go"
    ```go
    package zstdimpl

    import (
        "fmt"

        // We always import the component def folder to be able to return a 'def.Component' type. As a reminder fx
        // only work on type and will not try to convert a real type to an interface.
        "github.com/DataDog/datadog-agent/comp/compression/def"

        config "github.com/DataDog/datadog-agent/comp/core/config/def"
        log "github.com/DataDog/datadog-agent/comp/core/log/def"
    )

    // Here we list all Components and other types known by FX that we need.
    // The type and field needs to be public to be used in the `fx` folders.
    //
    // In our example we're going to need the config and log components.
    type Requires struct {
        Conf config.Component
        Log  log.Component
    }
    ```
    
!!! Info "Using other components"
    You want to use another component within your own? Simply add it to the `Requires` struct and `Fx` will give it to
    you at initialization ! Be careful of cycling dependencies though.
    
As for the output of our component we are going populate the `Provides` struct with the all the return values.
 . 
=== ":octicons-file-code-16: comp/compression/fx-zstd/component.go"
    ```go
    // Here we list all the types we're going to return. You can return as many types as you want and they will all
    // be available through FX in other components.
    // The type and field needs to be public to be used in the `fx` folders.
    //
    // In our example we're only returning our component.
    type Provides struct {
        Comp def.Component
    }
    ```
 
If we peace all together our component code looks like this:

=== ":octicons-file-code-16: comp/compression/fx-zstd/component.go"
    ```go
    package zstdimpl

    import (
        "fmt"
        "github.com/DataDog/datadog-agent/comp/compression/def"

        config "github.com/DataDog/datadog-agent/comp/core/config/def"
        log "github.com/DataDog/datadog-agent/comp/core/log/def"
    )

    type Requires struct {
        Conf config.Component
        Log  log.Component
    }

    type Provides struct {
        Comp def.Component
    }

    // The actual type implementing the 'Component' interface. This type MUST be private, we need the guarantee that
    // components can only be used through their interface.
    type compressor struct {
        // we keep a ref on the config and log component to be able to use them in the 'compressor' methods
        conf config.Component
        log  log.Component

        // any other field we might need
    }

    // NewComponent returns a new ZSTD implementation for the compression component
    func NewComponent(deps Requires) Provides {
        // Here we do whatever is needed to build a ZSTD compression comp.

        // And we create our Component
        comp := &compressor{
            conf: deps.Conf,
            log:  deps.Log,
        }

        return Provides{
            comp: comp,
        }
    }

    //
    // We then need to implement all method from our 'def.Component' interface
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
second form, a non-nil error will crash the agent at startup with a message containing the error.

Each implementation follows the same pattern.

### The fx folders

The `fx` folder must be the only folder importing and referencing `Fx`. It's meant to be a simple wrapper. It's only goal is to allow
dependency injection with `Fx` for your component.

All `fx.go` files must define a `func Module() fxutil.Module` function. The helpers contained in `fxutil` will handle all
the logic for you. Most `fx/fx.go` file should look the same as this:

=== ":octicons-file-code-16: comp/compression/fx-zstd/fx.go"
    ```go
    package fxzstd

    import (
        "github.com/DataDog/datadog-agent/pkg/util/fxutil"

        // You must import the implementation you are exposing through FX
        zstdimpl "github.com/DataDog/datadog-agent/comp/compression/impl-zstd"
    )

    // Module specifies the compression module.
    func Module() fxutil.Module {
        return fxutil.Component(
            // ProvideComponentConstructor will automatically detect the 'Requires' and 'Provides' structs
            // of your constructor function and map them to FX.
            fxutil.ProvideComponentConstructor(
                zstdimpl.NewComponent,
            )
        )
    }
    ```

!!! Info "Optional dependencies"
    Creating a conversion function to `optional.Option` will be done automatically by `ProvideComponentConstructor`.
    This means that you should be able to depends on any `comp` as a `optional.Option[comp]` if needed.

    More on this in the [FAQ](faq.md#optional-dependency).

You would create the same file in `fx-zip` folder for the ZIP implementation. In most case your component will have a
single implementation. In this case you will only have one `impl` and `fx` folder.

#### `fx-void`

Some part of the code base might have an optional dependency on you components (see [FAQ](faq.md#optional-dependency)).
To ease the creation of binary you need to provide a 

### The mock folder

To support testing, components MUST provide a mock implementation (unless your component has no public method in its
interface).

Your mock must implement the `Component` interface of the `def` folder but can expose more methods if needed. Your mock must
respect the same logic as other implementation: having a public constructor, being possible to take dependencies
through fx, ...

In the following case, our mock has no dependencies and returns the same string every time.

=== ":octicons-file-code-16: comp/compression/mock/mock.go"
    ```go
    //go:build test

    package mock

    import (
        "testing"

        "github.com/DataDog/datadog-agent/comp/compression/def"
    )

    type Provides struct {
        Comp def.Component
    }

    type mock struct {}

    // NewMockCompressor returns a mock compressor
    func NewMockCompressor() Provides {
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

We need a `Fx` wrapper:

=== ":octicons-file-code-16: comp/compression/fx-mock/fx.go"
    ```go

    package fxzstd

    import (
        "github.com/DataDog/datadog-agent/pkg/util/fxutil"

        mockimpl "github.com/DataDog/datadog-agent/comp/compression/mock"
    )

    // Module specifies the compression module.
    func Module() fxutil.Module {
        return fxutil.Component(fxutil.ProvideComponentConstructor(mockipl.NewMockCompressor))
    }
    ```

### Go module

Go module are not mandatory, but if you want to allow your component to be used outside the datadog-agent repository you
would create go module in the following places:

* In the `impl`/`impl-*` folder that you want to expose (you can only expose some implementations).
* In the `def` folder to expose the interface
* In the `mock` folder to expose the mock

You should, never, add a go module to the component folder (ie: `comp/compression`) nor any `Fx` ones.

## Final state

In the end, here what a classic component folder should look like:

```
comp/<component>/
├── def
│   └── component.go
├── fx
│   └── fx.go
├── fx-mock
│   └── fx.go
├── impl
│   └── component.go
└── mock
    └── mock.go

4 directories, 4 files
```

Our example, which has 2 implementations looks like this:

```
comp/core/compression/
├── def
│   └── component.go
├── fx-mock
│   └── fx.go
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

7 directories, 7 files
```

This can seems a lot for a simple compression component, but this design answer the exponentially increasing complexity
of the Agent ecosystem. Your component needs to behave correctly with many binaries composed of unique and shared
components, outside repositories that want to pull only specific features and everything in between.

!!! Info "Important"
    No components known how and where it will be used and MUST therefore respect all the rules above. It's a very
    common pattern for teams to work only on their use cases thinking their code will not be used anywhere else. But
    customers want common behavior between all our products (agent, serverless, agentless, helm, operator, ...).

    A key idea behind the component is to produce shareable and reusable code.

## General consideration about designing components

Your component must:

* Be thread safe.
* Any public methods should be able to be used as soon as your constructor is called. It's OK if some do nothing or
  drop data as long as the agent lifecycle is still in its init phase (see [lifecycle section for more | TODO]()).
* Be clearly documented (see section below).
* Be tested.

### Documentation

The documentation (both package-level and method-level) should include everything a user of the component needs to know.
In particular, any assumptions that might lead to panic if violated by the user should be documented.

Detailed documentation of how to avoid bugs in using a component is an indicator of excessive complexity and should be
treated as a bug. Simplifying the usage will improve the robustness of the Agent.

Documentation should include:

* Precise information on when each method may be called. Can methods be called concurrently?
* Precise information about data ownership of passed values and returned values. Users can assume that any mutable value
  returned by a component will not be modified by the user or the component after it is returned. Similarly, any mutable
  value passed to a component will not be later modified either by the component or the caller. Any deviation from these
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
