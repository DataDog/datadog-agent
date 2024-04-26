# Defining Components

You can use the [invoke](../../setup.md#invoke) task `inv components.new-component comp/<bundleName>/<component>` to generate a scaffold for your new component.

Below is a description of the different folders and files of your component.

Every public variable, function, struct, and interface of your component **must** be documented. Please refer to the [Documentation](#documentation) section below for details.

A component is defined in a dedicated package named `comp/<bundleName>/<component>`, where `<bundleName>` names the bundle that contains the component. The package must have the following defined in:

* `comp/<bundleName>/<component>/component.go`
    * A team-name comment of the form `// team: <teamname>`. This is used to generate CODEOWNERS information.

    * `Component` -- The component interface. This is the interface that other components can reference when declaring the component as a dependency via `fx`. It can be an empty interface, if there is no need for any methods.

* `comp/<bundleName>/<component>/<component>impl/<component>.go`
    * `Module` -- an `fx.Option` that can be included in the bundle's `Module` or an `fx.App` to make this component available. The `Module` is defined in a separate package from the component, allowing a package to import the interface without having to import the entire implementation. To assist with debugging, declare your Module using `fxutil.Component(options...)`.

!!! warning
    Components should not be nested; that is, no component's Go path should be a prefix of another component's Go path.

## Implementation

The Component interface and the `Module` definition are implemented in the file `comp/<bundleName>/<component>/<component>impl/<component>.go`.

!!! warning "Important"
    The Module definition function **must** be private.

=== ":octicons-file-code-16: comp/&lt;bundleName&gt;/&lt;component&gt;/&lt;component&gt;impl/&lt;component&gt;.go"
    ```go
    package config

    // Module defines the fx options for this component.
    func Module() fxutil.Module {
        return fxutil.Component(
            fx.Provide(newFoo),
        )
    }

    type foo struct {
        foos []string
    }

    type dependencies struct {
        fx.In

        Log log.Component
        Config config.Component
        // ...
    }
    
    type provides struct {
        fx.Out

        Comp comp.Component
        // ...
    }

    func newFoo(deps dependencies) Component { ...  }

    // foo implements Component#Foo.
    func (f *foo) Foo(key string) provides { ... }
    ```

The constructor `newFoo` is an `fx` constructor. It can refer to other dependencies and expect them to be automatically supplied via `fx`.

See [Using Components](using-components.md) for more details.

The constructor can return either a `Component`, if it is infallible, or `(Component, error)`, if it could fail. In the second form, a non-nil error will crash the agent at startup with a message containing the error. It is possible, and often necessary, to return multiple values. If the list of return values grows unwieldy, `fx.Out` can be used to create an output struct.

The constructor may call methods on other components, as long as the called method's documentation indicates it is OK.

## Testing Support

To support testing, components can optionally provide a mock implementation, with the following in:

* `comp/<bundleName>/<component>/component_mock.go`
    * `Mock` -- the type implemented by the mock version of the component. This should embed `pkg.Component`, and provide additional exported methods for manipulating the mock for use by other packages.
* `comp/<bundleName>/<component>/<component>impl/<component>_mock.go`
    * `MockModule` -- an `fx.Option` that can be included in a test `App` to get the component's mock implementation.

=== ":octicons-file-code-16: comp/&lt;bundleName&gt;/&lt;component&gt;/&lt;component_mock.go"
    ```go
    //go:build test

    package foo

    // Mock implements mock-specific methods.
    type Mock interface {
        // Component methods are included in Mock.
        Component

        // AddedFoos returns the foos added by AddFoo calls on the mock implementation.
        AddedFoos() []Foo
    }
    ```

===! ":octicons-file-code-16: comp/&lt;bundleName&gt;/&lt;component&gt;/&lt;component&gt;impl/&lt;component&gt;_mock.go"
    ```go
    //go:build test

    package foo

    // MockModule defines the fx options for the mock component.
    func MockModule() fxutil.Module {
        return fxutil.Component(
            fx.Provide(newMock),
        )
    }
    ```

    ```go
    type mock struct { ... }

    // Foo implements Component#Foo.
    func (m *mock) Foo(key string) string { ... }

    // AddedFoos implements Mock#AddedFoos.
    func (m *mock) AddedFoos() []Foo { ... }

    func newMock(deps dependencies) Component {
        return &mock{ ... }
    }
    ```

Users of the mock module can cast the `Component` to a `Mock` to access the mock methods, as described in [Using Components](using-components.md).

## Documentation

The documentation (both package-level and method-level) should include everything a user of the component needs to know. In particular, any assumptions that might lead to panics if violated by the user should be documented.

Detailed documentation of how to avoid bugs in using a component is an indicator of excessive complexity and should be treated as a bug. Simplifying the usage will improve the robustness of the Agent.

Documentation should include:

* Precise information on when each method may be called. Can methods be called concurrently? Are some methods invalid before the component has started? Such assumptions are difficult to verify. Where possible, try to make every method callable concurrently, at all times.
* Precise information about data ownership of passed values and returned values. Users can assume that any mutable value returned by a component will not be modified by the user or the component after it is returned. Similarly, any mutable value passed to a component will not be later modified either by the component or the caller. Any deviation from these defaults should be documented.

    !!! note
        It can be surprisingly hard to avoid mutating data -- for example, `append(..)` surprisingly mutates its first argument. It is also hard to detect these bugs, as they are often intermittent, cause silent data corruption, or introduce rare data races. Where performance is not an issue, prefer to copy mutable input and outputs to avoid any potential bugs.

* Precise information about goroutines and blocking. Users can assume that methods do not block indefinitely, so blocking methods should be documented as such. Methods that invoke callbacks should be clear about how the callback is invoked, and what it might do. For example, document whether the callback can block, and whether it might be called concurrently with other code.
* Precise information about channels. Is the channel buffered? What happens if the channel is not read from quickly enough, or if reading stops? Can the channel be closed by the sender, and if so, what does that mean?
