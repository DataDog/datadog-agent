# Defining Components

A component is defined in a dedicated package named `comp/<bundleName>/<component>`, where `<bundleName>` names the bundle that contains the component.
The package must have the following defined in `component.go`:

 * Extensive package-level documentation.
   This should define, as precisely as possible, the behavior of the component, acting as a contract on which users of the component may depend.
   See the "Documentation" section below for details.

 * A team-name comment of the form `// team: <teamname>`.
   This is used to generate CODEOWNERS information.

 * `Component` -- the interface type implemented by the component.
   This is the type by which other components will require this one via `fx`.
   It can be an empty interface, if there is no need for any methods.
   It should have a formulaic doc string like `// Component is the component type.`, deferring documentation to the package docs.
   All interface methods should be exported and thoroughly documented.

 * `Module` -- an `fx.Option` that can be included in the bundle's `Module` or an `fx.App` to make this component available.
   To assist with debugging, use `fxutil.Component(options...)`.
   This item should have a formulaic doc string like `// Module defines the fx options for this component.`

Components should not be nested; that is, no component's Go path should be a prefix of another component's Go path.

## Implementation

The completed `component.go` looks like this:

```go
// Package foo ... (detailed doc comment for the component)
package config

// team: some-team-name

// Component is the component type.
type Component interface {
	// Foo is ... (detailed doc comment)
	Foo(key string) string
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
    fx.Provide(newFoo),
)
```

The Component interface is implemented in another file by an unexported type with a sensible name such as `launcher` or `provider` or the classic `foo`.

```go
package config

type foo {
    foos []string
}

type dependencies struct {
    fx.In

    Log log.Component
    Config config.Component
    // ...
}

func newFoo(deps dependencies) Component { ...  }

// Foo implements Component#Foo.
func (f *foo) Foo(key string) string { ... }
```

The constructor `newFoo` is an `fx` constructor, so it can refer to other types and expect them to be automatically supplied.
See [Using Components](./using.md) for details.

The constructor can return either `Component`, if it is infallible, or `(Component, error)`, if it could fail.
In the second form, a non-nil error will crash the agent at startup with a message containing the error.
It is possible, and often necessary, to return multiple values.
If the list of return values grows unwieldy, `fx.Out` can be used to create an output struct.

The constructor may call methods on other components, as long as the called method's documentation indicates it is OK.

You can use the invoke task `inv new-component comp/<bundleName>/<component>` to generate a pre-filled `component.go` file for the given component.

## Documentation

The documentation (both package-level and method-level) should include everything a user of the component needs to know.
In particular, any assumptions that might lead to panics if violated by the user should be clearly documented.

Detailed documentation of how to avoid bugs in using a component is an indicator of excessive complexity and should be treated as a bug.
Simplifying the usage will improve the robustness of the Agent.

Documentation should include:

* Precise information on when each method may be called.
  Can methods be called concurrently?
  Are some methods invalid before the component has started?
  Such assumptions are difficult to verify, so where possible try to make every method callable concurrently, at all times.

* Precise information about data ownership of passed values and returned values.
  Users can assume that any mutable value returned by a component will not be modified by the user or the component after it is returned.
  Similarly, any mutable value passed to a component will not be later modified either by the component or the caller.
  Any deviation from these defaults should be clearly documented.

  _Note: It can be surprisingly hard to avoid mutating data -- for example, `append(..)` surprisingly mutates its first argument.
  It is also hard to detect these bugs, as they are often intermittent, cause silent data corruption, or introduce rare data races.
  Where performance is not an issue, prefer to copy mutable input and outputs to avoid any potential bugs._

* Precise information about goroutines and blocking.
  Users can assume that methods do not block indefinitely, so blocking methods should be documented as such.
  Methods that invoke callbacks should be clear about how the callback is invoked, and what it might do.
  For example, document whether the callback can block, and whether it might be called concurrently with other code.

* Precise information about channels.
  Is the channel buffered?
  What happens if the channel is not read from quickly enough, or if reading stops?
  Can the channel be closed by the sender, and if so, what does that mean?

## Testing Support

To support testing, components can optionally provide a mock implementation, with the following in `component.go`.

 * `Mock` -- the type implemented by the mock version of the component.
   This should embed `pkg.Component`, and provide additional exported methods for manipulating the mock for use by other packages.

 * `MockModule` -- an `fx.Option` that can be included in a test `App` to get the component's mock implementation.

```go
// Mock implements mock-specific methods.
type Mock interface {
    // Component methods are included in Mock.
    Component

    // AddedFoos returns the foos added by AddFoo calls on the mock implementation.
    AddedFoos() []Foo
}

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
    fx.Provide(newMockFoo),
)
```

```go
type mock struct { ... }

// Foo implements Component#Foo.
func (m *mock) Foo(key string) string { ... }

// AddedFoos implements Mock#AddedFoos.
func (m *mock) AddedFoos() []Foo { ... }

func newFoo(deps dependencies) Component {
    return &mock{ ... }
}
```

Users of the mock module can cast the `Component` to a `Mock` to access the mock methods, as described in [Using Components](./using.md).